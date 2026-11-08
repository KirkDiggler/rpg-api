# Run — 2026-11-08 — behavior on the asynchronous world path

Part of [#866](https://github.com/KirkDiggler/rpg-project/issues/866), integrating [runtime optimizations #879](https://github.com/KirkDiggler/rpg-project/issues/879).

## Shipped

- Prepared current continuation-aware API/protocol integration on real pushed canonical encounter/session pins; no API behavior logic or replacement runner is added — [authorization](https://github.com/KirkDiggler/rpg-project/issues/866#issuecomment-6717591263).

## Decided

- Run the canonical route and disturbance scenarios with the production continuation runner, not the shared harness's synchronous AnswerWhole shortcut; an early response is not completion or a save — [upstream contract](https://github.com/KirkDiggler/rpg-api/pull/1197).
- Read completion through an actual following GetWhere request on the same session guard, and drain an old host before recreating its in-process locker over Redis; a different locker is not the same coordination domain — [continuation contract](https://github.com/KirkDiggler/rpg-toolkit/pull/2314).
- Retain S7's subscriber-local live folding and S10's whole-commit broker units, because live delivery must remain complete while reconnect reads each observer's own story; no canonical-side broker or stream special case is introduced — [listeners](https://github.com/KirkDiggler/rpg-api/pull/1201), [commit units](https://github.com/KirkDiggler/rpg-api/pull/1199).
- Integrate the concurrently landed opt-in measurement harness unchanged rather than call the release catch-up manifest-only; its existing independent closure and a private-Redis smoke cover the test-only addition, while the production tree still differs only by released provider pins — [measurement closure](https://github.com/KirkDiggler/rpg-api/pull/1202#issuecomment-6718387181).
- The delayed-boundary test controls scheduling only before handing the unchanged continuation to the production runner; it supplies no test-only NPC picker, coordinator or movement — [#866](https://github.com/KirkDiggler/rpg-project/issues/866).

## Found

- The six-cell Move answers CONTINUING with five steps, Through and no save; the real guarded read waits for the rest, then the exact two-/three-item routes and ten-choice clang/resumption journey retain their original configuration through Redis and fresh hosts — [#866](https://github.com/KirkDiggler/rpg-project/issues/866).
- The joined S7/S9/S10 graph resolves canonical encounter ee0c1163 and session edaec8c9 alongside root v0.214.0/resolution v0.74.3; the same race-enabled behavior scenarios pass with the new listener/broker and composed-effects consumers rather than relying on the earlier graph's green result — [#866](https://github.com/KirkDiggler/rpg-project/issues/866).
- Released encounter v0.185.0/session v0.171.0 run the new measurement on the canonical 10k-cell/100-member garrison: three measured Moves per mode after three warm-ups, no missing beat or lag, 73–134 acts per verb, continued replies116–250ms. This bounded smoke excludes transport and is not proof of the separate100ms/250ms latency target or an apples-to-apples comparison with earlier content — [performance story](https://github.com/KirkDiggler/rpg-project/issues/879).
- A deliberately delayed continuation keeps the next request blocked, leaves storage unchanged before execution, survives request cancellation and a producer-config edit, delivers the sixth player step after Through, selects both NPCs once and refuses a repeated callback — [#866](https://github.com/KirkDiggler/rpg-project/issues/866).
- Replacing the production runner's WithoutCancel context with the canceled request in a virtual overlay makes that test fail at two expected choices versus zero; repository bytes remain unchanged — [#866](https://github.com/KirkDiggler/rpg-project/issues/866).

## Mistaken

- The release guard correctly stopped when concurrent main added the measurement test and its run record beyond go.mod/go.sum. The added files match reviewed upstream exactly; their real opt-in path was checked before resuming release gates — [measurement](https://github.com/KirkDiggler/rpg-api/pull/1202).
- The first disposable Redis startup loaded seven keys from a default working-directory snapshot; the harness refused to flush it. Restarting the owned process in a dedicated empty directory produced the valid smoke, without changing an operator store — [guard contract](https://github.com/KirkDiggler/rpg-api/pull/1202#issuecomment-6718387181).
- The old tests treated an early answer as completed movement and recreated a host while accepted work still held the original guard; observed status/progress mismatches and a closed-Redis continuation exposed the stale test lifecycle. They now follow the actual asynchronous contract without turning it off — [#866](https://github.com/KirkDiggler/rpg-project/issues/866).
- A first no-partial-save assertion reserialized native maps and compared their ordering; the storage values were unchanged. It now compares the complete decoded record, while Through and exact step/choice assertions independently prove the boundary — [#866](https://github.com/KirkDiggler/rpg-project/issues/866).
