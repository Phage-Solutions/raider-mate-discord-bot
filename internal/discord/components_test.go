package discord

import (
	"fmt"
	"testing"

	"github.com/bwmarrin/discordgo"

	"github.com/Raider-Mate/raider-mate-discord-bot/internal/client"
)

// Discord caps a row at five and refuses the whole message rather than the extra
// button, so an event nobody can answer is the failure mode.
func TestEventButtonsStayInsideTheRowCapAndOfferEveryAnswer(t *testing.T) {
	rows := eventButtons("0192f3c8-0000-7000-8000-000000000001")
	if len(rows) > 5 {
		t.Fatalf("rows = %d, over Discord's 5 per message", len(rows))
	}

	actions := map[Action]bool{}
	for _, row := range rows {
		actionsRow, ok := row.(discordgo.ActionsRow)
		if !ok {
			t.Fatalf("row = %T, want an ActionsRow", row)
		}
		if len(actionsRow.Components) > 5 {
			t.Fatalf("buttons = %d in one row, over Discord's 5", len(actionsRow.Components))
		}
		for _, component := range actionsRow.Components {
			button, ok := component.(discordgo.Button)
			if !ok {
				t.Fatalf("component = %T, want a Button", component)
			}
			id, err := ParseCustomID(button.CustomID)
			if err != nil {
				t.Fatalf("parsing %q: %v", button.CustomID, err)
			}
			actions[id.Action] = true
		}
	}

	for _, want := range []Action{
		ActionSignup, ActionTentative, ActionLate, ActionDecline, ActionAbsent, ActionWithdraw,
	} {
		if !actions[want] {
			t.Errorf("actions = %v, want %q offered", actions, want)
		}
	}
}

func TestCharacterSelectPutsTheMainFirstAndMarksTheAlts(t *testing.T) {
	menu := selectMenuIn(t, characterSelect("e1", ActionSignup, "", []client.Character{
		{ID: "c2", Name: "Zanther", Realm: "Draenor"},
		{ID: "c1", Name: "Joharian", Realm: "Draenor", IsMain: true},
		{ID: "c3", Name: "Johalic", Realm: "Silvermoon"},
	}))

	want := []string{"Joharian-Draenor", "Johalic-Silvermoon", "Zanther-Draenor"}
	for i, label := range want {
		if menu.Options[i].Label != label {
			t.Errorf("option %d = %q, want %q", i, menu.Options[i].Label, label)
		}
	}
	if menu.Options[0].Description != "Main" {
		t.Errorf("main description = %q, want %q", menu.Options[0].Description, "Main")
	}
	if menu.Options[1].Description != "Alt" {
		t.Errorf("alt description = %q, want %q", menu.Options[1].Description, "Alt")
	}

	// A ticked option cannot be picked: Discord sends no interaction when a selection
	// does not change, which is the trap roleSelect documents.
	for _, option := range menu.Options {
		if option.Default {
			t.Errorf("option %q is ticked by default, so picking it sends nothing", option.Label)
		}
	}

	id, err := ParseCustomID(menu.CustomID)
	if err != nil {
		t.Fatalf("parsing %q: %v", menu.CustomID, err)
	}
	if id.Action != ActionPick || id.Then != ActionSignup || id.EventID != "e1" {
		t.Errorf("parsed = %+v, want a pick carrying the signup it resumes", id)
	}
}

// A raider with more alts than Discord will render loses the tail of the list. Losing
// the main out of it would break the common case for the sake of the rare one.
func TestCharacterSelectTruncatesToTheOptionCapKeepingTheMain(t *testing.T) {
	characters := make([]client.Character, 0, maxSelectOptions+5)
	for i := range maxSelectOptions + 5 {
		characters = append(characters, client.Character{
			ID:    fmt.Sprintf("c%02d", i),
			Name:  fmt.Sprintf("Alt%02d", i),
			Realm: "Draenor",
		})
	}
	characters = append(characters, client.Character{ID: "main", Name: "Zzzmain", Realm: "Draenor", IsMain: true})

	menu := selectMenuIn(t, characterSelect("", ActionRoles, "", characters))
	if len(menu.Options) != maxSelectOptions {
		t.Fatalf("options = %d, want %d", len(menu.Options), maxSelectOptions)
	}
	if menu.Options[0].Value != "main" {
		t.Errorf("first option = %q, want the main", menu.Options[0].Value)
	}
}

func selectMenuIn(t *testing.T, components []discordgo.MessageComponent) discordgo.SelectMenu {
	t.Helper()
	row, ok := components[0].(discordgo.ActionsRow)
	if !ok {
		t.Fatalf("component = %T, want an ActionsRow", components[0])
	}
	menu, ok := row.Components[0].(discordgo.SelectMenu)
	if !ok {
		t.Fatalf("component = %T, want a SelectMenu", row.Components[0])
	}
	return menu
}

// The button opens a modal; it never writes LATE itself, because the status is only
// actionable with the arrival time the modal collects.
func TestLateButtonAndItsModalUseDifferentActions(t *testing.T) {
	if ActionLate == ActionLateModal {
		t.Fatal("the late button and its modal share an action, so the submit would route back to the button")
	}

	id, err := ParseCustomID(CustomID{Action: ActionLateModal, EventID: "e1"}.String())
	if err != nil {
		t.Fatalf("parsing the modal custom_id: %v", err)
	}
	if id.Action != ActionLateModal || id.EventID != "e1" {
		t.Errorf("parsed = %+v, want the modal action carrying its event", id)
	}
}

// The confirm button is the only place the character id survives the round trip from
// /character remove, since the picker is skipped for a raider with one character.
func TestRemoveConfirmButtonCarriesTheCharacter(t *testing.T) {
	rows := removeConfirmButton("0192f3c8-0000-7000-8000-000000000002")

	row, ok := rows[0].(discordgo.ActionsRow)
	if !ok {
		t.Fatalf("component is %T, want an actions row", rows[0])
	}
	button, ok := row.Components[0].(discordgo.Button)
	if !ok {
		t.Fatalf("component is %T, want a button", row.Components[0])
	}
	if button.Style != discordgo.DangerButton {
		t.Errorf("style = %v, want danger on a delete", button.Style)
	}

	id, err := ParseCustomID(button.CustomID)
	if err != nil {
		t.Fatalf("parsing %q: %v", button.CustomID, err)
	}
	if id.Action != ActionRemoveConfirm || id.CharacterID != "0192f3c8-0000-7000-8000-000000000002" {
		t.Errorf("parsed = %+v, want the confirm action carrying its character", id)
	}
}

// The pick and the confirm are separate actions on purpose: sharing one would make the
// picker delete on selection, with no confirmation step at all.
func TestRemovePickAndConfirmUseDifferentActions(t *testing.T) {
	if ActionRemove == ActionRemoveConfirm {
		t.Fatal("the removal picker and its confirm share an action, so picking a character would delete it")
	}
}
