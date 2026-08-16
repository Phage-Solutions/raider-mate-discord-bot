package client

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestPushingGuildChannelsSendsNoActorHeadersAndTheWholeSet(t *testing.T) {
	var gotHeader http.Header
	var gotBody pushGuildChannelsRequest
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Clone()
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decoding body: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	channels := []DiscordChannel{{ID: "555", Name: "general", Type: ChannelText}}
	if err := c.PushGuildChannels(context.Background(), 100, channels); err != nil {
		t.Fatalf("pushing channels: %v", err)
	}

	for header := range gotHeader {
		if strings.HasPrefix(header, "X-Actor-") {
			t.Errorf("request carried %s, want no actor headers on the catalog push route", header)
		}
	}
	if len(gotBody.Channels) != 1 || gotBody.Channels[0].ID != "555" {
		t.Errorf("body = %+v, want the one channel sent", gotBody)
	}
}

func TestPushingGuildRolesSendsNoActorHeadersAndTheWholeSet(t *testing.T) {
	var gotHeader http.Header
	var gotBody pushGuildRolesRequest
	c := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Clone()
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatalf("decoding body: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	roles := []DiscordRole{{ID: "781", Name: "Raid Lead", Color: 15158332, Position: 5}}
	if err := c.PushGuildRoles(context.Background(), 100, roles); err != nil {
		t.Fatalf("pushing roles: %v", err)
	}

	for header := range gotHeader {
		if strings.HasPrefix(header, "X-Actor-") {
			t.Errorf("request carried %s, want no actor headers on the catalog push route", header)
		}
	}
	if len(gotBody.Roles) != 1 || gotBody.Roles[0].ID != "781" {
		t.Errorf("body = %+v, want the one role sent", gotBody)
	}
}
