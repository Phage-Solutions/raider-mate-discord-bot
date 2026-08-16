package discord

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"

	"github.com/Phage-Solutions/raider-mate-discord-bot/internal/client"
)

func (b *Bot) onComponent(ctx context.Context, i *discordgo.InteractionCreate) {
	id, err := ParseCustomID(i.MessageComponentData().CustomID)
	if err != nil {
		if errors.Is(err, ErrForeignComponent) {
			return
		}
		// A pinned event message from an older deploy. Say so rather than guessing.
		if replyErr := b.replyEphemeral(i, userMessage(err)); replyErr != nil {
			b.logger.ErrorContext(ctx, "answering a stale component", "error", replyErr)
		}
		return
	}

	switch id.Action {
	case ActionSignup:
		b.startSignup(ctx, i, id.EventID)
	case ActionTentative:
		b.setStatus(ctx, i, id.EventID, client.StatusTentative)
	case ActionDecline:
		b.setStatus(ctx, i, id.EventID, client.StatusDeclined)
	case ActionAbsent:
		b.setStatus(ctx, i, id.EventID, client.StatusAbsent)
	case ActionLate:
		b.openLateModal(ctx, i, id.EventID)
	case ActionWithdraw:
		b.withdraw(ctx, i, id.EventID)
	case ActionRoles:
		b.submitRoles(ctx, i, id.EventID, id.CharacterID)
	case ActionAddCharacter:
		b.openCharacterModalFor(ctx, i, id.EventID)
	case ActionEventMentions:
		b.submitEventMentions(ctx, i)
	case ActionCharModal, ActionLateModal:
		b.logger.WarnContext(ctx, "component carried a modal action", "action", id.Action)
	}
}

// fieldLateUntil is the arrival time on the late modal.
const fieldLateUntil = "late_until"

// openLateModal asks when the raider will get there. LATE without a late_until is a
// status a raid lead cannot act on, and no single click can supply one, so this is the
// one signup control that opens a modal.
//
// It responds directly rather than deferring, and calls no API first: Discord refuses
// to open a modal in reply to a deferred interaction (hard rule 1's stated exception).
// The cost is that a raider with no characters registered only finds out on submit.
func (b *Bot) openLateModal(ctx context.Context, i *discordgo.InteractionCreate, eventID string) {
	err := b.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: CustomID{Action: ActionLateModal, EventID: eventID}.String(),
			Title:    "Running late",
			Components: []discordgo.MessageComponent{
				textInputRow(fieldLateUntil, "When will you get there?", "20:30", true),
			},
		},
	})
	if err != nil {
		b.logger.ErrorContext(ctx, "opening late modal", "error", err)
	}
}

