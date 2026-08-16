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
			b.deliverPending(ctx)
		case <-ticker.C:
			b.deliverPending(ctx)
		}
	}
}

// streamNotifications keeps the outbox stream open, waking the delivery loop on every
// event. The wake is a non-blocking send into a channel of one: a burst that lands while
// a batch is being sent collapses into the single drain that follows it.
func (b *Bot) streamNotifications(ctx context.Context, wake chan<- struct{}) {
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

	case client.Reminder1h:
		p, err := client.DecodePayload[client.Reminder1hPayload](n)
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

	default:
		return "", fmt.Errorf("unknown notification kind %q", n.Kind)
	}
}

func countSummary(counts map[client.SignupStatus]int) string {
	parts := make([]string, 0, len(counts))
	for _, status := range []client.SignupStatus{
		client.StatusConfirmed, client.StatusLate, client.StatusTentative, client.StatusDeclined,
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
