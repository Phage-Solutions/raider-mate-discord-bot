package discord

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Raider-Mate/raider-mate-discord-bot/internal/client"
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

// The ping says what is starting and links back to the signup sheet. Repeating the
// roster would duplicate the message it is linking to.
func TestPreEventPingLinksTheEventMessage(t *testing.T) {
	channelID := "222"
	messageID := "333"
	got, err := notificationText(client.Notification{
		Kind:           client.ReminderPreEvent,
		TargetKind:     client.TargetChannel,
		DiscordGuildID: "111",
		ChannelID:      &channelID,
		Payload: payload(t, client.ReminderPreEventPingPayload{
			Title:     "Heroic Nerub-ar Palace",
			StartsAt:  time.Unix(1_800_000_000, 0),
			MessageID: &messageID,
		}),
	})
	if err != nil {
		t.Fatalf("notificationText() error = %v", err)
	}
	if !strings.Contains(got, "**Heroic Nerub-ar Palace** starts <t:1800000000:R>.") {
		t.Errorf("notificationText() = %q, want the title and a relative timestamp", got)
	}
	if !strings.Contains(got, "[Signup sheet](https://discord.com/channels/111/222/333)") {
		t.Errorf("notificationText() = %q, want a jump link to the event message", got)
	}
}

// An event whose message never went up has nothing to link to. The reminder is still
// worth sending; a link to a message that does not exist is not.
func TestPreEventPingWithNoEventMessageOmitsTheLink(t *testing.T) {
	channelID := "222"
	got, err := notificationText(client.Notification{
		Kind:           client.ReminderPreEvent,
		TargetKind:     client.TargetChannel,
		DiscordGuildID: "111",
		ChannelID:      &channelID,
		Payload: payload(t, client.ReminderPreEventPingPayload{
			Title:    "Heroic Nerub-ar Palace",
			StartsAt: time.Unix(1_800_000_000, 0),
		}),
	})
	if err != nil {
		t.Fatalf("notificationText() error = %v", err)
	}
	if strings.Contains(got, "Signup sheet") {
		t.Errorf("notificationText() = %q, want no link", got)
	}
}

func TestPreEventDMNamesTheAssignedRoleWhenThereIsOne(t *testing.T) {
	tank := client.RoleTank
	tests := []struct {
		name string
		kind client.NotificationKind
		role *client.Role
		want string
	}{
		{name: "seated", kind: client.ReminderPreEvent, role: &tank, want: "You are on"},
		{name: "unseated", kind: client.ReminderPreEvent, want: "starts <t:1800000000:R>."},
		// A service still on the old release queues the old kind. Dropping it would cost
		// a rollout's worth of reminders.
		{name: "old kind", kind: client.Reminder1h, role: &tank, want: "You are on"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := notificationText(client.Notification{
				Kind:       tt.kind,
				TargetKind: client.TargetUser,
				Payload: payload(t, client.ReminderPreEventDMPayload{
					Title:        "Heroic Nerub-ar Palace",
					StartsAt:     time.Unix(1_800_000_000, 0),
					AssignedRole: tt.role,
				}),
			})
			if err != nil {
				t.Fatalf("notificationText() error = %v", err)
			}
			if !strings.Contains(got, tt.want) {
				t.Errorf("notificationText() = %q, want it to contain %q", got, tt.want)
			}
		})
	}
}

// Discord rejects a send naming more than 100 users outright, so the overflow is
// counted instead of mentioned: 130 raiders is a reminder for everyone rather than one
// for nobody.
func TestUserMentionsCapsAtDiscordsLimit(t *testing.T) {
	ids := make([]string, 130)
	for i := range ids {
		ids[i] = strconv.Itoa(i + 1)
	}

	prefix, allowed := userMentions(ids)

	if len(allowed) != discordMentionCap {
		t.Errorf("allowed = %d ids, want %d", len(allowed), discordMentionCap)
	}
	if strings.Count(prefix, "<@") != discordMentionCap {
		t.Errorf("prefix has %d mentions, want %d", strings.Count(prefix, "<@"), discordMentionCap)
	}
	if !strings.Contains(prefix, "and 30 more") {
		t.Errorf("prefix = %q, want the overflow counted", prefix)
	}
}

func TestUserMentionsRendersUserSyntaxNotRoleSyntax(t *testing.T) {
	prefix, allowed := userMentions([]string{"1", "2"})

	if prefix != "<@1> <@2> " {
		t.Errorf("prefix = %q, want %q", prefix, "<@1> <@2> ")
	}
	if len(allowed) != 2 {
		t.Errorf("allowed = %v, want both ids", allowed)
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
