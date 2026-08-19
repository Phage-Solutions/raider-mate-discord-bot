package discord

import (
	"context"
	"fmt"
	"strconv"

	"github.com/bwmarrin/discordgo"

	"github.com/Phage-Solutions/raider-mate-discord-bot/internal/client"
)

// eventOption is the event id argument the comp commands share. A UUID is a poor thing
// to ask someone to type, so the event embed's footer carries it to copy.
var eventOption = &discordgo.ApplicationCommandOption{
	Name:        "event",
	Description: "Event id, from the footer of the event message",
	Type:        discordgo.ApplicationCommandOptionString,
	Required:    true,
}

// reminderOption is the pre-event reminder lead time both create commands take. Minutes
// rather than a parsed duration: it is one number, and an integer option validates the
// range in Discord's client instead of in an error message.
var reminderOption = &discordgo.ApplicationCommandOption{
	Name:        "reminder",
	Description: "Minutes before the start to remind signups. 0 for none, blank for the guild default",
	Type:        discordgo.ApplicationCommandOptionInteger,
	MinValue:    ptr(0.0),
	MaxValue:    1440,
}

// commands is the slash command surface. Subcommand groups rather than flat commands,
// so the list stays readable as more arrive.
//
// Raid Mate is guild install only: a user-installed copy has no roster to act on.
var commands = []*discordgo.ApplicationCommand{
	{
		Name:        "raid",
		Description: "Raid nights",
		Options: []*discordgo.ApplicationCommandOption{{
			Name:        "create",
			Description: "Create a raid and post its signup sheet",
			Type:        discordgo.ApplicationCommandOptionSubCommand,
			// Required options first: Discord refuses a command whose optional
			// arguments come before required ones.
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:        "title",
					Description: "What you are running, for example Heroic Nerub-ar Palace",
					Type:        discordgo.ApplicationCommandOptionString,
					Required:    true,
				},
				{
					Name:        "starts",
					Description: "tomorrow 20:00, sat 20:00, or 2026-09-01 20:00",
					Type:        discordgo.ApplicationCommandOptionString,
					Required:    true,
				},
				{
					// Discord has no date component, so this is a picker for the one
					// field that can have one.
					Name:        "difficulty",
					Description: "Raid difficulty",
					Type:        discordgo.ApplicationCommandOptionString,
					Choices: []*discordgo.ApplicationCommandOptionChoice{
						{Name: "Normal", Value: string(client.DifficultyNormal)},
						{Name: "Heroic", Value: string(client.DifficultyHeroic)},
						{Name: "Mythic", Value: string(client.DifficultyMythic)},
					},
				},
				{
					Name:        "signups_close",
					Description: "Defaults to an hour before the raid",
					Type:        discordgo.ApplicationCommandOptionString,
				},
				{
					Name:        "tanks",
					Description: "Tanks wanted",
					Type:        discordgo.ApplicationCommandOptionInteger,
					MinValue:    ptr(0.0),
					MaxValue:    40,
				},
				{
					Name:        "healers",
					Description: "Healers wanted",
					Type:        discordgo.ApplicationCommandOptionInteger,
					MinValue:    ptr(0.0),
					MaxValue:    40,
				},
				{
					Name:        "melee",
					Description: "Melee places",
					Type:        discordgo.ApplicationCommandOptionInteger,
					MinValue:    ptr(0.0),
					MaxValue:    40,
				},
				{
					Name:        "ranged",
					Description: "Ranged places",
					Type:        discordgo.ApplicationCommandOptionInteger,
					MinValue:    ptr(0.0),
					MaxValue:    40,
				},
				reminderOption,
			},
		}},
	},
	// A Mythic+ group is its own command rather than a difficulty of /raid. The service
	// sizes it differently (five people, one tank, one healer) and ranks candidates on
	// score rather than item level, and a guild says "dungeon", not "mythic plus raid".
	{
		Name:        "dungeon",
		Description: "Mythic+ groups",
		Options: []*discordgo.ApplicationCommandOption{{
			Name:        "create",
			Description: "Create a Mythic+ group and post its signup sheet",
			Type:        discordgo.ApplicationCommandOptionSubCommand,
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:        "title",
					Description: "What you are running, for example +12 Ara-Kara",
					Type:        discordgo.ApplicationCommandOptionString,
					Required:    true,
				},
				{
					Name:        "starts",
					Description: "tonight 20:00, sat 20:00, or 2026-09-01 20:00",
					Type:        discordgo.ApplicationCommandOptionString,
					Required:    true,
				},
				{
					Name:        "signups_close",
					Description: "Defaults to an hour before the group, or the start time if that is sooner",
					Type:        discordgo.ApplicationCommandOptionString,
				},
				reminderOption,
			},
		}},
	},
	{
		Name:        "character",
		Description: "Your characters",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Name:        "add",
				Description: "Register a character",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
			},
			{
				Name:        "roles",
				Description: "Edit which roles your character can play",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
			},
			{
				Name:        "main",
				Description: "Choose which of your characters is your main",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
			},
		},
	},
	// Raid-lead controls are slash commands rather than buttons on the event message.
	// A message's components are the same for everyone who can see it, so a lock
	// button could not be gated on the lock link the way hard rule 6 requires. A
	// command's reply is ephemeral, which is where the gating can honestly happen.
	{
		Name:        "comp",
		Description: "Compositions",
		Options: []*discordgo.ApplicationCommandOption{
			{
				Name:        "show",
				Description: "Show an event's current comp",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Options:     []*discordgo.ApplicationCommandOption{eventOption},
			},
			{
				Name:        "lock",
				Description: "Run the assigner and post the result",
				Type:        discordgo.ApplicationCommandOptionSubCommand,
				Options:     []*discordgo.ApplicationCommandOption{eventOption},
			},
		},
	},
	{
		Name:        "config",
		Description: "Server settings",
		Options: []*discordgo.ApplicationCommandOption{{
			Name:        "channel",
			Description: "Choose where event messages get posted",
			Type:        discordgo.ApplicationCommandOptionSubCommand,
			Options: []*discordgo.ApplicationCommandOption{{
				Name:        "channel",
				Description: "Leave empty to post wherever the create command was run",
				Type:        discordgo.ApplicationCommandOptionChannel,
				ChannelTypes: []discordgo.ChannelType{
					discordgo.ChannelTypeGuildText,
					discordgo.ChannelTypeGuildNews,
				},
				Required: false,
			}},
		}, {
			Name:        "banner",
			Description: "Image shown on every event card",
			Type:        discordgo.ApplicationCommandOptionSubCommand,
			Options: []*discordgo.ApplicationCommandOption{{
				Name:        "url",
				Description: "https link to an image. Leave empty to remove it",
				Type:        discordgo.ApplicationCommandOptionString,
				Required:    false,
			}},
		}, {
			Name:        "mentions",
			Description: "Roles pinged when an event is posted",
			Type:        discordgo.ApplicationCommandOptionSubCommand,
		}, {
			Name:        "timezone",
			Description: "The zone your raid times are in, so \"tomorrow 20:00\" means what you meant",
			Type:        discordgo.ApplicationCommandOptionSubCommand,
			Options: []*discordgo.ApplicationCommandOption{{
				Name:        "zone",
				Description: "IANA name, for example Europe/Bratislava. Leave empty to require an offset instead",
				Type:        discordgo.ApplicationCommandOptionString,
				Required:    false,
			}},
		}, {
			Name:        "reminder",
			Description: "How and when signups are reminded an event is about to start",
			Type:        discordgo.ApplicationCommandOptionSubCommand,
			Options: []*discordgo.ApplicationCommandOption{{
				Name:        "minutes",
				Description: "Default minutes before the start. 0 for none. Leave empty for 30",
				Type:        discordgo.ApplicationCommandOptionInteger,
				MinValue:    ptr(0.0),
				MaxValue:    1440,
				Required:    false,
			}, {
				Name:        "delivery",
				Description: "Ping the events channel, DM each raider, or both. Leave empty for a ping",
				Type:        discordgo.ApplicationCommandOptionString,
				Choices: []*discordgo.ApplicationCommandOptionChoice{
					{Name: "Ping the events channel", Value: client.ReminderDeliveryPing},
					{Name: "DM each raider", Value: client.ReminderDeliveryDM},
					{Name: "Both", Value: client.ReminderDeliveryBoth},
				},
				Required: false,
			}},
		}},
	},
	{
		Name:        "help",
		Description: "What this bot does",
	},
}

