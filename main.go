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

	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())

	// Create Home Assistant manager with reconnection support
	hassManager := hass.NewManager(hass.DefaultManagerConfig(cfg.HassURL, cfg.HassToken))

	// Try initial connection - if it fails, we'll keep trying in the background
	if hassManager.TryConnectOnce() {
		log.Println("Home Assistant: initial connection successful")
	} else {
		log.Println("Home Assistant: initial connection failed, will retry in background")
	}

	// Start the manager in the background - it will handle reconnection automatically
	go hassManager.Run(ctx)

	// Register commands (state command needs the manager for client access)
	b.RegisterCommand(&commands.Ping{})
	b.RegisterCommand(&commands.ClearChannel{Config: cfg})
	b.RegisterCommand(&commands.State{HassManager: hassManager, SensorPrefix: cfg.SensorPrefix})
	b.RegisterCommand(&commands.Pause{})

	// Start event handlers
	go sensors.HandleHassEvents(ctx, b, hassManager.Events(), cfg.ChannelID, cfg.SensorPrefix)
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
