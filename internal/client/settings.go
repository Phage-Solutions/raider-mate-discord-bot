package client

import (
	"context"
	"net/http"
	"strconv"
)

// GuildSettings is a guild's bot configuration.
type GuildSettings struct {
	// EventsChannelID is where event messages are posted. Nil means the guild has not
	// chosen one, and the bot posts wherever the create command was run.
	EventsChannelID *string `json:"events_channel_id,omitempty"`
	// Timezone is an IANA name the guild types its raid times in. Nil means a time
	// without an explicit offset has nothing to be resolved against.
	Timezone *string `json:"timezone,omitempty"`
	// EventMentionRoleIDs are pinged when an event message goes up. Empty means ping
	// nobody, which is a setting rather than an absence.
	EventMentionRoleIDs []string `json:"event_mention_role_ids"`
	// EventBannerURL is artwork shown under the roster. Nil for a plain card.
	EventBannerURL *string `json:"event_banner_url,omitempty"`
	Links          Links   `json:"_links"`
}

// GuildSettings reads a guild's configuration. Readable by any member, since the bot
// loads it on every event creation and a raid lead is not necessarily an admin.
func (c *Client) GuildSettings(ctx context.Context, actor Actor) (GuildSettings, error) {
	var out GuildSettings
	_, err := c.do(ctx, &actor, http.MethodGet,
		"/api/guilds/"+strconv.FormatUint(actor.GuildID, 10)+"/settings", nil, &out)
	return out, err
}

type setGuildSettingsRequest struct {
	EventsChannelID     *string  `json:"events_channel_id,omitempty"`
	Timezone            *string  `json:"timezone,omitempty"`
	EventMentionRoleIDs []string `json:"event_mention_role_ids,omitempty"`
	EventBannerURL      *string  `json:"event_banner_url,omitempty"`
}

// SetGuildSettings replaces a guild's whole settings row. Guild admin only, and a
// whole-row write: an omitted field clears it, so callers read the current settings
// first and send back everything they are not changing.
func (c *Client) SetGuildSettings(ctx context.Context, actor Actor, in GuildSettings) (GuildSettings, error) {
	var out GuildSettings
	_, err := c.do(ctx, &actor, http.MethodPut,
		"/api/guilds/"+strconv.FormatUint(actor.GuildID, 10)+"/settings",
		setGuildSettingsRequest{
			EventsChannelID:     in.EventsChannelID,
			Timezone:            in.Timezone,
			EventMentionRoleIDs: in.EventMentionRoleIDs,
			EventBannerURL:      in.EventBannerURL,
		}, &out)
	return out, err
}
