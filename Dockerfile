# Build stage
FROM golang:1.24-alpine AS builder

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

# Switch to the non-root user
USER appuser

# Expose the gRPC port
EXPOSE 50051

# Run the server
CMD ["./server", "server"]