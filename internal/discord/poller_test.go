package discord

import (
	"reflect"
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
