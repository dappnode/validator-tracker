FROM golang:1.24-alpine AS builder

WORKDIR /app

# Install git for Go modules
RUN apk update && apk add --no-cache git

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the entire codebase
COPY . .

# Build the binary
RUN go build -o validator-tracker ./cmd/main.go

# Final image
FROM alpine:3.21

WORKDIR /app

# Copy built binary
COPY --from=builder /app/validator-tracker .

# Run the app
CMD ["./validator-tracker"]
