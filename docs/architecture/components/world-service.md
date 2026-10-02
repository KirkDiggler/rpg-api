# World configuration

The API owns one World repository keyed by WorldID (Discord GuildID). The Redis
adapter stores an `entities.World` JSON record under `world:v1:<WorldID>` without
an expiration. It contains the admin, builder, and player role IDs; Discord
credentials are never persisted in this record.

## Request flow

Identity authentication -> administrative world interceptor -> WorldService
handler -> world orchestrator -> World repository.

The handler declares the narrow orchestrator interface it consumes and uses
operation Input/Output types owned by `internal/orchestrators/world`.
`entities.World` is the shared data shape used by the handler conversion,
orchestrator and repository. There is no separate `services/world` package or
second business-layer contract package.

The administrative interceptor verifies same-token guild membership and actual
Discord ownership. It uses the OAuth `guilds` scope with
`GET /users/@me/guilds`, paginating by `after`; membership uses
`guilds.members.read` with the existing current-member endpoint. Missing scopes,
provider failures and invalid responses fail closed. World request IDs must
match the independently verified guild selector.

| RPC | Permission | Persistence |
|---|---|---|
| GetWorld | Verified server owner or configured world admin | Read existing configuration; absent world is NotFound |
| SetWorldRoles | Verified server owner only | Create or replace all three role assignments |
| SetWorldMemberRoles | Verified server owner or configured world admin | Update builder/player roles only |

Owner setup is independent of gameplay admission and works before the world's
roles are configured. A game admin role does not prove Discord server ownership.
For delegated changes, authorization uses the current stored admin role, not a
cached configuration grant. Redis WATCH/MULTI preserves the stored admin role
and aborts a conflicting update so an owner revocation cannot be overwritten by
a stale delegated write. Aborted writes require reauthorization on retry.

The membership cache stores detached copies of verified role-ID snapshots for
at most 30 seconds. It does not cache configuration-derived permissions.
WorldService management requests bypass that cache: Discord-side removal of a
world admin role is checked before every administrative read or mutation.

## Development and current limits

`RPG_DEV_WORLD_OWNER=true` supplies an explicit development owner only when
`AUTH_DEV_MODE=true`, the request uses the Dev scheme, and its player ID matches
`RPG_DEV_WORLD_OWNER_PLAYER_ID`. Discord credentials
always take the real provider-verification path. For WorldService local setup,
`RPG_DEV_WORLD_ID` must be a canonical nonzero numeric guild ID; the legacy
composition default `test-world` is not valid persisted World configuration.

An optional `RPG_DEV_WORLD_IDS` allowlist lets a Dev request manage a selected
world through `x-rpg-guild-id`; the resolver applies it before any handler, and
the selected world must be listed and must be canonical. Production ignores the
list, and a list that is malformed, duplicated, or missing the configured
default world stops server startup. See the [auth component](auth.md).

RoleAccess now gates explicitly classified gameplay unary and stream calls;
stream permission refresh also covers idle subscriptions. The paired web slice
requests the ownership scope and adds the configuration screen. Development role
fixtures are server-controlled and ignored in production; they demonstrate local
UI/gate behavior, not live Discord verification.

This implementation does not isolate characters, inventory or dungeon content
between guilds. WorldService and role admission are not proof of multi-server
readiness. Real Discord owner/membership verification still needs the operator's
walk with newly consented credentials.
