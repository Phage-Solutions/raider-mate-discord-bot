package discord

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bwmarrin/discordgo"

	"github.com/Raider-Mate/raider-mate-discord-bot/internal/client"
)

func TestChannelTypeMapsKnownTypesAndFallsBackToOther(t *testing.T) {
	cases := []struct {
		in   discordgo.ChannelType
		want client.DiscordChannelType
	}{
		{discordgo.ChannelTypeGuildText, client.ChannelText},
		{discordgo.ChannelTypeGuildNews, client.ChannelAnnouncement},
		{discordgo.ChannelTypeGuildVoice, client.ChannelVoice},
		{discordgo.ChannelTypeGuildStageVoice, client.ChannelStageVoice},
		{discordgo.ChannelTypeGuildForum, client.ChannelForum},
		{discordgo.ChannelTypeGuildCategory, client.ChannelCategory},
		{discordgo.ChannelTypeGuildPublicThread, client.ChannelOther},
		{discordgo.ChannelTypeDM, client.ChannelOther},
	}

	for _, tc := range cases {
		if got := channelType(tc.in); got != tc.want {
			t.Errorf("channelType(%d) = %s, want %s", tc.in, got, tc.want)
		}
	}
}

func TestPushRolesSkipsEveryoneAndManagedRoles(t *testing.T) {
	const guildID = "100"

	var gotBody struct {
		Roles []client.DiscordRole `json:"roles"`
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decoding body: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	b := &Bot{logger: slog.New(slog.NewTextHandler(io.Discard, nil)), api: client.New(server.URL, "key")}

	roles := []*discordgo.Role{
		{ID: guildID, Name: "@everyone", Position: 0},
		{ID: "900", Name: "Server Booster", Managed: true, Position: 3},
		{ID: "781", Name: "Raid Lead", Color: 0xE74C3C, Position: 5},
	}

	b.pushRoles(context.Background(), guildID, roles)

	if len(gotBody.Roles) != 1 || gotBody.Roles[0].ID != "781" {
		t.Fatalf("pushed roles = %+v, want only the Raid Lead role", gotBody.Roles)
	}
}
