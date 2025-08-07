package commands

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"hasscord/bot"
	"hasscord/hass"

	"github.com/bwmarrin/discordgo"
)

// State represents the state command.
type State struct {
	HassClient *hass.Client
}

// Name returns the command's name.
func (s *State) Name() string {
	return "state"
}

// Execute runs the command.
func (s *State) Execute(b bot.Messager, m *discordgo.MessageCreate, args []string) {
	if s.HassClient == nil {
		b.ChannelMessageSend(m.ChannelID, "Home Assistant client not initialized.")
		return
	}

	// Request all states from Home Assistant
	responseChan := make(chan hass.Message, 1)
	id := s.HassClient.NextMessageID()
	s.HassClient.RegisterPending(id, responseChan)

	req := map[string]interface{}{
		"id":   id,
		"type": "get_states",
	}

	err := s.HassClient.Conn.WriteJSON(req)
	if err != nil {
		log.Printf("Error sending get_states request: %v", err)
		b.ChannelMessageSend(m.ChannelID, "Error fetching states from Home Assistant.")
		return
	}

	select {
	case response := <-responseChan:
		if !response.Success {
			log.Printf("Failed to get states: %v", response.Error.Message)
			b.ChannelMessageSend(m.ChannelID, fmt.Sprintf("Failed to get states: %s", response.Error.Message))
			return
		}

		var states []hass.State
		err := json.Unmarshal(response.Result, &states)
		if err != nil {
			log.Printf("Error unmarshaling states: %v", err)
			b.ChannelMessageSend(m.ChannelID, "Error processing states from Home Assistant.")
			return
		}

		var sb strings.Builder
		sb.WriteString("**Home Assistant States (binary_sensor.dvere_):**\n")
		found := false
		for _, state := range states {
			if strings.HasPrefix(state.EntityID, "binary_sensor.dvere_") {
				sb.WriteString(fmt.Sprintf("- `%s`: `%s`\n", state.EntityID, state.State))
				found = true
			}
		}

		if !found {
			sb.WriteString("No matching entities found.\n")
		}

		b.ChannelMessageSend(m.ChannelID, sb.String())

	case <-time.After(5 * time.Second):
		b.ChannelMessageSend(m.ChannelID, "Timeout waiting for Home Assistant states.")
	}
}
