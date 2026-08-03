---
name: update proto dependency
description: How to pull updated proto definitions into rpg-api from rpg-api-protos
updated: 2026-08-03
---

# How to update the proto dependency

rpg-api consumes compiled proto Go code from `github.com/KirkDiggler/rpg-api-protos/gen/go`. The generated code is published on the `generated` branch of rpg-api-protos.

## When to update

- A new RPC has been added to rpg-api-protos
- An existing proto message has been extended with new fields
- Proto enums have been updated

## Update command

```bash
cd /home/kirk/personal/rpg-api
GOPROXY=direct go get github.com/KirkDiggler/rpg-api-protos/gen/go@generated
go mod tidy
```

The `GOPROXY=direct` bypasses the module proxy cache to pull directly from the git repository.

### Generated-Go versioning

The consumed module is the nested `gen/go` module, not the proto repository root.
Root release `v0.1.118` publishes generated commit `d9ef7d9fb49d`; its canonical
proxy-resolvable generated-Go artifact is
`v0.0.0-20260803143754-d9ef7d9fb49d`. Keep that pseudo-version in `go.mod`;
`v0.1.118` is a root-repository tag, **not** a valid version tag for
`github.com/KirkDiggler/rpg-api-protos/gen/go`.

## Verify the update

```bash
# Check the new version hash in go.mod
grep rpg-api-protos go.mod

# Build to verify no compilation errors
go build ./...
```

## If new fields break existing code

Proto3 is backward compatible — new fields with zero values don't break readers. However, if new required proto message fields are added, or if existing handler coverage needs to include the new fields:

1. Update the handler's converter functions to map new fields
2. Add test coverage for new conversions
3. Run `make pre-commit`

## Update toolkit dependency

For rpg-toolkit updates:
```bash
GOPROXY=direct go get github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e@latest
go mod tidy
```

Check which toolkit module you need — rpg-toolkit is published as multiple modules (`core`, `dice`, `events`, `rulebooks/dnd5e`, `tools/spatial`, `tools/environments`). Each has an independent version. See `go.mod` for current versions.

## Current dependency versions (as of 2026-08-03)

| Dependency | Version |
|---|---|
| `rpg-api-protos/gen/go` | `v0.0.0-20260803143754-d9ef7d9fb49d` (generated branch artifact `d9ef7d9fb49d`; root release `v0.1.118`) |
| `rpg-toolkit/rulebooks/dnd5e` | `v0.70.1` |
| `rpg-toolkit/core` | `v0.10.0` |
| `rpg-toolkit/dice` | `v0.3.2` |
| `rpg-toolkit/events` | `v0.6.2` |
| `rpg-toolkit/tools/environments` | `v0.4.4` |
| `rpg-toolkit/tools/spatial` | `v0.5.1` |
