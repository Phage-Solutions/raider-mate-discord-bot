package discord

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/Phage-Solutions/raider-mate-discord-bot/internal/client"
)

// notificationBatch is how many the poller claims per tick. A claim leases for five
// minutes, so a batch has to be small enough to send inside that.
const notificationBatch = 50

// streamBackoff is how long the bot waits before redialling a dropped stream. The
// service restarting is the common case, so this is short enough not to be noticed and
// long enough not to spin against a service that is down.
const streamBackoff = 5 * time.Second

// pollNotifications drains the service's outbox. The service decides when a reminder is
// due; the bot only delivers, so nothing here reads a clock or a schedule.
//
// Two things ask for a drain: the stream, which is how the bot hears about a row within
// the second, and the ticker behind it, which covers a stream that died quietly and
// claims whose lease lapsed when a delivery crashed. Both go through one loop, so a
// stream event during a tick cannot start a second batch alongside the first.
func (b *Bot) pollNotifications(ctx context.Context) {
	ticker := time.NewTicker(b.pollInterval)
	defer ticker.Stop()

	wake := make(chan struct{}, 1)
	go b.streamNotifications(ctx, wake)

	for {
		select {
		case <-ctx.Done():
			return
		case <-wake:
			b.deliverBatch(ctx)
		case <-ticker.C:
			b.deliverBatch(ctx)
		}
	}
}

// deliverBatch is deliverPending with a recover around it. The recover is per drain
// rather than around the loop on purpose: a notification the bot cannot render is one
// bad row, and ending the loop over it would stop every later reminder while leaving a
// bot that still answers commands, which looks like nothing is wrong. The claim on the
// batch lapses after five minutes and the next tick picks it up again.
func (b *Bot) deliverBatch(ctx context.Context) {
	defer b.recoverPanic(ctx, "notification delivery")
	b.deliverPending(ctx)
}

// streamNotifications keeps the outbox stream open, waking the delivery loop on every
// event. The wake is a non-blocking send into a channel of one: a burst that lands while
// a batch is being sent collapses into the single drain that follows it.
func (b *Bot) streamNotifications(ctx context.Context, wake chan<- struct{}) {
	// Losing this goroutine is survivable in a way that losing the process is not: the
	// ticker in pollNotifications is where the bot was before the stream existed, so a
	// panic here costs latency on reminders and nothing else.
	defer b.recoverPanic(ctx, "notification stream")

	for {
		err := b.api.StreamNotifications(ctx, func() {
			select {
			case wake <- struct{}{}:
			default:
			}
		})
		if ctx.Err() != nil {
			return
		}
		// Never fatal. Losing the stream drops the bot back to the ticker, which is
		// where it was before the stream existed.
		b.logger.WarnContext(ctx, "notification stream ended", "error", err, "retry_in", streamBackoff)

		select {
		case <-ctx.Done():
			return
		case <-time.After(streamBackoff):
		}
	}
}

func (b *Bot) deliverPending(ctx context.Context) {
	notifications, err := b.api.ClaimNotifications(ctx, notificationBatch)
	if err != nil {
		b.logger.ErrorContext(ctx, "claiming notifications", "error", err)
		return
	}

	for _, n := range notifications {
		// A raider with DMs closed must not take the batch down with them, so a failed
		// send is logged and acked rather than retried until the lease expires and
		// everyone else gets the message twice.
		if err := b.deliver(ctx, n); err != nil {
			b.logger.WarnContext(ctx, "delivering notification",
				"error", err, "kind", n.Kind, "notification_id", n.ID)
		}
		if err := b.api.MarkDelivered(ctx, n.ID); err != nil {
			b.logger.ErrorContext(ctx, "acking notification", "error", err, "notification_id", n.ID)
		}
	}
}

