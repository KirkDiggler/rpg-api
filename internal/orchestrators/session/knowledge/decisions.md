# Decisions — internal/orchestrators/session

One line per entry, dated and linked. Ingest is the writer; entries arrive through its reviewed PR.
A decision reads `- <date> · <the words> · options: <the options it closed> · because: <why> · <link> · upheld: <links, or none yet>`;
a finding, friction or open line reads `- <date> · <the words> · <link>`. A finding that needs more than a line links its case file in `findings/`.
Friction names both entries and never settles them.

## Decisions

- none yet

## Findings

- 2026-11-12 · The continuation runner detaches accepted work from request cancellation, recovers panics, and refuses new work once Drain starts. Failures after the early answer are logged, not reported as an ordinary successful save or a client-visible completion signal. · https://github.com/KirkDiggler/rpg-api/blob/c6183e148b1b840037dc93b1453b794887f55cf3/knowledge/runs/2026-11-07-alive-not-lagging-s1-api.md
- 2026-11-12 · Act and record broker buffers count committed units rather than individual beats. Live listener enumeration narrows live folding without changing each observer’s catch-up story; lag and close remove subscriptions from that enumeration. · https://github.com/KirkDiggler/rpg-api/blob/c6183e148b1b840037dc93b1453b794887f55cf3/knowledge/runs/2026-11-07-alive-not-lagging-s10-api.md, https://github.com/KirkDiggler/rpg-api/blob/c6183e148b1b840037dc93b1453b794887f55cf3/knowledge/runs/2026-11-07-alive-not-lagging-s7-s9-api.md
- 2026-11-12 · The Redis checkpoint transaction checks the fence before landing any aggregate writes, advances version with the checkpoint lifetime, and persists the record floor explicitly. Missing record pieces or prior checkpoint formats refuse rather than loading an empty story or inventing a migration. · https://github.com/KirkDiggler/rpg-api/blob/c6183e148b1b840037dc93b1453b794887f55cf3/knowledge/runs/2026-11-08-fenced-redis-save.md, https://github.com/KirkDiggler/rpg-api/blob/c6183e148b1b840037dc93b1453b794887f55cf3/knowledge/runs/2026-11-09-appended-record.md, https://github.com/KirkDiggler/rpg-api/blob/c6183e148b1b840037dc93b1453b794887f55cf3/knowledge/runs/2026-11-10-checkpoint-pieces.md
- 2026-11-12 · Content-addressed site pieces have no session TTL: a repeatedly refreshed checkpoint can outlive the once-written site. Collection of unreferenced site content is separate from checkpoint expiry. · https://github.com/KirkDiggler/rpg-api/blob/c6183e148b1b840037dc93b1453b794887f55cf3/knowledge/runs/2026-11-09-alive-not-lagging-site-shared-api.md

## Friction

- none yet

## Open

- none yet
