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
		b.ChannelMessageSend(m.ChannelID, "❌ **Error:** Home Assistant client not initialized.")
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
		b.ChannelMessageSend(m.ChannelID, "❌ **Error:** Failed to fetch states from Home Assistant.")
		return
	}

	select {
	case response := <-responseChan:
		if !response.Success {
			log.Printf("Failed to get states: %v", response.Error.Message)
			b.ChannelMessageSend(m.ChannelID, fmt.Sprintf("❌ **Error:** Failed to get states: %s", response.Error.Message))
			return
		}

		var states []hass.State
		err := json.Unmarshal(response.Result, &states)
		if err != nil {
			log.Printf("Error unmarshaling states: %v", err)
			b.ChannelMessageSend(m.ChannelID, "❌ **Error:** Failed to process states from Home Assistant.")
			return
		}

		var sb strings.Builder
		var doorSensors []hass.State

		// Separate door sensors from other binary sensors
		for _, state := range states {
			// grab all dvere_ sensors but not the opening ones
			if strings.HasPrefix(state.EntityID, "binary_sensor.dvere_") && !strings.HasSuffix(state.EntityID, "_opening") {
				doorSensors = append(doorSensors, state)
			}
		}

		// Build door sensor section
		if len(doorSensors) > 0 {
			sb.WriteString("🚪 **Door Sensors:**\n")
			for _, state := range doorSensors {
				status := "🔒 Closed"
				if state.State == "on" {
					status = "🔓 Open"
				}
				sb.WriteString(fmt.Sprintf("• `%s`: %s\n", strings.TrimPrefix(state.EntityID, "binary_sensor."), status))
			}
		} else {
			sb.WriteString("🚪 **Door Sensors:** None found\n")
		}

		// Add summary
		sb.WriteString(fmt.Sprintf("\n📊 **Summary:** %d door sensor(s)", len(doorSensors)))

		b.ChannelMessageSend(m.ChannelID, sb.String())

	case <-time.After(5 * time.Second):
		b.ChannelMessageSend(m.ChannelID, "⏰ **Timeout:** Request timed out waiting for Home Assistant states.")
	}
}
