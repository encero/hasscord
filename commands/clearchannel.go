package commands

import (
	"fmt"
	"log"
	"time"

	"github.com/bwmarrin/discordgo"
	"hasscord/bot"
	"hasscord/config"
)

// ClearChannel is a command that deletes all messages in a specified channel.
type ClearChannel struct {
	Config *config.Config
}

// Name returns the command's name.
func (c *ClearChannel) Name() string {
	return "clear"
}

// Execute runs the command.
func (c *ClearChannel) Execute(s bot.Messager, m *discordgo.MessageCreate, args []string) {
	if m.ChannelID != c.Config.ChannelID {
		s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("This command can only be used in the configured channel: <#%s>", c.Config.ChannelID))
		return
	}

	s.ChannelMessageSend(m.ChannelID, "Starting to clear channel...")

	deletedCount := 0
	lastMessageID := m.ID // Start from the message that triggered the command

	for {
		messages, err := s.ChannelMessages(c.Config.ChannelID, 100, lastMessageID, "", "")
		if err != nil {
			log.Printf("Error fetching messages: %v", err)
			s.ChannelMessageSend(m.ChannelID, "Error fetching messages.")
			return
		}

		if len(messages) == 0 {
			break // No more messages to delete
		}

		var messageIDs []string
		var oldMessageIDs []string

		for _, msg := range messages {
			if msg.ID == m.ID { // Don't delete the command message itself in this batch
				continue
			}

			// Discord only allows bulk deletion of messages less than 14 days old
			if time.Since(msg.Timestamp) > (14 * 24 * time.Hour) {
				oldMessageIDs = append(oldMessageIDs, msg.ID)
			} else {
				messageIDs = append(messageIDs, msg.ID)
			}
		}

		if len(messageIDs) > 0 {
			err = s.ChannelMessagesBulkDelete(c.Config.ChannelID, messageIDs)
			if err != nil {
				log.Printf("Error bulk deleting messages: %v", err)
				s.ChannelMessageSend(m.ChannelID, "Error bulk deleting messages.")
				return
			}
			deletedCount += len(messageIDs)
		}

		// Delete old messages one by one
		for _, msgID := range oldMessageIDs {
			err = s.ChannelMessageDelete(c.Config.ChannelID, msgID)
			if err != nil {
				log.Printf("Error deleting old message %s: %v", msgID, err)
				// Continue trying to delete other messages even if one fails
			}
			deletedCount++
			time.Sleep(1 * time.Second) // Avoid rate limits
		}

		if len(messages) > 0 {
			lastMessageID = messages[len(messages)-1].ID
		} else {
			break
		}

		time.Sleep(1 * time.Second) // Avoid rate limits
	}

	s.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Finished clearing channel. Deleted %d messages.", deletedCount))
}
