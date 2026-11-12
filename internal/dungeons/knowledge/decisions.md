# Decisions — internal/dungeons

One line per entry, dated and linked. Ingest is the writer; entries arrive through its reviewed PR.
A decision reads `- <date> · <the words> · options: <the options it closed> · because: <why> · <link> · upheld: <links, or none yet>`;
a finding, friction or open line reads `- <date> · <the words> · <link>`. A finding that needs more than a line links its case file in `findings/`.
Friction names both entries and never settles them.

## Decisions

- none yet

## Findings

- 2026-11-12 · The earlier reference-keep record describes a layout reference rather than a populated adventure; the later shipped-content cut removes reference and walk fixtures from the public shelf. The tracked-file allowlist distinguishes published content from untracked builder walk files. · https://github.com/KirkDiggler/rpg-api/blob/82bd5946a2d8911b58db74314015708b4587d43c/knowledge/runs/2026-11-01-keep-reference.md, https://github.com/KirkDiggler/rpg-api/blob/dev/docs/how-to/reference-keep.md, https://github.com/KirkDiggler/rpg-api/blob/c6183e148b1b840037dc93b1453b794887f55cf3/knowledge/runs/2026-11-08-content-ship-less.md
- 2026-11-12 · Test infrastructure compiles its immutable shipped content shelf once while each case keeps fresh writable stores. Eager TestMain setup keeps shared compilation from being charged to whichever test happens to ask first; it is not a runtime cache or permission to share mutable test state. · https://github.com/KirkDiggler/rpg-api/blob/c6183e148b1b840037dc93b1453b794887f55cf3/knowledge/runs/2026-11-08-lobby-suite-compiles-once.md, https://github.com/KirkDiggler/rpg-api/blob/c6183e148b1b840037dc93b1453b794887f55cf3/knowledge/runs/2026-11-09-dungeons-compiles-its-shelf-once.md

## Friction

- none yet

## Open

- none yet