// RegisterCommands publishes the command definitions, to the dev guild when one is
// configured and globally otherwise.
//
// Guild commands update instantly, which is what makes them the development setting.
// Global commands self-heal instead: Discord version-checks an invocation against a
// stale client and reloads rather than misrouting it.
//
// This is deliberately not part of startup. Discord allows 200 command creates per day
// per guild, and a restart loop during an afternoon of development would spend them.
func (b *Bot) RegisterCommands(ctx context.Context) error {
	// State.User is only populated once the gateway is open, and registering does not
	// open it, so the usual path here is the API lookup. The state check is for the
	// case where something already connected.
	var appID string
	if state := b.session.State; state != nil && state.User != nil {
		appID = state.User.ID
	}
	if appID == "" {
		app, err := b.session.Application("@me")
		if err != nil {
			return fmt.Errorf("looking up application: %w", err)
		}
		appID = app.ID
	}

	var guildID string
	if b.devGuildID != 0 {
		guildID = strconv.FormatUint(b.devGuildID, 10)
	}

	if _, err := b.session.ApplicationCommandBulkOverwrite(appID, guildID, commands, discordgo.WithContext(ctx)); err != nil {
		return fmt.Errorf("registering commands: %w", err)
	}

	b.logger.InfoContext(ctx, "registered commands", "count", len(commands), "guild_id", guildID)
	return nil
}

