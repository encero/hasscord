package main

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"hasscord/bot"
	"hasscord/commands"
	"hasscord/config"
	"hasscord/hass"
)

// Global map to track sensors that are "on" and their state information
var (
	onSensors      = make(map[string]SensorState)
	onSensorsMutex sync.Mutex
)

// SensorState holds information about a sensor that is currently "on".
type SensorState struct {
	OnTime   time.Time
	LastSent time.Time
}

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

	go hassClient.Listen()

	events, err := hassClient.SubscribeToEvents()
	if err != nil {
		log.Fatalf("Error subscribing to Home Assistant events: %v", err)
	}

	go handleHassEvents(b, events, cfg.ChannelID)
	go checkOnSensors(b, cfg.ChannelID, cfg.SensorOnTimeout)

	b.Start()
}

func handleHassEvents(b *bot.Bot, events <-chan hass.Event, channelID string) {
	for event := range events {
		if event.EventType == "state_changed" {
			var stateData hass.StateChangedData
			err := json.Unmarshal(event.Data, &stateData)
			if err != nil {
				log.Printf("Error unmarshaling state change data: %v", err)
				continue
			}

			// We only care about sensors
			if !strings.HasPrefix(stateData.EntityID, "binary_sensor.dvere_") {
				continue
			}

			onSensorsMutex.Lock()
			if stateData.NewState.State == "on" {
				if _, exists := onSensors[stateData.EntityID]; !exists {
					onSensors[stateData.EntityID] = SensorState{OnTime: time.Now(), LastSent: time.Time{}}
					log.Printf("Sensor %s turned on at %s", stateData.EntityID, onSensors[stateData.EntityID].OnTime.Format(time.RFC3339))
				}
			} else {
				if state, exists := onSensors[stateData.EntityID]; exists {
					if !state.LastSent.IsZero() {
						message := fmt.Sprintf("Door `%s` is now closed.", strings.TrimPrefix(stateData.EntityID, "binary_sensor."))
						b.Session.ChannelMessageSend(channelID, message)
					}
					delete(onSensors, stateData.EntityID)
					log.Printf("Sensor %s turned off or changed state to %s", stateData.EntityID, stateData.NewState.State)
				}
			}
			onSensorsMutex.Unlock()
		}
	}
}

func checkOnSensors(b *bot.Bot, channelID string, timeout int) {
	ticker := time.NewTicker(5 * time.Second) // Check every 5 seconds
	defer ticker.Stop()

	const reminderTime = 1 * time.Minute
	const maxRemindDuration = 1 * time.Hour

	for range ticker.C {
		onSensorsMutex.Lock()
		for entityID, state := range onSensors {
			durationOn := time.Since(state.OnTime)
			initialTimeoutDuration := time.Duration(timeout) * time.Second

			isInitialNotification := state.LastSent.IsZero()
			shouldSendInitial := isInitialNotification && durationOn >= initialTimeoutDuration

			hasBeenNotified := !state.LastSent.IsZero()
			isUnderAnHour := durationOn < maxRemindDuration
			timeForReminder := time.Since(state.LastSent) >= reminderTime
			shouldSendReminder := hasBeenNotified && isUnderAnHour && timeForReminder

			isOverAnHour := durationOn >= maxRemindDuration

			// Check for initial timeout
			if shouldSendInitial {
				message := fmt.Sprintf("Door `%s` has been open for more than %d seconds! @everyone", strings.TrimPrefix(entityID, "binary_sensor."), timeout)
				b.Session.ChannelMessageSend(channelID, message)
				state.LastSent = time.Now()
				onSensors[entityID] = state // Update the map with the new LastSent time
				log.Printf("Sent initial message for %s", entityID)
			} else if shouldSendReminder {
				// Resend message every 5 minutes, up to an hour
				message := fmt.Sprintf("Reminder: Door `%s` is still open (open for %s)! @everyone", strings.TrimPrefix(entityID, "binary_sensor."), durationOn.Round(time.Second).String())
				b.Session.ChannelMessageSend(channelID, message)
				state.LastSent = time.Now()
				onSensors[entityID] = state // Update the map with the new LastSent time
				log.Printf("Sent reminder message for %s", entityID)
			} else if isOverAnHour {
				// Remove after one hour
				message := fmt.Sprintf("Door `%s` has been open for over an hour. Stopping reminders.", strings.TrimPrefix(entityID, "binary_sensor."))
				b.Session.ChannelMessageSend(channelID, message)
				delete(onSensors, entityID)
				log.Printf("Removed %s from tracking after 1 hour", entityID)
			}
		}
		onSensorsMutex.Unlock()
	}
}
