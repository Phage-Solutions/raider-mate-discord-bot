package discord

import (
	"slices"
	"strings"

	"github.com/bwmarrin/discordgo"

	"github.com/Phage-Solutions/raider-mate-discord-bot/internal/client"
)

// eventButtons are the controls on an event message. All six are available to
// everyone who can see the message, which is why they can live here: a message's
// components are the same for every viewer, so anything that has to be gated per
// caller is a slash command with an ephemeral reply instead. That is also why the
// signup response's allowed_statuses does not reach this function: there is no viewer
// to compute it for.
//
// Discord caps a row at five and refuses the whole message rather than the extra
// button, so the answers fill the first row and Withdraw takes the second. Withdraw is
// the one that moves: it is not an answer, it is taking an answer back.
func eventButtons(eventID string) []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.Button{
				Label:    "Sign up",
				Style:    discordgo.SuccessButton,
				Emoji:    &discordgo.ComponentEmoji{Name: "✅"},
				CustomID: CustomID{Action: ActionSignup, EventID: eventID}.String(),
			},
			discordgo.Button{
				Label:    "Tentative",
				Style:    discordgo.SecondaryButton,
				Emoji:    &discordgo.ComponentEmoji{Name: "❓"},
				CustomID: CustomID{Action: ActionTentative, EventID: eventID}.String(),
			},
			discordgo.Button{
				Label:    "Late",
				Style:    discordgo.SecondaryButton,
				Emoji:    &discordgo.ComponentEmoji{Name: "🕒"},
				CustomID: CustomID{Action: ActionLate, EventID: eventID}.String(),
			},
			discordgo.Button{
				Label:    "Decline",
				Style:    discordgo.DangerButton,
				Emoji:    &discordgo.ComponentEmoji{Name: "❌"},
				CustomID: CustomID{Action: ActionDecline, EventID: eventID}.String(),
			},
			discordgo.Button{
				Label:    "Absent",
				Style:    discordgo.SecondaryButton,
				Emoji:    &discordgo.ComponentEmoji{Name: "🌙"},
				CustomID: CustomID{Action: ActionAbsent, EventID: eventID}.String(),
			},
		}},
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.Button{
				Label:    "Withdraw",
				Style:    discordgo.SecondaryButton,
				CustomID: CustomID{Action: ActionWithdraw, EventID: eventID}.String(),
			},
		}},
	}
}

// roleSelect is the ephemeral multi-select a raider picks their roles in, ordered by
// the priority they set.
//
// Passing current ticks their existing picks, which is right for editing a menu and
// wrong for signing up. Discord only sends an interaction when a selection *changes*,
// so a pre-ticked menu offered as a signup step cannot be submitted at all: the raider
// has to pick something else and change back. The signup path therefore passes nil and
// is only shown to raiders who have no menu yet.
func roleSelect(eventID, characterID string, current []client.RoleChoice) []discordgo.MessageComponent {
	chosen := make(map[client.Role]bool, len(current))
	for _, rc := range current {
		chosen[rc.Role] = true
	}

	options := make([]discordgo.SelectMenuOption, 0, len(roleColumns))
	for _, column := range orderByPriority(current) {
		options = append(options, discordgo.SelectMenuOption{
			Label:   column.Label,
			Value:   string(column.Role),
			Default: chosen[column.Role],
		})
	}

	return []discordgo.MessageComponent{
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.SelectMenu{
				CustomID:    CustomID{Action: ActionRoles, EventID: eventID, CharacterID: characterID}.String(),
				Placeholder: "Everything you can play, best first",
				MinValues:   ptr(1),
				MaxValues:   len(options),
				Options:     options,
			},
		}},
	}
}

// maxSelectOptions is Discord's cap on a select menu. A raider with more alts than this
// loses the tail of the list, which is why the main sorts first: the option that must
// never be the one cut.
const maxSelectOptions = 25

