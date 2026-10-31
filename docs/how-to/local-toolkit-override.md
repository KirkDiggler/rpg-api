# Legacy local toolkit override

This is not the current development workflow. Use
[toolkit pseudo-versions](toolkit-pseudo-versions.md): push the toolkit commit to
origin, pin it with `go get <module>@<commit>` in the consumer, and build the normal
API image. After the provider merges, adopt its CI-published release tag before
merging the consumer.

Do not start new work with `toolkit-local-override.sh`, `local-toolkit/`,
`RPG_TOOLKIT_PATH`/`RPG_TOOLKIT_REF`, `go.work` or `Dockerfile.local-toolkit`.
The legacy helper remains only for handling an existing owned override; this
page does not authorize deleting existing environments or other developers'
sources. The old instructions remain available in Git history.

## What the helper replaces

`rulebooks/dnd5e` is an umbrella directory, not one module. Directories below
it with their own `go.mod` (`session`, `encounter`, `resolution`, ...) are
separate Go modules. A `replace` for the umbrella alone leaves them on their
published tags, so the API silently builds old code for exactly the modules a
slice changes.

`on --target rulebooks/dnd5e` therefore writes a `replace` for:

- the umbrella, `github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e`;
- every nested module that the API's `go.mod` requires (direct or indirect)
  and the synced `local-toolkit/rulebooks/dnd5e/` tree provides. The helper
  finds them by walking that tree for `go.mod` files and reading each `module`
  line; a nested module the API does not require gets no replacement.

It then runs `go mod tidy`, replaces any nested module that tidy newly
requires, and lists every active replacement. It fails loudly when the API
requires a nested module that the source checkout does not provide, because
that module would otherwise build from its published tag. The ownership state
file records the nested set, so `off` removes exactly those replacements, runs
`go mod tidy` to restore the published sums, and rebuilds. `status` fails when
a required nested module is not replaced; `refresh` re-syncs and adds any that
are missing.

The legacy `encounter` target is unchanged: one replacement, no tidy.

`Dockerfile.local-toolkit` needs no change: it copies the whole build context,
including `local-toolkit/`, before `go mod download`.

## Verify the binary uses the local copies

```sh
scripts/toolkit-local-override.sh on --target rulebooks/dnd5e --src <toolkit checkout>
grep ^replace go.mod
go build -o /tmp/rpg-api-server ./cmd/server
go version -m /tmp/rpg-api-server | grep rpg-toolkit
```

Every replaced module shows a `=> ./local-toolkit/... (devel)` line under its
`dep` line. A `dep` line under `rulebooks/dnd5e` with no `=>` line means that
module is still on its published tag.

`scripts/toolkit-local-override.test.sh` exercises this offline against a
file-based module proxy.
