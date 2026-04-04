package sensors

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
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
	OnTime               time.Time
	LastSent             time.Time
	CurrentReminderDelay time.Duration
	Paused               bool
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
func HandleHassEvents(ctx context.Context, b *bot.Bot, events <-chan hass.Event, channelID string, sensorPrefix string) {
	for {
		select {
		case <-ctx.Done():
			log.Println("Event handler shutting down...")
			return
		case event, ok := <-events:
			if !ok {
				log.Println("Event channel closed, stopping handler")
				return
			}
			handleEvent(b, event, channelID, sensorPrefix)
		}
	}
}

func handleEvent(b *bot.Bot, event hass.Event, channelID string, sensorPrefix string) {
	if event.EventType != "state_changed" {
		return
	}

	var stateData hass.StateChangedData
	err := json.Unmarshal(event.Data, &stateData)
	if err != nil {
		log.Printf("Error unmarshaling state change data: %v", err)
		return
	}

	// We only care about sensors matching the configured prefix
	if !strings.HasPrefix(stateData.EntityID, sensorPrefix) {
		return
	}

	onSensorsMutex.Lock()
	defer onSensorsMutex.Unlock()

	if stateData.NewState.State == "on" {
		if _, exists := onSensors[stateData.EntityID]; !exists {
			onSensors[stateData.EntityID] = SensorState{
				OnTime:               time.Now(),
				LastSent:             time.Time{},
				CurrentReminderDelay: 0,
				Paused:               false,
			}
			log.Printf("Sensor %s turned on at %s", stateData.EntityID, onSensors[stateData.EntityID].OnTime.Format(time.RFC3339))
		}
	} else {
		if state, exists := onSensors[stateData.EntityID]; exists {
			if !state.LastSent.IsZero() {
				message := fmt.Sprintf("Door `%s` is now closed.", strings.TrimPrefix(stateData.EntityID, "binary_sensor."))
				if _, err := b.Session.ChannelMessageSend(channelID, message); err != nil {
					log.Printf("Error sending door closed message: %v", err)
				}
			}
			delete(onSensors, stateData.EntityID)
			log.Printf("Sensor %s turned off or changed state to %s", stateData.EntityID, stateData.NewState.State)
		}
	}
}

// CheckOnSensors monitors sensors that are "on" and sends notifications based on timeouts
func CheckOnSensors(ctx context.Context, b *bot.Bot, channelID string, timeout int, timeoutReminder int, timeoutReminderBackoff float64, timeoutReminderMax int, sensorPrefix string) {
	ticker := time.NewTicker(5 * time.Second) // Check every 5 seconds
	defer ticker.Stop()

	reminderTime := time.Duration(timeoutReminder) * time.Second
	maxReminderTime := time.Duration(timeoutReminderMax) * time.Second
	const maxRemindDuration = 1 * time.Hour

	for {
		select {
		case <-ctx.Done():
			log.Println("Sensor checker shutting down...")
			return
		case <-ticker.C:
		}
		onSensorsMutex.Lock()
		for entityID, state := range onSensors {
			durationOn := time.Since(state.OnTime)
			initialTimeoutDuration := time.Duration(timeout) * time.Second

			isInitialNotification := state.LastSent.IsZero()
			shouldSendInitial := isInitialNotification && durationOn >= initialTimeoutDuration

			hasBeenNotified := !state.LastSent.IsZero()
			isUnderAnHour := durationOn < maxRemindDuration
			currentReminderDelay := state.CurrentReminderDelay
			if currentReminderDelay <= 0 {
				currentReminderDelay = reminderTime
			}
			timeForReminder := time.Since(state.LastSent) >= currentReminderDelay
			shouldSendReminder := hasBeenNotified && isUnderAnHour && timeForReminder

			isOverAnHour := durationOn >= maxRemindDuration

			// Check for initial timeout
			if shouldSendInitial && !state.Paused {
				message := fmt.Sprintf("Door `%s` has been open for more than %d seconds! @everyone", strings.TrimPrefix(entityID, sensorPrefix), timeout)
				if _, err := b.Session.ChannelMessageSend(channelID, message); err != nil {
					log.Printf("Error sending initial door open message: %v", err)
				}
				state.LastSent = time.Now()
				state.CurrentReminderDelay = reminderTime
				onSensors[entityID] = state // Update the map with the new LastSent time
				log.Printf("Sent initial message for %s", entityID)
			} else if shouldSendReminder && !state.Paused {
				// Resend reminders with a configurable backoff, up to an hour of total tracking.
				message := fmt.Sprintf("Reminder: Door `%s` is still open (open for %s)! @everyone", strings.TrimPrefix(entityID, sensorPrefix), durationOn.Round(time.Second).String())
				if _, err := b.Session.ChannelMessageSend(channelID, message); err != nil {
					log.Printf("Error sending reminder message: %v", err)
				}
				state.LastSent = time.Now()
				state.CurrentReminderDelay = nextReminderDelay(currentReminderDelay, timeoutReminderBackoff, maxReminderTime)
				onSensors[entityID] = state // Update the map with the new LastSent time
				log.Printf("Sent reminder message for %s", entityID)
			} else if isOverAnHour {
				// Remove after one hour (always remove, regardless of pause state)
				if !state.Paused {
					message := fmt.Sprintf("Door `%s` has been open for over an hour. Stopping reminders.", strings.TrimPrefix(entityID, sensorPrefix))
					if _, err := b.Session.ChannelMessageSend(channelID, message); err != nil {
						log.Printf("Error sending hour limit message: %v", err)
					}
				}
				delete(onSensors, entityID)
				log.Printf("Removed %s from tracking after 1 hour", entityID)
			}
		}
		onSensorsMutex.Unlock()
	}
}

func nextReminderDelay(current time.Duration, backoff float64, max time.Duration) time.Duration {
	if current <= 0 {
		return max
	}
	if backoff <= 1 {
		return current
	}

	next := time.Duration(math.Ceil(float64(current) * backoff))
	if next < current {
		next = current
	}
	if max > 0 && next > max {
		return max
	}

	return next
}
