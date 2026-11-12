# Decisions — internal/handlers/dnd5e/session/v1alpha2

One line per entry, dated and linked. Ingest is the writer; entries arrive through its reviewed PR.
A decision reads `- <date> · <the words> · options: <the options it closed> · because: <why> · <link> · upheld: <links, or none yet>`;
a finding, friction or open line reads `- <date> · <the words> · <link>`. A finding that needs more than a line links its case file in `findings/`.
Friction names both entries and never settles them.

## Decisions

- none yet

## Findings

- 2026-11-02 · Act translation copies the SDK’s ending outcome and movement kind through closed-set lookups: empty ending means unspecified, empty movement kind means own, and unknown values refuse the act. The deprecated forced field is carried as supplied by the SDK, not inferred from a spell or cause. · https://github.com/KirkDiggler/rpg-api/blob/82bd5946a2d8911b58db74314015708b4587d43c/knowledge/runs/2026-10-30-franks-list-s9-screen.md, https://github.com/KirkDiggler/rpg-api/blob/82bd5946a2d8911b58db74314015708b4587d43c/knowledge/runs/2026-11-02-when-things-fire-s1.md
- 2026-11-12 · Record conversion copies owned raw payload bytes before attempting the legacy Struct projection. Malformed or unsupported payloads retain record identity and exact evidence; neither a plausible Struct nor a successful unrelated test establishes lossless live delivery. · https://github.com/KirkDiggler/rpg-api/blob/c6183e148b1b840037dc93b1453b794887f55cf3/knowledge/runs/2026-11-08-machine-inspection-capture.md, https://github.com/KirkDiggler/rpg-api/blob/c6183e148b1b840037dc93b1453b794887f55cf3/knowledge/runs/2026-11-09-inspection-released-integration.md, https://github.com/KirkDiggler/rpg-api/blob/c6183e148b1b840037dc93b1453b794887f55cf3/knowledge/runs/2026-11-09-lossless-record-wire.md

## Friction

- none yet

## Open

- none yet
