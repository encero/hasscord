# Hasscord

A Discord bot built with Go.

## Configuration

The bot is configured using environment variables. You can create a `.env` file in the root of the project for local development.

- `DISCORD_TOKEN`: Your Discord bot token.
- `BOT_PREFIX`: The prefix for bot commands (defaults to `!`).

## Usage

### Running Locally

1.  Install Go: [https://golang.org/doc/install](https://golang.org/doc/install)
2.  Clone the repository.
3.  Create a `.env` file and add your Discord token.
4.  Run the bot:

    ```bash
    go run main.go
    ```

### Docker

1.  Build the Docker image:

    ```bash
    docker build -t hasscord .
    ```

2.  Run the Docker container:

    ```bash
    docker run -it --env-file .env hasscord
    ```

## Adding Commands

1.  Create a new file in the `commands` directory (e.g., `commands/hello.go`).
2.  Implement the `bot.Command` interface.
3.  Register the new command in `main.go`.