// characterSelect asks which character the raider is acting as. then is what happens to
// the one they pick, and arg carries the single extra value that flow needs, since the
// pick arrives as a fresh interaction with no memory of the one before it.
//
// Unlike roleSelect, no option is ticked by default. Discord sends nothing when a
// selection does not change, so a pre-ticked option is one the raider cannot choose.
func characterSelect(eventID string, then Action, arg string, characters []client.Character) []discordgo.MessageComponent {
	sorted := mainFirst(characters)
	if len(sorted) > maxSelectOptions {
		sorted = sorted[:maxSelectOptions]
	}

	options := make([]discordgo.SelectMenuOption, 0, len(sorted))
	for _, c := range sorted {
		options = append(options, discordgo.SelectMenuOption{
			Label:       c.Name + "-" + c.Realm,
			Value:       c.ID,
			Description: characterDescription(c),
		})
	}

	return []discordgo.MessageComponent{
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.SelectMenu{
				CustomID:    CustomID{Action: ActionPick, EventID: eventID, Then: then, Arg: arg}.String(),
				Placeholder: "Which character?",
				MinValues:   ptr(1),
				MaxValues:   1,
				Options:     options,
			},
		}},
	}
}

// characterDescription is the grey line under an option: main or alt, and the class once
// the Raider.IO sync has filled it in.
func characterDescription(c client.Character) string {
	kind := "Alt"
	if c.IsMain {
		kind = "Main"
	}
	if c.Class == nil || *c.Class == "" {
		return kind
	}
	return kind + " - " + *c.Class
}

// mainFirst orders a raider's characters the way they think of them: the main, then the
// alts alphabetically. Returns a copy, since the caller's slice came off an API response
// other code is still reading.
func mainFirst(characters []client.Character) []client.Character {
	sorted := slices.Clone(characters)
	slices.SortStableFunc(sorted, func(a, b client.Character) int {
		if a.IsMain != b.IsMain {
			if a.IsMain {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Name, b.Name)
	})
	return sorted
}

// orderByPriority puts a raider's existing picks at the top in the order they ranked
// them, then the roles they have not chosen. Two clicks is the target for the common
// case, and that only holds if the first thing in the list is the thing they play.
func orderByPriority(current []client.RoleChoice) []struct {
	Role  client.Role
	Label string
} {
	type column = struct {
		Role  client.Role
		Label string
	}

	labels := make(map[client.Role]string, len(roleColumns))
	for _, c := range roleColumns {
		labels[c.Role] = c.Label
	}

	ordered := make([]column, 0, len(roleColumns))
	seen := make(map[client.Role]bool, len(current))
	for _, rc := range sortedByPriority(current) {
		if label, ok := labels[rc.Role]; ok && !seen[rc.Role] {
			ordered = append(ordered, column{Role: rc.Role, Label: label})
			seen[rc.Role] = true
		}
	}
	for _, c := range roleColumns {
		if !seen[c.Role] {
			ordered = append(ordered, column{Role: c.Role, Label: c.Label})
		}
	}
	return ordered
}

// addCharacterButton is the way out of the first-time path. Discord will not open a
// modal in reply to a deferred interaction, and the bot only learns a raider has no
// characters after deferring, so the modal opens from this fresh click instead.
func addCharacterButton(eventID string) []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.Button{
				Label:    "Add a character",
				Style:    discordgo.PrimaryButton,
				CustomID: CustomID{Action: ActionAddCharacter, EventID: eventID}.String(),
			},
		}},
	}
}

// removeConfirmButton is the second press of /character remove. Danger styled and
// labelled with the verb rather than "Yes", so a stray click on an old ephemeral
// message reads as what it does.
func removeConfirmButton(characterID string) []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.Button{
				Label:    "Remove it",
				Style:    discordgo.DangerButton,
				CustomID: CustomID{Action: ActionRemoveConfirm, CharacterID: characterID}.String(),
			},
		}},
	}
}

func ptr[T any](v T) *T { return &v }

// mentionRoleSelect is the role picker behind /config mentions. MinValues zero so an
// empty submission is how a guild says "ping nobody", rather than needing a separate
// clear command.
func mentionRoleSelect() []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{Components: []discordgo.MessageComponent{
			discordgo.SelectMenu{
				MenuType:    discordgo.RoleSelectMenu,
				CustomID:    CustomID{Action: ActionEventMentions}.String(),
				Placeholder: "Roles to ping, or none",
				MinValues:   ptr(0),
				MaxValues:   10,
			},
		}},
	}
}
