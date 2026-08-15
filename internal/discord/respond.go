package discord

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/bwmarrin/discordgo"

	"github.com/Phage-Solutions/raider-mate-discord-bot/internal/client"
)

// deferEphemeral answers immediately so the interaction does not expire, then leaves
// the real reply to a follow-up. Every handler that calls the service does this first:
// Discord allows three seconds, and the service is on the other side of a network.
func (b *Bot) deferEphemeral(i *discordgo.InteractionCreate) error {
	return b.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Flags: discordgo.MessageFlagsEphemeral},
	})
}

// edit replaces a deferred interaction's placeholder with the real answer.
func (b *Bot) edit(i *discordgo.InteractionCreate, content string, components []discordgo.MessageComponent) error {
	_, err := b.session.InteractionResponseEdit(i.Interaction, &discordgo.WebhookEdit{
		Content:    &content,
		Components: &components,
	})
	return err
}

// replyEphemeral answers an interaction that was not deferred.
func (b *Bot) replyEphemeral(i *discordgo.InteractionCreate, content string) error {
	return b.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{Content: content, Flags: discordgo.MessageFlagsEphemeral},
	})
}

// fail tells the raider what happened and logs what they should not see.
//
// The split matters. The service writes its 4xx messages for a human, so those are
// worth passing on. Its 5xx messages are "internal error" over whatever actually
// broke, and a stack of Go error wrapping in a Discord message helps nobody.
func (b *Bot) fail(ctx context.Context, i *discordgo.InteractionCreate, what string, err error) {
	b.logger.ErrorContext(ctx, what, "error", err)

	if editErr := b.edit(i, userMessage(err), nil); editErr != nil {
		b.logger.ErrorContext(ctx, "reporting failure to discord", "error", editErr, slog.String("about", what))
	}
}

// userMessage turns an error into something a player reads without being annoyed by
// it. Short, plain, and never apologetic boilerplate.
func userMessage(err error) string {
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		if apiErr.Safe() && apiErr.Message != "" {
			return apiErr.Message
		}
		if apiErr.Status == http.StatusUnauthorized {
			return "The bot is not configured to talk to the roster service. Tell an admin."
		}
		return "The roster service is having a bad time. Try again in a minute."
	}

	switch {
	case errors.Is(err, client.ErrCompIsManual):
		return "That comp is hand-built, so the assigner will not touch it. Edit it in the dashboard, or convert it to auto first."
	case errors.Is(err, client.ErrAlreadyDecided):
		return "Someone else already decided that one."
	case errors.Is(err, ErrStaleComponent):
		return "This event message is out of date. Ask a raid lead to repost it."
	}

	return "Can't reach the roster service, try again in a minute."
}