func (b *Bot) deliver(ctx context.Context, n client.Notification) error {
	// A redraw addresses nobody, so it never reaches notificationText: there is no
	// content to write, only an edit of a message this bot posted earlier.
	if n.TargetKind == client.TargetMessage {
		actor, err := serviceActor(n, b.appID())
		if err != nil {
			return err
		}
		b.redraw(ctx, actor, n.EventID)
		return nil
	}

	content, err := notificationText(n)
	if err != nil {
		return err
	}

	switch n.TargetKind {
	case client.TargetUser:
		if n.DiscordID == nil {
			return fmt.Errorf("user notification has no recipient")
		}
		return b.sendDM(ctx, *n.DiscordID, content)
	case client.TargetRole:
		if n.ChannelID == nil {
			return fmt.Errorf("role notification has no channel")
		}
		return b.sendToChannel(ctx, *n.ChannelID, mentions(n.RoleIDs)+content)
	case client.TargetChannel:
		if n.ChannelID == nil {
			return fmt.Errorf("channel notification has no channel")
		}
		prefix, allowed := userMentions(n.DiscordIDs)
		return b.sendMentioning(ctx, *n.ChannelID, prefix+content, allowed)
	default:
		return fmt.Errorf("unknown target kind %q", n.TargetKind)
	}
}

// serviceActor is who the bot is when nobody clicked anything. Redraws come off a
// timer in the service, so there is no interaction to read a member from: the bot
// speaks as itself, in the guild the notification names, with no roles. The reads a
// redraw makes are of a message the whole guild can already see.
func serviceActor(n client.Notification, appID string) (client.Actor, error) {
	guildID, err := strconv.ParseUint(n.DiscordGuildID, 10, 64)
	if err != nil {
		return client.Actor{}, fmt.Errorf("parsing guild id %q: %w", n.DiscordGuildID, err)
	}

	// Empty before the gateway is open, which is not a state the poller runs in.
	discordID, err := strconv.ParseUint(appID, 10, 64)
	if err != nil {
		return client.Actor{}, fmt.Errorf("parsing application id %q: %w", appID, err)
	}

	return client.Actor{DiscordID: discordID, GuildID: guildID}, nil
}

// notificationText turns a payload into the message a raider reads. Short and plain:
// these arrive unprompted, so anything longer is an interruption.
func notificationText(n client.Notification) (string, error) {
	switch n.Kind {
	case client.Reminder24h:
		p, err := client.DecodePayload[client.Reminder24hPayload](n)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("**%s** is %s and you have not answered. Signups close %s.",
			p.Title, discordTime(p.StartsAt.Unix(), "R"), discordTime(p.Deadline.Unix(), "F")), nil

	// Reminder1h is the pre-event reminder's old name, still delivered so a bot that is
	// deployed before the service keeps working during the rollout.
	case client.ReminderPreEvent, client.Reminder1h:
		if n.TargetKind == client.TargetChannel {
			return preEventPingText(n)
		}

		p, err := client.DecodePayload[client.ReminderPreEventDMPayload](n)
		if err != nil {
			return "", err
		}
		if p.AssignedRole == nil {
			return fmt.Sprintf("**%s** starts %s.", p.Title, discordTime(p.StartsAt.Unix(), "R")), nil
		}
		return fmt.Sprintf("**%s** starts %s. You are on %s.",
			p.Title, discordTime(p.StartsAt.Unix(), "R"), roleShortNames[*p.AssignedRole]), nil

	case client.SignupDeadline:
		p, err := client.DecodePayload[client.SignupDeadlinePayload](n)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Signups are closed for **%s**. %s. The comp needs finalising.",
			p.Title, countSummary(p.Counts)), nil

	case client.CompNag:
		p, err := client.DecodePayload[client.CompNagPayload](n)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("**%s** starts %s and the comp is not locked.",
			p.Title, discordTime(p.StartsAt.Unix(), "R")), nil

	case client.LateRequestFiled:
		p, err := client.DecodePayload[client.LateRequestFiledPayload](n)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("Someone wants in on **%s** after the deadline, as %s. It is waiting in the dashboard.",
			p.EventTitle, strings.ToLower(string(p.Status))), nil

	case client.CompSlotDropped:
		p, err := client.DecodePayload[client.CompSlotDroppedPayload](n)
		if err != nil {
			return "", err
		}
		// No status means the signup was withdrawn rather than changed, and there is
		// nothing to name. Saying "is declined" there would be the wrong sentence.
		answer := "has withdrawn from"
		if p.Status != nil {
			answer = "is " + strings.ToLower(string(*p.Status)) + " for"
		}
		return fmt.Sprintf("A raider %s **%s** and has come out of %s. Re-lock when you are ready.",
			answer, p.EventTitle, compList(p.CompNames)), nil

	default:
		return "", fmt.Errorf("unknown notification kind %q", n.Kind)
	}
}

