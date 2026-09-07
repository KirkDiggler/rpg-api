---
name: auth
description: Discord token validation, caching, and gRPC interceptors for player identity
updated: 2026-05-02
confidence: high — verified by reading interceptor.go, discord.go, cache.go, context.go
---

# auth

The auth package handles player identity for all gRPC endpoints. It validates Discord OAuth2 tokens, caches results in memory, injects `playerID` into the request context, and provides unary + streaming gRPC interceptors.

## Files

| File | Purpose |
|---|---|
| `auth/interceptor.go` | Unary + stream gRPC interceptors |
| `auth/discord.go` | `TokenValidator` interface + Discord API client |
| `auth/cache.go` | In-memory LRU token cache |
| `auth/context.go` | Context helpers: `WithPlayerID`, `PlayerIDFromContext` |
| `auth/errors.go` | Sentinel errors: `ErrMissingToken`, `ErrInvalidToken`, etc. |

## Auth schemes

Two schemes are supported (checked via `Authorization` header prefix):

| Scheme | Format | Validation |
|---|---|---|
| `Discord <token>` | Real Discord JWT | Validated with Discord API; result cached |
| `Dev <player_id>` | Bare player ID | Passed directly; only allowed if `DevMode=true` |

Dev mode is used by the integration test harness (`harness.go`) and local development. It is explicitly never production-safe (documented in `InterceptorConfig.DevMode`).

`CompositionService` adds a second use of this same deployment boundary: the entire
service is registered only when `AUTH_DEV_MODE=true`. Its local WorldID defaults to
`test-world` and can be overridden with `RPG_DEV_WORLD_ID`; that selector is accepted
only after `auth.GetPlayerID` succeeds and is not treated as authorization proof.
Non-dev servers do not register the service even if `RPG_DEV_WORLD_ID` is set. A future
production handler must replace the stub with verified Discord guild-to-world context.

## Skip-auth methods

Health check and gRPC reflection endpoints bypass auth:
- `/grpc.health.v1.Health/Check`, `/grpc.health.v1.Health/Watch`
- `/grpc.reflection.v1alpha.ServerReflection/ServerReflectionInfo`
- `/grpc.reflection.v1.ServerReflection/ServerReflectionInfo`

## Context pattern

After validation, `playerID` is injected into the context:
```go
ctx = auth.WithPlayerID(ctx, userID)
// Later in handlers:
playerID := auth.PlayerIDFromContext(ctx)
```

Handlers call `auth.PlayerIDFromContext` and pass `playerID` in their orchestrator input structs. The orchestrator does not access the context for auth information — it receives the ID explicitly.

## Token cache

`cache.go` implements an in-memory token → userID cache. Tokens are cached after successful Discord validation to avoid a Discord API call on every request. Cache entries expire based on TTL. No Redis-backed token cache — the cache is per-process.

**Gap:** The cache is lost on restart, so every Discord token requires re-validation after a server restart. For a production workload with concurrent users, this creates a burst of Discord API calls on startup. Not a correctness issue, but an operational consideration.

## Known issues

None significant. The auth component is well-scoped with clear responsibilities. The Dev mode boundary is documented. No known security issues with the implementation pattern.
