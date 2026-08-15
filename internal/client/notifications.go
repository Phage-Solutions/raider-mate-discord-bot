package client

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

// Reminder24hPayload nudges whoever has not answered yet.
type Reminder24hPayload struct {
	Title    string    `json:"title"`
	StartsAt time.Time `json:"starts_at"`
	Deadline time.Time `json:"deadline"`
}

// Reminder1hPayload tells a seated raider which role they are playing.
type Reminder1hPayload struct {
	Title        string    `json:"title"`
	StartsAt     time.Time `json:"starts_at"`
	AssignedRole *Role     `json:"assigned_role"`
}

// SignupDeadlinePayload tells the raid lead signups are closed, with the tally.
type SignupDeadlinePayload struct {
	Title  string               `json:"title"`
	Counts map[SignupStatus]int `json:"counts"`
}

// CompNagPayload fires two hours out when nothing has been locked.
type CompNagPayload struct {
	Title    string    `json:"title"`
	StartsAt time.Time `json:"starts_at"`
}

// LateRequestFiledPayload tells the raid lead someone wants in after the deadline.
type LateRequestFiledPayload struct {
	EventTitle  string       `json:"event_title"`
	CharacterID string       `json:"character_id"`
	Status      SignupStatus `json:"status"`
}

// ClaimNotifications takes up to limit undelivered notifications across every guild
// and marks them claimed, so a second poller gets a different batch.
//
// This is the one route the bot calls as itself rather than for a raider, so it sends
// no actor headers. Delivery is at-least-once: a claim leases for five minutes, and a
// bot that sends and dies before acking will send again. That is the right trade for a
// reminder, and it is what stops two pollers DMing everyone twice on every tick.
func (c *Client) ClaimNotifications(ctx context.Context, limit int) ([]Notification, error) {
	var out []Notification
	_, err := c.do(ctx, nil, http.MethodGet,
		"/api/notifications?limit="+strconv.Itoa(limit), nil, &out)
	return out, err
}

// MarkDelivered acks one notification so it is not sent again.
func (c *Client) MarkDelivered(ctx context.Context, notificationID string) error {
	_, err := c.do(ctx, nil, http.MethodPost,
		"/api/notifications/"+notificationID+"/delivered", nil, nil)
	return err
}

// DecodePayload reads a notification's payload into the struct matching its kind.
func DecodePayload[T any](n Notification) (T, error) {
	var out T
	if err := json.Unmarshal(n.Payload, &out); err != nil {
		return out, err
	}
	return out, nil
}