// countSummary still skips a zero even though the service now sends a key for every
// status: "4 confirmed, 0 late, 0 tentative, 0 declined, 0 absent" is a worse sentence
// than "4 confirmed". NO_SHOW is left out because it is decided after the night, not
// at the signup deadline this message reports on.
func countSummary(counts map[client.SignupStatus]int) string {
	parts := make([]string, 0, len(counts))
	for _, status := range []client.SignupStatus{
		client.StatusConfirmed, client.StatusLate, client.StatusTentative,
		client.StatusDeclined, client.StatusAbsent,
	} {
		if counts[status] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", counts[status], strings.ToLower(string(status))))
		}
	}
	if len(parts) == 0 {
		return "Nobody answered"
	}
	return strings.Join(parts, ", ")
}

// compList reads comp names as a sentence, since this lands in a channel rather than a
// table. The empty case cannot come from the service, which only queues the
// notification when it emptied something, but a nil slice would slice out of range.
func compList(names []string) string {
	switch len(names) {
	case 0:
		return "the comp"
	case 1:
		return names[0]
	default:
		return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
	}
}

// preEventPingText is what the events channel sees. It does not repeat the roster or
// name anyone's role: the link goes back to the event message, which already shows both
// and is the thing people have pinned.
func preEventPingText(n client.Notification) (string, error) {
	p, err := client.DecodePayload[client.ReminderPreEventPingPayload](n)
	if err != nil {
		return "", err
	}

	text := fmt.Sprintf("**%s** starts %s.", p.Title, discordTime(p.StartsAt.Unix(), "R"))
	if p.MessageID != nil && n.ChannelID != nil {
		text += fmt.Sprintf(" [Signup sheet](https://discord.com/channels/%s/%s/%s)",
			n.DiscordGuildID, *n.ChannelID, *p.MessageID)
	}
	return text, nil
}

// discordMentionCap is Discord's limit on allowed_mentions.users. Thirty raiders is
// nowhere near it, a guild-wide event could be, and a send over the cap is rejected
// rather than trimmed.
const discordMentionCap = 100

// userMentions renders the ping prefix and returns the ids Discord may notify. Past the
// cap the rest are counted rather than mentioned, since a rejected message reminds
// nobody at all.
func userMentions(discordIDs []string) (string, []string) {
	if len(discordIDs) == 0 {
		return "", nil
	}

	allowed := discordIDs
	overflow := 0
	if len(allowed) > discordMentionCap {
		overflow = len(allowed) - discordMentionCap
		allowed = allowed[:discordMentionCap]
	}

	parts := make([]string, len(allowed))
	for i, id := range allowed {
		parts[i] = "<@" + id + ">"
	}

	prefix := strings.Join(parts, " ")
	if overflow > 0 {
		prefix += " and " + strconv.Itoa(overflow) + " more"
	}
	return prefix + " ", allowed
}

func mentions(roleIDs []string) string {
	if len(roleIDs) == 0 {
		return ""
	}
	parts := make([]string, len(roleIDs))
	for i, id := range roleIDs {
		parts[i] = "<@&" + id + ">"
	}
	return strings.Join(parts, " ") + " "
}

func (b *Bot) sendDM(ctx context.Context, discordID, content string) error {
	channel, err := b.session.UserChannelCreate(discordID, discordgo.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("opening dm with %s: %w", discordID, err)
	}
	if _, err := b.session.ChannelMessageSend(channel.ID, content, discordgo.WithContext(ctx)); err != nil {
		return fmt.Errorf("sending dm to %s: %w", discordID, err)
	}
	return nil
}

func (b *Bot) sendToChannel(ctx context.Context, channelID, content string) error {
	if _, err := b.session.ChannelMessageSend(channelID, content, discordgo.WithContext(ctx)); err != nil {
		return fmt.Errorf("posting in %s: %w", channelID, err)
	}
	return nil
}

// sendMentioning posts and names the only users the message may ping, so an event
// titled "@everyone last chance" cannot widen its own reminder.
func (b *Bot) sendMentioning(ctx context.Context, channelID, content string, userIDs []string) error {
	_, err := b.session.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		Content:         content,
		AllowedMentions: &discordgo.MessageAllowedMentions{Users: userIDs},
	}, discordgo.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("posting in %s: %w", channelID, err)
	}
	return nil
}
