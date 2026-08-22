package discord

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/Raider-Mate/raider-mate-discord-bot/internal/client"
)

// setEventsChannel points the guild's event messages at one channel. Admin only, and
// the service enforces that: passing no channel clears the setting and returns the
// guild to posting wherever the create command was run.
func (b *Bot) setEventsChannel(ctx context.Context, i *discordgo.InteractionCreate, options []*discordgo.ApplicationCommandInteractionDataOption) {
	actor, current, ok := b.beginConfigChange(ctx, i)
	if !ok {
		return
	}

	var channelID *string
	for _, option := range options {
		if option.Name == "channel" && option.ChannelValue(nil) != nil {
			id := option.ChannelValue(nil).ID
			channelID = &id
		}
	}

	current.EventsChannelID = channelID
	if _, err := b.api.SetGuildSettings(ctx, actor, current); err != nil {
		b.fail(ctx, i, "setting events channel", err)
		return
	}

	if channelID == nil {
		b.editOrLog(ctx, i, "Cleared. Events will post wherever `/raid create` is run.")
		return
	}
	b.editOrLog(ctx, i, "Events will post in <#"+*channelID+">.")
}

// setTimezone records the zone the guild types its raid times in, so "tomorrow 20:00"
// resolves to the hour they meant. Admin only.
func (b *Bot) setTimezone(ctx context.Context, i *discordgo.InteractionCreate, options []*discordgo.ApplicationCommandInteractionDataOption) {
	actor, current, ok := b.beginConfigChange(ctx, i)
	if !ok {
		return
	}

	var timezone *string
	for _, option := range options {
		if option.Name == "zone" {
			if name := strings.TrimSpace(option.StringValue()); name != "" {
				timezone = &name
			}
		}
	}

	// Checked here as well as service-side so the raid lead gets the example rather
	// than a bare validation error relayed from an API.
	if timezone != nil {
		if _, err := time.LoadLocation(*timezone); err != nil {
			b.editOrLog(ctx, i, "I do not know that zone. It needs an IANA name, like `Europe/Bratislava` or `America/New_York`.")
			return
		}
	}

	current.Timezone = timezone
	if _, err := b.api.SetGuildSettings(ctx, actor, current); err != nil {
		b.fail(ctx, i, "setting timezone", err)
		return
	}

	if timezone == nil {
		b.editOrLog(ctx, i, "Cleared. Raid times now need an explicit offset, like `2026-09-01 20:00 +02:00`.")
		return
	}

	now := time.Now()
	loc, _ := time.LoadLocation(*timezone)
	b.editOrLog(ctx, i, "Raid times will be read as **"+*timezone+"**, where it is currently "+
		now.In(loc).Format("15:04")+".")
}

// beginConfigChange defers, then reads the current settings. Settings are a whole-row
// write, so a command changes one field on what it read back and sends the rest
// untouched: without that, setting the channel would silently drop the timezone.
func (b *Bot) beginConfigChange(ctx context.Context, i *discordgo.InteractionCreate) (client.Actor, client.GuildSettings, bool) {
	actor, err := actorFrom(i)
	if err != nil {
		b.logger.ErrorContext(ctx, "reading actor", "error", err)
		return client.Actor{}, client.GuildSettings{}, false
	}
	if err := b.deferEphemeral(i); err != nil {
		b.logger.ErrorContext(ctx, "deferring config change", "error", err)
		return client.Actor{}, client.GuildSettings{}, false
	}

	current, err := b.api.GuildSettings(ctx, actor)
	if err != nil {
		b.fail(ctx, i, "reading guild settings", err)
		return client.Actor{}, client.GuildSettings{}, false
	}

	return actor, current, true
}

// setEventMentions chooses which roles get pinged when an event message goes up.
//
// A role select menu rather than command options, because a guild pings two or three
// roles and Discord's role option takes one apiece. Dashboard editing of this is the
// eventual home; this is the surface until then.
func (b *Bot) setEventMentions(ctx context.Context, i *discordgo.InteractionCreate) {
	_, current, ok := b.beginConfigChange(ctx, i)
	if !ok {
		return
	}

	prompt := "Who should be pinged when an event is posted?"
	if len(current.EventMentionRoleIDs) > 0 {
		prompt += "\nCurrently " + mentionList(current.EventMentionRoleIDs) + "."
	}

	if err := b.edit(i, prompt, mentionRoleSelect()); err != nil {
		b.logger.ErrorContext(ctx, "showing mention role select", "error", err)
	}
}

