#!/bin/bash

# This script compiles and runs the Hasscord bot.

# Load environment variables from .env file
if [ -f .env ]; then
  export $(cat .env | sed 's/#.*//g' | xargs)
fi

# Check if DISCORD_TOKEN is set
if [ -z "${DISCORD_TOKEN}" ]; then
  echo "Error: DISCORD_TOKEN is not set. Please create a .env file and add your Discord token." >&2
  exit 1
fi

# Compile the application
go build -o hasscord main.go

# Run the bot
./hasscord
