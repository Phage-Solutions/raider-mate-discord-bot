package main

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds the values the bot reads from the environment at startup.
type Config struct {
	DiscordToken   string
	ServiceBaseURL string
	ServiceAPIKey  string
	LogLevel       string
	// DevGuildID registers slash commands to one guild instead of globally. Guild
	// commands update instantly, so this is the development setting; production
	// leaves it zero and registers globally.
	DevGuildID   uint64
	PollInterval time.Duration
	// HealthAddr is the listen address for the liveness endpoint. The bot itself
	// accepts no inbound traffic; this exists only because Scaleway's container
	// health check requires a port to watch.
	HealthAddr string
}

func loadConfig() (Config, error) {
	token := os.Getenv("DISCORD_TOKEN")
	if token == "" {
		return Config{}, fmt.Errorf("DISCORD_TOKEN is required")
	}

	baseURL := os.Getenv("SERVICE_BASE_URL")
	if baseURL == "" {
		return Config{}, fmt.Errorf("SERVICE_BASE_URL is required")
	}

	apiKey := os.Getenv("SERVICE_API_KEY")
	if apiKey == "" {
		return Config{}, fmt.Errorf("SERVICE_API_KEY is required")
	}

	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "info"
	}

	var devGuildID uint64
	if raw := os.Getenv("DISCORD_DEV_GUILD_ID"); raw != "" {
		id, err := strconv.ParseUint(raw, 10, 64)
		if err != nil {
			return Config{}, fmt.Errorf("DISCORD_DEV_GUILD_ID: %w", err)
		}
		devGuildID = id
	}

	// The outbox stream is what makes delivery prompt. This is the fallback behind it,
	// covering a stream that died quietly and claims whose lease lapsed, so it is rare
	// on purpose.
	pollInterval := 5 * time.Minute
	if raw := os.Getenv("NOTIFICATION_POLL_INTERVAL"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("NOTIFICATION_POLL_INTERVAL: %w", err)
		}
		pollInterval = d
	}

	// PORT matches Scaleway's convention: the platform tells the container which port
	// its health check will hit.
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	return Config{
		DiscordToken:   token,
		ServiceBaseURL: baseURL,
		ServiceAPIKey:  apiKey,
		LogLevel:       logLevel,
		DevGuildID:     devGuildID,
		PollInterval:   pollInterval,
		HealthAddr:     ":" + port,
	}, nil
}
