# Discord OAuth Authentication Design

**Issue:** #285
**Date:** 2025-12-13
**Status:** Ready for implementation

## Problem

Currently, `player_id` is passed as a field in request messages with no server-side validation. Any client could claim to be any player.

## Solution

Implement Discord token validation via gRPC interceptor in rpg-api.

## Architecture

```
Client Request
     │ (authorization: Discord <token>)
     ▼
┌─────────────────┐
│ gRPC Interceptor│
│  - Extract token│
│  - Check cache  │──hit──▶ Get userID from cache
│  - Call Discord │◀─miss─┐
│  - Cache result │───────┘
│  - Inject ctx   │
└────────┬────────┘
         │ (userID in context)
         ▼
    Handler
```

### Components

**1. Discord Client (`internal/auth/discord.go`)**
- Validates Discord access tokens via `GET https://discord.com/api/users/@me`
- Returns Discord user ID on success
- 5 second HTTP timeout

**2. Token Cache (`internal/auth/cache.go`)**
- In-memory cache: `token -> userID`
- 5-minute TTL per entry
- Thread-safe (sync.RWMutex)
- Lazy cleanup of expired entries on Get()

**3. gRPC Interceptor (`internal/auth/interceptor.go`)**
- Extracts token from `authorization` metadata
- Format: `Discord <token>`
- Cache hit: use cached userID
- Cache miss: validate with Discord, cache result
- Injects userID into request context
- Returns `codes.Unauthenticated` on failure

**4. Context Helpers (`internal/auth/context.go`)**
```go
func GetPlayerID(ctx context.Context) string
func withPlayerID(ctx context.Context, id string) context.Context
```

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Where auth lives | gRPC interceptor in rpg-api | Simple, all in one place, easy to test |
| Token validation | Discord API with caching | Discord is source of truth |
| Cache TTL | 5 minutes | Balance of performance and freshness |
| Public endpoints | None - all require auth | Discord Activity = always authenticated |
| Header format | `authorization: Discord <token>` | Standard header, explicit scheme |

## Implementation Details

### Discord Client

```go
type DiscordClient struct {
    httpClient *http.Client
}

type DiscordUser struct {
    ID       string `json:"id"`
    Username string `json:"username"`
}

func (c *DiscordClient) GetCurrentUser(token string) (*DiscordUser, error)
```

Errors:
- `ErrInvalidToken` - 401 from Discord
- `ErrDiscordUnavailable` - 5xx or network error

### Cache

```go
type TokenCache struct {
    mu      sync.RWMutex
    entries map[string]*cacheEntry
    ttl     time.Duration
}

type cacheEntry struct {
    userID    string
    expiresAt time.Time
}

func (c *TokenCache) Get(token string) (string, bool)
func (c *TokenCache) Set(token, userID string)
```

### Interceptors

```go
func UnaryAuthInterceptor(discord *DiscordClient, cache *TokenCache) grpc.UnaryServerInterceptor
func StreamAuthInterceptor(discord *DiscordClient, cache *TokenCache) grpc.StreamServerInterceptor
```

## File Structure

```
internal/auth/
├── discord.go           # Discord API client
├── discord_test.go
├── cache.go             # Token cache
├── cache_test.go
├── interceptor.go       # gRPC auth interceptors
├── interceptor_test.go
├── context.go           # Context helpers
├── context_test.go
└── errors.go            # Sentinel errors
```

## Changes to Existing Code

### Server Setup (`cmd/server/server.go`)

```go
discordClient := auth.NewDiscordClient()
tokenCache := auth.NewTokenCache(5 * time.Minute)

srv := grpc.NewServer(
    grpc.ChainUnaryInterceptor(
        auth.UnaryAuthInterceptor(discordClient, tokenCache),
        grpc_logging.UnaryServerInterceptor(...),
        grpc_recovery.UnaryServerInterceptor(),
    ),
    grpc.ChainStreamInterceptor(
        auth.StreamAuthInterceptor(discordClient, tokenCache),
        ...
    ),
)
```

### Handlers

Before:
```go
playerID := req.GetPlayerId()  // Trust client
```

After:
```go
playerID := auth.GetPlayerID(ctx)  // From validated token
```

### React Client (`rpg-dnd5e-web`)

**DiscordProvider.tsx** - expose accessToken in context

**client.ts** - add auth interceptor:
```typescript
const authInterceptor: Interceptor = (next) => async (req) => {
    const token = getDiscordToken();
    req.header.set('authorization', `Discord ${token}`);
    return next(req);
};
```

## Testing Strategy

**Unit tests** - mock dependencies, test in isolation:
- Discord client: use `httptest.Server`
- Cache: test TTL, concurrency
- Interceptor: mock Discord client interface

**Handler tests** - inject playerID directly into context (don't go through interceptor)

**Integration tests** - few happy-path tests through full chain

## Implementation Order

1. `internal/auth/errors.go` - sentinel errors
2. `internal/auth/context.go` - context helpers
3. `internal/auth/discord.go` + tests - Discord client
4. `internal/auth/cache.go` + tests - token cache
5. `internal/auth/interceptor.go` + tests - gRPC interceptors
6. `cmd/server/server.go` - wire up interceptors
7. Update handlers to use `auth.GetPlayerID(ctx)`
8. React client changes (separate PR to rpg-dnd5e-web)

## Blocked

- PR #284 (Lobby Flow) - waiting on this auth work
