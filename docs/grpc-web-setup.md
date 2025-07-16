# gRPC-Web Setup

This document explains how we expose our gRPC services to web clients using Envoy proxy.

## Architecture

```
TypeScript Client (gRPC-Web) 
    ↓ HTTP/8080
Envoy Proxy (translates gRPC-Web ↔ gRPC)
    ↓ gRPC/50051
Go gRPC Server (pure gRPC)
    ↓
Redis
```

## Local Development

```bash
# Start all services
docker-compose up

# Services available at:
# - gRPC: localhost:50051 (for Go clients)
# - HTTP: localhost:8080 (for TypeScript/web clients)
# - Envoy Admin: localhost:9901
```

## Why Envoy?

1. **Clean Architecture**: Our Go server remains pure gRPC
2. **No Code Changes**: Web support without modifying server code
3. **Production Ready**: Same setup works in production
4. **Additional Features**: Free observability, rate limiting, auth can be added at proxy layer

## Configuration

The Envoy configuration (`envoy/envoy.yaml`) is mostly static. It only needs updates when:
- Adding a new gRPC service
- Changing service ports
- Modifying CORS settings

## TypeScript Client Usage

```typescript
import { createGrpcWebTransport } from "@connectrpc/connect-web";
import { CharacterServiceClient } from "./generated/dnd5e/api/v1alpha1/character_connect";

const transport = createGrpcWebTransport({
  baseUrl: "http://localhost:8080", // Envoy proxy
});

const client = new CharacterServiceClient(transport);
```