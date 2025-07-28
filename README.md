# Validator Tracker Notifier

## Development Mode (Dev Mode)

This project supports a development mode with automatic reload on code changes using [air](https://github.com/cosmtrek/air).

### Prerequisites

- [Go](https://golang.org/dl/) installed
- [air](https://github.com/cosmtrek/air) installed (install with `go install github.com/cosmtrek/air@latest`)

### Running in Dev Mode

To start the application in development mode with live reload and debug logging:

```sh
LOG_LEVEL=DEBUG air
```

This will watch for file changes and automatically restart the app. You can edit code and see changes reflected immediately.

### Example

```sh
# Install air if you haven't already
go install github.com/air-verse/air@latest

# Run in dev mode with debug logs
LOG_LEVEL=DEBUG air
```
