package sensors

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"hasscord/bot"
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
	Paused   bool
}

// PauseNotifications pauses notifications for currently open doors
func PauseNotifications() {
	onSensorsMutex.Lock()
	defer onSensorsMutex.Unlock()

	count := 0
	for entityID, state := range onSensors {
		if !state.Paused {
			state.Paused = true
			onSensors[entityID] = state
			count++
		}
	}

	if count > 0 {
		log.Printf("Paused notifications for %d currently open doors", count)
	} else {
		log.Printf("No open doors to pause notifications for")
	}
}

// ResumeNotifications resumes notifications for currently open doors
func ResumeNotifications() {
	onSensorsMutex.Lock()
	defer onSensorsMutex.Unlock()

	count := 0
	for entityID, state := range onSensors {
		if state.Paused {
			state.Paused = false
			onSensors[entityID] = state
			count++
		}
	}

	if count > 0 {
		log.Printf("Resumed notifications for %d currently open doors", count)
	} else {
		log.Printf("No paused doors to resume notifications for")
	}
}

// GetPauseStatus returns information about currently paused doors
func GetPauseStatus() (int, int) {
	onSensorsMutex.Lock()
	defer onSensorsMutex.Unlock()

	total := len(onSensors)
	paused := 0

	for _, state := range onSensors {
		if state.Paused {
			paused++
		}
	}

	return total, paused
}

// HandleHassEvents processes Home Assistant events and tracks sensor states
func HandleHassEvents(b *bot.Bot, events <-chan hass.Event, channelID string) {
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
					onSensors[stateData.EntityID] = SensorState{OnTime: time.Now(), LastSent: time.Time{}, Paused: false}
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

// CheckOnSensors monitors sensors that are "on" and sends notifications based on timeouts
func CheckOnSensors(b *bot.Bot, channelID string, timeout int, timeoutReminder int) {
	ticker := time.NewTicker(5 * time.Second) // Check every 5 seconds
	defer ticker.Stop()

	reminderTime := time.Duration(timeoutReminder) * time.Second
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
			if shouldSendInitial && !state.Paused {
				message := fmt.Sprintf("Door `%s` has been open for more than %d seconds! @everyone", strings.TrimPrefix(entityID, "binary_sensor."), timeout)
				b.Session.ChannelMessageSend(channelID, message)
				state.LastSent = time.Now()
				onSensors[entityID] = state // Update the map with the new LastSent time
				log.Printf("Sent initial message for %s", entityID)
			} else if shouldSendReminder && !state.Paused {
				// Resend message every 5 minutes, up to an hour
				message := fmt.Sprintf("Reminder: Door `%s` is still open (open for %s)! @everyone", strings.TrimPrefix(entityID, "binary_sensor."), durationOn.Round(time.Second).String())
				b.Session.ChannelMessageSend(channelID, message)
				state.LastSent = time.Now()
				onSensors[entityID] = state // Update the map with the new LastSent time
				log.Printf("Sent reminder message for %s", entityID)
			} else if isOverAnHour {
				// Remove after one hour (always remove, regardless of pause state)
				if !state.Paused {
					message := fmt.Sprintf("Door `%s` has been open for over an hour. Stopping reminders.", strings.TrimPrefix(entityID, "binary_sensor."))
					b.Session.ChannelMessageSend(channelID, message)
				}
				delete(onSensors, entityID)
				log.Printf("Removed %s from tracking after 1 hour", entityID)
			}
		}
		onSensorsMutex.Unlock()
	}
}
