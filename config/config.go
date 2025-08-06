package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

// Config stores the application's configuration.
type Config struct {
	Token  string
	Prefix string
}

// Load loads the configuration from environment variables.
func Load() *Config {
	// Load .env file for local development.
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found, using environment variables.")
	}

	return &Config{
		Token:  getEnv("DISCORD_TOKEN", ""),
		Prefix: getEnv("BOT_PREFIX", "!"),
	}
}

// getEnv gets an environment variable or returns a default value.
func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}
