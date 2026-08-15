package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// SignupResult is what a signup write came back as. Past the signup deadline a
// player's write is not refused, it becomes a request for the raid lead, so exactly
// one of these two is set.
type SignupResult struct {
	Signup      *Signup
	LateRequest *LateRequest
}

// Filed reports whether the deadline had passed and this became a late request.
func (r SignupResult) Filed() bool {
	return r.LateRequest != nil
}

// WriteSignup is what a raider said about an event. LateUntil is only kept when Status
// is LATE; the service drops it otherwise.
type WriteSignup struct {
	Status    SignupStatus `json:"status"`
	Note      *string      `json:"note,omitempty"`
	LateUntil *time.Time   `json:"late_until,omitempty"`
}

// Signup writes a raider's answer for one character. It is an upsert keyed on the
// event and the character, so signing up twice is not an error and needs no read
// first.
func (c *Client) Signup(ctx context.Context, actor Actor, eventID, characterID string, in WriteSignup) (SignupResult, error) {
	return c.signupWrite(ctx, actor, http.MethodPut,
		"/api/events/"+eventID+"/signups/"+characterID, in)
}

// Withdraw removes a raider's signup. Past the deadline this files a late request
// carrying DECLINED rather than deleting quietly, so the raid lead sees the change.
func (c *Client) Withdraw(ctx context.Context, actor Actor, eventID, characterID string) (SignupResult, error) {
	return c.signupWrite(ctx, actor, http.MethodDelete,
		"/api/events/"+eventID+"/signups/"+characterID, nil)
}

func (c *Client) signupWrite(ctx context.Context, actor Actor, method, path string, body any) (SignupResult, error) {
	var raw json.RawMessage
	status, err := c.do(ctx, &actor, method, path, body, &raw)
	if err != nil {
		return SignupResult{}, err
	}

	if status == http.StatusAccepted {
		var late LateRequest
		if err := json.Unmarshal(raw, &late); err != nil {
			return SignupResult{}, fmt.Errorf("decoding late request: %w", err)
		}
		return SignupResult{LateRequest: &late}, nil
	}

	// A withdrawal inside the deadline answers 204 with no body.
	if status == http.StatusNoContent {
		return SignupResult{}, nil
	}

	var signup Signup
	if err := json.Unmarshal(raw, &signup); err != nil {
		return SignupResult{}, fmt.Errorf("decoding signup: %w", err)
	}
	return SignupResult{Signup: &signup}, nil
}

// Signups returns every signup for an event, each carrying its character and that
// character's role menu.
func (c *Client) Signups(ctx context.Context, actor Actor, eventID string) ([]Signup, error) {
	var out []Signup
	_, err := c.do(ctx, &actor, http.MethodGet, "/api/events/"+eventID+"/signups", nil, &out)
	return out, err
}

// LateRequests returns an event's late signup requests. Raid lead only.
func (c *Client) LateRequests(ctx context.Context, actor Actor, eventID string) ([]LateRequest, error) {
	var out []LateRequest
	_, err := c.do(ctx, &actor, http.MethodGet, "/api/events/"+eventID+"/late-requests", nil, &out)
	return out, err
}

// ApproveLateRequest accepts a request, writing the signup it asked for.
func (c *Client) ApproveLateRequest(ctx context.Context, actor Actor, eventID, requestID string) error {
	return c.decideLateRequest(ctx, actor, eventID, requestID, "approve")
}

// RejectLateRequest turns a request down.
func (c *Client) RejectLateRequest(ctx context.Context, actor Actor, eventID, requestID string) error {
	return c.decideLateRequest(ctx, actor, eventID, requestID, "reject")
}

func (c *Client) decideLateRequest(ctx context.Context, actor Actor, eventID, requestID, decision string) error {
	status, err := c.do(ctx, &actor, http.MethodPost,
		"/api/events/"+eventID+"/late-requests/"+requestID+"/"+decision, nil, nil)
	// Two raid leads can reach the same pending request. The loser gets a conflict,
	// which is a sentence about what happened, not a failure to retry.
	if status == http.StatusConflict {
		return ErrAlreadyDecided
	}
	return err
}
