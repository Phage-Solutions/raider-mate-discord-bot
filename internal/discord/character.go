package discord

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/bwmarrin/discordgo"

	"github.com/Phage-Solutions/raider-mate-discord-bot/internal/client"
)

const (
	fieldCharacterName = "name"
	fieldRealm         = "realm"
	fieldRegion        = "region"
)

// openCharacterModal is the /character add entry point. A command interaction has not
// been deferred, so the modal can open as its immediate response.
func (b *Bot) openCharacterModal(ctx context.Context, i *discordgo.InteractionCreate) {
	b.openCharacterModalFor(ctx, i, "")
}

// openCharacterModalFor opens the registration modal. eventID is carried through when
// this came from the signup path, so the raider lands back on the role select for the
// event they were looking at rather than nowhere.
func (b *Bot) openCharacterModalFor(ctx context.Context, i *discordgo.InteractionCreate, eventID string) {
	err := b.session.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: CustomID{Action: ActionCharModal, EventID: eventID}.String(),
			Title:    "Register a character",
			Components: []discordgo.MessageComponent{
				textInputRow(fieldCharacterName, "Character name", "Danthrax", true),
				textInputRow(fieldRealm, "Realm", "Draenor", true),
				textInputRow(fieldRegion, "Region", "eu", true),
			},
		},
	})
	if err != nil {
		b.logger.ErrorContext(ctx, "opening character modal", "error", err)
	}
}

func (b *Bot) onModalSubmit(ctx context.Context, i *discordgo.InteractionCreate) {
	id, err := ParseCustomID(i.ModalSubmitData().CustomID)
	if err != nil {
		b.logger.WarnContext(ctx, "unroutable modal", "error", err)
		return
	}

	switch id.Action {
	case ActionCharModal:
		b.registerCharacter(ctx, i, id.EventID)
	case ActionLateModal:
		b.submitLate(ctx, i, id.EventID)
	default:
		b.logger.WarnContext(ctx, "unrouted modal", "action", id.Action)
	}
}

func (b *Bot) registerCharacter(ctx context.Context, i *discordgo.InteractionCreate, eventID string) {
	actor, err := actorFrom(i)
	if err != nil {
		b.logger.ErrorContext(ctx, "reading actor", "error", err)
		return
	}
	if err := b.deferEphemeral(i); err != nil {
		b.logger.ErrorContext(ctx, "deferring character registration", "error", err)
		return
	}

	fields := modalFields(i.ModalSubmitData())
	character, err := b.api.RegisterCharacter(ctx, actor,
		strings.TrimSpace(fields[fieldCharacterName]),
		strings.TrimSpace(fields[fieldRealm]),
		strings.ToLower(strings.TrimSpace(fields[fieldRegion])),
		// The first character a raider registers is their main until they say
		// otherwise. Nobody wants a second modal question about it.
		true,
	)
	if err != nil {
		b.fail(ctx, i, "registering character", err)
		return
	}

	// Class, spec and ilvl arrive on the service's next Raider.IO sync, so promising
	// them now would be a lie the embed then has to keep.
	message := fmt.Sprintf("Registered **%s-%s**%s. Gear and score fill in shortly.",
		character.Name, character.Realm, altMarker(character.IsMain))

	if eventID == "" {
		message += "\nUse `/character roles` to say what it can play."
		if !character.IsMain {
			message += " `/character main` if it should be your main."
		}
		if err := b.edit(i, message, nil); err != nil {
			b.logger.ErrorContext(ctx, "confirming registration", "error", err)
		}
		return
	}

	message += "\nNow, what can it play tonight?"
	if err := b.edit(i, message, roleSelect(eventID, character.ID, nil)); err != nil {
		b.logger.ErrorContext(ctx, "showing role select after registration", "error", err)
	}
}

