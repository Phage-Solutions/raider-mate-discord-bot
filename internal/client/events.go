package client

import (
	"context"
	"net/http"
	"strconv"
	"time"
)

// CreateEvent is what creating a raid or a Mythic+ group needs.
type CreateEvent struct {
	Type           EventType    `json:"type"`
	Title          string       `json:"title"`
	StartsAt       time.Time    `json:"starts_at"`
	SignupDeadline time.Time    `json:"signup_deadline"`
	CompTemplate   CompTemplate `json:"comp_template"`
	Difficulty     *Difficulty  `json:"difficulty,omitempty"`
}

// CreateEvent creates an event and schedules its reminder jobs. Raid lead only.
func (c *Client) CreateEvent(ctx context.Context, actor Actor, in CreateEvent) (Event, error) {
	var out Event
	_, err := c.do(ctx, &actor, http.MethodPost,
		"/api/guilds/"+strconv.FormatUint(actor.GuildID, 10)+"/events", in, &out)
	return out, err
}

// GuildEvents returns the guild's upcoming events, soonest first.
func (c *Client) GuildEvents(ctx context.Context, actor Actor) ([]Event, error) {
	var out []Event
	_, err := c.do(ctx, &actor, http.MethodGet,
		"/api/guilds/"+strconv.FormatUint(actor.GuildID, 10)+"/events", nil, &out)
	return out, err
}

// Event returns one event.
func (c *Client) Event(ctx context.Context, actor Actor, eventID string) (Event, error) {
	var out Event
	_, err := c.do(ctx, &actor, http.MethodGet, "/api/events/"+eventID, nil, &out)
	return out, err
}

// EditEvent is a partial update: nil fields are left alone. Changing StartsAt or
// SignupDeadline cancels and recomputes the event's scheduled jobs, so a run of
// reminders never fires against a time that has moved.
type EditEvent struct {
	Title          *string       `json:"title,omitempty"`
	StartsAt       *time.Time    `json:"starts_at,omitempty"`
	SignupDeadline *time.Time    `json:"signup_deadline,omitempty"`
	CompTemplate   *CompTemplate `json:"comp_template,omitempty"`
	Difficulty     *Difficulty   `json:"difficulty,omitempty"`
	MessageID      *string       `json:"message_id,omitempty"`
	ChannelID      *string       `json:"channel_id,omitempty"`
}

// EditEvent updates an event. Raid lead only.
func (c *Client) EditEvent(ctx context.Context, actor Actor, eventID string, in EditEvent) (Event, error) {
	var out Event
	_, err := c.do(ctx, &actor, http.MethodPatch, "/api/events/"+eventID, in, &out)
	return out, err
}

// AttachMessage records where the bot posted an event's embed. Until this lands the
// service has no channel to post a signup-deadline or comp-nag notification in, and
// drops those jobs rather than queueing something undeliverable.
func (c *Client) AttachMessage(ctx context.Context, actor Actor, eventID, channelID, messageID string) (Event, error) {
	return c.EditEvent(ctx, actor, eventID, EditEvent{ChannelID: &channelID, MessageID: &messageID})
}

// DeleteEvent cancels an event. Raid lead only.
func (c *Client) DeleteEvent(ctx context.Context, actor Actor, eventID string) error {
	_, err := c.do(ctx, &actor, http.MethodDelete, "/api/events/"+eventID, nil, nil)
	return err
}
