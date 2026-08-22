package discord

import (
	"context"
	"fmt"

	"github.com/bwmarrin/discordgo"

	"github.com/Raider-Mate/raider-mate-discord-bot/internal/client"
)

// announce posts the signup sheet for an event created somewhere the bot could not
// post from, the dashboard being the one that exists. A slash command posts its own
// sheet and never reaches here.
//
// The channel is the guild's configured events channel, resolved by the service when it
// queued this. There is no "here" to fall back to: nobody ran a command.
func (b *Bot) announce(ctx context.Context, n client.Notification) error {
	if n.ChannelID == nil {
		return fmt.Errorf("event announcement has no channel")
	}

	actor, err := serviceActor(n, b.appID())
	if err != nil {
		return err
	}

	event, err := b.api.Event(ctx, actor, n.EventID)
	if err != nil {
		return fmt.Errorf("reading event to announce: %w", err)
	}

	// Delivery is at-least-once, so a redelivered announcement must not put a second
	// sheet up. An event that already knows its message has been posted.
	if event.MessageID != nil {
		return nil
	}

	// Settings carry the banner and the roles to ping. A failed read costs the artwork
	// and the ping, not the post.
	settings, err := b.api.GuildSettings(ctx, actor)
	if err != nil {
		b.logger.WarnContext(ctx, "reading guild settings to announce an event", "error", err)
	}

	message, err := b.session.ChannelMessageSendComplex(*n.ChannelID, &discordgo.MessageSend{
		Content:         mentionList(settings.EventMentionRoleIDs),
		Embeds:          []*discordgo.MessageEmbed{BuildEventEmbed(b.viewFor(event, nil, nil, settings))},
		Components:      eventButtons(event.ID),
		AllowedMentions: allowedRoleMentions(settings.EventMentionRoleIDs),
	}, discordgo.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("posting announced event: %w", err)
	}

	// Same reason the create command does this: without it the service has no channel
	// for a deadline or comp-nag notification, and no message for a redraw to edit.
	if err := b.api.RecordEventMessage(ctx, event.ID, message.ChannelID, message.ID); err != nil {
		return fmt.Errorf("recording announced event message: %w", err)
	}
	return nil
}
