package client

import (
	"context"
	"net/http"
)

// DefaultComp is the comp name the bot uses. Comps are keyed by event and name, so an
// event can hold several ("prog" and "farm"), but Discord offers one board and the
// dashboard is where extra ones get built.
const DefaultComp = "main"

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
	_, err := c.do(ctx, &actor, http.MethodGet, "/api/events/"+eventID+"/comps/"+name, nil, &out)
	return out, err
}

// LockComp runs the assigner and persists the result, creating the comp if this is the
// first lock. Raid lead only.
//
// A hand-built comp returns ErrCompIsManual and nothing is written. That is the point:
// a raid lead who built a board and then hit lock keeps their board.
func (c *Client) LockComp(ctx context.Context, actor Actor, eventID, name string) (Board, error) {
	var out Board
	status, err := c.do(ctx, &actor, http.MethodPost,
		"/api/events/"+eventID+"/comps/"+name+"/lock", nil, &out)
	if status == http.StatusConflict {
		return Board{}, ErrCompIsManual
	}
	if err != nil {
		return Board{}, err
	}
	return out, nil
}
