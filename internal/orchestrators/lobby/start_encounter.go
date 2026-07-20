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

// chamberWidth/chamberHeight size EACH chamber of the two-chamber dungeon
// (The Dungeon wave 2 Slice 2, rpg-api#676) — not the combined space, which
// InitTwoChamberRoom computes as 2*chamberWidth+1 wide. Deliberately large
// relative to memberSightRange (10), same rationale wave 1's single room
// carried forward: PatternRandom's default wall density is sparse enough
// that raw chamber size, not just walls, needs to give seedGoblins'
// out-of-sight PositionOracle room to find a hiding spot. chamberPattern is
// the only pattern besides "empty" the toolkit ships (environments.
// PatternRandom / PatternEmpty — tools/environments/wall_patterns.go's
// WallPatterns registry).
const (
	chamberWidth   = 20
	chamberHeight  = 20
	chamberPattern = environments.PatternRandom
)

// chamberDoorID is the entity id of the single door connecting chamber 1
// and chamber 2 (Slice 2 — one plain door; Slice 3 adds a second, locked
// door to a boss chamber). Interact(chamberDoorID) is how a client opens
// it; project.go's wallsToProto projects it onto the wire as a
// DOOR_CLOSED/DOOR_OPEN Wall carrying this same id (types.proto's Wall.id
// bridge, rpg-api-protos#186).
const chamberDoorID core.EntityID = "door-chamber1-chamber2"

// goblinsPerChamber is the fixed number of goblins StartEncounter seeds
// into EACH chamber (The Dungeon wave 2 Slice 2 design doc §Q5/closing
// playtest script: "chamber-1 goblins" / "chamber-2 goblins" — both
// plural). This doubles wave 1's single-room goblinCount (2) across the two
// chambers rather than splitting it, so each chamber is a real fight once
// found, not a lone straggler; dynamic SpawnConfig-driven counts remain
// future work.
const goblinsPerChamber = 2

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

	// InitTwoChamberRoom must run before any AddPlayer/AddMonster call:
	// AddPlayer's initial reveal (perception.VisibleHexesAt) and AddMonster's
	// inline combat-entry check (perception.CanSeeAt) both consult e.room, and
	// InitTwoChamberRoom is what sets it (via AddDoor's internal
	// rebuildRoomFromData — rpg-toolkit encounter/space.go's doc). Building the
	// two-chamber dungeon (2 chambers + 1 plain door + entrance + per-chamber
	// region tags) is entirely the toolkit's call — rpg-api only supplies
	// sizing/pattern/door-id by key (The Dungeon wave 2 Slice 2, rpg-api#676).
	// RandomSeed stays zero (entropy-seeded), matching InitRoom's own default
	// (rpg-toolkit#787) — no devseed fixture needs a fixed dungeon layout yet.
	if err := enc.InitTwoChamberRoom(tkenc.TwoChamberRoomParams{
		ChamberWidth:  chamberWidth,
		ChamberHeight: chamberHeight,
		Pattern:       chamberPattern,
		DoorID:        chamberDoorID,
	}); err != nil {
		return nil, fmt.Errorf("init two-chamber room for encounter %q: %w", encID, err)
	}

	// Entrance-anchored spawn (rpg-api#648, rpg-api#676): the party drops just
	// inside chamber 1's entrance, not the room's geometric center —
	// SpaceData.Entrance is the toolkit's designated spawn-anchor cell for
	// exactly this purpose (InitTwoChamberRoom's doc). This retires
	// roomCenterHex() and the wall-adjacent-spawn debt it carried (wave 1,
	// rpg-api#656) — the two-chamber generator's own required-path guarantee
	// (entrance-to-door corridor) makes a center placeholder unnecessary.
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

	// Seed goblinsPerChamber goblins into EACH chamber, out of sight, region-
	// tag-scoped (The Dungeon wave 2 Slice 2). See seedGoblins' doc for why
	// placement is verified via the toolkit's real perception.CanSeeAt rather
	// than the spawn engine's own (stubbed) position search.
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

// chamberSpawnSpec pairs a two-chamber region with the out-of-sight anchor
// its goblins are seeded against (see chamberEntryAnchor) and the id prefix
// its goblins get, so seedGoblins can drive both chambers through one
// shared path instead of duplicating the loop.
type chamberSpawnSpec struct {
	regionID string
	anchor   core.Hex
	idPrefix string
}

