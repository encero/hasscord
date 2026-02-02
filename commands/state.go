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
	HassClient   *hass.Client
	SensorPrefix string
}

// Name returns the command's name.
func (s *State) Name() string {
	return "state"
}

// Execute runs the command.
func (s *State) Execute(b bot.Messager, m *discordgo.MessageCreate, args []string) {
	if s.HassClient == nil {
		if _, err := b.ChannelMessageSend(m.ChannelID, "❌ **Error:** Home Assistant client not initialized."); err != nil {
			log.Printf("Error sending hass client error message: %v", err)
		}
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
		if _, sendErr := b.ChannelMessageSend(m.ChannelID, "❌ **Error:** Failed to fetch states from Home Assistant."); sendErr != nil {
			log.Printf("Error sending fetch error message: %v", sendErr)
		}
		return
	}

	select {
	case response := <-responseChan:
		if !response.Success {
			errMsg := "unknown error"
			if response.Error != nil {
				errMsg = response.Error.Message
			}
			log.Printf("Failed to get states: %v", errMsg)
			if _, sendErr := b.ChannelMessageSend(m.ChannelID, fmt.Sprintf("❌ **Error:** Failed to get states: %s", errMsg)); sendErr != nil {
				log.Printf("Error sending get states error message: %v", sendErr)
			}
			return
		}

		var states []hass.State
		err := json.Unmarshal(response.Result, &states)
		if err != nil {
			log.Printf("Error unmarshaling states: %v", err)
			if _, sendErr := b.ChannelMessageSend(m.ChannelID, "❌ **Error:** Failed to process states from Home Assistant."); sendErr != nil {
				log.Printf("Error sending unmarshal error message: %v", sendErr)
			}
			return
		}

		var sb strings.Builder
		var doorSensors []hass.State

		// Separate door sensors from other binary sensors
		for _, state := range states {
			// grab sensors matching prefix but not the opening ones
			if strings.HasPrefix(state.EntityID, s.SensorPrefix) && !strings.HasSuffix(state.EntityID, "_opening") {
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
				sb.WriteString(fmt.Sprintf("• `%s`: %s\n", strings.TrimPrefix(state.EntityID, s.SensorPrefix), status))
			}
		} else {
			sb.WriteString("🚪 **Door Sensors:** None found\n")
		}

		// Add summary
		sb.WriteString(fmt.Sprintf("\n📊 **Summary:** %d door sensor(s)", len(doorSensors)))

		if _, err := b.ChannelMessageSend(m.ChannelID, sb.String()); err != nil {
			log.Printf("Error sending state response: %v", err)
		}

	case <-time.After(5 * time.Second):
		s.HassClient.RemovePending(id) // Clean up pending channel on timeout
		if _, err := b.ChannelMessageSend(m.ChannelID, "⏰ **Timeout:** Request timed out waiting for Home Assistant states."); err != nil {
			log.Printf("Error sending timeout message: %v", err)
		}
	}
}
