package bot

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"hasscord/config"
)

// Messager is an interface that abstracts the discordgo.Session for testing.
type Messager interface {
	ChannelMessageSend(channelID, content string, options ...discordgo.RequestOption) (*discordgo.Message, error)
	ChannelVoiceJoin(guildID, channelID string, mute, deaf bool) (voice *discordgo.VoiceConnection, err error)
	ChannelMessages(channelID string, limit int, beforeID, afterID, aroundID string, options ...discordgo.RequestOption) ([]*discordgo.Message, error)
	ChannelMessagesBulkDelete(channelID string, messages []string, options ...discordgo.RequestOption) error
	ChannelMessageDelete(channelID, messageID string, options ...discordgo.RequestOption) error
}

// Command is the interface for all bot commands.
type Command interface {
	Name() string
	Execute(s Messager, m *discordgo.MessageCreate, args []string)
}

// Bot represents the Discord bot.
type Bot struct {
	Session  *discordgo.Session
	Config   *config.Config
	Commands map[string]Command
}

// New creates a new Bot instance.
func New(cfg *config.Config) (*Bot, error) {
	s, err := discordgo.New("Bot " + cfg.Token)
	if err != nil {
		return nil, err
	}

	// Set the necessary intents.
	s.Identify.Intents = discordgo.IntentsGuilds | discordgo.IntentsGuildMessages

	return &Bot{
		Session:  s,
		Config:   cfg,
		Commands: make(map[string]Command),
	}, nil
}

// RegisterCommand registers a new command.
func (b *Bot) RegisterCommand(cmd Command) {
	b.Commands[cmd.Name()] = cmd
}

// Start starts the bot and connects to Discord. It blocks until shutdown signal is received.
// The returned cancel function should be called to signal goroutines to stop.
// It will retry connecting to Discord with exponential backoff if the connection fails.
func (b *Bot) Start(cancel context.CancelFunc) {
	b.Session.AddHandler(b.ready)
	b.Session.AddHandler(b.messageCreate)

	// Retry connecting to Discord with exponential backoff
	maxRetries := 10
	baseBackoff := time.Second
	maxBackoff := 5 * time.Minute

	for attempt := 0; attempt < maxRetries; attempt++ {
		err := b.Session.Open()
		if err == nil {
			break
		}

		if attempt == maxRetries-1 {
			log.Fatalf("Failed to connect to Discord after %d attempts: %v", maxRetries, err)
		}

		backoff := baseBackoff * time.Duration(1<<attempt)
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
		log.Printf("Discord connection failed: %v. Retrying in %v (attempt %d/%d)", err, backoff, attempt+1, maxRetries)
		time.Sleep(backoff)
	}

	fmt.Println("Bot is now running. Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-sc

	fmt.Println("Shutting down...")
	cancel() // Signal all goroutines to stop
	b.Session.Close()
}

// ready is the handler for the ready event.
func (b *Bot) ready(s *discordgo.Session, event *discordgo.Ready) {
	fmt.Println("Bot is connected and ready!")
	fmt.Println("Currently in the following guilds:")
	for _, guild := range s.State.Guilds {
		fmt.Printf("- %s\n", guild.Name)
	}
}

// messageCreate is the handler for new messages.
func (b *Bot) messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.ID == s.State.User.ID {
		return
	}

	if !strings.HasPrefix(m.Content, b.Config.Prefix) {
		return
	}

	content := strings.TrimPrefix(m.Content, b.Config.Prefix)
	parts := strings.Fields(content)
	if len(parts) == 0 {
		return
	}

	cmdName := parts[0]
	args := parts[1:]

	cmd, ok := b.Commands[cmdName]
	if !ok {
		return
	}

	cmd.Execute(s, m, args)
}
