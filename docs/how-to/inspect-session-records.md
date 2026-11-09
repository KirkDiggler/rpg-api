# Inspect session diagnostic records

The API writes schema1 JSON lines to **stdout**, independently of browser
subscriptions. Ordinary server logging stays on stderr. The records contain
private configurations and observations; restrict access to captures accordingly.

## Live playtest view

The v1alpha2 RecordService is enabled only when the server's existing
`AUTH_DEV_MODE` is enabled. Outside that mode both record RPCs refuse access,
even for a seated caller. This does not disable ordinary StoryService or server
capture. Do not enable development authentication on a production server merely
to inspect a run; inspect its server-side capture instead.

## Capture and inspect

Collect the API container's stdout through the normal container log collector.
A local snapshot can be collected separately from server stderr:

```sh
docker logs <api-container> > /tmp/session-records.jsonl 2> /tmp/api-server.log
```

Filter one session for the playtest reader, retaining capture status lines:

```sh
jq -c 'select(.kind == "capture" or .session == "SESSION_ID")' \
  /tmp/session-records.jsonl > /tmp/one-session.jsonl
```

Do not discard malformed lines, missing symbols or incomplete markers to make a
capture appear valid. The snapshot of a running server may not have an end marker.

## Format and meaning

Each `kind: "record"` line contains:

- `schema: 1`, `capture`: format version and process-capture identity;
- `logged_at`: host export time, not simulated game time;
- `session`, `id`, `act`, `at`, `beat`, `fields`: the SDK record's identifiers,
  game-clock reading and opaque JSON payload. The host does not derive reasons
  or reevaluate game rules. Known large integers and scalar values remain in the
  captured raw JSON without conversion through a protobuf Struct.

`behavior_config` supplies the exact bound configuration under its `config`
identity. `behavior_decision` records actual alternatives and references the
weighted selection's `pick_record`. `behavior_work` references `decision_record`
and distinguishes selection, partial work, pause, completion and abandonment.
These facts are not additions to the player-facing story.

`kind: "capture"` lines describe the export, not gameplay:

- `started`: the process began capturing. `history_complete: false` does not
  claim constructor records or earlier run history were published to this sink.
- `drained`: all accepted batches finished writing. This does not mean the run
  ended or that history before capture is complete.
- `incomplete`: the bounded queue overflowed; already accepted batches drained,
  but this capture stopped accepting new ones. `reason: "queue_full"` is explicit.

A write/encoding/panic failure also stops capture and is reported to stderr and
by shutdown drain. A broken writer may be unable to write its own incomplete
marker. Missing end/status data is therefore never evidence of completeness.
Gameplay was already saved before publication and is not rolled back by export
failure. No database, exactly-once audit guarantee or simulation replay is
provided by this stream.