// chamberEntryAnchor returns the hex a player first stands at when arriving
// IN the named region — chamber 1's designated Entrance, or (for any other
// region) the door's neighbor cell that RegionAt itself tags into that same
// region: the first hex actually inside it once stepped through. This
// generalizes wave 1's single room-center out-of-sight anchor (The Dungeon
// wave 2 design doc §Q5: "the same perception.CanSeeAt oracle wave-1 uses
// from the room center — generalized to the door") to each chamber's own
// entry point, using only toolkit-exposed surface — perception.HexNeighbors
// (the canonical six-neighbor set) filtered by SpaceData.RegionAt — never
// hand-rolled hex-grid geometry.
//
// Fixed (Copilot review, PR #677): this used to hardcode the neighbor match
// to tkenc.RegionChamber1 regardless of which regionID was asked for, so
// chamber 2's anchor was silently a chamber-1 hex — violating the "entry
// hex FOR THE NAMED region" contract this doc promises. It happened to be
// harmless today only because the door is always closed at seed time (a
// closed door fully blocks LoS at its own cell — rpg-toolkit#790 — so
// virtually nothing in chamber 2 is visible from EITHER side of it,
// masking the wrong anchor); it stops being harmless the moment any caller
// reuses this helper somewhere that assumption doesn't hold. Matching
// against regionID itself (not a hardcoded chamber-1 constant) is also the
// only way this generalizes to a 3rd region (Slice 3's boss chamber)
// without another hardcoded case.
//
// KNOWN TRAP (rpg-api#676, from #675's devcombat gate note): the door cell
// itself belongs to NEITHER region (RegionAt(door.Position) is ("", false))
// — it is not a valid anchor on its own, hence walking its neighbors here
// rather than using door.Position directly.
func chamberEntryAnchor(space *tkenc.SpaceData, door *tkenc.DoorData, regionID string) (core.Hex, error) {
	if regionID == tkenc.RegionChamber1 {
		return space.Entrance, nil
	}
	for _, n := range perception.HexNeighbors(door.Position) {
		if id, ok := space.RegionAt(n); ok && id == regionID {
			return n, nil
		}
	}
	return core.Hex{}, fmt.Errorf(
		"lobby orchestrator: door %q has no %q-side neighbor to anchor its out-of-sight seeding",
		door.ID, regionID)
}

// chamberOutOfSight builds a spawn.PositionOracle that accepts only
// candidate hexes tagged into regionID (SpaceData.RegionAt) which are (a)
// unoccupied — room.GetEntitiesInRange(pos, 0), an exact-cell occupancy
// check, NOT a player-visibility one; see the KNOWN TRAP note on
// seedGoblins below for why that distinction matters — and (b) not
// currently visible from anchor, nor from any already-seated player's real
// View, via the toolkit's own wall-aware perception.CanSeeAt — the SAME
// predicate checkCombatEntry uses internally, never hand-rolled LOS math.
func chamberOutOfSight(enc *tkenc.Encounter, room spatial.Room, regionID string, anchor core.Hex) spawn.PositionOracle {
	anchorView := &perception.View{Position: anchor, SightRange: memberSightRange}
	return func(pos spatial.Position) bool {
		if len(room.GetEntitiesInRange(pos, 0)) > 0 {
			return false
		}
		hex := core.HexFromPosition(pos)
		if id, ok := enc.ToData().Space.RegionAt(hex); !ok || id != regionID {
			return false
		}
		if perception.CanSeeAt(anchorView, hex, room) {
			return false
		}
		for _, p := range enc.ToData().Players {
			if p.View != nil && perception.CanSeeAt(p.View, hex, room) {
				return false
			}
		}
		return true
	}
}

