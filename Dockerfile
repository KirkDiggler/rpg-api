# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -installsuffix cgo -o server ./cmd/server

# Final stage
FROM alpine:3.20

# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates

# Create a non-root user and group
RUN adduser -D -g '' appuser

# Set the working directory
WORKDIR /home/appuser

# Copy the binary from builder stage and adjust ownership
COPY --from=builder /app/server .
RUN chown appuser:appuser /home/appuser/server

# Shipped dungeon content (rpg-api#806). The registry loads every *.yaml
# under RPG_CONTENT_DIR at boot and refuses to start on one that does not
# compile; with the variable unset it reads ./content, which is this copy.
#
# To author against a persistent directory, point RPG_CONTENT_DIR at a
# mounted volume (e.g. /content). If it lacks reference-tomb.yaml the server
# seeds it from the IMMUTABLE copy at /usr/share/rpg-api/content -- kept
# outside the working directory on purpose, so a volume mounted over
# /home/appuser/content can never hide the seed (rpg-project#256).
COPY --from=builder --chown=appuser:appuser /app/content ./content
COPY --from=builder /app/content /usr/share/rpg-api/content
ENV RPG_SHIPPED_CONTENT_DIR=/usr/share/rpg-api/content

# Switch to the non-root user
USER appuser

# Expose the gRPC port
EXPOSE 50051

# Run the server
CMD ["./server", "server"]