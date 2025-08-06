package main

import (
	"log"

	"hasscord/bot"
	"hasscord/commands"
	"hasscord/config"
)

func main() {
	cfg := config.Load()

	b, err := bot.New(cfg)
	if err != nil {
		log.Fatalf("Error creating bot: %v", err)
	}

	// Register commands
	b.RegisterCommand(&commands.Ping{})

	b.Start()
}