// submitEventMentions writes the roles picked in that select.
func (b *Bot) submitEventMentions(ctx context.Context, i *discordgo.InteractionCreate) {
	actor, current, ok := b.beginConfigChange(ctx, i)
	if !ok {
		return
	}

	current.EventMentionRoleIDs = i.MessageComponentData().Values
	if _, err := b.api.SetGuildSettings(ctx, actor, current); err != nil {
		b.fail(ctx, i, "setting event mention roles", err)
		return
	}

	if len(current.EventMentionRoleIDs) == 0 {
		b.editOrLog(ctx, i, "Cleared. Event messages will go up without pinging anyone.")
		return
	}
	b.editOrLog(ctx, i, "Event messages will ping "+mentionList(current.EventMentionRoleIDs)+".")
}

// mentionList renders role ids as Discord mentions. Inside an ephemeral confirmation
// these render as role names without pinging anyone, which is what makes them readable
// back to the admin who just set them.
func mentionList(roleIDs []string) string {
	parts := make([]string, len(roleIDs))
	for i, id := range roleIDs {
		parts[i] = "<@&" + id + ">"
	}
	return strings.Join(parts, " ")
}

// setEventBanner points every event card at one image. Admin only. Passing no url
// clears it.
func (b *Bot) setEventBanner(ctx context.Context, i *discordgo.InteractionCreate, options []*discordgo.ApplicationCommandInteractionDataOption) {
	actor, current, ok := b.beginConfigChange(ctx, i)
	if !ok {
		return
	}

	var bannerURL *string
	for _, option := range options {
		if option.Name == "url" {
			if raw := strings.TrimSpace(option.StringValue()); raw != "" {
				bannerURL = &raw
			}
		}
	}

	current.EventBannerURL = bannerURL
	if _, err := b.api.SetGuildSettings(ctx, actor, current); err != nil {
		b.fail(ctx, i, "setting event banner", err)
		return
	}

	if bannerURL == nil {
		b.editOrLog(ctx, i, "Cleared. Event cards will go up without artwork.")
		return
	}
	b.editOrLog(ctx, i, "Event cards will show that image.")
}

// setReminder sets the guild's default reminder lead time and how it is delivered.
// Admin only. An event can still name its own lead time; this is what one gets when it
// does not.
//
// Both options are cleared by leaving them out, which returns the guild to 30 minutes
// as a ping. There is no way to write one and keep the other: the settings resource is
// a whole-row PUT, and pretending otherwise here would hide it rather than remove it.
func (b *Bot) setReminder(ctx context.Context, i *discordgo.InteractionCreate, options []*discordgo.ApplicationCommandInteractionDataOption) {
	actor, current, ok := b.beginConfigChange(ctx, i)
	if !ok {
		return
	}

	var minutes *int
	var delivery *string
	for _, option := range options {
		switch option.Name {
		case "minutes":
			value := int(option.IntValue())
			minutes = &value
		case "delivery":
			value := option.StringValue()
			delivery = &value
		}
	}

	current.ReminderLeadMinutes = minutes
	current.ReminderDelivery = delivery
	if _, err := b.api.SetGuildSettings(ctx, actor, current); err != nil {
		b.fail(ctx, i, "setting reminder", err)
		return
	}

	b.editOrLog(ctx, i, reminderSummary(minutes, delivery))
}

func reminderSummary(minutes *int, delivery *string) string {
	lead := "30 minutes"
	if minutes != nil {
		if *minutes == 0 {
			return "Reminders are off for new events. An event can still ask for one with the `reminder` option."
		}
		lead = strconv.Itoa(*minutes) + " minutes"
	}

	how := "a ping in the events channel"
	if delivery != nil {
		switch *delivery {
		case client.ReminderDeliveryDM:
			how = "a DM to each of them"
		case client.ReminderDeliveryBoth:
			how = "a ping in the events channel and a DM to each of them"
		}
	}

	return "Everyone signed up gets " + how + " " + lead + " before an event starts."
}
