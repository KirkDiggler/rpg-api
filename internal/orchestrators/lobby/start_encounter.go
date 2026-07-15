package lobby

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand"

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
// from a near-origin party purely by distance, so safeGoblinHexes always
// has a hiding spot regardless of the random wall roll; walls create
// additional closer-in pockets on top of that. roomPattern is the only
// pattern besides "empty" the toolkit ships (environments.PatternRandom /
// PatternEmpty — see tools/environments/wall_patterns.go's WallPatterns
// registry).
const (
	roomWidth   = 20
	roomHeight  = 20
	roomPattern = environments.PatternRandom
)

// goblinCount is the fixed number of goblins StartEncounter seeds into the
// freshly-walled room (design doc fork 2, Kirk 2026-07-13: "fixed placement
// of 2 goblins via monster.NewGoblin through the fixed spawn engine";
// dynamic SpawnConfig-driven counts are wave 2+).
const goblinCount = 2

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

	for i, m := range members {
		snap, snapErr := o.seedMemberCombatSnapshot(ctx, m.CharacterID)
		if snapErr != nil {
			return nil, fmt.Errorf("seed combat snapshot for character %q: %w", m.CharacterID, snapErr)
		}
		q := i * spawnPositionSpacing
		if addErr := enc.AddPlayer(tkenc.PlayerInput{
			PlayerID:   core.PlayerID(m.PlayerID),
			EntityID:   core.EntityID(m.CharacterID),
			Position:   core.Hex{Q: q, R: 0, S: -q},
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
// KNOWN TOOLKIT GAP (flag for a follow-up rpg-toolkit issue): the spawn
// engine's own position search — BasicSpawnEngine.findValidPosition (a raw
// random X,Y in [0,10), zero room awareness) and, when SpatialRules are
// configured, ConstraintSolver.FindValidPositions (a hardcoded 1.0-10.0
// grid sweep) — never calls room.CanPlaceEntity or room.IsLineOfSightBlocked
// against the actual spatial.Room this wave built. Its optional LineOfSight
// constraint is a Euclidean-distance stub (tools/spawn/constraints.go's
// hasLineOfSight: "Real implementation would use spatial room queries for
// wall intersections" — distance <= 8.0, walls not considered at all).
// SpawnConfig also has no fixed/target-position injection point for a
// caller that already knows where it wants an entity. So PopulateRoom's own
// SpawnedEntity.Position is unusable for this wave's correctness bar and is
// deliberately discarded below in favor of a position computed from real
// toolkit predicates (safeGoblinHexes). PopulateRoom is still called (not
// bypassed) so the RoomOrchestrator wiring is genuinely exercised end to
// end; this does leave each goblin also registered as a room occupant at
// that discarded scattered position, a side effect that's inert for wave 1
// (monsters aren't otherwise tracked in the spatial room — only walls
// occupy it, see rpg-toolkit encounter/space.go's wallCheckEntity doc — and
// nothing in this flow queries room occupancy for monster positions).
func (o *Orchestrator) seedGoblins(ctx context.Context, enc *tkenc.Encounter, encID core.EncounterID) error {
	goblinIDs := make([]core.EntityID, goblinCount)
	goblins := make([]*monster.Monster, goblinCount)
	entities := make([]rpgcore.Entity, goblinCount)
	for i := 0; i < goblinCount; i++ {
		id := core.EntityID(fmt.Sprintf("goblin-%d", i+1))
		g := monster.NewGoblin(string(id))
		goblinIDs[i] = id
		goblins[i] = g
		entities[i] = g
	}

	registry := spawn.NewBasicSelectablesRegistry()
	if err := registry.RegisterTable("goblins", entities); err != nil {
		return fmt.Errorf("register goblin selection table: %w", err)
	}
	engine := spawn.NewBasicSpawnEngine(spawn.BasicSpawnEngineConfig{
		ID:               "startencounter-goblins",
		SelectablesReg:   registry,
		RoomOrchestrator: enc.RoomOrchestrator(),
	})
	quantity := goblinCount
	if _, err := engine.PopulateRoom(ctx, string(encID), spawn.SpawnConfig{
		EntityGroups: []spawn.EntityGroup{{
			ID:             "goblins",
			Type:           "monster",
			SelectionTable: "goblins",
			Quantity:       spawn.QuantitySpec{Fixed: &quantity},
		}},
		Pattern: spawn.PatternScattered,
	}); err != nil {
		return fmt.Errorf("spawn engine populate room: %w", err)
	}

	safeHexes, err := safeGoblinHexes(enc, goblinCount)
	if err != nil {
		return err
	}

	for i, id := range goblinIDs {
		goblinData := goblins[i].ToData()
		goblinDataJSON, marshalErr := json.Marshal(goblinData)
		if marshalErr != nil {
			return fmt.Errorf("marshal goblin %q data: %w", id, marshalErr)
		}
		if addErr := enc.AddMonster(tkenc.MonsterInput{
			ID:          id,
			Position:    safeHexes[i],
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

// safeGoblinHexes returns n distinct hexes within the encounter's room that
// are (a) not wall-occupied (room.CanPlaceEntity) and (b) not currently
// visible to ANY player in the encounter, verified via the toolkit's own
// wall-aware perception.CanSeeAt. Enumerates every offset-coordinate cell in
// the room's bounds — core.HexFromPosition is the same offset<->cube bridge
// rpg-toolkit's rebuildRoomFromData uses to place walls, so this walks
// exactly the grid the room's wall placements are snapped to.
func safeGoblinHexes(enc *tkenc.Encounter, n int) ([]core.Hex, error) {
	room := enc.Room()
	if room == nil {
		return nil, errors.New("lobby orchestrator: encounter has no room to place goblins in (InitRoom not called)")
	}
	data := enc.ToData()

	var candidates []core.Hex
	for x := 0; x < roomWidth; x++ {
		for y := 0; y < roomHeight; y++ {
			hex := core.HexFromPosition(spatial.Position{X: float64(x), Y: float64(y)})
			if !room.CanPlaceEntity(placementProbe{}, hex.ToPosition()) {
				continue // wall-occupied
			}
			visibleToSomeone := false
			for _, p := range data.Players {
				if p.View != nil && perception.CanSeeAt(p.View, hex, room) {
					visibleToSomeone = true
					break
				}
			}
			if !visibleToSomeone {
				candidates = append(candidates, hex)
			}
		}
	}
	if len(candidates) < n {
		return nil, fmt.Errorf(
			"lobby orchestrator: found only %d safe (unwalled, unseen) hex(es) for %d goblins in a %dx%d room",
			len(candidates), n, roomWidth, roomHeight)
	}

	// Random, distinct pick from the safe set so goblins scatter rather than
	// always landing in the same corner. #nosec G404: game placement, not
	// security-sensitive.
	rand.Shuffle(len(candidates), func(i, j int) { candidates[i], candidates[j] = candidates[j], candidates[i] })
	return candidates[:n], nil
}

// placementProbe is a throwaway core.Entity used only to query
// room.CanPlaceEntity for wall-occupancy. Mirrors rpg-toolkit's own
// wallCheckEntity (encounter/space.go), which is unexported: goblins
// themselves are never placed into the spatial room (only walls occupy it —
// see that same doc), so this identity is never compared against a real
// occupant.
type placementProbe struct{}

func (placementProbe) GetID() string               { return "placement-probe" }
func (placementProbe) GetType() rpgcore.EntityType { return "placement-probe" }
