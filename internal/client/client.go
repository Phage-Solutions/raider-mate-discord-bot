// Package client is a typed HTTP client for raider-mate-service. It holds no domain
// logic and no discordgo types: what it knows is how to reach the service, how to
// present the caller, and how to read what comes back.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Actor is who a request is being made on behalf of. The service trusts the bot about
// this and resolves raid-lead capability itself, so the bot reports the raw Discord
// facts and never decides what they mean.
type Actor struct {
	DiscordID  uint64
	GuildID    uint64
	RoleIDs    []uint64
	GuildAdmin bool
}

// Client talks to raider-mate-service.
type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
}

// New builds a Client. baseURL is the service root, with no trailing /api.
func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		apiKey:  apiKey,
		// A Discord interaction must be answered within three seconds and the handler
		// defers first, so a service call that has not landed in ten is already too
		// late to be useful.
		http: &http.Client{Timeout: 10 * time.Second},
	}
}

// do sends one request and decodes a JSON body into out, which may be nil for the
// responses that carry none. A nil actor omits the actor headers, for the outbox
// routes the bot calls as itself.
//
// It returns the status code alongside the error because some 2xx codes are not
// interchangeable: a signup write answers 200 with a signup and 202 with a late
// request, and the caller has to tell them apart.
func (c *Client) do(ctx context.Context, actor *Actor, method, path string, body, out any) (int, error) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return 0, fmt.Errorf("encoding request body: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, reader)
	if err != nil {
		return 0, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if actor != nil {
		setActorHeaders(req.Header, *actor)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("calling %s %s: %w", method, path, err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode >= http.StatusBadRequest {
		return resp.StatusCode, readAPIError(resp)
	}

	if out == nil || resp.StatusCode == http.StatusNoContent {
		return resp.StatusCode, nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return resp.StatusCode, fmt.Errorf("decoding %s %s: %w", method, path, err)
	}
	return resp.StatusCode, nil
}

// Follow calls a link the service handed back, rather than a path the bot built. Use
// it whenever a response already named the transition: the absence of a link is the
// authorization answer, and following one cannot drift from what the service offered.
func (c *Client) Follow(ctx context.Context, actor *Actor, link Link, body, out any) error {
	method := link.Method
	if method == "" {
		method = http.MethodGet
	}
	_, err := c.do(ctx, actor, method, link.Href, body, out)
	return err
}

func setActorHeaders(h http.Header, actor Actor) {
	h.Set("X-Actor-Discord-Id", strconv.FormatUint(actor.DiscordID, 10))
	h.Set("X-Actor-Guild-Id", strconv.FormatUint(actor.GuildID, 10))
	h.Set("X-Actor-Guild-Admin", strconv.FormatBool(actor.GuildAdmin))
	if len(actor.RoleIDs) > 0 {
		ids := make([]string, len(actor.RoleIDs))
		for i, id := range actor.RoleIDs {
			ids[i] = strconv.FormatUint(id, 10)
		}
		h.Set("X-Actor-Roles", strings.Join(ids, ","))
	}
}
