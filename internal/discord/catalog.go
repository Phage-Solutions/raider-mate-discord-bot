package discord

import (
	"context"
	"strconv"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/Phage-Solutions/raider-mate-discord-bot/internal/client"
)

// catalogPushTimeout bounds one channel or role catalog push. These fire off gateway
// events, not an interaction, so there is no three-second deadline to honour, only a
// sane upper bound so a stalled service call cannot leak a goroutine forever.
const catalogPushTimeout = 10 * time.Second

// onGuildCreate pushes the guild's channel and role catalog as soon as the bot sees
// it. Discord sends this both on first join and on every gateway reconnect, so a bot
// restart alone keeps the catalog current even with no channel or role activity in
// between.
func (b *Bot) onGuildCreate(_ *discordgo.Session, g *discordgo.GuildCreate) {
	ctx, cancel := context.WithTimeout(context.Background(), catalogPushTimeout)
	defer cancel()
	defer b.recoverPanic(ctx, "guild create catalog push")

	b.pushChannels(ctx, g.ID, g.Channels)
	b.pushRoles(ctx, g.ID, g.Roles)
}

func (b *Bot) onChannelCreate(_ *discordgo.Session, e *discordgo.ChannelCreate) {
	b.refreshChannels(e.GuildID)
}
func (b *Bot) onChannelUpdate(_ *discordgo.Session, e *discordgo.ChannelUpdate) {
	b.refreshChannels(e.GuildID)
}
func (b *Bot) onChannelDelete(_ *discordgo.Session, e *discordgo.ChannelDelete) {
	b.refreshChannels(e.GuildID)
}

func (b *Bot) onGuildRoleCreate(_ *discordgo.Session, e *discordgo.GuildRoleCreate) {
	b.refreshRoles(e.GuildID)
}
func (b *Bot) onGuildRoleUpdate(_ *discordgo.Session, e *discordgo.GuildRoleUpdate) {
	b.refreshRoles(e.GuildID)
}
func (b *Bot) onGuildRoleDelete(_ *discordgo.Session, e *discordgo.GuildRoleDelete) {
	b.refreshRoles(e.GuildID)
}

// refreshChannels re-pushes a guild's whole channel catalog after a create, update or
// delete. The event itself carries only the one channel that changed; state already
// holds the guild's full list by the time this fires, since discordgo applies state
// updates before dispatching to registered handlers, so a wholesale replace push needs
// nothing more than reading it back.
func (b *Bot) refreshChannels(guildID string) {
	if guildID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), catalogPushTimeout)
	defer cancel()
	defer b.recoverPanic(ctx, "channel catalog push")

	guild, err := b.session.State.Guild(guildID)
	if err != nil {
		b.logger.ErrorContext(ctx, "reading guild state for channel catalog", "error", err, "guild_id", guildID)
		return
	}
	b.pushChannels(ctx, guildID, guild.Channels)
}

// refreshRoles is refreshChannels for a guild's role catalog.
func (b *Bot) refreshRoles(guildID string) {
	if guildID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), catalogPushTimeout)
	defer cancel()
	defer b.recoverPanic(ctx, "role catalog push")

	guild, err := b.session.State.Guild(guildID)
	if err != nil {
		b.logger.ErrorContext(ctx, "reading guild state for role catalog", "error", err, "guild_id", guildID)
		return
	}
	b.pushRoles(ctx, guildID, guild.Roles)
}

func (b *Bot) pushChannels(ctx context.Context, guildID string, channels []*discordgo.Channel) {
	id, err := strconv.ParseUint(guildID, 10, 64)
	if err != nil {
		b.logger.ErrorContext(ctx, "parsing guild id for channel catalog", "error", err, "guild_id", guildID)
		return
	}

	out := make([]client.DiscordChannel, 0, len(channels))
	for _, c := range channels {
		out = append(out, client.DiscordChannel{ID: c.ID, Name: c.Name, Type: channelType(c.Type)})
	}

	if err := b.api.PushGuildChannels(ctx, id, out); err != nil {
		b.logger.ErrorContext(ctx, "pushing channel catalog", "error", err, "guild_id", guildID)
	}
}

func (b *Bot) pushRoles(ctx context.Context, guildID string, roles []*discordgo.Role) {
	id, err := strconv.ParseUint(guildID, 10, 64)
	if err != nil {
		b.logger.ErrorContext(ctx, "parsing guild id for role catalog", "error", err, "guild_id", guildID)
		return
	}

	out := make([]client.DiscordRole, 0, len(roles))
	for _, r := range roles {
		// @everyone and integration-managed roles are not something a raid lead would
		// ever pick as an events-channel ping, so they are left out rather than pushed
		// for the dashboard to filter back out.
		if r.ID == guildID || r.Managed {
			continue
		}
		out = append(out, client.DiscordRole{
			ID: r.ID, Name: r.Name,
			Color: int32(r.Color), Position: int32(r.Position), //nolint:gosec
		})
	}

	if err := b.api.PushGuildRoles(ctx, id, out); err != nil {
		b.logger.ErrorContext(ctx, "pushing role catalog", "error", err, "guild_id", guildID)
	}
}

// channelType maps discordgo's channel type to the service's plain-string catalog of
// them. Anything the service does not carry a category for (threads, the deprecated
// store type, and the DM/group types that never appear in a guild's channel list)
// maps to OTHER rather than being dropped: the dashboard only needs to filter to what
// it can post events into, not to reject what it does not recognise.
func channelType(t discordgo.ChannelType) client.DiscordChannelType {
	switch t {
	case discordgo.ChannelTypeGuildText:
		return client.ChannelText
	case discordgo.ChannelTypeGuildNews:
		return client.ChannelAnnouncement
	case discordgo.ChannelTypeGuildVoice:
		return client.ChannelVoice
	case discordgo.ChannelTypeGuildStageVoice:
		return client.ChannelStageVoice
	case discordgo.ChannelTypeGuildForum:
		return client.ChannelForum
	case discordgo.ChannelTypeGuildCategory:
		return client.ChannelCategory
	default:
		return client.ChannelOther
	}
}
