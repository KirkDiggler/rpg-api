---
name: local rpg-toolkit override loop
description: Iterate on rpg-toolkit/encounter changes in the local Docker loop without publish -> tag -> go get
updated: 2026-07-27
---

# How to iterate on rpg-toolkit changes without publishing

The normal toolkit dependency loop (see [update-proto-dependency.md](update-proto-dependency.md)'s
"Update toolkit dependency" section) is: edit rpg-toolkit, commit, tag a new
module version, `go get` it into rpg-api. That's the right process for a
change you're confident in — but a bug that needs several edit-and-look
cycles (e.g. rpg-toolkit#858, #859) makes a publish-per-attempt loop
impractical.

This script lets you point rpg-api's build at a **local** rpg-toolkit
checkout instead, for `github.com/KirkDiggler/rpg-toolkit/encounter` only.
It rejects any unapproved replacement or second replacement; one local module
is the entire override budget for an iteration.

## The constraint this solves

The local rpg-api container is built with:

```bash
docker build -t rpg-api:local <rpg-api-dir>
```

The build context is the rpg-api directory **alone**. A `go.mod` `replace`
pointing outside it (e.g. `../../rpg-toolkit/encounter`) compiles fine with
a plain `go build` on the host, then **fails inside Docker**, because the
toolkit source was never sent to the build context.

The fix: sync the toolkit module's source into a gitignored directory
**inside** this repo (`local-toolkit/encounter/`) and point the `replace`
at that relative path, so it's part of the context Docker actually sees.

Only `rpg-toolkit/encounter` is overridden. Its own dependencies on sibling
toolkit modules (`core`, `events`, `tools/spatial`, ...) stay at whatever
published versions its `go.mod` already names — this script does not sync
the whole toolkit.

## Turning it on

```bash
scripts/toolkit-local-override.sh on [--src <path-to-rpg-toolkit-checkout>]
```

Defaults to `$TOOLKIT_SRC_DIR`, then `~/game-dev/rpg-toolkit`, then a couple
of relative guesses if you didn't pass `--src`. This:

1. rsyncs `<src>/encounter/` into `local-toolkit/encounter/` (gitignored).
2. Runs `go mod edit -replace github.com/KirkDiggler/rpg-toolkit/encounter=./local-toolkit/encounter`.
3. Sanity-builds on the host (`go build ./...`).

Then rebuild the image with the **dev-only Dockerfile** (`Dockerfile.local-toolkit`,
not the normal `Dockerfile` — see why below) and redeploy the primary
`rpg-api` container only:

```bash
docker build -f Dockerfile.local-toolkit -t rpg-api:local .

cd /home/kirk/game-dev/rpg-deployment
docker compose -f docker-compose.local-dev.yml \
               -f docker-compose.local-api-src.yml \
               up -d rpg-api
```

Never touch `rpg-api-lab` / `rpg-api-lab2` — those are separate playtest
instances; redeploy only the primary `rpg-api` service.

### Why `Dockerfile.local-toolkit`, not `Dockerfile`

The normal `Dockerfile` copies `go.mod`/`go.sum` first, runs `go mod
download`, and only copies the rest of the source afterward (a layer-caching
optimization). With an active `replace` pointing at `local-toolkit/encounter/`,
that directory doesn't exist yet at the `go mod download` step, and the
build fails. `Dockerfile.local-toolkit` is identical except it copies the
full source (including `local-toolkit/`) *before* `go mod download` — it
loses that one caching optimization, which doesn't matter here since a
toolkit edit means you want a full rebuild anyway. Without an active
override, it produces the exact same image as `Dockerfile`.

### Edit-and-look cycle

Each time you change something in `<rpg-toolkit-checkout>/encounter/`:

```bash
scripts/toolkit-local-override.sh on   # re-syncs + re-points the replace
docker build -f Dockerfile.local-toolkit -t rpg-api:local .
# redeploy rpg-api as above
```

## Turning it off

```bash
scripts/toolkit-local-override.sh off
```

This drops the `replace` directive and deletes `local-toolkit/` entirely,
restoring `go.mod`/`go.sum` to the published pin with no residue, then
sanity-builds to confirm. Follow with a normal `docker build -t rpg-api:local .`
(the regular `Dockerfile` again) and redeploy `rpg-api` to go back to the
published toolkit version.

## Status

```bash
scripts/toolkit-local-override.sh status
```

Reports the complete active `go.mod` replacement module list, then whether
this exact approved override is active and whether `local-toolkit/encounter/`
exists on disk. It exits non-zero if a second or unapproved replacement exists;
that output is the verification record before running a local build.

## Deploy discipline — do not skip this

**The `replace` must never reach a merged branch.** It points at a directory
that exists only on your machine; it doesn't exist in CI or on anyone else's
checkout, and CI's own `go build` would fail loudly on an escaped replace —
but that's defense in depth, not the only line. Treat it as a hard rule:

1. Iterate locally with the override ON until the toolkit fix is right.
2. **Publish the toolkit change**: commit + push in rpg-toolkit, tag the new
   `encounter/vX.Y.Z` version.
3. Pull the real published version into rpg-api the normal way:
   ```bash
   GOPROXY=direct go get github.com/KirkDiggler/rpg-toolkit/encounter@vX.Y.Z
   ```
4. **Remove the local override**: `scripts/toolkit-local-override.sh off`.
5. Commit the real `go.mod`/`go.sum` version bump — never a `replace` line.

Before opening a PR (or even committing), check `git diff go.mod` — if it
shows a `replace ... => ./local-toolkit/encounter` line, the override is
still on. Run `off` first.
