# World configuration

The API owns one World repository keyed by WorldID (Discord GuildID). The Redis
adapter stores an `entities.World` JSON record under `world:v1:<WorldID>` without
an expiration. It contains the admin, builder, and player role IDs; Discord
credentials are never persisted in this record.

## Request flow

Identity authentication -> administrative world interceptor -> WorldService
handler -> world orchestrator -> World repository.

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

## Development and current limits

`RPG_DEV_WORLD_OWNER=true` supplies an explicit development owner only when
`AUTH_DEV_MODE=true` and the request uses the Dev scheme. Discord credentials
always take the real provider-verification path. For WorldService local setup,
`RPG_DEV_WORLD_ID` must be a canonical nonzero numeric guild ID; the legacy
composition default `test-world` is not valid persisted World configuration.

This implementation does not yet role-gate existing gameplay services, refresh
live stream admission, add a web configuration screen, or isolate characters,
inventory and dungeon content between guilds. Those remain in the tracked
role-access/isolation work; WorldService alone is not proof of multi-server
readiness. The web must request the owner-verification OAuth scope before a real
owner setup can succeed with its credentials.
