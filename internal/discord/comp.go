package discord

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"

	"github.com/Phage-Solutions/raider-mate-discord-bot/internal/client"
)

// showComp renders the current board without changing anything.
func (b *Bot) showComp(ctx context.Context, i *discordgo.InteractionCreate, eventID string) {
	actor, err := actorFrom(i)
	if err != nil {
		b.logger.ErrorContext(ctx, "reading actor", "error", err)
		return
	}
	if err := b.deferEphemeral(i); err != nil {
		b.logger.ErrorContext(ctx, "deferring comp show", "error", err)
		return
	}

	board, err := b.api.Comp(ctx, actor, eventID, client.DefaultComp)
	if errors.Is(err, client.ErrNotFound) {
		b.editOrLog(ctx, i, "Nothing locked yet for that event.")
		return
	}
	if err != nil {
		b.fail(ctx, i, "reading comp", err)
		return
	}

	b.editOrLog(ctx, i, renderBoard(board))
}

// lockComp runs the assigner and puts the result back on the event message.
//
// The lock link is the permission check: the service omits it for anyone who is not a
// raid lead, and for a manual comp, which is why this asks for the comp list first
// instead of trying and reading the refusal.
func (b *Bot) lockComp(ctx context.Context, i *discordgo.InteractionCreate, eventID string) {
	actor, err := actorFrom(i)
	if err != nil {
		b.logger.ErrorContext(ctx, "reading actor", "error", err)
		return
	}
	if err := b.deferEphemeral(i); err != nil {
		b.logger.ErrorContext(ctx, "deferring comp lock", "error", err)
		return
	}

	board, err := b.api.LockComp(ctx, actor, eventID, client.DefaultComp)
	if err != nil {
		b.fail(ctx, i, "locking comp", err)
		return
	}

	b.editOrLog(ctx, i, renderBoard(board))
	b.embeds.schedule(actor, eventID)
}

// renderBoard is the raid lead's view: every seat with the sentence explaining it.
// "Why did Bob get benched" is the question that gets asked, and the answer is already
// in the response, so hiding it would be a choice.
func renderBoard(board client.Board) string {
	if len(board.Slots) == 0 {
		return "Nobody signed up, so there is nothing to assign."
	}

	var b strings.Builder
	fmt.Fprintf(&b, "**%s** (%s)\n", board.Name, strings.ToLower(string(board.Mode)))

	for _, slot := range board.Slots {
		name := "unknown raider"
		if slot.Character != nil {
			name = slot.Character.Name
		}
		seat := roleShortNames[slot.Role]
		if slot.IsBench {
			seat = "bench"
		}
		fmt.Fprintf(&b, "\n`%-6s` %s — %s", seat, name, slot.Reason)
	}

	for _, advisory := range board.Advisories {
		fmt.Fprintf(&b, "\n\n%s", advisory.Message)
	}

	// Discord refuses a message over 2000 characters, and a twenty-five person board
	// with reasons gets there.
	const maxMessage = 2000
	out := b.String()
	if len(out) > maxMessage {
		return out[:maxMessage-3] + "..."
	}
	return out
}

// Locking does not DM the seated roster, and cannot yet: a comp slot names a
// character, and no response on the service exposes the Discord user behind one. The
// REMINDER_1H notification does carry a discord_id and does tell each raider their
// assigned role, so nobody goes uninformed, they are told an hour out rather than at
// lock. Closing that gap means a discord_id on the character summary.
