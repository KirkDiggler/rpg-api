# Decisions — internal/integration/session

One line per entry, dated and linked. Ingest is the writer; entries arrive through its reviewed PR.
A decision reads `- <date> · <the words> · options: <the options it closed> · because: <why> · <link> · upheld: <links, or none yet>`;
a finding, friction or open line reads `- <date> · <the words> · <link>`. A finding that needs more than a line links its case file in `findings/`.
Friction names both entries and never settles them.

## Decisions

- none yet

## Findings

- 2026-11-12 · Asynchronous behavior acceptance uses the production runner and a following guarded read to observe completion. Recreating a host with a new in-process locker while the old continuation still owns its guard is not reload evidence; drain accepted work before replacing the coordination domain. · https://github.com/KirkDiggler/rpg-api/blob/c6183e148b1b840037dc93b1453b794887f55cf3/knowledge/runs/2026-11-07-canonical-behavior-integration.md, https://github.com/KirkDiggler/rpg-api/blob/c6183e148b1b840037dc93b1453b794887f55cf3/knowledge/runs/2026-11-08-behavior-async-integration.md
- 2026-11-12 · Performance evidence distinguishes answer, first beat after answer, final drain, cold read-path reload and write costs. Broker-channel timing excludes transport; load, blob age, content and warm/cold residency change the experiment. Early-answer latency is not total completion cost, and a read-path load does not measure every write-opener stage. · https://github.com/KirkDiggler/rpg-api/blob/c6183e148b1b840037dc93b1453b794887f55cf3/knowledge/runs/2026-11-07-alive-not-lagging-s0-live-measure-api.md, https://github.com/KirkDiggler/rpg-api/blob/c6183e148b1b840037dc93b1453b794887f55cf3/knowledge/runs/2026-11-08-alive-not-lagging-cold-reload-api.md, https://github.com/KirkDiggler/rpg-api/blob/c6183e148b1b840037dc93b1453b794887f55cf3/knowledge/runs/2026-11-09-alive-not-lagging-walker-step-api.md, https://github.com/KirkDiggler/rpg-api/blob/c6183e148b1b840037dc93b1453b794887f55cf3/knowledge/runs/2026-11-10-pace-commits.md

## Friction

- none yet

## Open

- none yet
