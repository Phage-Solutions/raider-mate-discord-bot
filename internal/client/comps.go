package client

import (
	"context"
	"net/http"
	"net/url"
)

// DefaultComp is the comp name the bot creates. Comps are keyed by event and name, so
// an event can hold several ("prog" and "farm"), but Discord offers one board and the
// dashboard is where extra ones get built.
//
// It is the name this repo writes, never one it assumes when reading. A raid lead can
// rename a comp from the dashboard, so which board Discord shows is resolved from the
// event's own comp list.
const DefaultComp = "main"

// compPath escapes a comp name into a path segment. The name is a raid lead's to choose
// and it travels in the URL, so "Main guild comp" has to survive the trip and a name
// carrying a slash or a hash must not quietly address something else.
func compPath(eventID, name string) string {
	return "/api/events/" + eventID + "/comps/" + url.PathEscape(name)
}

// Comps lists an event's comps without their slots.
func (c *Client) Comps(ctx context.Context, actor Actor, eventID string) ([]CompInfo, error) {
	var out []CompInfo
	_, err := c.do(ctx, &actor, http.MethodGet, "/api/events/"+eventID+"/comps", nil, &out)
	return out, err
}

// Comp returns one comp's board. A comp that has never been locked or saved does not
// exist yet, and this returns ErrNotFound rather than an empty board.
func (c *Client) Comp(ctx context.Context, actor Actor, eventID, name string) (Board, error) {
	var out Board
	_, err := c.do(ctx, &actor, http.MethodGet, compPath(eventID, name), nil, &out)
	return out, err
}

// LockComp runs the assigner and persists the result, creating the comp if this is the
// first lock. Raid lead only.
//
// A hand-built comp returns ErrCompIsManual and nothing is written. That is the point:
// a raid lead who built a board and then hit lock keeps their board.
func (c *Client) LockComp(ctx context.Context, actor Actor, eventID, name string) (Board, error) {
	var out Board
	status, err := c.do(ctx, &actor, http.MethodPost, compPath(eventID, name)+"/lock", nil, &out)
	if status == http.StatusConflict {
		return Board{}, ErrCompIsManual
	}
	if err != nil {
		return Board{}, err
	}
	return out, nil
}
