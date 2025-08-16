package main

import (
	"log"

	"hasscord/bot"
	"hasscord/commands"
	"hasscord/config"
	"hasscord/hass"
	"hasscord/sensors"
)

func main() {
	cfg := config.Load()

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
	b.RegisterCommand(&commands.State{HassClient: hassClient})
	b.RegisterCommand(&commands.Pause{})

	go hassClient.Listen()

	events, err := hassClient.SubscribeToEvents()
	if err != nil {
		log.Fatalf("Error subscribing to Home Assistant events: %v", err)
	}

	go sensors.HandleHassEvents(b, events, cfg.ChannelID)
	go sensors.CheckOnSensors(b, cfg.ChannelID, cfg.SensorOnTimeout, cfg.SensorOnTimeoutReminder)

	b.Start()
}
