package client

import (
	"context"
	"net/http"
	"strconv"
)

// DiscordChannelType categorises a channel for the dashboard's picker, matching the
// service's discord_channel_type enum.
type DiscordChannelType string

const (
	ChannelText         DiscordChannelType = "TEXT"
	ChannelAnnouncement DiscordChannelType = "ANNOUNCEMENT"
	ChannelVoice        DiscordChannelType = "VOICE"
	ChannelStageVoice   DiscordChannelType = "STAGE_VOICE"
	ChannelForum        DiscordChannelType = "FORUM"
	ChannelCategory     DiscordChannelType = "CATEGORY"
	ChannelOther        DiscordChannelType = "OTHER"
)

// DiscordChannel is one of a guild's channels, as the bot reports it to the service.
type DiscordChannel struct {
	ID   string             `json:"id"`
	Name string             `json:"name"`
	Type DiscordChannelType `json:"type"`
}

// DiscordRole is one of a guild's roles, as the bot reports it to the service.
type DiscordRole struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Color    int32  `json:"color"`
	Position int32  `json:"position"`
}

type pushGuildChannelsRequest struct {
	Channels []DiscordChannel `json:"channels"`
}

// PushGuildChannels replaces the service's whole channel catalog for a guild with
// this snapshot. This is the bot reporting its own view of the guild rather than a
// raider's request, so it sends no actor headers, the same as the outbox routes.
func (c *Client) PushGuildChannels(ctx context.Context, guildID uint64, channels []DiscordChannel) error {
	_, err := c.do(ctx, nil, http.MethodPut,
		"/api/guilds/"+strconv.FormatUint(guildID, 10)+"/discord-channels",
		pushGuildChannelsRequest{Channels: channels}, nil)
	return err
}

type pushGuildRolesRequest struct {
	Roles []DiscordRole `json:"roles"`
}

// PushGuildRoles replaces the service's whole role catalog for a guild with this
// snapshot. Same reasoning as PushGuildChannels.
func (c *Client) PushGuildRoles(ctx context.Context, guildID uint64, roles []DiscordRole) error {
	_, err := c.do(ctx, nil, http.MethodPut,
		"/api/guilds/"+strconv.FormatUint(guildID, 10)+"/discord-roles",
		pushGuildRolesRequest{Roles: roles}, nil)
	return err
}
