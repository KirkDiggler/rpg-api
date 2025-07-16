#!/bin/bash
# Script to run services locally without Docker for quick development

# Colors for output
GREEN='\033[0;32m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${GREEN}Starting local development environment...${NC}"

# Check if Redis is running
if ! nc -z localhost 6379 2>/dev/null; then
    echo -e "${RED}Redis is not running on port 6379${NC}"
    echo "Please start Redis first: redis-server"
    exit 1
fi

# Start the gRPC server in background
echo -e "${GREEN}Starting gRPC server on port 50051...${NC}"
go run ./cmd/server server &
GRPC_PID=$!

# Give the server time to start
sleep 2

# Check if Envoy is installed
if ! command -v envoy &> /dev/null; then
    echo -e "${RED}Envoy is not installed${NC}"
    echo "On macOS: brew install envoy"
    echo "Or use Docker: docker run -p 8080:8080 -v $PWD/envoy/envoy.yaml:/etc/envoy/envoy.yaml envoyproxy/envoy:v1.31-latest"
    kill $GRPC_PID
    exit 1
fi

# Start Envoy
echo -e "${GREEN}Starting Envoy proxy on port 8080...${NC}"
envoy -c ./envoy/envoy.yaml &
ENVOY_PID=$!

echo -e "${GREEN}Services are running!${NC}"
echo "- gRPC server: localhost:50051"
echo "- HTTP/gRPC-Web: localhost:8080"
echo "- Envoy admin: localhost:9901"
echo ""
echo "Press Ctrl+C to stop all services"

# Function to cleanup on exit
cleanup() {
    echo -e "\n${GREEN}Stopping services...${NC}"
    kill $GRPC_PID 2>/dev/null
    kill $ENVOY_PID 2>/dev/null
    exit 0
}

# Set trap to cleanup on Ctrl+C
trap cleanup INT

# Wait for services
wait