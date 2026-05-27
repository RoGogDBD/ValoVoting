package config

import (
	"fmt"
	"os"
	"strconv"
)

// TwitchClientID is embedded at build time.
// Override: go build -ldflags="-X github.com/kudryavtsevmakar/valovoting/internal/config.TwitchClientID=xxx"
var TwitchClientID = "xiujcmpf7fwfalp9hiobyzydpf60v2"

type Config struct {
	TwitchClientID      string
	TwitchBroadcasterID string
	TwitchAccessToken   string
	TwitchChannel       string
	ChatCommand         string
	DefaultPollDuration int
	Port                string
}

func Load() (*Config, error) {
	cfg := &Config{
		// Client ID comes from the binary — env var can override for development
		TwitchClientID:      getEnv("TWITCH_CLIENT_ID", TwitchClientID),
		TwitchBroadcasterID: os.Getenv("TWITCH_BROADCASTER_ID"),
		TwitchAccessToken:   os.Getenv("TWITCH_ACCESS_TOKEN"),
		TwitchChannel:       os.Getenv("TWITCH_CHANNEL"),
		ChatCommand:         getEnv("CHAT_COMMAND", "!mapvote"),
		Port:                getEnv("PORT", "8080"),
	}

	dur := os.Getenv("DEFAULT_POLL_DURATION")
	if dur == "" {
		cfg.DefaultPollDuration = 60
	} else {
		n, err := strconv.Atoi(dur)
		if err != nil || n < 15 || n > 1800 {
			return nil, fmt.Errorf("DEFAULT_POLL_DURATION must be 15–1800")
		}
		cfg.DefaultPollDuration = n
	}

	if cfg.TwitchBroadcasterID == "" || cfg.TwitchAccessToken == "" || cfg.TwitchChannel == "" {
		return nil, fmt.Errorf("missing required env vars: TWITCH_BROADCASTER_ID, TWITCH_ACCESS_TOKEN, TWITCH_CHANNEL")
	}
	return cfg, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
