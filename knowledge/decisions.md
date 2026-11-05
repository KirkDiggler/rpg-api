# Decisions — the repo root

One line per entry, dated and linked. Ingest is the writer; entries arrive through its reviewed PR.
A decision reads `- <date> · <the words> · options: <the options it closed> · because: <why> · <link> · upheld: <links, or none yet>`;
a finding, friction or open line reads `- <date> · <the words> · <link>`. A finding that needs more than a line links its case file in `findings/`.
Friction names both entries and never settles them.

## Decisions

- 2026-10-28 · the session SDK is rpg-api's single point of contact with the toolkit, with one named exception held on its written reason: encounter.HexCellAt in sessionworld, a lookup not arithmetic, exported by the toolkit so a content caller asks for the offset-to-axial conversion rather than reimplementing it (rpg-toolkit#1150, one basis, one place); the other direct call, resolution.ProjectCharacter in the character view, is a leak and is filed · options: filing the hex lookup as a leak; accepting both as exceptions · because: the call site already records its reason, it causes no pain and slows no other idea; revisit when it does (the operator, 2026-10-28); the first case where a because written at the call site protected a decision from being refiled as a defect · [rpg-api#1149](https://github.com/KirkDiggler/rpg-api/issues/1149) · upheld: [rpg-toolkit#1787](https://github.com/KirkDiggler/rpg-toolkit/pull/1787) (2026-09-16: level-up built in rpg-api's orchestrator moved into session on the ruling)

## Findings

- 2026-11-01 · reference-keep is a read-only shipped layout/placement reference copied under an independent key for authoring: its 28,934 cells, 28 areas and annotations are not a populated adventure. Its Performance/Deception DC 11 replies and gate facts do not enforce admission or arrest; the later entrance requirement remains separate. · https://github.com/KirkDiggler/rpg-api/blob/82bd5946a2d8911b58db74314015708b4587d43c/knowledge/runs/2026-11-01-keep-reference.md, https://github.com/KirkDiggler/rpg-api/blob/dev/docs/how-to/reference-keep.md

## Friction

- none yet

## Open

- none yet
