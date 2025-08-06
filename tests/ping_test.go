package tests

import (
	"testing"

	"github.com/bwmarrin/discordgo"
	"hasscord/commands"
)

func TestPingCommand(t *testing.T) {
	pingCmd := &commands.Ping{}

	m := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ChannelID: "test_channel",
		},
	}

	mockSession := &MockSession{}
	pingCmd.Execute(mockSession, m, []string{})

	if mockSession.ChannelID != "test_channel" {
		t.Errorf("Expected channel ID to be 'test_channel', got '%s'", mockSession.ChannelID)
	}

	if mockSession.Message != "Pong!" {
		t.Errorf("Expected message to be 'Pong!', got '%s'", mockSession.Message)
	}
}