package config

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config stores the application's configuration.
type Config struct {
	Token                   string
	Prefix                  string
	HassURL                 string
	HassToken               string
	ChannelID               string
	SensorOnTimeout         int    // in seconds
	SensorOnTimeoutReminder int    // in seconds
	SensorPrefix            string // entity ID prefix for door sensors (e.g., "binary_sensor.dvere_")
}

// Load loads the configuration from environment variables.
func Load() *Config {
	// Load .env file for local development.
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found, using environment variables.")
	}

	sensorOnTimeoutStr := getEnv("SENSOR_ON_TIMEOUT", "15")
	sensorOnTimeout, err := strconv.Atoi(sensorOnTimeoutStr)
	if err != nil {
		log.Printf("Invalid SENSOR_ON_TIMEOUT value '%s', using default of 15 seconds.", sensorOnTimeoutStr)
		sensorOnTimeout = 15
	}
	sensorOnTimeoutReminderStr := getEnv("SENSOR_ON_TIMEOUT_REMINDER", "60")
	sensorOnTimeoutReminder, err := strconv.Atoi(sensorOnTimeoutReminderStr)
	if err != nil {
		log.Printf("Invalid SENSOR_ON_TIMEOUT_REMINDER value '%s', using default of 60 seconds.", sensorOnTimeoutReminderStr)
		sensorOnTimeoutReminder = 60
	}

	return &Config{
		Token:                   getEnv("DISCORD_TOKEN", ""),
		Prefix:                  getEnv("BOT_PREFIX", "!"),
		HassURL:                 getEnv("HASS_URL", ""),
		HassToken:               getEnv("HASS_TOKEN", ""),
		ChannelID:               getEnv("CHANNEL_ID", ""),
		SensorOnTimeout:         sensorOnTimeout,
		SensorOnTimeoutReminder: sensorOnTimeoutReminder,
		SensorPrefix:            getEnv("SENSOR_PREFIX", "binary_sensor.dvere_"),
	}
}

// getEnv gets an environment variable or returns a default value.
func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

// Validate checks that all required configuration values are present.
func (c *Config) Validate() error {
	var missing []string

	if c.Token == "" {
		missing = append(missing, "DISCORD_TOKEN")
	}
	if c.HassURL == "" {
		missing = append(missing, "HASS_URL")
	}
	if c.HassToken == "" {
		missing = append(missing, "HASS_TOKEN")
	}
	if c.ChannelID == "" {
		missing = append(missing, "CHANNEL_ID")
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing required environment variables: %v", missing)
	}

	return nil
}
