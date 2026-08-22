package discord

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/Raider-Mate/raider-mate-discord-bot/internal/client"
)

// createEvent creates a raid from the command's options and posts its signup sheet.
//
// The arguments are command options rather than a modal because a modal holds text
// inputs and nothing else: Discord has no date component at all, and a select menu
// inside a modal needs a Label component this version of discordgo cannot build. A
// command option can at least offer difficulty as a real picker.
func (b *Bot) createEvent(ctx context.Context, i *discordgo.InteractionCreate, eventType client.EventType, options []*discordgo.ApplicationCommandInteractionDataOption) {
	actor, err := actorFrom(i)
	if err != nil {
		b.logger.ErrorContext(ctx, "reading actor", "error", err)
		return
	}
	if err := b.deferEphemeral(i); err != nil {
		b.logger.ErrorContext(ctx, "deferring event creation", "error", err)
		return
	}

	settings, err := b.api.GuildSettings(ctx, actor)
	if err != nil {
		b.fail(ctx, i, "reading guild settings", err)
		return
	}

	loc, err := guildLocation(settings.Timezone)
	if err != nil {
		b.editOrLog(ctx, i, "This server's timezone is not one Go recognises. An admin can fix it with `/config timezone`.")
		return
	}

	named := optionsByName(options)
	now := time.Now()

	startsAt, err := parseEventTime(named["starts"].StringValue(), now, loc)
	if err != nil {
		b.editOrLog(ctx, i, cannotReadTime("start time", settings.Timezone))
		return
	}

	deadline := defaultDeadline(startsAt, now)
	if raw, ok := named["signups_close"]; ok {
		deadline, err = parseEventTime(raw.StringValue(), now, loc)
		if err != nil {
			b.editOrLog(ctx, i, cannotReadTime("signup deadline", settings.Timezone))
			return
		}
	}
	if deadline.After(startsAt) {
		b.editOrLog(ctx, i, "Signups would close after the raid starts. Check the dates.")
		return
	}

	var difficulty *client.Difficulty
	if raw, ok := named["difficulty"]; ok {
		value := client.Difficulty(raw.StringValue())
		difficulty = &value
	}

	// Left out stays nil so the service applies the guild's default. Zero is a raid lead
	// asking for no reminder, which is not the same thing.
	var reminder *int
	if raw, ok := named["reminder"]; ok {
		minutes := int(raw.IntValue())
		reminder = &minutes
	}

	event, err := b.api.CreateEvent(ctx, actor, client.CreateEvent{
		Type:                eventType,
		Title:               strings.TrimSpace(named["title"].StringValue()),
		StartsAt:            startsAt,
		SignupDeadline:      deadline,
		CompTemplate:        templateFrom(named),
		Difficulty:          difficulty,
		ReminderLeadMinutes: reminder,
	})
	if err != nil {
		b.fail(ctx, i, "creating event", err)
		return
	}

	channelID := i.ChannelID
	if settings.EventsChannelID != nil {
		channelID = *settings.EventsChannelID
	}

	message, err := b.session.ChannelMessageSendComplex(channelID, &discordgo.MessageSend{
		// The ping lives in the message content, not the embed. Mentions inside an
		// embed render as role names and notify nobody, which is exactly wrong for the
		// one message a guild wants its raiders to see.
		Content:         mentionList(settings.EventMentionRoleIDs),
		Embeds:          []*discordgo.MessageEmbed{BuildEventEmbed(b.viewFor(event, nil, nil, settings))},
		Components:      eventButtons(event.ID),
		AllowedMentions: allowedRoleMentions(settings.EventMentionRoleIDs),
	}, discordgo.WithContext(ctx))
	if err != nil {
		b.fail(ctx, i, "posting event message", err)
		return
	}

	// Without this the service has no channel to post a deadline or comp-nag
	// notification in, and drops those jobs rather than queueing something it cannot
	// deliver. It is also how every later edit finds the message again.
	if _, err := b.api.AttachMessage(ctx, actor, event.ID, message.ChannelID, message.ID); err != nil {
		b.fail(ctx, i, "recording event message", err)
		return
	}

	b.editOrLog(ctx, i, "Posted in <#"+channelID+">. Event id `"+event.ID+"`, also in the message footer.")
}

func (b *Bot) editOrLog(ctx context.Context, i *discordgo.InteractionCreate, message string) {
	if err := b.edit(i, message, nil); err != nil {
		b.logger.ErrorContext(ctx, "editing interaction response", "error", err)
	}
}

func optionsByName(options []*discordgo.ApplicationCommandInteractionDataOption) map[string]*discordgo.ApplicationCommandInteractionDataOption {
	out := make(map[string]*discordgo.ApplicationCommandInteractionDataOption, len(options))
	for _, option := range options {
		out[option.Name] = option
	}
	return out
}

// templateFrom reads the comp size options. Anything left out stays nil and the service
// derives it, which is not the same as asking for none.
//
// A dungeon offers none of these, so it always sends an empty template. That is
// deliberate: a Mythic+ group is five people with one tank and one healer, and that
// rule lives in the service's ModeMythicPlus, not here (hard rule 3).
func templateFrom(named map[string]*discordgo.ApplicationCommandInteractionDataOption) client.CompTemplate {
	var tmpl client.CompTemplate
	for name, field := range map[string]**int{
		"tanks":   &tmpl.Tanks,
		"healers": &tmpl.Healers,
		"melee":   &tmpl.MaxMelee,
		"ranged":  &tmpl.MaxRanged,
	} {
		if option, ok := named[name]; ok {
			count := int(option.IntValue())
			*field = &count
		}
	}
	return tmpl
}

// guildLocation resolves the guild's configured zone. A guild that has set none falls
// back to UTC, which only matters for a time written without an offset, and the message
// on a parse failure says so.
func guildLocation(timezone *string) (*time.Location, error) {
	if timezone == nil || *timezone == "" {
		return time.UTC, nil
	}
	return time.LoadLocation(*timezone)
}

func cannotReadTime(field string, timezone *string) string {
	message := fmt.Sprintf("I could not read the %s. Try `%s`, `sat 20:00`, or `2026-09-01 20:00`.", field, timeExample)
	if timezone == nil || *timezone == "" {
		message += "\nThis server has no timezone set, so times without an offset are read as UTC. An admin can fix that with `/config timezone`."
	}
	return message
}

// allowedRoleMentions permits exactly the configured roles to ping and nothing else.
//
// Without an explicit allow list Discord falls back to parsing the content, which would
// let an @everyone in a raid title ping the server. Naming the ids also means a role
// removed from the setting stops pinging even if an older message still contains it.
func allowedRoleMentions(roleIDs []string) *discordgo.MessageAllowedMentions {
	if len(roleIDs) == 0 {
		return &discordgo.MessageAllowedMentions{Parse: []discordgo.AllowedMentionType{}}
	}
	return &discordgo.MessageAllowedMentions{
		Parse: []discordgo.AllowedMentionType{},
		Roles: roleIDs,
	}
}
