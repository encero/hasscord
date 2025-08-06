package commands

import (
	"github.com/bwmarrin/discordgo"
	"hasscord/bot"
)

// Ping is a simple command that responds with "Pong!".
type Ping struct{}

// Name returns the command's name.
func (p *Ping) Name() string {
	return "ping"
}

// Execute runs the command.
func (p *Ping) Execute(s bot.Messager, m *discordgo.MessageCreate, args []string) {
	s.ChannelMessageSend(m.ChannelID, "Pong!")
}
