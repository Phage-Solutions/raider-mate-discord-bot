package discord

import (
	"errors"
	"testing"
)

func TestCustomIDRoundTripsAnEventAction(t *testing.T) {
	want := CustomID{Action: ActionSignup, EventID: "0192f3c8-0000-7000-8000-000000000001"}

	got, err := ParseCustomID(want.String())
	if err != nil {
		t.Fatalf("parsing %q: %v", want.String(), err)
	}
	if got != want {
		t.Errorf("parsed = %+v, want %+v", got, want)
	}
}

func TestCustomIDRoundTripsACharacterAction(t *testing.T) {
	want := CustomID{
		Action:      ActionRoles,
		EventID:     "0192f3c8-0000-7000-8000-000000000001",
		CharacterID: "0192f3c8-0000-7000-8000-000000000002",
	}

	got, err := ParseCustomID(want.String())
	if err != nil {
		t.Fatalf("parsing %q: %v", want.String(), err)
	}
	if got != want {
		t.Errorf("parsed = %+v, want %+v", got, want)
	}
}

// Two UUIDs is the longest thing the scheme carries. If it ever stops fitting, the
// component silently fails to render rather than erroring, so pin it here.
func TestTheLongestCustomIDFitsDiscordsCap(t *testing.T) {
	id := CustomID{
		Action:      ActionRoles,
		EventID:     "0192f3c8-1111-7222-8333-444455556666",
		CharacterID: "0192f3c8-1111-7222-8333-444455556667",
	}.String()

	if len(id) > maxCustomID {
		t.Errorf("custom_id is %d characters, want at most %d: %q", len(id), maxCustomID, id)
	}
}

func TestParseCustomIDIgnoresAnotherBotsComponent(t *testing.T) {
	for _, raw := range []string{"", "other:1:signup:abc", "signup", "rm:notanumber:signup:abc"} {
		if _, err := ParseCustomID(raw); !errors.Is(err, ErrForeignComponent) {
			t.Errorf("ParseCustomID(%q) error = %v, want ErrForeignComponent", raw, err)
		}
	}
}

// A pinned embed outlives the deploy that posted it, so an old version segment has to
// be recognisable as stale rather than routed to whatever action name it happens to
// share with the current scheme.
func TestParseCustomIDReportsAnOlderVersionAsStale(t *testing.T) {
	if _, err := ParseCustomID("rm:0:signup:0192f3c8-0000-7000-8000-000000000001"); !errors.Is(err, ErrStaleComponent) {
		t.Errorf("error = %v, want ErrStaleComponent", err)
	}
}
