package discord

import (
	"testing"

	"github.com/bwmarrin/discordgo"
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
