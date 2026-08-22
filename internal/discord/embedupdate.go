package discord

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/bwmarrin/discordgo"

	"github.com/Raider-Mate/raider-mate-discord-bot/internal/client"
)

// coalesceWindow is how long a pending redraw waits for the next signup. Twenty people
// answering in the same minute should cost one edit, not twenty, and Discord rate
// limits message edits per channel.
const coalesceWindow = time.Second

// embedUpdater redraws event messages, one edit per burst of signups.
type embedUpdater struct {
	bot *Bot

	mu      sync.Mutex
	pending map[string]*time.Timer
}

func newEmbedUpdater(bot *Bot) *embedUpdater {
	return &embedUpdater{bot: bot, pending: map[string]*time.Timer{}}
}

// schedule asks for a redraw of one event, collapsing anything already queued for it.
// The actor is the raider whose click triggered this; the read it authorises is the
// public event message, so any signed-in member is enough.
func (u *embedUpdater) schedule(actor client.Actor, eventID string) {
	u.mu.Lock()
	defer u.mu.Unlock()

	if timer, ok := u.pending[eventID]; ok {
		timer.Reset(coalesceWindow)
		return
	}

	u.pending[eventID] = time.AfterFunc(coalesceWindow, func() {
		u.mu.Lock()
		delete(u.pending, eventID)
		u.mu.Unlock()

		// Detached from the interaction on purpose: the redraw outlives the click that
		// asked for it, and the raider has already been told their answer landed.
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		u.bot.redraw(ctx, actor, eventID)
	})
}

// redraw rebuilds an event message from the service and edits it in place. Never a
// repost: people pin these, and a repost breaks every link to them.
func (b *Bot) redraw(ctx context.Context, actor client.Actor, eventID string) {
	event, err := b.api.Event(ctx, actor, eventID)
	if err != nil {
		b.logger.ErrorContext(ctx, "reading event for redraw", "error", err, "event_id", eventID)
		return
	}
	if event.MessageID == nil || event.ChannelID == nil {
		b.logger.WarnContext(ctx, "event has no message to edit", "event_id", eventID)
		return
	}

	signups, err := b.api.Signups(ctx, actor, eventID)
	if err != nil {
		b.logger.ErrorContext(ctx, "reading signups for redraw", "error", err, "event_id", eventID)
		return
	}

	// Settings carry the banner. A failed read costs the artwork, not the redraw.
	settings, err := b.api.GuildSettings(ctx, actor)
	if err != nil {
		b.logger.WarnContext(ctx, "reading guild settings for redraw", "error", err)
	}

	view := b.viewFor(event, signups, nil, settings)

	// A comp that has never been locked does not exist, which is not a failure: it is
	// the state the embed groups by what raiders can play rather than by seat.
	//
	// The name is resolved, not assumed. A comp renamed from the dashboard fell straight
	// through the not-found arm below, so every save queued a redraw that then rebuilt
	// the card without the board it had been queued for, and nothing anywhere said so.
	name, err := b.boardComp(ctx, actor, eventID)
	if err != nil {
		b.logger.ErrorContext(ctx, "listing comps for redraw", "error", err, "event_id", eventID)
	}
	if name != "" {
		board, err := b.api.Comp(ctx, actor, eventID, name)
		switch {
		case err == nil:
			view.Board = &board
		case errors.Is(err, client.ErrNotFound):
		default:
			b.logger.ErrorContext(ctx, "reading comp for redraw", "error", err, "event_id", eventID)
		}
	}

	embeds := []*discordgo.MessageEmbed{BuildEventEmbed(view)}
	components := eventButtons(eventID)
	_, err = b.session.ChannelMessageEditComplex(&discordgo.MessageEdit{
		Channel:    *event.ChannelID,
		ID:         *event.MessageID,
		Embeds:     &embeds,
		Components: &components,
	}, discordgo.WithContext(ctx))
	if err != nil {
		b.logger.ErrorContext(ctx, "editing event message", "error", err, "event_id", eventID)
	}
}

// viewFor gathers what the embed builder needs. The builder stays pure; this is where
// the pieces that came from three different calls are put together.
func (b *Bot) viewFor(event client.Event, signups []client.Signup, board *client.Board, settings client.GuildSettings) EventView {
	view := EventView{Event: event, Signups: signups, Board: board, Icons: b.icons}
	if settings.EventBannerURL != nil {
		view.BannerURL = *settings.EventBannerURL
	}
	if b.dashboardURL != "" {
		view.DashboardURL = b.dashboardURL + "/events/" + event.ID
	}
	return view
}
