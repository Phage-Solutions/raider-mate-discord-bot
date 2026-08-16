// Package discord is the bot's Discord surface: interaction handlers, embed
// rendering, and component builders. It decides nothing about signups or comps; it
// asks raider-mate-service and draws the answer.
package discord

import (
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/Phage-Solutions/raider-mate-discord-bot/internal/client"
)

// Options is what building a Bot needs.
type Options struct {
	Token        string
	DevGuildID   uint64
	PollInterval time.Duration
	API          *client.Client
	Logger       *slog.Logger
}

// Bot is the running Discord connection and everything hanging off it.
type Bot struct {
	session      *discordgo.Session
	api          *client.Client
	logger       *slog.Logger
	devGuildID   uint64
	pollInterval time.Duration
	embeds       *embedUpdater
	// icons are read once at startup. They change only when someone uploads new
	// application emoji, which is not something a running bot needs to notice.
	icons IconSet
}

// New builds a Bot. It opens no connection; Start does that.
func New(opts Options) (*Bot, error) {
	session, err := discordgo.New("Bot " + opts.Token)
	if err != nil {
		return nil, fmt.Errorf("creating discord session: %w", err)
	}

	// Guilds only. Message content, presence and guild members are privileged, and
	// needing one of them turns verification past 100 servers into a negotiation.
	// Everything this bot reads about a member arrives on the interaction payload.
	session.Identify.Intents = discordgo.IntentsGuilds

	bot := &Bot{
		session:      session,
		api:          opts.API,
		logger:       opts.Logger,
		devGuildID:   opts.DevGuildID,
		pollInterval: opts.PollInterval,
	}
	bot.embeds = newEmbedUpdater(bot)

	session.AddHandler(bot.onInteraction)
	return bot, nil
}

// Start opens the gateway connection and begins polling for reminders.
func (b *Bot) Start(ctx context.Context) error {
	if err := b.session.Open(); err != nil {
		return fmt.Errorf("opening gateway: %w", err)
	}

	b.icons = loadIcons(ctx, b.session, b.appID())
	b.logger.InfoContext(ctx, "loaded icons", "count", len(b.icons))

	go b.pollNotifications(ctx)
	return nil
}

// appID is the application's own id. State carries it once the gateway is open, which
// is the only place this is called from.
func (b *Bot) appID() string {
	if state := b.session.State; state != nil && state.User != nil {
		return state.User.ID
	}
	return ""
}

// Stop closes the gateway connection.
func (b *Bot) Stop() {
	if err := b.session.Close(); err != nil {
		b.logger.Error("closing gateway", "error", err)
	}
}

// recoverPanic turns a panic into a logged error instead of a dead process. discordgo
// dispatches every handler in its own goroutine, so without this one nil dereference
// anywhere below takes the whole bot with it: the runtime unwinds past main, and the
// image is distroless, so there is no shell and no supervisor to notice. The trace also
// goes to stderr as plain text rather than through the logger, which is why a crash
// like that leaves nothing behind in a JSON log stream.
//
// what names the goroutine. A stack says where it broke, not which loop stopped.
func (b *Bot) recoverPanic(ctx context.Context, what string) {
	rec := recover()
	if rec == nil {
		return
	}
	b.logger.ErrorContext(ctx, "recovered panic", "in", what, "panic", rec, "stack", string(debug.Stack()))
}

// recoverInteraction is recoverPanic for the one goroutine with a raider waiting on the
// other end. A deferred interaction shows "thinking" until something edits it, so a
// panic that says nothing leaves them watching a spinner until it expires.
func (b *Bot) recoverInteraction(ctx context.Context, i *discordgo.InteractionCreate) {
	rec := recover()
	if rec == nil {
		return
	}
	b.logger.ErrorContext(ctx, "recovered panic", "in", "interaction", "panic", rec, "stack", string(debug.Stack()))

	// Whether the handler got as far as deferring decides which of these two works, and
	// a recover cannot know how far it got, so try the deferred case and fall back.
	const sorry = "Something broke on our side. It has been logged."
	if err := b.edit(i, sorry, nil); err != nil {
		if err := b.replyEphemeral(i, sorry); err != nil {
			b.logger.ErrorContext(ctx, "reporting panic to discord", "error", err)
		}
	}
}

// onInteraction is the single entry point Discord calls. It routes and does nothing
// else, so the logic worth testing sits in the functions it calls.
func (b *Bot) onInteraction(_ *discordgo.Session, i *discordgo.InteractionCreate) {
	// Discord gives an interaction three seconds to be answered and fifteen minutes
	// to be edited afterwards, so this is the outer bound on anything below.
	ctx, cancel := context.WithTimeout(context.Background(), 14*time.Minute)
	defer cancel()

	// After cancel, so the apology is written before the context it uses is cancelled.
	defer b.recoverInteraction(ctx, i)

	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		b.onCommand(ctx, i)
	case discordgo.InteractionMessageComponent:
		b.onComponent(ctx, i)
	case discordgo.InteractionModalSubmit:
		b.onModalSubmit(ctx, i)
	case discordgo.InteractionPing, discordgo.InteractionApplicationCommandAutocomplete:
	}
}

// actorFrom reads the raw Discord facts off an interaction. The service decides what
// they mean: role names vary per guild and it cannot ask Discord itself, so the bot
// reports and never judges.
func actorFrom(i *discordgo.InteractionCreate) (client.Actor, error) {
	guildID, err := strconv.ParseUint(i.GuildID, 10, 64)
	if err != nil {
		return client.Actor{}, fmt.Errorf("parsing guild id %q: %w", i.GuildID, err)
	}

	member := i.Member
	if member == nil || member.User == nil {
		return client.Actor{}, fmt.Errorf("interaction has no guild member")
	}

	discordID, err := strconv.ParseUint(member.User.ID, 10, 64)
	if err != nil {
		return client.Actor{}, fmt.Errorf("parsing user id %q: %w", member.User.ID, err)
	}

	roleIDs := make([]uint64, 0, len(member.Roles))
	for _, raw := range member.Roles {
		id, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return client.Actor{}, fmt.Errorf("parsing role id %q: %w", raw, err)
		}
		roleIDs = append(roleIDs, id)
	}

	return client.Actor{
		DiscordID:  discordID,
		GuildID:    guildID,
		RoleIDs:    roleIDs,
		GuildAdmin: member.Permissions&discordgo.PermissionAdministrator != 0,
	}, nil
}
