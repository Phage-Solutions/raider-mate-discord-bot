package discord

import (
	"context"
	"fmt"
	"strings"

	"github.com/bwmarrin/discordgo"
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
	message := fmt.Sprintf("Registered **%s-%s**. Gear and score fill in shortly.",
		character.Name, character.Realm)

	if eventID == "" {
		message += "\nUse `/character roles` to say what it can play."
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

	character := pickMain(characters)
	roles, err := b.api.CharacterRoles(ctx, actor, character.ID)
	if err != nil {
		b.fail(ctx, i, "reading role menu", err)
		return
	}

	// The nudge is not padding. Discord sends nothing when a selection is unchanged, so
	// a raider who opens this and re-picks the same roles sees no response and
	// reasonably concludes it is broken.
	prompt := fmt.Sprintf("What can **%s** play? Best first.\nChange the selection to save; picking the same roles again does nothing.", character.Name)
	if err := b.edit(i, prompt, roleSelect("", character.ID, roles)); err != nil {
		b.logger.ErrorContext(ctx, "showing role select", "error", err)
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
