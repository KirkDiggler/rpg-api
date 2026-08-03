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

rpg-toolkit is published as independently versioned Go modules. Inspect the direct `github.com/KirkDiggler/rpg-toolkit/...` requirements in `go.mod`, then update only the module that provides the API you need.

## Generated-Go version example (2026-08-03)

This proto bump resolved the generated-Go module to the following version. Treat
`go.mod` as the source of truth for the currently pinned dependency graph.

| Dependency              | Version                                                                                                  |
| ----------------------- | -------------------------------------------------------------------------------------------------------- |
| `rpg-api-protos/gen/go` | `v0.0.0-20260803143754-d9ef7d9fb49d` (generated branch artifact `d9ef7d9fb49d`; root release `v0.1.118`) |
