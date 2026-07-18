package lobby

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
	rpgcore "github.com/KirkDiggler/rpg-toolkit/core"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
	"github.com/KirkDiggler/rpg-toolkit/encounter/perception"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster"
	"github.com/KirkDiggler/rpg-toolkit/tools/environments"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
	"github.com/KirkDiggler/rpg-toolkit/tools/spawn"
)

// StartEncounterInput carries the entity-typed StartEncounter request.
type StartEncounterInput struct {
	// PlayerID is the authenticated caller. Must be the lobby's host.
	PlayerID string
	LobbyID  string
}

// StartEncounterOutput carries the freshly constructed encounter's ID.
// Clients drop the lobby stream and subscribe StreamEncounter(EncounterID)
// on receipt of the parallel EncounterStarted broadcast.
type StartEncounterOutput struct {
	EncounterID string
}

// spawnPositionSpacing is the per-member hex offset StartEncounter uses to
// seed distinct starting positions along one axis (Q increases, S mirrors it
// to keep the cube coordinate valid). This is a placeholder: no room/spawn-point
// system exists yet for a freshly-created lobby encounter, so members are
// spread along a line rather than stacked on a single hex. Real spawn-point
// selection is future work once room integration lands.
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

// roomWidth/roomHeight are the freshly-created encounter's room dimensions
// (rpg-api#644, The Dungeon wave 1). Deliberately large relative to
// memberSightRange (10): PatternRandom's default wall density is sparse
// (~1 wall per 10 sq units — see environments.RandomPattern) and not
// reliably enough on its own to hide a monster from a party spawned near
// the room's origin. A 20x20 room guarantees floor beyond hex-distance 10
// from a near-origin party purely by distance, so seedGoblins' PositionOracle
// (out-of-sight requirement) always has a hiding spot regardless of the
// random wall roll; walls create additional closer-in pockets on top of
// that. roomPattern is the only pattern besides "empty" the toolkit ships
// (environments.PatternRandom / PatternEmpty — see
// tools/environments/wall_patterns.go's WallPatterns registry).
const (
	roomWidth   = 20
	roomHeight  = 20
	roomPattern = environments.PatternRandom
)

// roomCenterHex returns the cube hex at the room's geometric center —
// offset (roomWidth/2, roomHeight/2) converted back to cube coordinates via
// the same core.HexFromPosition bridge seedGoblins/wallsToProto already use.
//
// rpg-api#656: the party used to spawn at raw cube-origin {0,0,0}, which
// Hex.ToPosition() maps DIRECTLY to the room's offset-coordinate origin
// (0,0) — the room's CORNER, not its center. InitRoom's room
// (environments.QuickRoom) spans offset [0,roomWidth) x [0,roomHeight)
// with no auto-centering around wherever entities get added, and the
// toolkit's Move() truncates a path the instant a step's offset position
// falls outside those bounds (space.go's truncateAtWall, indistinguishable
// from hitting a real wall — rpg-toolkit#757). A corner-spawned party has
// roughly half of the six hex directions leave the room on the very first
// step, silently no-opping the move (RPC succeeds, position unchanged) —
// reproduced and root-caused live (grpcurl repro, offset-position tracing)
// during the reconnect-fidelity wave's closing playtest. This was NOT
// introduced by #650/#655: it has existed since InitRoom's introduction in
// wave-1 (rpg-api#644) and simply was never triggered by the specific
// movement directions earlier playtests happened to exercise. Spawning the
// party at the room's center instead gives every direction roomWidth/2 (10)
// hexes of clearance — matching this file's own "near-origin party" comment
// above, which already assumed a centered party.
func roomCenterHex() core.Hex {
	return core.HexFromPosition(spatial.Position{X: float64(roomWidth) / 2, Y: float64(roomHeight) / 2})
}

// goblinCount is the fixed number of goblins StartEncounter seeds into the
// freshly-walled room (design doc fork 2, Kirk 2026-07-13: "fixed placement
// of 2 goblins via monster.NewGoblin through the fixed spawn engine";
// dynamic SpawnConfig-driven counts are wave 2+).
const goblinCount = 2

// entityGroupTypeMonster is the spawn.EntityGroup.Type tag for goblin
// placement groups in seedGoblins below.
const entityGroupTypeMonster = "monster"

