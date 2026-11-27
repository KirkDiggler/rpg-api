# Run — 2026-11-26 — Following the renamed contract

The second leg of the R2 wave ([rpg-project#1155](https://github.com/KirkDiggler/rpg-project/pull/1155#issuecomment-7186155785)); the contract's leg is [rpg-api-protos#457](https://github.com/KirkDiggler/rpg-api-protos/pull/457).

## Shipped

- `InteractResponse` and `TradeResponse` read as `NpcDescriptor` in the two handlers and as `GetNpcDescriptor()` in five test reads; pinned to the generated branch at `430c294`, the commit of tag `v0.1.292` — [https://github.com/KirkDiggler/rpg-api/pull/1292](https://github.com/KirkDiggler/rpg-api/pull/1292).

## Decided

- The pin is a pseudo-version at the published commit, as the module was pinned before; the tag is the web's to consume — [https://github.com/KirkDiggler/rpg-api/pull/1292](https://github.com/KirkDiggler/rpg-api/pull/1292).
- Every site was left to the compiler to find, against a local copy of the renamed contract, before the pin existed; the branch stayed unpushed while it carried a `replace` — [https://github.com/KirkDiggler/rpg-api/pull/1292](https://github.com/KirkDiggler/rpg-api/pull/1292).

## Found

- protoc-gen-go had mangled the field to `Descriptor_`, so nothing in rpg-api ever saw the name `descriptor`: two source lines and five test reads are the whole of it — [rpg-api#1290](https://github.com/KirkDiggler/rpg-api/issues/1290).
- A grep for `GetDescriptor()` misses `GetDescriptor_()` by one character — [rpg-api#1292](https://github.com/KirkDiggler/rpg-api/pull/1292).

## Mistaken

- "No test reads the field" was said in chat and in a PR body before the tests were compiled; five do. Both were corrected the same hour — [rpg-api-protos#457](https://github.com/KirkDiggler/rpg-api-protos/pull/457).
- A publish was called done on a completed run that belonged to the previous merge; it was re-read by the run's own commit, and the tag that followed was `v0.1.292`, not `v0.1.291` — [rpg-api-protos#457](https://github.com/KirkDiggler/rpg-api-protos/pull/457).
