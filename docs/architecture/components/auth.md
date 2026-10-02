---
name: auth
description: Discord identity plus method-scoped trusted guild world context
updated: 2026-10-02
confidence: high — verified by focused interceptor, provider, cache, handler, and race tests
---

# auth

The auth package establishes player identity for authenticated gRPC endpoints. A
world-access boundary derives trusted guild context and checks configured game
roles for explicitly classified gameplay RPCs, including character APIs and
streams. WorldService owner/bootstrap authorization is separate so an owner
can configure access before any gameplay role exists.

## Files

| File | Purpose |
|---|---|
| `auth/interceptor.go` | Global unary and stream player authentication |
| `auth/world_interceptor.go` | Shared guild selector validation; legacy composition-only interceptor tests |
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
| `Dev <player_id>` | Accepted only with `AUTH_DEV_MODE=true` | `RPG_DEV_WORLD_ID`, defaulting to `test-world`; the untrusted guild selector is ignored unless `RPG_DEV_WORLD_IDS` opts in |

Production does not accept `Dev` auth and never uses the development world. A
Discord request on a dev-enabled server still follows Discord membership
verification rather than inheriting the configured Dev world.

## Development world selector

`RPG_DEV_WORLD_IDS` is an optional comma-separated allowlist of canonical
non-zero decimal `uint64` guild-shaped IDs. It is honored only when
`AUTH_DEV_MODE=true` and the request uses authenticated `Dev` credentials; a
Discord credential always verifies real membership, and setting the list alone
never enables Dev auth in production. `NewWorldResolver` refuses an entry that
is empty, non-canonical, or duplicated, and refuses a list that omits the
configured `RPG_DEV_WORLD_ID` default, so malformed configuration stops server
construction instead of silently broadening access.

With an allowlist configured, a Dev request may select an allowed world through
the existing `x-rpg-guild-id` header; the resolver parses it inside the
authenticated Dev branch, before any handler runs. An absent selector uses the
configured default; an unknown selector is `PermissionDenied`; a repeated or
non-canonical selector is `InvalidArgument`. Without an allowlist the fixed
`RPG_DEV_WORLD_ID` behavior is preserved and every incoming selector is ignored
as untrusted.

## Trusted composition boundary

The server uses RoleAccess for all classified game unary and stream RPCs.
Composition writes require build; rendering reads require play. Character,
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

Role grants use current World repository configuration, never a cached
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
permission configuration grants nothing. Development owner authority additionally
requires RPG_DEV_WORLD_OWNER=true and a matching RPG_DEV_WORLD_OWNER_PLAYER_ID.

OAuth transaction/session binding is separate deferred work in rpg-project#403.
World access configuration is not multi-server data isolation: character travel,
server-local inventory/progression and world-scoped gameplay records remain
separate work. See [world-service](world-service.md).
