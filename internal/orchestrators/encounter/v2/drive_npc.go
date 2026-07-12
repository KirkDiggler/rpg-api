package encounter

import (
	"context"
	"errors"
	"fmt"

	encountersv2 "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/v2"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
)

// DriveStalledNPCTurnInput carries the parameters for DriveStalledNPCTurn.
type DriveStalledNPCTurnInput struct {
	// EncounterID identifies the encounter to check/drive.
	EncounterID string

	// PlayerID is the authenticated caller triggering the check (a
	// StreamEncounter/GetEncounter connect). load verifies membership and
	// returns ErrPlayerNotInEncounter otherwise — callers that want a
	// best-effort, non-blocking self-heal (StreamEncounter, GetEncounter)
	// should log and swallow that error rather than fail their own RPC on it,
	// since a non-member's connect-time read is a pre-existing, unrelated
	// concern this kick must not start gating.
	PlayerID core.PlayerID
}

// DriveStalledNPCTurnOutput reports whether a stalled NPC turn was found and
// driven.
type DriveStalledNPCTurnOutput struct {
	// Kicked is true when the active actor was an NPC that had not yet acted
	// and this call ran its turn (and any chained NPC turns after it) server
	// side. False means there was nothing to drive — the encounter is not
	// turn-based, has no active actor, or the active actor is already a
	// player — a legitimate, expected outcome on most calls, not an error.
	Kicked bool
}

// DriveStalledNPCTurn is the combat-entry kick (rpg-api#636): it loads the
// encounter and, if the active actor is an NPC, runs the SAME NPCAct+EndTurn
// dispatch cycle EndTurn's own NPC dispatch loop uses (driveNPCChain,
// end_turn.go) — until a player is active, the encounter ends, or the chain
// cap is reached — then persists.
//
// Why this exists: EndTurn's dispatch loop only runs on the EndTurn RPC, which
// requires the CURRENT active actor's owning player to call it. When a
// TURN_BASED encounter's FIRST active actor is an NPC (an out-of-process
// devseed --inject-combat roll, a devseed fixture where the monster wins
// initiative, or — later — a real server-side combat-entry trigger that rolls
// NPC-first), there is no preceding EndTurn to chain off of: every player is
// correctly locked out (not their turn) and nothing ever calls NPCAct. The
// encounter stalls forever. This method is the missing "something must drive
// the very first NPC" seam, called from wherever the server first touches
// such an encounter (StreamEncounter subscribe, GetEncounter — see
// handler.go's callers) instead of being patched into the dev tool: a future
// in-process combat-entry trigger can (and should) call this — or driveNPCChain
// directly — right after its own SetMode, so the same fix covers both today's
// out-of-process injection and tomorrow's real trigger.
//
// Single-flight per encounter (npcDriveLocks): StreamEncounter is a read path
// that up to N clients can hit concurrently (e.g. four players reconnecting at
// once). Without serializing, each would independently load the same
// NPC-active snapshot, and the toolkit's own "am I still the active actor"
// guard (NPCAct checks e.data.ActiveActor() against the in-memory copy IT
// loaded) does not protect across separately loaded copies — every caller's
// own load would see the NPC as active and all would dispatch its turn,
// resolving the same attack multiple times. Locking on EncounterID for the
// full load+drive+persist means only the first caller does real work; every
// other concurrent caller blocks, then (once unblocked) loads the
// ALREADY-persisted post-drive state and no-ops (Kicked=false) because the
// active actor is by then a player.
//
// Idempotent when there is nothing to drive: if the active actor is not an
// NPC (already a player, encounter not turn-based, or no active actor), this
// does not call driveNPCChain and does not persist — so a connect-time kick on
// every StreamEncounter/GetEncounter never costs an extra write on the common
// case (it's already a player's turn).
//
// Cheap pre-check (Copilot review #638): StreamEncounter/GetEncounter call
// this on EVERY connect-time read, and the common case is "already a
// player's turn" — the full o.load(WithCharacterData: true) pays for a
// character-store fetch + #598 economy seeding per player seat, which would
// otherwise run on every single connect regardless of whether there is
// anything to drive. A plain encRepo.Get + membership + raw-snapshot IsNPC
// check (cheapActiveIsNPC) happens first, INSIDE the per-encounter lock so
// the decision and the real work stay atomic; only when the raw snapshot's
// active actor is genuinely an NPC does this pay for the expensive load.
func (o *Orchestrator) DriveStalledNPCTurn(
	ctx context.Context,
	in *DriveStalledNPCTurnInput,
) (*DriveStalledNPCTurnOutput, error) {
	if in == nil {
		return nil, errors.New("encounter orchestrator: DriveStalledNPCTurnInput is required")
	}

	unlock := o.npcDriveLocks.Lock(in.EncounterID)
	defer unlock()

	data, err := o.encRepo.Get(ctx, in.EncounterID)
	if err != nil {
		if errors.Is(err, encountersv2.ErrNotFound) {
			return nil, ErrEncounterNotFound
		}
		return nil, fmt.Errorf("check stalled npc turn %q: %w", in.EncounterID, err)
	}
	if _, ok := data.Players[in.PlayerID]; !ok {
		return nil, ErrPlayerNotInEncounter
	}
	if !cheapActiveIsNPC(data) {
		return &DriveStalledNPCTurnOutput{Kicked: false}, nil
	}

	enc, err := o.load(ctx, loadInput{
		EncounterID: in.EncounterID,
		PlayerID:    in.PlayerID,
		// WithCharacterData: an NPC attack can trigger a player reaction
		// (Shield, an opportunity attack readiness check), which needs the
		// reactor's held character — same requirement as EndTurn's own load.
		WithCharacterData: true,
	})
	if err != nil {
		return nil, err
	}

	active := enc.ActiveActor()
	if active == "" || !enc.IsNPC(active) {
		// Raced with something else that advanced past the NPC between the
		// cheap check above and this real load (e.g. a concurrent EndTurn
		// outside this kick's own lock) — nothing left to drive.
		return &DriveStalledNPCTurnOutput{Kicked: false}, nil
	}

	if err := o.driveNPCChain(ctx, enc, in.EncounterID, active, true); err != nil {
		return nil, fmt.Errorf("drive stalled npc turn %q: %w", in.EncounterID, err)
	}

	return &DriveStalledNPCTurnOutput{Kicked: true}, nil
}

// cheapActiveIsNPC reports whether data's active actor is an NPC (a monster
// seat), using only the raw persisted snapshot — no character-store fetch, no
// hydration. Mirrors tkenc.Encounter's ActiveActor()/IsNPC() logic exactly
// (rpg-toolkit encounter/encounter.go), just against *Data instead of a
// loaded *Encounter, so DriveStalledNPCTurn's connect-time pre-check can skip
// the expensive o.load(WithCharacterData: true) call on the common
// "already a player's turn" case.
func cheapActiveIsNPC(data *tkenc.Data) bool {
	if data.Mode != core.ModeTurnBased || len(data.Initiative) == 0 {
		return false
	}
	if data.ActiveIdx < 0 || data.ActiveIdx >= len(data.Initiative) {
		return false
	}
	active := data.Initiative[data.ActiveIdx]
	_, isMonster := data.Monsters[active]
	return isMonster
}