// startRoleEdit is /character roles: the same select as the signup path, with no event
// attached, so submitting it writes the menu and nothing else.
func (b *Bot) startRoleEdit(ctx context.Context, i *discordgo.InteractionCreate) {
	actor, err := actorFrom(i)
	if err != nil {
		b.logger.ErrorContext(ctx, "reading actor", "error", err)
		return
	}
	if err := b.deferEphemeral(i); err != nil {
		b.logger.ErrorContext(ctx, "deferring role edit", "error", err)
		return
	}

	characters, err := b.api.UserCharacters(ctx, actor, actor.DiscordID)
	if err != nil {
		b.fail(ctx, i, "listing characters", err)
		return
	}
	if len(characters) == 0 {
		if err := b.edit(i, "Register a character first with `/character add`.", nil); err != nil {
			b.logger.ErrorContext(ctx, "answering role edit", "error", err)
		}
		return
	}

	character, ok := b.chooseCharacter(ctx, i, characters, "", ActionRoles, "")
	if !ok {
		return
	}
	b.showRoleEdit(ctx, i, actor, character)
}

// showRoleEdit opens the role menu of one character for editing.
func (b *Bot) showRoleEdit(ctx context.Context, i *discordgo.InteractionCreate, actor client.Actor, character client.Character) {
	roles, err := b.api.CharacterRoles(ctx, actor, character.ID)
	if err != nil {
		b.fail(ctx, i, "reading role menu", err)
		return
	}

	// The nudge is not padding. Discord sends nothing when a selection is unchanged, so
	// a raider who opens this and re-picks the same roles sees no response and
	// reasonably concludes it is broken.
	prompt := fmt.Sprintf("What can **%s**%s play? Best first.\nChange the selection to save; picking the same roles again does nothing.",
		character.Name, altMarker(character.IsMain))
	if err := b.edit(i, prompt, roleSelect("", character.ID, roles)); err != nil {
		b.logger.ErrorContext(ctx, "showing role select", "error", err)
	}
}

// startMainSwitch is /character main. Only the alts are offered: promoting the character
// that already holds the flag writes nothing, and Discord sends no interaction at all
// when a selection does not change, so listing the main would be an option that silently
// does nothing when picked.
func (b *Bot) startMainSwitch(ctx context.Context, i *discordgo.InteractionCreate) {
	actor, err := actorFrom(i)
	if err != nil {
		b.logger.ErrorContext(ctx, "reading actor", "error", err)
		return
	}
	if err := b.deferEphemeral(i); err != nil {
		b.logger.ErrorContext(ctx, "deferring main switch", "error", err)
		return
	}

	characters, err := b.api.UserCharacters(ctx, actor, actor.DiscordID)
	if err != nil {
		b.fail(ctx, i, "listing characters", err)
		return
	}
	if len(characters) == 0 {
		b.editOrLog(ctx, i, "Register a character first with `/character add`.")
		return
	}
	if len(characters) == 1 {
		b.editOrLog(ctx, i, fmt.Sprintf("**%s** is your only character, so it is already your main.", characters[0].Name))
		return
	}

	alts := make([]client.Character, 0, len(characters)-1)
	for _, c := range characters {
		if !c.IsMain {
			alts = append(alts, c)
		}
	}
	if len(alts) == 0 {
		b.editOrLog(ctx, i, "Nothing to switch to.")
		return
	}

	if err := b.edit(i, "Which character is your main?", characterSelect("", ActionSetMain, "", alts)); err != nil {
		b.logger.ErrorContext(ctx, "showing main select", "error", err)
	}
}

// promoteToMain moves the main flag. The service demotes the previous main in the same
// transaction, so there is no second call and no moment where a raider has two mains.
func (b *Bot) promoteToMain(ctx context.Context, i *discordgo.InteractionCreate, actor client.Actor, character client.Character) {
	updated, err := b.api.SetCharacterMain(ctx, actor, character.ID, true)
	if err != nil {
		b.fail(ctx, i, "setting main", err)
		return
	}

	message := fmt.Sprintf("**%s-%s** is your main now. Everything else is an alt.",
		updated.Name, updated.Realm)
	if err := b.edit(i, message, nil); err != nil {
		b.logger.ErrorContext(ctx, "confirming main switch", "error", err)
	}
}