// seedGoblins selects goblinsPerChamber goblins PER CHAMBER (region-tag-
// scoped, The Dungeon wave 2 Slice 2) via the tools/spawn engine (wired to
// the encounter's own RoomOrchestrator, exercising rpg-toolkit#757/#759's
// getRoomFromSpatial/placeEntityInRoom fix from a real caller) and adds
// them to enc at positions verified to be outside every current player's
// sight and outside that chamber's own entry point — via
// chamberOutOfSight/chamberEntryAnchor above. This is the design doc's hard
// requirement (ideas/the-dungeon/design.md fork 2, carried into wave 2):
// combat must not start at spawn, in EITHER chamber. AddMonster
// inline-checks combat entry (rpg-toolkit#759) on every call — a goblin
// added within a player's sight would flip the encounter to TURN_BASED
// immediately, so placement has to be pre-verified before AddMonster runs,
// not after.
//
// KNOWN TRAP (rpg-api#676, carried over from #675's devcombat work):
// room.GetEntitiesInRange is BLIND to players — it only sees spatial-grid
// entities (walls, other monsters), never the player seats this package
// tracks separately in PlayerData. It is used above ONLY for exact-cell
// occupancy dedup between already-placed goblins (mirroring wave 1's
// safeGoblinHexes distinctness guarantee), never to validate a candidate
// against player positions — that job belongs to perception.CanSeeAt
// (chamberOutOfSight), the only predicate that actually sees players.
//
// rpg-toolkit#760 (tools/spawn v0.3.0) made the position search itself
// room-aware (Room.CanPlaceEntity/bounds are consulted by the search, not
// just by a post-hoc discard-and-recompute) and added EntityGroup's
// PositionOracle: a caller predicate ANDed into that search. The engine's
// returned SpawnedEntity.Position is used directly; there is no discarded
// position and no separate double-registration side effect.
//
// Each goblin gets its OWN single-entity selection table and its own
// EntityGroup (Quantity.Fixed=1), rather than one shared table with
// Quantity.Fixed=goblinsPerChamber. This is deliberate, not cosmetic:
// BasicSelectablesRegistry.GetEntities samples WITH replacement
// (selectables_registry.go: `index := r.random.Intn(len(table))` per pick,
// independently each iteration, no dedup) — a multi-entity table asked for
// N entities can return the same *monster.Monster pointer twice. This
// function correlates each SpawnedEntity back to a specific pre-built
// goblin via Entity.GetID() (see the position-matching loop below), so a
// duplicate selection would silently place one goblin twice and drop
// another. A 1-entity table can never return a duplicate (there is nothing
// else to duplicate against) — these are individually pre-identified fixed
// entities (monster.NewGoblin per ID), not a random pick from a shared
// pool, so one-group-per-entity is the more honest modeling anyway.
func (o *Orchestrator) seedGoblins(ctx context.Context, enc *tkenc.Encounter, encID core.EncounterID) error {
	room := enc.Room()
	if room == nil {
		return errors.New("lobby orchestrator: encounter has no room to place goblins in (InitTwoChamberRoom not called)")
	}
	space := enc.ToData().Space
	if space == nil {
		return errors.New("lobby orchestrator: encounter has no space snapshot to seed goblins into")
	}
	door, ok := enc.ToData().Doors[chamberDoorID]
	if !ok {
		return fmt.Errorf("lobby orchestrator: door %q not found (InitTwoChamberRoom must add it)", chamberDoorID)
	}

	specs := make([]chamberSpawnSpec, 0, 2)
	for _, regionID := range []string{tkenc.RegionChamber1, tkenc.RegionChamber2} {
		anchor, anchorErr := chamberEntryAnchor(space, door, regionID)
		if anchorErr != nil {
			return anchorErr
		}
		specs = append(specs, chamberSpawnSpec{regionID: regionID, anchor: anchor, idPrefix: regionID})
	}

	totalGoblins := goblinsPerChamber * len(specs)
	goblinIDs := make([]core.EntityID, 0, totalGoblins)
	goblins := make([]*monster.Monster, 0, totalGoblins)
	registry := spawn.NewBasicSelectablesRegistry()
	groups := make([]spawn.EntityGroup, 0, totalGoblins)
	one := 1

	for _, spec := range specs {
		oracle := chamberOutOfSight(enc, room, spec.regionID, spec.anchor)
		for i := 0; i < goblinsPerChamber; i++ {
			id := core.EntityID(fmt.Sprintf("goblin-%s-%d", spec.idPrefix, i+1))
			g := monster.NewGoblin(string(id))
			goblinIDs = append(goblinIDs, id)
			goblins = append(goblins, g)

			tableID := string(id)
			if err := registry.RegisterTable(tableID, []rpgcore.Entity{g}); err != nil {
				return fmt.Errorf("register goblin selection table %q: %w", tableID, err)
			}
			groups = append(groups, spawn.EntityGroup{
				ID:             tableID,
				Type:           entityGroupTypeMonster,
				SelectionTable: tableID,
				Quantity:       spawn.QuantitySpec{Fixed: &one},
				PositionOracle: oracle,
			})
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
			len(result.Failures), totalGoblins, result.Failures)
	}

	positions := make(map[core.EntityID]core.Hex, len(result.SpawnedEntities))
	for _, spawned := range result.SpawnedEntities {
		positions[core.EntityID(spawned.Entity.GetID())] = core.HexFromPosition(spawned.Position)
	}

	for i, id := range goblinIDs {
		pos, posOK := positions[id]
		if !posOK {
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
