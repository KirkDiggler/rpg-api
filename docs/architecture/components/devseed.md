---
name: devseed
description: CLI that seeds Redis with playable characters + a turn-based encounter for the playtest harness
updated: 2026-05-18
confidence: high — verified by running against local Redis and round-tripping through character.LoadFromData
---

# devseed

`cmd/devseed` is a small CLI that writes a ready-to-play v2 encounter plus the three canonical playtest characters (alice-rogue, bob-barbarian, wendy-wizard) and a goblin directly into Redis. It exists so the rpg-dnd5e-web playtest harness and MCP-driven verification flows can hit a populated game state without manually walking through `CreateDraft → UpdateRace → ... → FinalizeDraft → CreateEncounter` every session.

## Why it exists

The Wave 2.11d reaction-flow work requires verifying behaviors that depend on specific character shapes:
- Alice firing **SneakAttack** on a finesse-weapon hit with an adjacent ally.
- Bob's **Rage** feature being available to activate (level-1 barbarian, 2 uses).
- Wendy's **Shield** reaction being Apply()'d at character load (gated on a 1st-level spell slot).
- The encounter being in **TURN_BASED** mode so the harness can exercise `TakeAction` / `EndTurn` / `SubmitCheck` flows.

Seeding raw `PlayerData` (the previous version of this tool) was enough for plumbing checks but couldn't verify any of the actual reaction behaviors — the rpg-api handler loads a real `*character.Data` from the character repo, and without a properly-shaped character there's nothing to fire.

## Files

| File | Purpose |
|---|---|
| `cmd/devseed/main.go` | Single-file CLI: flag parsing, Redis client, character + encounter builders |

## What it writes

| Key | Shape | TTL |
|---|---|---|
| `character:char-alice` | `entities.Character{Data: *toolkitchar.Data}` — level-2 rogue, shortsword, SneakAttack condition persisted | none |
| `character:char-bob` | level-1 barbarian, greataxe, Rage feature + RageCharges resource (2/2, long-rest reset) | none |
| `character:char-wendy` | level-1 wizard, SpellSlots[1] = {Max:2,Used:0} (Shield-eligible) | none |
| `enc:v2:<encounter-id>` | `*tkenc.Data` with 3 players + 1 goblin, mode=TURN_BASED by default | 24h |

Key prefixes and TTLs match what the production repositories use (`internal/repositories/character/redis.go`, `internal/repositories/encounters/v2/redis.go`). The character envelope mirrors `entities.Character` — the `data` field wraps the toolkit's `*character.Data` directly so the character repo's `Get` can `json.Unmarshal` it without changes.

## How conditions get applied at load

devseed is intentionally narrow about what it persists vs what gets reattached at load time. The split mirrors what `applyReactionConditions` (in `internal/handlers/dnd5e/v2/encounter/reaction_conditions.go`) does on every encounter rehydration:

| Reaction | Persisted in character JSON? | Apply()'d at load? |
|---|---|---|
| **SneakAttack** (alice) | yes — in `Conditions: []json.RawMessage{...}` | by `character.LoadFromData` for every condition in the blob |
| **OpportunityAttack** (alice, bob, wendy) | no | by `applyReactionConditions` for every melee combatant |
| **Shield** (wendy) | no | by `applyReactionConditions` when `hasFirstLevelSpellSlot(char) == true` |
| **Rage** (bob) | as a feature, not a condition — Rage isn't reactive, it's activatable | not auto-Apply()'d; player activates via the Rage feature, which then creates the RagingCondition |

This split is the same one the integration tests (`integration_sneak_attack_test.go`, `integration_player_shield_test.go`) use to build their fixtures — devseed is essentially "the integration fixture, but written to Redis."

The split survives Wave 2.11d's `LoadFromData` round-trip fix (rpg-toolkit#660 / dnd5e v0.58.1): SpellSlots, ClassResources, and the rest of the runtime state now round-trip correctly through Get → LoadFromData → ToData. Before that fix, wendy's spell slot would silently drop on rehydration and Shield would never Apply.

## How to invoke

```bash
# Default: localhost:6379, encounter-id=dev-encounter, mode=turn_based
go run ./cmd/devseed

# Custom encounter id (matches the harness URL ?encounterId=…)
go run ./cmd/devseed --encounter-id=my-encounter

# Free-roam mode for non-combat verification (no initiative)
go run ./cmd/devseed --mode=free_roam

# Custom Redis target
REDIS_ADDR=redis.example.com:6379 go run ./cmd/devseed
# or
go run ./cmd/devseed --redis-addr=redis.example.com:6379
```

Stderr breadcrumbs report each key written. Stdout is empty — this is not a `redis-cli SET`-pipeable shape any more; devseed connects to Redis directly.

## Verification

After running:

```bash
redis-cli --scan --pattern 'character:char-*'
# → character:char-alice, character:char-bob, character:char-wendy

redis-cli GET 'enc:v2:dev-encounter' | python3 -c \
  'import json,sys; d=json.load(sys.stdin); print("mode:", d["mode"]); print("players:", list(d["players"]))'
# → mode: 2  (1 = FREE_ROAM, 2 = TURN_BASED)
#   players: ['alice', 'bob', 'wendy']
```

## What it deliberately does NOT do

- **No appearance field.** The harness verification target is combat behavior, not rendering. The character repo's `entities.Character.Appearance` is `omitempty`; we leave it nil.
- **No spell list.** Wendy has spell slots but no `PreparedSpells` / `SpellsKnown` field — Shield Apply() gates on the slot count heuristic in `applyReactionConditions.hasFirstLevelSpellSlot`, not on a real "spell is prepared" check. A future wave can tighten this; devseed mirrors what the runtime gate actually inspects.
- **No CreateEncounter RPC.** devseed writes Redis keys directly to bypass auth, draft state, dungeon generation, and the full create flow. This is a developer / harness tool, never a production code path.
- **No goblin variety.** Always one goblin via `monster.NewGoblin` (SRD stats, AC 15, HP 7, scimitar 1d6+2). Add monster variants here if a future scenario needs them; do not invent ad-hoc monster shapes (use `monster.New` or one of the named constructors).

## Known gaps

- **Single encounter, single goblin.** No way to seed multi-room dungeons or multi-encounter scenarios from this tool. If the playtest goal grows past "one room, one fight", extend or supersede.
- **Character resource recovery state.** Bob's RageCharges write through as `Maximum=2, Current=2`. There's no `Used=N` knob for testing "what happens when bob has only 1 rage left" — would need to be added if a Wave 2.11e+ scenario asks for it.
- **No reseed-idempotency guard.** Re-running `devseed` against the same Redis overwrites the keys. Acceptable for a dev tool; do not point at production Redis (the connection target is intentionally explicit via `--redis-addr` / `REDIS_ADDR`).
