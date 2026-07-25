package lobby

import (
	"context"
	"errors"
	"fmt"

	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
	rpgcore "github.com/KirkDiggler/rpg-toolkit/core"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
	"github.com/KirkDiggler/rpg-toolkit/encounter/perception"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

// StartEncounterInput carries the entity-typed StartEncounter request.
type StartEncounterInput struct {
	// PlayerID is the authenticated caller. Must be the lobby's host.
	PlayerID string
	LobbyID  string

	// DungeonKey selects the toolkit InitDungeon spec this encounter is
	// built from (rpg-api#688). The zero value resolves to
	// defaultDungeonKey — "a named default for now" per the issue's Scope
	// section; no proto field carries a caller-supplied key yet
	// (lobby-settings-driven selection is future work), so every real
	// handler call leaves this at the zero value.
	DungeonKey DungeonKey

	// RandomSeed reproduces the WHOLE dungeon layout when non-zero,
	// passed straight through to tkenc.DungeonParams.RandomSeed —
	// entropy-seeded otherwise (matches every other InitRoom/InitDungeon
	// caller in this codebase). No proto field carries this yet either;
	// zero value is what every real handler call leaves it at.
	RandomSeed int64
}

// StartEncounterOutput carries the freshly constructed encounter's ID.
// Clients drop the lobby stream and subscribe StreamEncounter(EncounterID)
// on receipt of the parallel EncounterStarted broadcast.
type StartEncounterOutput struct {
	EncounterID string
}

// spawnPositionSpacing is the per-member hex offset StartEncounter uses to
// seed distinct starting positions along one axis (Q increases, S mirrors it
// to keep the cube coordinate valid). This is a placeholder: no per-member
// spawn-point system exists yet, so members are spread along a line rather
// than stacked on a single hex. The line runs from the chamber-1 entrance
// toward the chamber's interior (core.Hex{Q+1,R,S-1} per step — verified
// against tools/spatial's pointy-top offset conversion to move away from the
// entrance's edge column, not off the grid), so it stays safe regardless of
// where in the chamber the entrance sits. Real spawn-point selection is
// future work once room integration lands further.
const spawnPositionSpacing = 1

// memberSightRange is the initial perception radius seeded for every member
// added to a freshly-started encounter (rpg-api#632). Without it,
// tkenc.PlayerInput.SightRange defaults to 0 and AddPlayer's initial reveal
// (encounter.go's VisibleHexesAt(pos, 0)) shows each player exactly one hex,
// so party members can never see each other. 10 matches the devseed fixture
// (cmd/devseed/main.go) for parity between the harness and real lobby-started
// encounters; character-derived vision is future work (deferred per
// rpg-api#632's correction comment).
const memberSightRange = 10

// chamberWidth/chamberHeight sized EACH chamber of the retired two-chamber
// dungeon (The Dungeon wave 2 Slice 2, rpg-api#676) — replaced by
// resolveDungeonSpec (dungeon_spec.go), keyed by DungeonKey instead of
// hardcoded per-call constants (rpg-api#688). rpg-api#694 goes further for
// the crypt key: resolveDungeonSpec forwards to the toolkit's own
// tkenc.CryptDungeonParams constructor rather than holding an API-owned
// literal region/height/obstacle spec — no geometry constants remain in
// this file.

// entityGroupTypeMonster is the spawn.EntityGroup.Type tag for monster
// placement groups (crypt_monster_seed.go's seedRegionMonsters).
const entityGroupTypeMonster = "monster"

// rpg-api#689 (2026-07-23 approved first-swing simplification) retired the
// old goblinsPerRegion/monsterRefGoblin out-of-sight goblin seeding
// entirely — every populated region got the SAME fixed goblin count by
// chain POSITION regardless of archetype. crypt_monster_seed.go replaces
// it with an archetype-driven table (one deterministic-anchor skeleton
// minion in the entrance region, zero in the corridor, one deterministic-
// anchor non-wight skeleton-captain boss — rpg-toolkit#816) seeded via
// spawn.FixedPositions only, with no PositionOracle search/retry.

// StartEncounter is the lobby -> encounter seam. Host-only, all-ready
// gated, atomic member-set snapshot (guarded by the per-lobby lock so a
// racing LeaveLobby lands either before this snapshot — member excluded —
// or after — FailedPrecondition, lobby-surface.md "Start/leave atomicity").
//
// This subsumes the deleted v1alpha2 CreateEncounter RPC (lobby-surface.md
// "StartEncounter subsumes CreateEncounter"): it is now the ONLY way an
// encounter comes into existence. It builds a fresh toolkit encounter, adds
// one player per ready member (HP and AC seeded from the character store,
// generalizing the single-caller seedPlayerHP the old handler-layer
// CreateEncounter used — see seedMemberCombatSnapshot in character.go),
// persists it ONCE to the encounter repo, transitions the lobby to STARTED,
// and only then publishes EncounterStarted. Persist-then-emit ordering is
// load-bearing: a client reacting to EncounterStarted must find the
// encounter already in the encounter repo.
func (o *Orchestrator) StartEncounter(ctx context.Context, in *StartEncounterInput) (*StartEncounterOutput, error) {
	if in == nil {
		return nil, errors.New("lobby orchestrator: StartEncounterInput is required")
	}

	// effectiveKey substitutes Task E2b's RPG_DUNGEON_KEY override ONLY
	// when the caller supplied no key at all — an explicit in.DungeonKey
	// (once a real proto surface exists to set one) always wins.
	// o.dungeonKeyOverride is "" when RPG_DUNGEON_KEY was unset at
	// startup, so this is a no-op today for every real caller (zero
	// player-facing change, per design.md) — no real handler builds
	// in.DungeonKey from the request yet, so every real call leaves it at
	// the zero value and gets whatever the override resolves to (or the
	// legacy default when there's no override either).
	effectiveKey := in.DungeonKey
	if effectiveKey == "" {
		effectiveKey = o.dungeonKeyOverride
	}

	// Content-backed resolution (Task E2) runs FIRST, before touching the
	// lobby lock or building anything — same "fail loudly, zero side
	// effects" posture resolveDungeonSpec below already has. A content key
	// that's FOUND but disabled (its file failed dungeonspec.Load at
	// startup) must return immediately here; a key content doesn't
	// recognize at all falls through to the legacy resolveDungeonSpec/
	// dungeonSpecs path below, completely untouched by this branch.
	contentCompiled, foundInContent, contentErr := o.resolveContentDungeonSpec(effectiveKey)
	if foundInContent && contentErr != nil {
		return nil, contentErr // *DisabledDungeonKeyError; lobbyStatusError maps it (Task E3)
	}

	// The legacy path only runs when effectiveKey isn't content-backed at
	// all — resolveDungeonSpec ITSELF is untouched: it still applies its
	// own hardcoded defaultDungeonKey fallback for an empty key, and
	// effectiveKey is only empty here when the caller supplied none
	// (today, every real caller), so today's exact fallback behavior
	// (crypt) is unchanged for every key content doesn't claim.
	var dungeonKey DungeonKey
	var dungeonParams tkenc.DungeonParams
	if !foundInContent {
		var specErr error
		dungeonKey, dungeonParams, specErr = resolveDungeonSpec(effectiveKey, in.RandomSeed)
		if specErr != nil {
			return nil, specErr
		}
	}

	unlock := o.locks.Lock(in.LobbyID)
	defer unlock()

	data, err := o.lobbyRepo.Get(ctx, in.LobbyID)
	if err != nil {
		if errors.Is(err, lobbyrepo.ErrNotFound) {
			return nil, ErrLobbyNotFound
		}
		return nil, fmt.Errorf("load lobby %q: %w", in.LobbyID, err)
	}
	if data.Status == lobbyrepo.StatusStarted {
		return nil, ErrLobbyAlreadyStarted
	}
	if data.HostPlayerID != in.PlayerID {
		return nil, ErrNotHost
	}
	members := orderedMembers(data)
	for _, m := range members {
		if !m.IsReady {
			return nil, ErrNotAllReady
		}
	}

	encID := core.EncounterID(o.encounterIDGen.Generate())
	enc := tkenc.New(ctx, encID, o.encounterBroker,
		tkenc.WithCharacterResolver(o.characterResolver),
		tkenc.WithCombatResolver(o.buildCombatResolver(nil)),
		tkenc.WithMovementResolver(o.buildMovementResolver(nil)),
	)

	// InitDungeon must run before any AddPlayer/AddMonster call: AddPlayer's
	// initial reveal (perception.VisibleHexesAt) and AddMonster's inline
	// combat-entry check (perception.CanSeeAt) both consult e.room, and
	// InitDungeon is what sets it (via AddDoor's internal
	// rebuildRoomFromData — rpg-toolkit encounter/space.go's doc). Building
	// the dungeon (N regions + N-1 plain doors + entrance + per-region
	// archetype tags + opaque theme + physical set-piece obstacles) is
	// entirely the toolkit's call — rpg-api only supplies the resolved
	// key's toolkit-constructed params verbatim (rpg-api#688, rpg-api#694),
	// or (Task E2) the content-compiled params verbatim.
	if foundInContent {
		// compiled.Params.RandomSeed is a FIELD dungeonspec.Load leaves at
		// its zero value (Load has no opinion on reproducibility) — the
		// caller must set it before InitDungeon, exactly like
		// resolveDungeonSpec already threads in.RandomSeed into the
		// legacy path's params below.
		contentCompiled.Params.RandomSeed = in.RandomSeed
		if err := enc.InitDungeon(contentCompiled.Params); err != nil {
			return nil, fmt.Errorf("init dungeon (key=%q) for encounter %q: %w", effectiveKey, encID, err)
		}
	} else if err := enc.InitDungeon(dungeonParams); err != nil {
		return nil, fmt.Errorf("init dungeon (key=%q) for encounter %q: %w", dungeonKey, encID, err)
	}

	// Entrance-anchored spawn (rpg-api#648, rpg-api#676, preserved by
	// rpg-api#688): the party drops just inside the entrance region's
	// designated anchor, not the room's geometric center — SpaceData.
	// Entrance is the toolkit's designated spawn-anchor cell for exactly
	// this purpose (InitDungeon's doc).
	partyBase := enc.ToData().Space.Entrance
	for i, m := range members {
		snap, snapErr := o.seedMemberCombatSnapshot(ctx, m.CharacterID)
		if snapErr != nil {
			return nil, fmt.Errorf("seed combat snapshot for character %q: %w", m.CharacterID, snapErr)
		}
		q := i * spawnPositionSpacing
		if addErr := enc.AddPlayer(tkenc.PlayerInput{
			PlayerID:   core.PlayerID(m.PlayerID),
			EntityID:   core.EntityID(m.CharacterID),
			Position:   core.Hex{Q: partyBase.Q + q, R: partyBase.R, S: partyBase.S - q},
			SightRange: memberSightRange,
			HP:         snap.hp,
			MaxHP:      snap.maxHP,
			AC:         snap.ac,
			// AttackBonus/DamageDice/DamageType stay zero: a stored character
			// carries no precomputed field for them (real rules math, derived
			// at attack time from equipped weapon + ability scores +
			// proficiency bonus — see memberCombatSnapshot's doc comment in
			// character.go). isPlayerCombatant (rpg-toolkit combat.go) treats
			// a hydrated seat as combat-ready instead of gating on these flat
			// fields; the v2 encounter orchestrator's characterData.Attach
			// cascade rehydrates this player from EntityID on every
			// combat-capable RPC (hydrate_players.go), independent of what
			// StartEncounter seeds here.
		}); addErr != nil {
			return nil, fmt.Errorf("add member %q to encounter: %w", m.PlayerID, addErr)
		}
	}

	// Seed monsters — Task E2's content-backed branch uses the toolkit's
	// generic Encounter.SeedMonsters against the compiled spawn plan
	// (dungeonspec's own placed-instruction batching/atomic-combat-entry
	// invariants); the legacy crypt branch is UNCHANGED: one
	// deterministic-anchor minion in the entrance region, zero in the
	// corridor, one deterministic-anchor boss in the boss region — via
	// spawn.FixedPositions only (crypt_monster_seed.go's
	// seedRegionMonsters/buildMonsterSeedGroups/regionMonsterAnchor), no
	// PositionOracle search/retry. Both branches run AFTER every member has
	// been added above (SeedMonsters' documented caller-ordering
	// requirement; seedRegionMonsters already relies on the same thing via
	// perception.CanSeeAt's combat-entry check).
	if foundInContent {
		if err := enc.SeedMonsters(contentCompiled.Spawns); err != nil {
			return nil, fmt.Errorf("seed monsters for encounter %q: %w", encID, err)
		}
	} else if err := o.seedRegionMonsters(ctx, enc, encID, dungeonParams, dungeonKey); err != nil {
		return nil, fmt.Errorf("seed monsters for encounter %q: %w", encID, err)
	}

	if err := o.encounterRepo.Save(ctx, enc.ToData()); err != nil {
		return nil, fmt.Errorf("save encounter %q: %w", encID, err)
	}

	data.Status = lobbyrepo.StatusStarted
	data.EncounterID = string(encID)
	if err := o.lobbyRepo.Save(ctx, data); err != nil {
		return nil, fmt.Errorf("save lobby %q: %w", in.LobbyID, err)
	}

	// Persist-then-emit: both the encounter and the lobby's STARTED state are
	// durable above this line before any client can be told to switch streams.
	o.lobbyBroker.Publish(in.LobbyID, &Event{
		Kind:             EventKindEncounterStarted,
		EncounterStarted: &EncounterStartedPayload{EncounterID: string(encID)},
	})

	return &StartEncounterOutput{EncounterID: string(encID)}, nil
}

// regionEntryAnchor returns the hex a player first stands at when
// stepping INTO the named region through door — the door's neighbor cell
// that SpaceData.RegionAt itself tags into that region: the first hex
// actually inside it once stepped through. Uses only toolkit-exposed
// surface — perception.HexNeighbors (the canonical six-neighbor set)
// filtered by SpaceData.RegionAt, and (when room is non-nil)
// room.CanPlaceEntity — never hand-rolled hex-grid geometry, direction, or
// orientation reasoning.
//
// rpg-api#688: generalized from the retired chamberEntryAnchor, which
// special-cased the FIRST chamber by hardcoding a tkenc.RegionChamber1
// comparison (itself a Copilot-review fix on PR #677 for an earlier,
// worse hardcode — see region_entry_anchor_internal_test.go's doc). That
// special case doesn't generalize to an N-region chain at all.
//
// rpg-api#689: now called for BOTH the entrance and boss anchors (via
// crypt_monster_seed.go's regionMonsterAnchor/regionAnchorDoor), against
// whichever connector door borders each region — not special-cased to any
// one region by position. This is the ONLY piece of the retired
// seedGoblins path this file keeps, but the CONTRACT is hardened over the
// #688 version:
//
// EVIDENCE-BACKED HARDENING (rpg-api#689, 2026-07-23): a region's
// RegionData.Hexes tags its WHOLE rectangular footprint (including any
// interior wall cells PatternRandom placed off the region's OWN required-
// path row — see rpg-toolkit's dungeon.go doc). perception.HexNeighbors'
// fixed six-direction iteration order is NOT guaranteed to check the
// required-path-row neighbor before an off-path diagonal one; which
// direction happens to be "safe" depends on which side of the chain the
// door is on, an orientation detail rpg-api must not reason about
// (crypt_dungeon_params_fixture_internal_test.go's
// TestSeedRegionMonsters_AgainstRealCryptDungeonParams_SeedSweep_1To1000
// caught this empirically: 40/1000 seeds' ENTRANCE anchor — always the
// entrance's own OUTGOING-door neighbor, never the boss's incoming-door
// neighbor — landed on a real interior wall cell one row off the door's
// row, against rpg-toolkit's own CryptDungeonParams dimensions). See the
// PR body for the filed rpg-toolkit follow-up proposing the toolkit expose
// a per-region anchor analogous to SpaceData.Entrance so this room-
// validity fallback becomes unnecessary.
//
// Until that seam lands, this function additionally verifies each
// region-tagged candidate against room.CanPlaceEntity (the SAME toolkit-
// exposed validity check spawn.EntityGroup.FixedPositions itself already
// performs before use) and skips a candidate that fails it, trying the
// NEXT of the fixed six neighbors — a bounded, deterministic disambiguation
// among an already-known, already-computed candidate set, not a search:
// no relaxed constraints, no retries beyond these six, no randomness. room
// may be nil (preserves the #688 pure region-tag-matching contract for
// callers/tests with no real room, e.g. region_entry_anchor_internal_
// test.go's synthetic single-hex-region fixtures).
//
// KNOWN TRAP (rpg-api#676, from #675's devcombat gate note, still true):
// the door cell itself belongs to NEITHER region (RegionAt(door.Position)
// is ("", false)) — it is not a valid anchor on its own, hence walking its
// neighbors here rather than using door.Position directly.
func regionEntryAnchor(space *tkenc.SpaceData, door *tkenc.DoorData, regionID string, room spatial.Room) (core.Hex, error) {
	for _, n := range perception.HexNeighbors(door.Position) {
		id, ok := space.RegionAt(n)
		if !ok || id != regionID {
			continue
		}
		if room != nil && !room.CanPlaceEntity(anchorProbeEntity{}, n.ToPosition()) {
			continue
		}
		return n, nil
	}
	return core.Hex{}, fmt.Errorf(
		"lobby orchestrator: door %q has no walkable %q-side neighbor to anchor seeding", door.ID, regionID)
}

// anchorProbeEntity is a zero-footprint core.Entity used ONLY to query
// room.CanPlaceEntity for a candidate anchor hex — it is never placed. A
// dedicated probe type (rather than reusing any real game entity) makes
// clear this is a read-only validity check, not a placement side effect.
type anchorProbeEntity struct{}

func (anchorProbeEntity) GetID() string               { return "anchor-probe" }
func (anchorProbeEntity) GetType() rpgcore.EntityType { return "anchor-probe" }
