package config

import (
	"log"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

// Config stores the application's configuration.
type Config struct {
	Token         string
	Prefix        string
	HassURL       string
	HassToken     string
	ChannelID     string
	SensorOnTimeout int // in seconds
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

	return &Config{
		Token:     getEnv("DISCORD_TOKEN", ""),
		Prefix:    getEnv("BOT_PREFIX", "!"),
		HassURL:   getEnv("HASS_URL", ""),
		HassToken: getEnv("HASS_TOKEN", ""),
		ChannelID: getEnv("CHANNEL_ID", ""),
		SensorOnTimeout: sensorOnTimeout,
	}
}

// getEnv gets an environment variable or returns a default value.
func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}
