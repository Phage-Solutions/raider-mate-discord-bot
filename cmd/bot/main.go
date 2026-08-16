package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/Phage-Solutions/raider-mate-discord-bot/internal/client"
	"github.com/Phage-Solutions/raider-mate-discord-bot/internal/discord"
)

// Set at build time with -X main.version. Unlike a JVM manifest, a Go binary carries
// no version unless the linker is told one.
var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	// Registration is a flag rather than something startup does, because Discord
	// allows 200 command creates per day per guild and a restart loop would eat that.
	register := flag.Bool("register", false, "publish slash commands, then exit")
	uploadIcons := flag.String("upload-icons", "", "publish every PNG in this directory as a class or spec emoji, then exit")
	flag.Parse()

	if err := loadDotEnv(dotEnvPath); err != nil {
		return fmt.Errorf("loading %s: %w", dotEnvPath, err)
	}

	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	logger := newLogger(cfg.LogLevel)
	// Before the Discord session opens, so a token or gateway failure still says which
	// build hit it.
	logger.Info("starting bot", "version", version)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	api := client.New(cfg.ServiceBaseURL, cfg.ServiceAPIKey)

	bot, err := discord.New(discord.Options{
		Token:        cfg.DiscordToken,
		DevGuildID:   cfg.DevGuildID,
		PollInterval: cfg.PollInterval,
		API:          api,
		Logger:       logger,
	})
	if err != nil {
		return fmt.Errorf("building bot: %w", err)
	}

	if *register {
		return bot.RegisterCommands(ctx)
	}

	if *uploadIcons != "" {
		return bot.UploadIcons(ctx, *uploadIcons)
	}

	if err := bot.Start(ctx); err != nil {
		return fmt.Errorf("starting bot: %w", err)
	}
	defer bot.Stop()

	// Started only once the gateway is open, so a passing health check means the bot
	// actually reached a running state.
	health := startHealthServer(cfg.HealthAddr, logger)
	defer stopHealthServer(context.Background(), health, logger)

	logger.Info("bot running", "dev_guild_id", cfg.DevGuildID, "health_addr", cfg.HealthAddr)
	<-ctx.Done()
	// Hand the signals back to the runtime so a second Ctrl-C kills a hung shutdown.
	stop()
	logger.Info("shutting down")

	return nil
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	if err := lvl.UnmarshalText([]byte(level)); err != nil {
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}
