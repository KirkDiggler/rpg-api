---
name: auth
description: Discord identity plus method-scoped trusted guild world context
updated: 2026-09-08
confidence: high — verified by focused interceptor, provider, cache, handler, and race tests
---

# auth

The auth package establishes player identity for authenticated gRPC endpoints. A
second unary interceptor derives trusted world context only for the four
`CompositionService` methods. Global player and character APIs remain unguilded,
and stream authentication has not acquired a world requirement.

## Files

| File | Purpose |
|---|---|
| `auth/interceptor.go` | Global unary and stream player authentication |
| `auth/world_interceptor.go` | Exact Composition unary allowlist and guild selector validation |
| `auth/world_resolver.go` | Dev-world selection or same-token Discord membership verification |
| `auth/discord.go` | Discord current-user and current-user-guild-member client |
| `auth/cache.go` | Existing token-to-player identity cache |
| `auth/membership_cache.go` | Bounded positive membership cache keyed by token digest and GuildID |
| `auth/context.go` | Player context plus auth-private same-request credential carrier |
| `worldcontext/context.go` | Handler-visible trusted `WorldID` only |

## Auth schemes

| Scheme | Validation | Composition world |
|---|---|---|
| `Discord <token>` | `/api/users/@me`; identity may be cached | Exactly one canonical `x-rpg-guild-id`, verified with `/api/users/@me/guilds/{guild_id}/member` using the same request token |
| `Dev <player_id>` | Accepted only with `AUTH_DEV_MODE=true` | `RPG_DEV_WORLD_ID`, defaulting to `test-world`; the untrusted guild selector is ignored |

Production does not accept `Dev` auth and never uses the development world. A
Discord request on a dev-enabled server still follows Discord membership
verification rather than inheriting the configured Dev world.

## Trusted composition boundary

`UnaryWorldContextInterceptor` applies only to Create, Get, List, and Delete on
`api.composition.v1alpha1.CompositionService`. Discord selectors must be one
non-zero canonical decimal `uint64`; missing selectors are `FailedPrecondition`,
and empty, repeated, combined, signed, whitespace-bearing, leading-zero,
non-decimal, or overflowing values are `InvalidArgument`.

A successful current-member response must name the same user as the globally
authenticated player. The verified GuildID is used directly as the toolkit-domain
WorldID. The interceptor removes its private auth credential before invoking any
handler and exposes only `worldcontext.Value{WorldID: ...}`. Handlers therefore
cannot read the token or use a request body as authority.

Discord `401` maps to `Unauthenticated` and removes the existing identity entry
plus every membership entry for that token. `403`/`404` map to
`PermissionDenied`. Timeouts, rate limits, server errors, malformed responses,
and missing or mismatched member users map to `Unavailable`; failures are never
cached.

## Caches and deferred hardening

The existing identity cache remains a five-minute, process-local raw-token map.
This slice adds only the `Delete` operation required after an observed membership
`401`; its raw-key and bounds redesign remains deferred to rpg-api#937.

The new cache stores only successful `(SHA-256 token digest, GuildID)` decisions,
for at most 30 seconds and at most 1,024 entries. It never stores raw tokens,
denials, expired decisions, or stale-on-provider-error results. Eviction is
oldest-expiry with deterministic insertion-order tie breaking. Browser sign-out
has no server eviction signal, so a prior positive decision may remain valid only
until that bounded TTL expires.

OAuth transaction/session binding is separate deferred work in rpg-project#403.
This API boundary does not add a world registry, roles, owner/admin checks,
character travel, or guild requirements to other RPCs.
