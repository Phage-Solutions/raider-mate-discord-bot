package discord

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/Phage-Solutions/raider-mate-discord-bot/internal/client"
)

func TestServiceActor(t *testing.T) {
	tests := []struct {
		name    string
		guildID string
		appID   string
		want    client.Actor
		wantErr bool
	}{
		{
			name:    "guild from the notification, identity from the bot",
			guildID: "1062029706309931028",
			appID:   "9001",
			want:    client.Actor{DiscordID: 9001, GuildID: 1062029706309931028},
		},
		{
			// The gateway has not opened, so appID is empty. Better to fail the one
			// redraw than to send the service a guild scope with no actor behind it.
			name:    "no application id",
			guildID: "1062029706309931028",
			appID:   "",
			wantErr: true,
		},
		{
			name:    "unparseable guild id",
			guildID: "not-a-snowflake",
			appID:   "9001",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := serviceActor(client.Notification{DiscordGuildID: tt.guildID}, tt.appID)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("serviceActor() error = nil, want an error")
				}
				return
			}
			if err != nil {
				t.Fatalf("serviceActor() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("serviceActor() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestCompSlotDroppedTextNamesEveryCompThatLostASeat(t *testing.T) {
	tests := []struct {
		name  string
		comps []string
		want  string
	}{
		{name: "one comp", comps: []string{"prog"}, want: "out of prog."},
		{name: "two comps", comps: []string{"prog", "farm"}, want: "out of prog and farm."},
		{name: "three comps", comps: []string{"prog", "farm", "alt"}, want: "out of prog, farm and alt."},
		// Cannot come from the service, which queues this only when it emptied
		// something, but a nil slice must not slice out of range.
		{name: "no comps", comps: nil, want: "out of the comp."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			absent := client.StatusAbsent
			got, err := notificationText(client.Notification{
				Kind: client.CompSlotDropped,
				Payload: payload(t, client.CompSlotDroppedPayload{
					EventTitle: "Heroic Nerub-ar Palace",
					Status:     &absent,
					CompNames:  tt.comps,
				}),
			})
			if err != nil {
				t.Fatalf("notificationText() error = %v", err)
			}
			if !strings.Contains(got, tt.want) {
				t.Errorf("notificationText() = %q, want it to contain %q", got, tt.want)
			}
			if !strings.Contains(got, "is absent for") {
				t.Errorf("notificationText() = %q, want the status named", got)
			}
		})
	}
}

// A withdrawal deletes the signup, so the payload carries no status. Naming one anyway
// would tell the raid lead the raider declined when they took their name off entirely.
func TestCompSlotDroppedTextReadsAsAWithdrawalWhenNoStatusIsGiven(t *testing.T) {
	got, err := notificationText(client.Notification{
		Kind: client.CompSlotDropped,
		Payload: payload(t, client.CompSlotDroppedPayload{
			EventTitle: "Heroic Nerub-ar Palace",
			CompNames:  []string{"prog"},
		}),
	})
	if err != nil {
		t.Fatalf("notificationText() error = %v", err)
	}
	if !strings.Contains(got, "has withdrawn from") {
		t.Errorf("notificationText() = %q, want it to read as a withdrawal", got)
	}
	if strings.Contains(got, "is  for") {
		t.Errorf("notificationText() = %q, want no empty status left in the sentence", got)
	}
}

// The service now sends a key per status, zeros included. Printing them all would turn
// a one-line summary into a table nobody reads.
func TestSignupDeadlineSummarySkipsZeroesAndNamesAbsences(t *testing.T) {
	got, err := notificationText(client.Notification{
		Kind: client.SignupDeadline,
		Payload: payload(t, client.SignupDeadlinePayload{
			Title: "Heroic Nerub-ar Palace",
			Counts: map[client.SignupStatus]int{
				client.StatusConfirmed: 18,
				client.StatusLate:      0,
				client.StatusTentative: 0,
				client.StatusDeclined:  2,
				client.StatusAbsent:    3,
				client.StatusNoShow:    0,
			},
		}),
	})
	if err != nil {
		t.Fatalf("notificationText() error = %v", err)
	}
	if !strings.Contains(got, "18 confirmed, 2 declined, 3 absent") {
		t.Errorf("notificationText() = %q, want the non-zero counts only", got)
	}
}

func payload(t *testing.T, v any) json.RawMessage {
	t.Helper()

	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("encoding payload: %v", err)
	}
	return raw
}
