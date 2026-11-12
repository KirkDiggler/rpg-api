# Run — 2026-11-09 — exact diagnostic payload on the live wire

processed: 2026-11-12T165555Z

## Shipped

- Adopted CI-published proto v0.1.263 and populated fields_json from the original SDK payload, retaining the deprecated Struct only as an approximate projection — [R9 provider](https://github.com/KirkDiggler/rpg-api-protos/pull/426).

## Decided

- Copy raw bytes before attempting the legacy projection, because malformed/unsupported payloads must retain identity and original evidence rather than silently fall back to a different value — [R9](https://github.com/KirkDiggler/rpg-project/pull/902).
- Pin the official tag's generated commit for the nested Go module, because root v-tags are not gen/go-prefixed module tags; no local replace or hand publication is used — [publication](https://github.com/KirkDiggler/rpg-api-protos/pull/426).

## Found

- Raw converter and both record RPC tests fail before the copy, then preserve large integers, zero/false/empty strings, whitespace and byte ownership. Malformed/absent raw evidence stays explicit — [#900](https://github.com/KirkDiggler/rpg-project/issues/900).
- Real authored source with inspection-count9007199254740993 compiles, runs through ordinary continuation/Redis/reload, and has identical raw payload bytes in capture and GetRecord — [#900](https://github.com/KirkDiggler/rpg-project/issues/900).

## Mistaken

- go get on the root release tag resolves the parent repository module rather than its nested gen/go module. The actual CI-generated tag content and parent were verified, then its generated commit was consumed normally — [#426](https://github.com/KirkDiggler/rpg-api-protos/pull/426).
