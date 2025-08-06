package tests

import (
	"testing"

	"github.com/bwmarrin/discordgo"
	"hasscord/bot"
	"hasscord/config"
)

// MockSession is a mock implementation of the Discord session for testing.
type MockSession struct {
	ChannelID string
	Message   string
}

// ChannelMessageSend is a mock implementation of the ChannelMessageSend method.
func (s *MockSession) ChannelMessageSend(channelID, content string, options ...discordgo.RequestOption) (*discordgo.Message, error) {
	s.ChannelID = channelID
	s.Message = content
	return nil, nil
}

func (s *MockSession) ChannelVoiceJoin(guildID, channelID string, mute, deaf bool) (voice *discordgo.VoiceConnection, err error) {
	return nil, nil
}

// PingCommand is a mock implementation of the Command interface for testing.
type PingCommand struct{}

// Name returns the command's name.
func (p *PingCommand) Name() string {
	return "ping"
}

// Execute runs the command.
func (p *PingCommand) Execute(s bot.Messager, m *discordgo.MessageCreate, args []string) {
	s.ChannelMessageSend(m.ChannelID, "Pong!")
}

func TestBot(t *testing.T) {
	cfg := &config.Config{
		Token:  "test_token",
		Prefix: "!",
	}

	b, err := bot.New(cfg)
	if err != nil {
		t.Fatalf("Error creating bot: %v", err)
	}

	b.RegisterCommand(&PingCommand{})

	m := &discordgo.MessageCreate{
		Message: &discordgo.Message{
			ChannelID: "test_channel",
			Content:   "!ping",
			Author: &discordgo.User{
				ID: "test_user",
			},
		},
	}

	cmd, ok := b.Commands["ping"]
	if !ok {
		t.Fatal("Command not registered")
	}

	mockSession := &MockSession{}
	cmd.Execute(mockSession, m, []string{})

	if mockSession.ChannelID != "test_channel" {
		t.Errorf("Expected channel ID to be 'test_channel', got '%s'", mockSession.ChannelID)
	}

	if mockSession.Message != "Pong!" {
		t.Errorf("Expected message to be 'Pong!', got '%s'", mockSession.Message)
	}
}