func (b *Bot) onCommand(ctx context.Context, i *discordgo.InteractionCreate) {
	data := i.ApplicationCommandData()

	var sub string
	if len(data.Options) > 0 && data.Options[0].Type == discordgo.ApplicationCommandOptionSubCommand {
		sub = data.Options[0].Name
	}

	switch data.Name + " " + sub {
	case "raid create":
		b.createEvent(ctx, i, client.EventRaid, data.Options[0].Options)
	case "dungeon create":
		b.createEvent(ctx, i, client.EventMythicPlus, data.Options[0].Options)
	case "character add":
		b.openCharacterModal(ctx, i)
	case "character roles":
		b.startRoleEdit(ctx, i)
	case "character main":
		b.startMainSwitch(ctx, i)
	case "comp show":
		b.showComp(ctx, i, data.Options[0].Options[0].StringValue())
	case "comp lock":
		b.lockComp(ctx, i, data.Options[0].Options[0].StringValue())
	case "config channel":
		b.setEventsChannel(ctx, i, data.Options[0].Options)
	case "config timezone":
		b.setTimezone(ctx, i, data.Options[0].Options)
	case "config mentions":
		b.setEventMentions(ctx, i)
	case "config banner":
		b.setEventBanner(ctx, i, data.Options[0].Options)
	case "config reminder":
		b.setReminder(ctx, i, data.Options[0].Options)
	case "help ":
		b.showHelp(i)
	default:
		b.logger.WarnContext(ctx, "unrouted command", "name", data.Name, "subcommand", sub)
	}
}

func (b *Bot) showHelp(i *discordgo.InteractionCreate) {
	const help = "**Raider Mate**\n" +
		"`/raid create` posts a raid signup sheet. Raid leads only.\n" +
		"`/dungeon create` does the same for a Mythic+ group.\n" +
		"`/character add` registers a character. You need one before you can sign up.\n" +
		"`/character roles` sets which roles that character can play, in the order you would rather play them.\n" +
		"`/character main` moves the main flag to another of your characters.\n" +
		"`/comp show <event id>` shows the current comp.\n" +
		"`/comp lock <event id>` runs the assigner. Raid leads only.\n" +
		"`/config channel` picks where event messages get posted. Server admins only.\n" +
		"`/config timezone` sets the zone raid times are typed in. Server admins only.\n" +
		"`/config mentions` picks which roles get pinged by a new event. Server admins only.\n" +
		"`/config reminder` sets how long before an event signups are reminded, and whether that is a ping or a DM. Server admins only.\n\n" +
		"Sign up from the buttons on the event message. If you can play more than one role, say so: " +
		"that is the whole point, and it is what lets a raid lead find a tank at ten to eight."

	if err := b.replyEphemeral(i, help); err != nil {
		b.logger.Error("replying to help", "error", err)
	}
}
