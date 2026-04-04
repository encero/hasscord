package main

import (
	"context"
	"log"
	"time"

	"hasscord/bot"
	"hasscord/commands"
	"hasscord/config"
	"hasscord/hass"
	"hasscord/sensors"
)

// Build time variables - these will be set during compilation
var (
	BuildTime   string
	BuildCommit string
	BuildDate   string
)

func main() {
	// Print build information at startup
	if BuildTime != "" {
		log.Printf("🚀 HassCord starting up...")
		log.Printf("📦 Build Time: %s", BuildTime)
		if BuildCommit != "" {
			log.Printf("🔗 Build Commit: %s", BuildCommit)
		}
		if BuildDate != "" {
			log.Printf("📅 Build Date: %s", BuildDate)
		}
		log.Printf("⏰ Current Time: %s", time.Now().Format(time.RFC3339))
		log.Printf("")
	} else {
		log.Printf("🚀 HassCord starting up... (development build)")
		log.Printf("⏰ Current Time: %s", time.Now().Format(time.RFC3339))
		log.Printf("")
	}

	cfg := config.Load()

	if err := cfg.Validate(); err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	b, err := bot.New(cfg)
	if err != nil {
		log.Fatalf("Error creating bot: %v", err)
	}

	// Start Home Assistant client
	hassClient, err := hass.New(cfg.HassURL, cfg.HassToken)
	if err != nil {
		log.Fatalf("Error creating Home Assistant client: %v", err)
	}

	err = hassClient.Authenticate()
	if err != nil {
		log.Fatalf("Error authenticating with Home Assistant: %v", err)
	}

	// Register commands
	b.RegisterCommand(&commands.Ping{})
	b.RegisterCommand(&commands.ClearChannel{Config: cfg})
	b.RegisterCommand(&commands.State{HassClient: hassClient, SensorPrefix: cfg.SensorPrefix})
	b.RegisterCommand(&commands.Pause{})

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())

	go hassClient.Listen(ctx)

	events, err := hassClient.SubscribeToEvents()
	if err != nil {
		log.Fatalf("Error subscribing to Home Assistant events: %v", err)
	}

	go sensors.HandleHassEvents(ctx, b, events, cfg.ChannelID, cfg.SensorPrefix)
	go sensors.CheckOnSensors(
		ctx,
		b,
		cfg.ChannelID,
		cfg.SensorOnTimeout,
		cfg.SensorOnTimeoutReminder,
		cfg.SensorOnTimeoutReminderBackoff,
		cfg.SensorOnTimeoutReminderMax,
		cfg.SensorPrefix,
	)

	b.Start(cancel)
}