// submitLate writes the LATE signup with the arrival time the raider typed.
func (b *Bot) submitLate(ctx context.Context, i *discordgo.InteractionCreate, eventID string) {
	actor, err := actorFrom(i)
	if err != nil {
		b.logger.ErrorContext(ctx, "reading actor", "error", err)
		return
	}
	if err := b.deferEphemeral(i); err != nil {
		b.logger.ErrorContext(ctx, "deferring late submit", "error", err)
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

	event, err := b.api.Event(ctx, actor, eventID)
	if err != nil {
		b.fail(ctx, i, "reading event", err)
		return
	}

	// Anchored on the raid's start rather than on now: someone answering at lunchtime
	// who types "20:30" means half an hour into tonight's raid, and parseEventTime
	// reads a bare clock time as the next one after the anchor it is given.
	raw := modalFields(i.ModalSubmitData())[fieldLateUntil]
	lateUntil, err := parseEventTime(raw, event.StartsAt, loc)
	if err != nil {
		b.editOrLog(ctx, i, cannotReadTime("arrival time", settings.Timezone))
		return
	}
	if !lateUntil.After(event.StartsAt) {
		b.editOrLog(ctx, i, "That is not after the raid starts, so you are not late. Use `Sign up` instead.")
		return
	}

	characters, err := b.api.UserCharacters(ctx, actor, actor.DiscordID)
	if err != nil {
		b.fail(ctx, i, "listing characters", err)
		return
	}
	if len(characters) == 0 {
		const invite = "You have no characters registered yet, so there is nothing to answer with."
		if err := b.edit(i, invite, addCharacterButton(eventID)); err != nil {
			b.logger.ErrorContext(ctx, "offering registration", "error", err)
		}
		return
	}

	result, err := b.api.Signup(ctx, actor, eventID, pickMain(characters).ID,
		client.WriteSignup{Status: client.StatusLate, LateUntil: &lateUntil})
	if err != nil {
		b.fail(ctx, i, "writing signup", err)
		return
	}

	message := signupConfirmation(result, nil)
	if !result.Filed() {
		message = "Noted: late, from " + discordTime(lateUntil.Unix(), "t") + "."
	}
	if err := b.edit(i, message, nil); err != nil {
		b.logger.ErrorContext(ctx, "confirming late signup", "error", err)
	}
	b.embeds.schedule(actor, eventID)
}

// startSignup opens the role select for the raider's character, or sends them down the
// registration path if they have none.
func (b *Bot) startSignup(ctx context.Context, i *discordgo.InteractionCreate, eventID string) {
	actor, err := actorFrom(i)
	if err != nil {
		b.logger.ErrorContext(ctx, "reading actor", "error", err)
		return
	}
	if err := b.deferEphemeral(i); err != nil {
		b.logger.ErrorContext(ctx, "deferring signup", "error", err)
		return
	}

	characters, err := b.api.UserCharacters(ctx, actor, actor.DiscordID)
	if err != nil {
		b.fail(ctx, i, "listing characters", err)
		return
	}

	if len(characters) == 0 {
		const invite = "You have no characters registered yet. That takes about twenty seconds, once."
		if err := b.edit(i, invite, addCharacterButton(eventID)); err != nil {
			b.logger.ErrorContext(ctx, "offering registration", "error", err)
		}
		return
	}

	// v0.1 signs up the main and does not offer alt selection. A raider with alts and
	// no main flag gets the first character the service returned.
	character := pickMain(characters)

	roles, err := b.api.CharacterRoles(ctx, actor, character.ID)
	if err != nil {
		b.fail(ctx, i, "reading role menu", err)
		return
	}

	// A raider who has already said what they play is signed up on the spot. Asking
	// them to confirm a menu they set weeks ago is a click that answers nothing, and a
	// select menu cannot even be submitted unchanged: Discord only sends an interaction
	// when the selection differs from what is already ticked, so a returning raider had
	// no way to say yes at all.
	if len(roles) > 0 {
		b.confirmSignup(ctx, i, actor, eventID, character, roles)
		return
	}

	prompt := fmt.Sprintf("Signing up **%s**. What can you play?", character.Name)
	if err := b.edit(i, prompt, roleSelect(eventID, character.ID, nil)); err != nil {
		b.logger.ErrorContext(ctx, "showing role select", "error", err)
	}
}

// confirmSignup writes the signup for a raider whose role menu already exists.
func (b *Bot) confirmSignup(ctx context.Context, i *discordgo.InteractionCreate, actor client.Actor, eventID string, character client.Character, roles []client.RoleChoice) {
	result, err := b.api.Signup(ctx, actor, eventID, character.ID, client.WriteSignup{Status: client.StatusConfirmed})
	if err != nil {
		b.fail(ctx, i, "writing signup", err)
		return
	}

	message := signupConfirmation(result, roles)
	if !result.Filed() {
		message += "\nPlaying something else? `/character roles`."
	}
	if err := b.edit(i, message, nil); err != nil {
		b.logger.ErrorContext(ctx, "confirming signup", "error", err)
	}
	b.embeds.schedule(actor, eventID)
}

// submitRoles writes the role menu the raider picked, then the signup itself. Two
// writes, because roles live on the character and not on the signup: that is what lets
// the assigner move a flex raider between roles without asking them again.
func (b *Bot) submitRoles(ctx context.Context, i *discordgo.InteractionCreate, eventID, characterID string) {
	actor, err := actorFrom(i)
	if err != nil {
		b.logger.ErrorContext(ctx, "reading actor", "error", err)
		return
	}
	if err := b.deferEphemeral(i); err != nil {
		b.logger.ErrorContext(ctx, "deferring role submit", "error", err)
		return
	}

	// Selected order is the priority order: the raider picked their best first.
	picked := i.MessageComponentData().Values
	roles := make([]client.RoleChoice, len(picked))
	for idx, raw := range picked {
		roles[idx] = client.RoleChoice{Role: client.Role(raw), Priority: int16(idx + 1)}
	}

	if err := b.api.SetCharacterRoles(ctx, actor, characterID, roles); err != nil {
		b.fail(ctx, i, "setting role menu", err)
		return
	}

	result, err := b.api.Signup(ctx, actor, eventID, characterID, client.WriteSignup{Status: client.StatusConfirmed})
	if err != nil {
		b.fail(ctx, i, "writing signup", err)
		return
	}

	if err := b.edit(i, signupConfirmation(result, roles), nil); err != nil {
		b.logger.ErrorContext(ctx, "confirming signup", "error", err)
	}
	b.embeds.schedule(actor, eventID)
}

// setStatus is the one-click path: tentative, decline, and absent need no role menu,
// since none of them puts anyone in the raid.
func (b *Bot) setStatus(ctx context.Context, i *discordgo.InteractionCreate, eventID string, status client.SignupStatus) {
	actor, err := actorFrom(i)
	if err != nil {
		b.logger.ErrorContext(ctx, "reading actor", "error", err)
		return
	}
	if err := b.deferEphemeral(i); err != nil {
		b.logger.ErrorContext(ctx, "deferring status write", "error", err)
		return
	}

	characters, err := b.api.UserCharacters(ctx, actor, actor.DiscordID)
	if err != nil {
		b.fail(ctx, i, "listing characters", err)
		return
	}
	if len(characters) == 0 {
		const invite = "You have no characters registered yet, so there is nothing to answer with."
		if err := b.edit(i, invite, addCharacterButton(eventID)); err != nil {
			b.logger.ErrorContext(ctx, "offering registration", "error", err)
		}
		return
	}

	result, err := b.api.Signup(ctx, actor, eventID, pickMain(characters).ID, client.WriteSignup{Status: status})
	if err != nil {
		b.fail(ctx, i, "writing signup", err)
		return
	}

	if err := b.edit(i, signupConfirmation(result, nil), nil); err != nil {
		b.logger.ErrorContext(ctx, "confirming status", "error", err)
	}
	b.embeds.schedule(actor, eventID)
}

// withdraw takes a raider back off the sheet. No confirmation dialog: signing up again
// is one click, so asking "are you sure" would cost more than the mistake.
func (b *Bot) withdraw(ctx context.Context, i *discordgo.InteractionCreate, eventID string) {
	actor, err := actorFrom(i)
	if err != nil {
		b.logger.ErrorContext(ctx, "reading actor", "error", err)
		return
	}
	if err := b.deferEphemeral(i); err != nil {
		b.logger.ErrorContext(ctx, "deferring withdraw", "error", err)
		return
	}

	characters, err := b.api.UserCharacters(ctx, actor, actor.DiscordID)
	if err != nil {
		b.fail(ctx, i, "listing characters", err)
		return
	}
	if len(characters) == 0 {
		if err := b.edit(i, "You are not signed up.", nil); err != nil {
			b.logger.ErrorContext(ctx, "answering withdraw", "error", err)
		}
		return
	}

	result, err := b.api.Withdraw(ctx, actor, eventID, pickMain(characters).ID)
	if err != nil {
		b.fail(ctx, i, "withdrawing", err)
		return
	}

	message := "Off the list. Sign up again whenever."
	if result.Filed() {
		message = "Signups already closed, so a raid lead has to sign that off. They have been told."
	}
	if err := b.edit(i, message, nil); err != nil {
		b.logger.ErrorContext(ctx, "confirming withdraw", "error", err)
	}
	b.embeds.schedule(actor, eventID)
}

// signupConfirmation is the ephemeral line a raider sees after answering.
func signupConfirmation(result client.SignupResult, roles []client.RoleChoice) string {
	if result.Filed() {
		return "Signups already closed, so that went to a raid lead as a request. They have been told."
	}

	if len(roles) == 0 {
		if result.Signup == nil {
			return "Noted."
		}
		return "Noted: " + strings.ToLower(string(result.Signup.Status)) + "."
	}

	names := make([]string, len(roles))
	for i, rc := range roles {
		names[i] = roleShortNames[rc.Role]
	}
	return "You are in as " + strings.Join(names, "/") + "."
}

// pickMain chooses which character a raider is answering with. v0.1 does not offer alt
// selection, so the main is the answer, and the first registration when nobody set one.
func pickMain(characters []client.Character) client.Character {
	for _, c := range characters {
		if c.IsMain {
			return c
		}
	}
	return characters[0]
}
