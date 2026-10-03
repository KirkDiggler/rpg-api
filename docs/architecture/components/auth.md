---
name: auth
description: Discord identity plus method-scoped trusted guild world context
updated: 2026-10-03
confidence: high — verified by focused interceptor, provider, cache, handler, and race tests
---

# auth

The auth package establishes player identity for authenticated gRPC endpoints.
World-role admission for gameplay is explicitly enabled by
`RPG_WORLD_ACCESS_ENFORCEMENT=true`; unset or `false` preserves the existing
identity-authenticated gameplay contract. Both modes retain the composition
world/membership boundary and separate WorldService owner/bootstrap authorization.
Enforcement is an environment readiness choice, not a side effect of release.

## Files

| File | Purpose |
|---|---|
| `auth/interceptor.go` | Global unary and stream player authentication |
| `auth/world_interceptor.go` | Shared guild selector validation; composition-only world boundary in compatibility mode |
| `cmd/server/world_access_rollout.go` | Default-off gameplay role-enforcement selection; invalid settings fail startup |
| `auth/role_policy.go` | Explicit gameplay method-to-permission policy; unknown methods fail closed |
| `auth/role_interceptor.go` | Unary/stream role admission and idle-stream permission refresh |
| `auth/world_management_interceptor.go` | WorldService membership/ownership boundary independent of game-role admission |
| `auth/discord_ownership.go` | Same-token paginated server-owner verification |
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
Discord request requiring a world on a dev-enabled server still follows Discord
membership verification rather than inheriting the configured Dev world.

## Trusted composition boundary

With enforcement enabled, the server uses RoleAccess for all classified game
unary and stream RPCs. With it disabled, ordinary gameplay does not acquire new
guild/role prerequisites; the existing composition-only world interceptor remains
in the unary chain. Stream authentication strips its private credential itself
when there is no role interceptor to consume it. No mode weakens token validation
or enables Dev credentials in production.

Under enforcement, composition writes require build; rendering reads require play. Character,
dice, lobby, session and presentation calls require play. Authoring writes and
builder catalogs require build. A coverage test requires classification of all
published methods in these services; unknown methods are refused.

Discord selectors must be one
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

The new cache stores only successful `(SHA-256 token digest, GuildID)` membership/role snapshots,
for at most 30 seconds and at most 1,024 entries. It never stores raw tokens,
denials, expired decisions, or stale-on-provider-error results. Eviction is
oldest-expiry with deterministic insertion-order tie breaking. Browser sign-out
has no server eviction signal, so a prior positive decision may remain valid only
until that bounded TTL expires.

When enforcement is enabled, role grants use current World repository configuration, never a cached
configuration-derived permission. Streams fetch a fresh role snapshot on
subscription and every 15 seconds, bypassing the positive membership cache.
Refresh calls have a 15-second deadline; permission loss/provider failure
cancels an idle subscription within at most 30 seconds and stops its worker.
Existing character/lobby/session ownership checks remain additional gates.

Development permission fixtures require AUTH_DEV_MODE and Dev credentials.
RPG_DEV_PERMISSION_LEVEL selects an explicit fallback (none/player/builder/admin);
RPG_DEV_PLAYER_PERMISSIONS provides per-player overrides. RPG_DEV_PLAYER_ROLES
provides explicit role snapshots for administrative UI tests. Production ignores
these fixtures, and Discord credentials never inherit them. Missing development
permission configuration grants nothing when enforcement is enabled. Ordinary
compatibility-mode gameplay does not consult these role fixtures. Development owner authority additionally
requires RPG_DEV_WORLD_OWNER=true and a matching RPG_DEV_WORLD_OWNER_PLAYER_ID.

OAuth transaction/session binding is separate deferred work in rpg-project#403.
World access configuration is not multi-server data isolation: character travel,
server-local inventory/progression and world-scoped gameplay records remain
separate work. See [world-service](world-service.md).