// startCharacterRemove is /character remove. The pick does not delete: the service
// cascades the delete to signups, comp slots and gear snapshots, so the raider is told
// that and asked to press a second button.
func (b *Bot) startCharacterRemove(ctx context.Context, i *discordgo.InteractionCreate) {
	actor, err := actorFrom(i)
	if err != nil {
		b.logger.ErrorContext(ctx, "reading actor", "error", err)
		return
	}
	if err := b.deferEphemeral(i); err != nil {
		b.logger.ErrorContext(ctx, "deferring character removal", "error", err)
		return
	}

	characters, err := b.api.UserCharacters(ctx, actor, actor.DiscordID)
	if err != nil {
		b.fail(ctx, i, "listing characters", err)
		return
	}
	if len(characters) == 0 {
		b.editOrLog(ctx, i, "You have no characters registered.")
		return
	}

	character, ok := b.chooseCharacter(ctx, i, characters, "", ActionRemove, "")
	if !ok {
		return
	}
	b.confirmRemove(ctx, i, character)
}

// confirmRemove is the second press. Naming the character in the prompt is the point:
// the picker is skipped entirely for a raider with one character, so this is the only
// place the cascade gets said out loud.
func (b *Bot) confirmRemove(ctx context.Context, i *discordgo.InteractionCreate, character client.Character) {
	prompt := fmt.Sprintf("Remove **%s-%s**%s? Its signups and comp slots go with it, and none of it comes back.",
		character.Name, character.Realm, altMarker(character.IsMain))
	if err := b.edit(i, prompt, removeConfirmButton(character.ID)); err != nil {
		b.logger.ErrorContext(ctx, "showing removal confirmation", "error", err)
	}
}

// removeCharacter is the confirm button coming back. The character id rode in on the
// custom_id, which is client-supplied, so it is matched against the raider's own roster
// before anything is deleted.
func (b *Bot) removeCharacter(ctx context.Context, i *discordgo.InteractionCreate, characterID string) {
	actor, err := actorFrom(i)
	if err != nil {
		b.logger.ErrorContext(ctx, "reading actor", "error", err)
		return
	}
	if err := b.deferEphemeral(i); err != nil {
		b.logger.ErrorContext(ctx, "deferring character removal", "error", err)
		return
	}

	characters, err := b.api.UserCharacters(ctx, actor, actor.DiscordID)
	if err != nil {
		b.fail(ctx, i, "listing characters", err)
		return
	}
	idx := slices.IndexFunc(characters, func(c client.Character) bool { return c.ID == characterID })
	if idx < 0 {
		b.editOrLog(ctx, i, "That character is not on your roster anymore.")
		return
	}
	character := characters[idx]

	if err := b.api.DeleteCharacter(ctx, actor, character.ID); err != nil {
		b.fail(ctx, i, "removing character", err)
		return
	}

	message := fmt.Sprintf("**%s-%s** is gone.", character.Name, character.Realm)
	// Which character holds the main flag afterwards is the service's business, so
	// this points at the command rather than claiming an answer.
	if len(characters) > 1 {
		message += " Use `/character main` to set which of the rest is your main."
	}
	if err := b.edit(i, message, nil); err != nil {
		b.logger.ErrorContext(ctx, "confirming removal", "error", err)
	}
}

func textInputRow(id, label, placeholder string, required bool) discordgo.ActionsRow {
	return discordgo.ActionsRow{Components: []discordgo.MessageComponent{
		discordgo.TextInput{
			CustomID:    id,
			Label:       label,
			Style:       discordgo.TextInputShort,
			Placeholder: placeholder,
			Required:    required,
		},
	}}
}

func modalFields(data discordgo.ModalSubmitInteractionData) map[string]string {
	out := map[string]string{}
	for _, row := range data.Components {
		actions, ok := row.(*discordgo.ActionsRow)
		if !ok {
			continue
		}
		for _, component := range actions.Components {
			input, ok := component.(*discordgo.TextInput)
			if ok {
				out[input.CustomID] = input.Value
			}
		}
	}
	return out
}
