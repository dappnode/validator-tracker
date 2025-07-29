FROM --platform=$BUILDPLATFORM golang:1.24-alpine AS builder

ARG TARGETOS
ARG TARGETARCH
WORKDIR /app

# Install git for Go modules
RUN apk update && apk add --no-cache git

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy the entire codebase
COPY . .

# Build a static binary for the target platform
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o validator-tracker ./cmd/main.go

# Final image
FROM alpine:3.21

WORKDIR /app

# Copy built binary
COPY --from=builder /app/validator-tracker .

# Run the app
CMD ["./validator-tracker"]