// Goblin combat-snapshot constants, matching every other goblin fixture in
// this codebase (cmd/devseed/main.go, internal/pkg/devcombat/inject.go) so
// StartEncounter's real goblins behave identically to the dev-tooling ones.
const (
	monsterRefGoblin  = "dnd5e:monsters:goblin"
	goblinDamageDice  = "1d6+2"
	goblinAttackBonus = 4
	goblinSpeed       = 6 // 30ft / 5ft per hex
	goblinDamageType  = "slashing"
)

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

	// InitRoom must run before any AddPlayer/AddMonster call: AddPlayer's
	// initial reveal (perception.VisibleHexesAt) and AddMonster's inline
	// combat-entry check (perception.CanSeeAt) both consult e.room, and
	// InitRoom is what sets it (rpg-toolkit encounter/space.go's doc).
	if err := enc.InitRoom(roomWidth, roomHeight, roomPattern); err != nil {
		return nil, fmt.Errorf("init room for encounter %q: %w", encID, err)
	}

	partyBase := roomCenterHex()
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

	// Seed 2 goblins into the walled room (rpg-api#644, The Dungeon wave 1).
	// See seedGoblins' doc for why placement is verified via the toolkit's
	// real perception.CanSeeAt rather than the spawn engine's own (stubbed)
	// position search.
	if err := o.seedGoblins(ctx, enc, encID); err != nil {
		return nil, fmt.Errorf("seed goblins for encounter %q: %w", encID, err)
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

// seedGoblins selects goblinCount goblins via the tools/spawn engine (wired
// to the encounter's own RoomOrchestrator, exercising rpg-toolkit#757/#759's
// getRoomFromSpatial/placeEntityInRoom fix from a real caller) and adds them
// to enc at positions verified to be outside every current player's sight —
// via the toolkit's own wall-aware perception.CanSeeAt, the SAME predicate
// checkCombatEntry uses internally, never hand-rolled LOS math. This is the
// design doc's hard requirement (ideas/the-dungeon/design.md fork 2):
// combat must not start at spawn. AddMonster inline-checks combat entry
// (rpg-toolkit#759) on every call — a goblin added within a player's sight
// would flip the encounter to TURN_BASED immediately, so placement has to be
// pre-verified before AddMonster runs, not after.
//
// rpg-toolkit#760 (tools/spawn v0.3.0) made the position search itself
// room-aware (Room.CanPlaceEntity/bounds are consulted by the search, not
// just by a post-hoc discard-and-recompute) and added EntityGroup's
// PositionOracle: a caller predicate ANDed into that search. The
// out-of-sight requirement is expressed here as a PositionOracle closing
// over the toolkit's own perception.CanSeeAt — the same predicate this
// function's doc above already required, just handed to the engine instead
// of re-derived by a manual grid walk afterward. The engine's returned
// SpawnedEntity.Position is now used directly; there is no discarded
// position and no separate double-registration side effect (previously
// noted as inert — see PR #645 gate note N1 — now gone because there is
// only one placement per goblin, not a scattered one PopulateRoom picks
// plus a recomputed one this function used to override it with).
//
// Each goblin gets its OWN single-entity selection table and its own
// EntityGroup (Quantity.Fixed=1), rather than one shared "goblins" table
// with Quantity.Fixed=goblinCount. This is deliberate, not cosmetic:
// BasicSelectablesRegistry.GetEntities samples WITH replacement
// (selectables_registry.go: `index := r.random.Intn(len(table))` per pick,
// independently each iteration, no dedup) — a 2-entity table asked for 2
// entities can return the same *monster.Monster pointer twice. That was
// invisible to the OLD seedGoblins (it discarded the selection's identity
// entirely and matched by slice index against its own separately-tracked
// goblinIDs), but this function now correlates each SpawnedEntity back to a
// specific pre-built goblin via Entity.GetID() (see the position-matching
// loop below), so a duplicate selection would silently place one goblin
// twice and drop the other. A 1-entity table can never return a duplicate
// (there is nothing else to duplicate against), which sidesteps the gap
// entirely without depending on a toolkit-side sampling fix — these are
// individually pre-identified fixed entities (monster.NewGoblin per ID), not
// a random pick from a shared pool, so one-group-per-entity is the more
// honest modeling anyway, not a workaround bolted onto the wrong tool.
func (o *Orchestrator) seedGoblins(ctx context.Context, enc *tkenc.Encounter, encID core.EncounterID) error {
	room := enc.Room()
	if room == nil {
		return errors.New("lobby orchestrator: encounter has no room to place goblins in (InitRoom not called)")
	}

	goblinIDs := make([]core.EntityID, goblinCount)
	goblins := make([]*monster.Monster, goblinCount)
	registry := spawn.NewBasicSelectablesRegistry()
	groups := make([]spawn.EntityGroup, goblinCount)

	// The oracle re-derives the players' view each call rather than
	// snapshotting once: the search may probe many candidates per entity, and
	// re-reading enc.ToData() per candidate keeps this in lockstep with
	// whatever the engine has placed so far in this same call (no player
	// positions change mid-seed, so this is for safety against future
	// reordering, not a currently-observed staleness risk).
	//
	// Also rejects positions already occupied by an entity placed earlier in
	// THIS SAME PopulateRoom call (i.e. an already-seeded goblin): room's
	// baseline CanPlaceEntity (which the engine's search already runs)
	// type-asserts the occupant to spatial.Placeable and only blocks if
	// BlocksMovement() is true — monster.Monster implements neither, so two
	// goblins could otherwise land on the exact same hex with only
	// probability (not a guarantee) preventing it. This restores the
	// distinctness the old safeGoblinHexes gave for free by construction
	// (picking N distinct cells from one candidate set) — flagged by the gate
	// review on rpg-api#650's PR. GetEntitiesInRange(pos, 0) is an exact-cell
	// occupancy check (Distance(...) <= 0), type-agnostic, so it catches
	// players too, not just other goblins.
	validGoblinPosition := func(pos spatial.Position) bool {
		if len(room.GetEntitiesInRange(pos, 0)) > 0 {
			return false
		}
		hex := core.HexFromPosition(pos)
		for _, p := range enc.ToData().Players {
			if p.View != nil && perception.CanSeeAt(p.View, hex, room) {
				return false
			}
		}
		return true
	}

	one := 1
	for i := 0; i < goblinCount; i++ {
		id := core.EntityID(fmt.Sprintf("goblin-%d", i+1))
		g := monster.NewGoblin(string(id))
		goblinIDs[i] = id
		goblins[i] = g

		tableID := string(id)
		if err := registry.RegisterTable(tableID, []rpgcore.Entity{g}); err != nil {
			return fmt.Errorf("register goblin selection table %q: %w", tableID, err)
		}
		groups[i] = spawn.EntityGroup{
			ID:             tableID,
			Type:           entityGroupTypeMonster,
			SelectionTable: tableID,
			Quantity:       spawn.QuantitySpec{Fixed: &one},
			PositionOracle: validGoblinPosition,
		}
	}

	engine := spawn.NewBasicSpawnEngine(spawn.BasicSpawnEngineConfig{
		ID:               "startencounter-goblins",
		SelectablesReg:   registry,
		RoomOrchestrator: enc.RoomOrchestrator(),
	})

	result, err := engine.PopulateRoom(ctx, string(encID), spawn.SpawnConfig{
		EntityGroups: groups,
		Pattern:      spawn.PatternScattered,
	})
	if err != nil {
		return fmt.Errorf("spawn engine populate room: %w", err)
	}
	if len(result.Failures) > 0 {
		return fmt.Errorf("lobby orchestrator: spawn engine failed to place %d of %d goblin(s): %+v",
			len(result.Failures), goblinCount, result.Failures)
	}

	positions := make(map[core.EntityID]core.Hex, len(result.SpawnedEntities))
	for _, spawned := range result.SpawnedEntities {
		positions[core.EntityID(spawned.Entity.GetID())] = core.HexFromPosition(spawned.Position)
	}

	for i, id := range goblinIDs {
		pos, ok := positions[id]
		if !ok {
			return fmt.Errorf("lobby orchestrator: spawn engine reported no position for goblin %q", id)
		}
		goblinData := goblins[i].ToData()
		goblinDataJSON, marshalErr := json.Marshal(goblinData)
		if marshalErr != nil {
			return fmt.Errorf("marshal goblin %q data: %w", id, marshalErr)
		}
		if addErr := enc.AddMonster(tkenc.MonsterInput{
			ID:          id,
			Position:    pos,
			HP:          goblinData.HitPoints,
			MaxHP:       goblinData.MaxHitPoints,
			AC:          goblinData.ArmorClass,
			Speed:       goblinSpeed,
			MonsterRef:  monsterRefGoblin,
			AttackBonus: goblinAttackBonus,
			DamageDice:  goblinDamageDice,
			DamageType:  goblinDamageType,
			DataJSON:    goblinDataJSON,
		}); addErr != nil {
			return fmt.Errorf("add goblin %q to encounter: %w", id, addErr)
		}
	}
	return nil
}
