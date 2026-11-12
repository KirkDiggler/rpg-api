# Run — 2026-11-09 — inspection on released providers and fenced storage

processed: 2026-11-12T165555Z

## Shipped

- Prepared API inspection on published machinev0.2.0, behaviorv0.22.0, encounterv0.186.0, sessionv0.173.0 and recordv0.3.0; the official proto v0.1.263 generated commit remains pinned. This is the API candidate, not a production deployment — [API PR](https://github.com/KirkDiggler/rpg-api/pull/1207).

## Decided

- Merge published dev81a3b38 on top, preserving its independently reviewed fenced Redis adapter, immutable test-shelf compilation and measurement harness; no fork or unmerged adapter copy — [fence](https://github.com/KirkDiggler/rpg-api/pull/1208), [test shelf](https://github.com/KirkDiggler/rpg-api/pull/1210), [measurement](https://github.com/KirkDiggler/rpg-api/pull/1211).
- Resolve only dependency-manifest conflicts, then let go get/tidy select the actual released graph. Capture, raw conversion and diagnostic-gate implementation retain the reviewed behavior — [retained inspection review](https://github.com/KirkDiggler/rpg-api/pull/1207#pullrequestreview-5821167418).
- Verify new-format save/reload/capture on compatible runs rather than reuse old preview blobs: published recordv0.3.0 and fenced storage deliberately refuse their old forms — [record contract](https://github.com/KirkDiggler/rpg-toolkit/pull/2331), [fenced storage](https://github.com/KirkDiggler/rpg-api/pull/1208).

## Found

- Named real-compiler capture/reload control passes under race0.41s, including original bound wide integers and raw-field equality; the explicit fence control passes0.08s and the complete raw record handler suite passes — [candidate evidence](https://github.com/KirkDiggler/rpg-api/pull/1207).
- Full make test and make ci-check pass on the joined released graph, including cmd/server and the real integration harness; release-pin guard confirms no go.work, replace or local toolkit override — [candidate evidence](https://github.com/KirkDiggler/rpg-api/pull/1207).

## Mistaken

- The first combined go-test filter was not used as proof that the nested capture test ran. Separate anchored verbose invocations show both exact test names and PASS results; no package-only green substitutes for those named controls — [candidate evidence](https://github.com/KirkDiggler/rpg-api/pull/1207).
