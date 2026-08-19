package client

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Reminder24hPayload nudges whoever has not answered yet.
type Reminder24hPayload struct {
	Title    string    `json:"title"`
	StartsAt time.Time `json:"starts_at"`
	Deadline time.Time `json:"deadline"`
}

// ReminderPreEventDMPayload reminds one raider the event is about to start, and names
// the role they are playing when the comp is locked and they hold a seat.
type ReminderPreEventDMPayload struct {
	Title        string    `json:"title"`
	StartsAt     time.Time `json:"starts_at"`
	AssignedRole *Role     `json:"assigned_role"`
}

// ReminderPreEventPingPayload is the channel post reminding everyone signed up. Who it
// mentions rides on the notification's DiscordIDs, not in here.
type ReminderPreEventPingPayload struct {
	Title    string    `json:"title"`
	StartsAt time.Time `json:"starts_at"`
	// MessageID is the event message to link back to, nil if the bot never posted one.
	MessageID *string `json:"message_id"`
}

// SignupDeadlinePayload tells the raid lead signups are closed, with the tally.
type SignupDeadlinePayload struct {
	Title  string               `json:"title"`
	Counts map[SignupStatus]int `json:"counts"`
}

// LateRequestFiledPayload tells the raid lead someone wants in after the deadline.
type LateRequestFiledPayload struct {
	EventTitle  string       `json:"event_title"`
	CharacterID string       `json:"character_id"`
	Status      SignupStatus `json:"status"`
}

// CompSlotDroppedPayload tells the raid lead a locked comp just lost someone. CompNames
// is every comp the raider held a seat in, since a hole in the prog comp and a hole in
// the farm comp are two different problems.
//
// Status is absent when the raider withdrew: the signup row is gone rather than
// restated, and there is no status to name.
type CompSlotDroppedPayload struct {
	EventTitle  string        `json:"event_title"`
	CharacterID string        `json:"character_id"`
	Status      *SignupStatus `json:"status,omitempty"`
	CompNames   []string      `json:"comp_names"`
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

// StreamNotifications holds the service's Server-Sent Events stream open and calls
// onEvent every time something lands in the outbox. It blocks until the stream ends or
// ctx is cancelled, and the caller decides whether to redial.
//
// The events carry no data. They say "claim now" and nothing else, so delivery stays on
// ClaimNotifications with its lease and its ack, and a missed event costs latency rather
// than a lost reminder, because the poll behind this still runs.
func (c *Client) StreamNotifications(ctx context.Context, onEvent func()) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/notifications/stream", nil)
	if err != nil {
		return fmt.Errorf("building stream request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.stream.Do(req)
	if err != nil {
		return fmt.Errorf("opening notification stream: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("opening notification stream: unexpected status %d", resp.StatusCode)
	}

	// The only line worth reading is the event name. Heartbeat comments start with a
	// colon and the data line is an empty object, so both are read and dropped.
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		if strings.HasPrefix(scanner.Text(), "event:") {
			onEvent()
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("reading notification stream: %w", err)
	}
	return nil
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
