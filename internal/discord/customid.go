package discord

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Action is what a component click means.
type Action string

const (
	ActionSignup    Action = "signup"
	ActionTentative Action = "tentative"
	ActionDecline   Action = "decline"
	// ActionAbsent is "I am out for a while", wider than a decline of this one event.
	ActionAbsent Action = "absent"
	// ActionLate opens the arrival-time modal rather than writing anything. LATE
	// without a late_until is a status a raid lead cannot act on, and a modal is the
	// only control that can ask for one.
	ActionLate      Action = "late"
	ActionLateModal Action = "latemodal"
	ActionWithdraw  Action = "withdraw"
	// ActionRoles is the ephemeral multi-select coming back with the roles a raider
	// picked. It carries the character as well as the event, because the select is
	// what turns a bare "I am coming" into a role menu the assigner can read.
	ActionRoles Action = "roles"
	// ActionAddCharacter opens the registration modal. It has to be a separate,
	// undeferred interaction: Discord will not open a modal in reply to a deferred
	// one, and the bot only learns a raider has no characters after deferring.
	ActionAddCharacter Action = "addchar"
	ActionCharModal    Action = "charmodal"
	// ActionEventMentions is the role picker behind /config mentions coming back.
	ActionEventMentions Action = "mentions"
	// ActionPick is the character select shown to a raider with more than one
	// character. The chosen character comes back in the select's values, so the
	// custom_id carries the action waiting on it rather than a character id.
	ActionPick Action = "pick"
	// ActionSetMain is /character main coming back from the picker.
	ActionSetMain Action = "setmain"
)

// noEvent stands in for an absent event id. Some flows have no event behind them,
// /character roles among them, and a custom_id needs every segment present to parse.
const noEvent = "-"

// prefix marks a component as this bot's, so components from other bots in the same
// channel are ignored rather than misrouted.
const prefix = "rm"

// version lets a pinned message from an older deploy be recognised instead of
// misread. Event embeds live a long time; people pin them.
const version = 1

// maxCustomID is Discord's cap. Two UUIDs and the routing prefix come to 84, which
// leaves room, but not enough to stop checking.
const maxCustomID = 100

// ErrForeignComponent means the component was not built by this bot.
var ErrForeignComponent = errors.New("not a raider mate component")

// ErrStaleComponent means the component came from an older version of the scheme. The
// message is out of date rather than broken, and the raider is told so.
var ErrStaleComponent = errors.New("component is from an older version")

// CustomID is a component's routing information, which on a Discord interaction is the
// only state that comes back.
type CustomID struct {
	Action Action
	// EventID is the event the component sits on.
	EventID string
	// CharacterID is set only on the actions that name one character. A picker names
	// none yet, so it carries Then and Arg in the same position instead.
	CharacterID string
	// Then is the flow resuming once the raider has picked a character. ActionPick only.
	Then Action
	// Arg is the one value a resumed flow cannot recover on its own: the LATE arrival
	// time, as unix seconds. Empty everywhere else.
	Arg string
}

// String renders a custom_id. Never encode the invoking user: the interaction payload
// already carries them, and trusting a client-supplied id would be an impersonation
// route.
func (c CustomID) String() string {
	eventID := c.EventID
	if eventID == "" {
		eventID = noEvent
	}

	parts := []string{prefix, strconv.Itoa(version), string(c.Action), eventID}
	switch {
	case c.Action == ActionPick:
		parts = append(parts, string(c.Then))
		if c.Arg != "" {
			parts = append(parts, c.Arg)
		}
	case c.CharacterID != "":
		parts = append(parts, c.CharacterID)
	}

	id := strings.Join(parts, ":")
	if len(id) > maxCustomID {
		// Unreachable with UUIDs and the actions above, and a panic in a message
		// builder beats a component Discord silently refuses to render.
		panic(fmt.Sprintf("custom_id %q is %d characters, over Discord's %d", id, len(id), maxCustomID))
	}
	return id
}

// ParseCustomID reads a component's custom_id back.
func ParseCustomID(raw string) (CustomID, error) {
	parts := strings.Split(raw, ":")
	if len(parts) < 4 || parts[0] != prefix {
		return CustomID{}, ErrForeignComponent
	}

	v, err := strconv.Atoi(parts[1])
	if err != nil {
		return CustomID{}, ErrForeignComponent
	}
	if v != version {
		return CustomID{}, ErrStaleComponent
	}

	id := CustomID{Action: Action(parts[2]), EventID: parts[3]}
	if id.EventID == noEvent {
		id.EventID = ""
	}
	if id.Action == ActionPick {
		if len(parts) > 4 {
			id.Then = Action(parts[4])
		}
		if len(parts) > 5 {
			id.Arg = parts[5]
		}
		return id, nil
	}
	if len(parts) > 4 {
		id.CharacterID = parts[4]
	}
	return id, nil
}
