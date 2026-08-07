// Package integration_test's dungeon_crypt_test.go is the rpg-api#688 gate
// (superseding the retired rpg-api#676 two-chamber gate, dungeon_two_
// chamber_test.go's former name): the crypt dungeon spec (rpg-toolkit#814's
// Approved Slice 3 corrections, selected by DungeonKeyCrypt/the default key)
// end-to-end through the REAL LobbyService.StartEncounter + v1alpha2
// EncounterService RPC surface, not a hand-seeded tkenc.New/AddPlayer
// fixture. It proves the same four gate facts the two-chamber suite proved,
// generalized from 2 regions/1 door to the crypt spec's 3 regions/2 doors:
// (1) the party spawns at the entrance region's designated entrance, not a
// room center; (2) a connector door projects on the wire as a DOOR_CLOSED
// Wall carrying an id; (3) a Move that sights the entrance-region monster
// starts a COMBAT POCKET scoped to the entrance region — the boss is absent
// from initiative; (4) the boss exists but emits no EntityAppeared until
// BOTH connector doors open AND a sightline actually forms (opening alone
// doesn't reveal it).
//
// rpg-api#689 (2026-07-23 approved first-swing simplification) changed the
// composition this suite exercises: retired 2 goblins/region (out-of-sight
// PositionOracle search) for exactly 1 deterministic-anchor skeleton in the
// entrance region, 0 in the corridor, and exactly 1 deterministic-anchor
// non-wight skeleton-captain boss (rpg-toolkit#816) — no goblins anywhere.
// The four gate facts above are unchanged (they're about doors/regions/
// zones, not monster count or ref) — only the monster-count/ref assertions
// below were updated; every "goblin" identifier was renamed to "monster" for
// accuracy (the composition is no longer plural).
package integration_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc/metadata"

	lobbyv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/lobby/v1alpha1"
	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	"github.com/KirkDiggler/rpg-api/internal/integration/harness"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
	"github.com/KirkDiggler/rpg-toolkit/encounter/perception"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	"github.com/KirkDiggler/rpg-toolkit/tools/environments"
)

// dungeonGateHP is deliberately generous (not the usual 12 HP other
// fixtures use): these tests deliberately walk the lone party member onto
// a monster's own hex to force a sighted Move, which can flip the encounter
// to TURN_BASED and let the toolkit's driveNPCChain resolve a real (random)
// monster attack synchronously inside the same MoveEntity call
// (move_entity.go's doc). A low-HP single-player fixture risks a flaky TPK
// (ModeEnded instead of the TURN_BASED pocket this suite asserts) purely
// from unlucky dice; 100 HP absorbs worst-case skeleton-captain multiattack
// damage (2x 1d8+3, at most a couple of rounds) with room to spare.
const (
	dungeonGateHP              = 100
	cryptEntranceDoorID string = "crypt-door-entrance-corridor"
	cryptBossDoorID     string = "crypt-door-corridor-boss"
)

type DungeonCryptSuite struct {
	suite.Suite
	ctx     context.Context
	cancel  context.CancelFunc
	srv     *harness.TestServer
	release func()
}

func TestDungeonCryptSuite(t *testing.T) {
	suite.Run(t, new(DungeonCryptSuite))
}

func (s *DungeonCryptSuite) SetupTest() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 2*time.Minute)

	// Lease the package's shared Redis container (rpg-api#699) — see
	// main_test.go. Released in TearDownTest.
	s.release = sharedRedis.Lease()

	var err error
	s.srv, err = harness.NewWithRedis(s.ctx, nil, sharedRedis.Addr)
	s.Require().NoError(err, "failed to create test server")
	s.Require().NoError(s.srv.FlushRedis(s.ctx), "failed to flush shared redis")
}

func (s *DungeonCryptSuite) TearDownTest() {
	if s.srv != nil {
		s.srv.Close()
	}
	if s.cancel != nil {
		s.cancel()
	}
	if s.release != nil {
		s.release()
	}
}

func (s *DungeonCryptSuite) authCtx(playerID string) context.Context {
	return metadata.AppendToOutgoingContext(s.ctx, "authorization", "Dev "+playerID)
}

// startCryptDungeon creates a character + lobby, readies it, and starts the
// encounter via the REAL LobbyService.StartEncounter RPC — the same path a
// real client drives, not a hand-seeded tkenc fixture. StartEncounter's
// DungeonKey defaults to the crypt spec (no proto field carries a key yet
// — rpg-api#688's Scope section). Returns the encounter id, the seated
// character id, and the freshly persisted encounter data for direct
// inspection.
func (s *DungeonCryptSuite) startCryptDungeon(suffix, playerID string) (encounterID, characterID string, encData *tkenc.Data) {
	characterID = "char-" + suffix
	_, err := s.srv.CharacterRepo.Create(s.ctx, characterrepo.CreateInput{
		Character: &entities.Character{
			Data: &toolkitchar.Data{
				ID: characterID, PlayerID: playerID, Name: "Alice",
				HitPoints: dungeonGateHP, MaxHitPoints: dungeonGateHP,
			},
		},
	})
	s.Require().NoError(err)

	createResp, err := s.srv.LobbyClient.CreateLobby(s.authCtx(playerID), &lobbyv1alpha1.CreateLobbyRequest{
		CampaignId: "campaign-" + suffix, CharacterId: characterID,
	})
	s.Require().NoError(err)
	lobbyID := createResp.GetLobbyId()

	_, err = s.srv.LobbyClient.SetReady(s.authCtx(playerID), &lobbyv1alpha1.SetReadyRequest{
		LobbyId: lobbyID, Ready: true,
	})
	s.Require().NoError(err)

	startResp, err := s.srv.LobbyClient.StartEncounter(s.authCtx(playerID), &lobbyv1alpha1.StartEncounterRequest{
		LobbyId: lobbyID,
	})
	s.Require().NoError(err, "StartEncounter must succeed via the real RPC path")
	encounterID = startResp.GetEncounterId()
	s.Require().NotEmpty(encounterID)

	encData, err = s.srv.EncRepoV2.Get(s.ctx, encounterID)
	s.Require().NoError(err)
	return encounterID, characterID, encData
}

// regionArchetypeAt returns the RegionArchetype of the region containing
// hex, and whether one was found — the archetype-keyed sibling of
// SpaceData.RegionAt (which only returns the spec-specific region ID
// string). Used throughout this suite so assertions key off the toolkit's
// fixed generic-role vocabulary instead of rpg-api's own unexported
// per-spec region ID constants (cryptRegionIDEntrance etc. live in
// internal/orchestrators/lobby, unexported, by design).
func regionArchetypeAt(space *tkenc.SpaceData, hex core.Hex) (tkenc.RegionArchetype, bool) {
	for _, r := range space.Regions {
		if r.Hexes.Has(hex) {
			return r.Archetype, true
		}
	}
	return "", false
}

// hexRecordAt finds the HexRecord at the given cube position in hexes, or
// nil if the viewer has no knowledge of that hex at all — the new contract's
// per-hex knowledge gate (rpg-api-protos#197) means a hex nobody has
// explored or seen is simply absent, not present-with-empty-edges.
func hexRecordAt(hexes []*encounterv2pb.HexRecord, h core.Hex) *encounterv2pb.HexRecord {
	for _, rec := range hexes {
		p := rec.GetPosition()
		if p.GetX() == int32(h.Q) && p.GetY() == int32(h.R) && p.GetZ() == int32(h.S) {
			return rec
		}
	}
	return nil
}

// doorEdgeByID scans every hex record's Edges for one carrying doorID —
// edges now live on the HexRecord they sit on rather than in a flat
// Space.Walls list (rpg-api-protos#197), so a door's wire wall is wherever
// its own hex record happens to be, not a fixed lookup.
func doorEdgeByID(hexes []*encounterv2pb.HexRecord, doorID string) *encounterv2pb.Wall {
	for _, rec := range hexes {
		for _, w := range rec.GetEdges() {
			if w.GetId() == doorID {
				return w
			}
		}
	}
	return nil
}

// archetypeMonsters splits encData.Monsters by their SpaceData region
// archetype.
func archetypeMonsters(encData *tkenc.Data) (entrance, boss []core.EntityID) {
	for id, m := range encData.Monsters {
		archetype, ok := regionArchetypeAt(encData.Space, m.Position)
		if !ok {
			continue
		}
		switch archetype {
		case tkenc.ArchetypeEntrance:
			entrance = append(entrance, id)
		case tkenc.ArchetypeBoss:
			boss = append(boss, id)
		}
	}
	return entrance, boss
}

func TestPlanWalk_UsesAlternateRouteAndRespectsClosedDoor(t *testing.T) {
	a := core.Hex{Q: 0, R: 0, S: 0}
	b := core.Hex{Q: 1, R: 0, S: -1}
	c := core.Hex{Q: 0, R: 1, S: -1}
	d := core.Hex{Q: 1, R: 1, S: -2}
	e := core.Hex{Q: 2, R: 0, S: -2}
	space := &tkenc.SpaceData{
		Regions:   []tkenc.RegionData{{Hexes: core.NewHexSet(a, b, c, e)}},
		Walls:     []environments.WallSegmentData{{Start: b.ToCube(), BlocksMovement: true}},
		Obstacles: []tkenc.ObstacleData{{Position: b, BlocksMovement: true}},
	}
	doors := map[core.EntityID]*tkenc.DoorData{"door": {Position: d}}

	path, err := planWalk(space, doors, a, e)
	if err == nil {
		t.Fatalf("closed door path unexpectedly succeeded: %#v", path)
	}
	doors["door"].Open = true
	path, err = planWalk(space, doors, a, e)
	if err != nil {
		t.Fatal(err)
	}
	if len(path) != 3 || path[0] != c || path[1] != d || path[2] != e {
		t.Fatalf("got %#v, want alternate route [%+v %+v %+v]", path, c, d, e)
	}
	previous := a
	for _, step := range path {
		if !isHexNeighbor(previous, step) {
			t.Fatalf("non-adjacent step %+v -> %+v", previous, step)
		}
		if step == b {
			t.Fatalf("path used movement blocker %+v", b)
		}
		previous = step
	}
}

// planWalk is test-only BFS over persisted encounter geometry. It deliberately
// emits adjacent destinations so this suite never relies on the toolkit's
// current acceptance of non-contiguous submitted Move paths.
func planWalk(space *tkenc.SpaceData, doors map[core.EntityID]*tkenc.DoorData, from, target core.Hex) ([]core.Hex, error) {
	legal := make(map[core.Hex]bool)
	for _, region := range space.Regions {
		for _, h := range region.Hexes.Slice() {
			legal[h] = true
		}
	}
	blocked := make(map[core.Hex]bool)
	for _, door := range doors {
		if door.Open {
			// Connector cells sit between RegionData hex sets, so an open door
			// is the sole legal-cell exception needed to join adjacent regions.
			legal[door.Position] = true
		} else {
			blocked[door.Position] = true
		}
	}
	for _, wall := range space.Walls {
		// isDegenerateBlockingWall (lobby_start_then_move_test.go, same
		// package) is the shared rpg-api#704 filter: a boundary-edge
		// perimeter segment (Start != End) is real walkable floor with a
		// render-only wall on one edge, never an actual blocker cell.
		if isDegenerateBlockingWall(wall) {
			blocked[core.HexFromCube(wall.Start)] = true
		}
	}
	for _, o := range space.Obstacles {
		if o.BlocksMovement {
			blocked[o.Position] = true
		}
	}
	if !legal[from] || !legal[target] || blocked[target] {
		return nil, fmt.Errorf("no walkable route from %+v to %+v", from, target)
	}
	previous := map[core.Hex]core.Hex{}
	seen := map[core.Hex]bool{from: true}
	queue := []core.Hex{from}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current == target {
			path := make([]core.Hex, 0)
			for current != from {
				path = append([]core.Hex{current}, path...)
				current = previous[current]
			}
			return path, nil
		}
		for _, next := range perception.HexNeighbors(current) {
			if legal[next] && !blocked[next] && !seen[next] {
				seen[next] = true
				previous[next] = current
				queue = append(queue, next)
			}
		}
	}
	return nil, fmt.Errorf("no walkable route from %+v to %+v", from, target)
}

// moveAlongPath reloads persisted geometry and replans after every real RPC.
// Each submitted destination is adjacent and chunks contain at most one turn's
// six 5ft steps; closed doors and all persisted blockers are avoided locally.
func (s *DungeonCryptSuite) moveAlongPath(encounterID, characterID string, target core.Hex, turnCap int) error {
	turns := 0
	for {
		data, err := s.srv.EncRepoV2.Get(s.ctx, encounterID)
		if err != nil {
			return err
		}
		alicePD := data.Players[core.PlayerID("alice")]
		from := alicePD.View.Position
		if from == target {
			return nil
		}
		path, err := planWalk(data.Space, data.Doors, from, target)
		if err != nil {
			return err
		}
		if len(path) > 6 {
			path = path[:6]
		}
		proposed := make([]*encounterv2pb.Position, len(path))
		previous := from
		for i, step := range path {
			if !isHexNeighbor(previous, step) {
				return fmt.Errorf("planned non-adjacent step %+v -> %+v", previous, step)
			}
			proposed[i] = &encounterv2pb.Position{X: int32(step.Q), Y: int32(step.R), Z: int32(step.S)}
			previous = step
		}
		_, moveErr := s.srv.EncounterClientV2.MoveEntity(s.authCtx("alice"), &encounterv2pb.MoveEntityRequest{
			EncounterId: encounterID, EntityId: characterID, ProposedPath: proposed,
		})
		if moveErr == nil {
			after, err := s.srv.EncRepoV2.Get(s.ctx, encounterID)
			if err != nil {
				return err
			}
			if after.Players[core.PlayerID("alice")].View.Position == from {
				return fmt.Errorf("move toward %+v made no progress from %+v", target, from)
			}
			continue
		}
		if data.Mode != core.ModeTurnBased || turns == turnCap {
			return moveErr
		}
		if _, endErr := s.srv.EncounterClientV2.EndTurn(s.authCtx("alice"), &encounterv2pb.EndTurnRequest{
			EncounterId: encounterID, EntityId: characterID,
		}); endErr != nil {
			return fmt.Errorf("end turn while walking to %+v: %w", target, endErr)
		}
		turns++
	}
}

func isHexNeighbor(from, to core.Hex) bool {
	for _, neighbor := range perception.HexNeighbors(from) {
		if neighbor == to {
			return true
		}
	}
	return false
}

// walkAdjacentToDoor moves alice to a hex adjacent to doorID before any
// Interact call (rpg-toolkit#864: OpenDoor/AttemptUnlock now require
// adjacency). Tries each of the door's 6 neighbors via moveAlongPath until
// one is reachable — the "wrong side" neighbors (across the still-closed
// door, potentially in an unexplored/unconnected region) are expected to
// fail planWalk's BFS; the near-side neighbor succeeds. A no-op if alice is
// already there (moveAlongPath returns immediately when from == target).
func (s *DungeonCryptSuite) walkAdjacentToDoor(encounterID, characterID, doorID string) {
	data, err := s.srv.EncRepoV2.Get(s.ctx, encounterID)
	s.Require().NoError(err)
	door, ok := data.Doors[core.EntityID(doorID)]
	s.Require().True(ok, "door %q must exist in the encounter", doorID)

	var walkErr error
	for _, neighbor := range perception.HexNeighbors(door.Position) {
		if walkErr = s.moveAlongPath(encounterID, characterID, neighbor, 10); walkErr == nil {
			return
		}
	}
	s.Require().NoError(walkErr, "no reachable approach hex adjacent to door %q", doorID)
}

// openCryptDoor uses the existing Interact -> SubmitCheck path when the
// toolkit-owned crypt connector is locked. It supplies only the d20 roll; the
// toolkit owns the configured DC, ability, tool, and success side effect.
//
// rpg-toolkit#864: Interact (OpenDoor/AttemptUnlock) now requires the actor
// to be adjacent to the door, so this walks alice there first via
// walkAdjacentToDoor — she was previously interacting from wherever she
// happened to be (sometimes her spawn point, hexes away), which is exactly
// the unvalidated-range shortcut #864 closes. This does not affect any of
// this suite's sightline/reveal assertions: walking to be ADJACENT to a
// still-closed door does not grant a sightline PAST it (that still requires
// the door to open AND a further Move forming line of sight), which is what
// every "must not reveal" assertion in this file actually depends on.
func (s *DungeonCryptSuite) openCryptDoor(encounterID, characterID, doorID string) {
	s.walkAdjacentToDoor(encounterID, characterID, doorID)
	resp, err := s.srv.EncounterClientV2.Interact(s.authCtx("alice"), &encounterv2pb.InteractRequest{
		EncounterId: encounterID, TargetEntityId: doorID,
	})
	s.Require().NoError(err, "opening door %q must succeed", doorID)
	if resp.GetInputRequired() == nil {
		return
	}
	resolved, err := s.srv.EncounterClientV2.SubmitCheck(s.authCtx("alice"), &encounterv2pb.SubmitCheckRequest{
		EncounterId: encounterID, EntityId: characterID, Roll: 20,
	})
	s.Require().NoError(err, "unlocking door %q must succeed", doorID)
	s.Require().True(resolved.GetSuccess(), "roll 20 must resolve the production boss lock")
}

// TestStartEncounter_SpawnsAtEntrance_ProjectsCanonicalCryptDoors verifies
// the production crypt's toolkit-owned connector contract: the entrance stays
// plain and closed, while the boss connector starts locked and closed.
func (s *DungeonCryptSuite) TestStartEncounter_SpawnsAtEntrance_ProjectsCanonicalCryptDoors() {
	encounterID, _, encData := s.startCryptDungeon("gate1", "alice")

	alicePD, ok := encData.Players[core.PlayerID("alice")]
	s.Require().True(ok, "alice must be seated in the encounter")
	s.Require().NotNil(alicePD.View)
	s.Require().Equal(encData.Space.Entrance, alicePD.View.Position,
		"StartEncounter must spawn the party at the entrance region's designated anchor, not a room center")

	resp, err := s.srv.EncounterClientV2.GetEncounter(s.authCtx("alice"), &encounterv2pb.GetEncounterRequest{
		EncounterId: encounterID,
	})
	s.Require().NoError(err)

	entrance := encData.Doors[core.EntityID(cryptEntranceDoorID)]
	boss := encData.Doors[core.EntityID(cryptBossDoorID)]
	s.Require().NotNil(entrance)
	s.Require().NotNil(boss)
	s.Require().False(entrance.Locked)
	s.Require().False(entrance.Open)
	s.Require().True(boss.Locked)
	s.Require().False(boss.Open)
	s.Require().Equal(12, boss.LockDC)
	s.Require().Equal("dex", boss.LockAbility)
	s.Require().Empty(boss.LockTool)

	// Edges now ride the HexRecord they sit on, gated by hex knowledge
	// (rpg-api-protos#197) — unlike the retired unconditional
	// doorWallsToProto, a door only reaches the wire once its own hex is
	// known to the viewer. Alice spawns at the entrance, immediately
	// adjacent to the entrance connector, so it is known and visible; the
	// boss connector sits two closed doors and a whole corridor away and
	// must NOT appear at all — the same gate fact (4) this file's
	// TestBossMonster_NoEntityAppeared_UntilBothDoorsOpenAndSightlinesForm
	// proves for the boss monster, now true of the boss door too.
	hexes := resp.GetEncounter().GetSpace().GetHexes()
	entranceEdge := doorEdgeByID(hexes, cryptEntranceDoorID)
	s.Require().NotNil(entranceEdge, "the entrance connector is adjacent to the spawn and must project")
	s.Require().Equal(encounterv2pb.WallKind_WALL_KIND_DOOR_CLOSED, entranceEdge.GetKind())

	s.Require().Nil(doorEdgeByID(hexes, cryptBossDoorID),
		"the boss connector must not leak onto the wire before alice has ever seen it")
}

// TestBossLock_FailedThenSuccessfulCheck_ProjectsPersistedDoorState proves the
// production crypt reuses the established unlock flow. The API supplies only
// the d20 roll; all lock configuration and resolution remain toolkit-owned.
//
// The wire-projection half of this test changed under rpg-api-protos#197,
// and again under rpg-toolkit#864 (see the KNOWN LIMITATION note on the test
// itself): originally this test never moved alice at all — it Interacted
// with the boss door directly by id from her entrance spawn, two closed
// connector doors away, proving that a door's locked state doesn't leak onto
// the wire "unconditionally" (the "every viewer's snapshot carries every
// door regardless of RevealedHexes" leak TestStartEncounter_
// SpawnsAtEntrance_ProjectsCanonicalCryptDoors's boss-door assertion closes).
// #864 requires Interact/AttemptUnlock to be adjacency-gated, and adjacency
// to a door reveals it — so "Interact with a door alice has never seen" is
// no longer reachable; alice must walk to the door (through the now-open
// entrance connector) before interacting. Persisted state (verified directly
// via EncRepoV2.Get below) remains the authoritative proof the failed/
// successful checks did the right thing; the remaining wire assertion (at
// spawn, before any movement) is the part of the original wire-leak proof
// that's still meaningful post-#864.
// KNOWN LIMITATION (rpg-toolkit#864 rebase, flagged for coordinator review —
// not resolved unilaterally by this fork): this test's wire-projection half
// used to prove a door alice had NEVER SEEN could still be Interact-ed
// (server-authoritative check, no leak onto the wire). rpg-toolkit#864 now
// requires adjacency for Interact/AttemptUnlock, and adjacency to a door
// necessarily reveals that door's own hex (confirmed empirically: standing
// next to the still-locked boss door DOES put its Wall/edge on the wire) —
// so "attempt a check against a door you've never seen" is no longer a
// reachable scenario at all, and the original "must not leak even after a
// failed/successful check attempt" assertions are now testing something
// that can't happen. This rewrite keeps a genuine wire-leak proof (the boss
// door is invisible from spawn, before any movement) and keeps the fully
// unaffected persisted-state assertions (Locked/Open via EncRepoV2.Get), but
// drops the two "still doesn't leak after interacting" assertions rather
// than leave them silently vacuous. If the pre-#864 wire-leak-while-
// unseen contract still matters, closing that gap needs a design call, not
// a mechanical test fix — flagging rather than deciding.
func (s *DungeonCryptSuite) TestBossLock_FailedThenSuccessfulCheck_ProjectsPersistedDoorState() {
	encounterID, characterID, _ := s.startCryptDungeon("boss-lock", "alice")

	// Baseline: from spawn, before any movement, the boss door must not be
	// on the wire at all — this is the wire-leak proof that's still valid
	// under #864 (genuinely unseen, not merely "un-interacted-with").
	spawnProjection, err := s.srv.EncounterClientV2.GetEncounter(s.authCtx("alice"), &encounterv2pb.GetEncounterRequest{EncounterId: encounterID})
	s.Require().NoError(err)
	s.Require().Nil(doorEdgeByID(spawnProjection.GetEncounter().GetSpace().GetHexes(), cryptBossDoorID),
		"the boss door must not project onto the wire from spawn, before alice has ever seen it")

	// rpg-toolkit#864: AttemptUnlock now requires adjacency. The entrance
	// connector (unlocked, per TestStartEncounter's own assertion) is a
	// genuine physical blocker between alice's spawn and the boss door —
	// walkAdjacentToDoor's BFS cannot route through a still-closed door —
	// so it must be opened first.
	s.openCryptDoor(encounterID, characterID, cryptEntranceDoorID)
	s.walkAdjacentToDoor(encounterID, characterID, cryptBossDoorID)

	prompt, err := s.srv.EncounterClientV2.Interact(s.authCtx("alice"), &encounterv2pb.InteractRequest{
		EncounterId: encounterID, TargetEntityId: cryptBossDoorID,
	})
	s.Require().NoError(err)
	s.Require().NotNil(prompt.GetInputRequired().GetSkillCheck())
	s.Require().Equal(int32(12), prompt.GetInputRequired().GetSkillCheck().GetDc())
	s.Require().Equal("dex", prompt.GetInputRequired().GetSkillCheck().GetAbility())
	s.Require().Nil(prompt.GetInputRequired().GetSkillCheck().GetTool())

	failed, err := s.srv.EncounterClientV2.SubmitCheck(s.authCtx("alice"), &encounterv2pb.SubmitCheckRequest{
		EncounterId: encounterID, EntityId: characterID, Roll: 1,
	})
	s.Require().NoError(err)
	s.Require().False(failed.GetSuccess())

	afterFailure, err := s.srv.EncRepoV2.Get(s.ctx, encounterID)
	s.Require().NoError(err)
	s.Require().True(afterFailure.Doors[core.EntityID(cryptBossDoorID)].Locked)
	s.Require().False(afterFailure.Doors[core.EntityID(cryptBossDoorID)].Open)

	_, err = s.srv.EncounterClientV2.Interact(s.authCtx("alice"), &encounterv2pb.InteractRequest{
		EncounterId: encounterID, TargetEntityId: cryptBossDoorID,
	})
	s.Require().NoError(err)
	succeeded, err := s.srv.EncounterClientV2.SubmitCheck(s.authCtx("alice"), &encounterv2pb.SubmitCheckRequest{
		EncounterId: encounterID, EntityId: characterID, Roll: 20,
	})
	s.Require().NoError(err)
	s.Require().True(succeeded.GetSuccess())

	afterSuccess, err := s.srv.EncRepoV2.Get(s.ctx, encounterID)
	s.Require().NoError(err)
	s.Require().False(afterSuccess.Doors[core.EntityID(cryptBossDoorID)].Locked)
	s.Require().True(afterSuccess.Doors[core.EntityID(cryptBossDoorID)].Open)
}

// TestEntranceSighting_StartsPocketScopedCombat is gate fact (3): a Move
// that brings an entrance-region monster into sight starts a combat pocket
// scoped to the entrance region — the toolkit's LoS-scoped rollInitiative
// (rpg-toolkit#796, Slice 1b) must NOT drag the still-unsighted boss (two
// closed connector doors away) into the same initiative roster, or the
// whole dungeon rolls into one un-endable fight (design doc gap 5).
// TestEntranceSighting_StartsPocketScopedCombat is gate fact (3): sighting
// an entrance-region monster starts a combat pocket scoped to the entrance
// region — the toolkit's LoS-scoped rollInitiative (rpg-toolkit#796, Slice
// 1b) must NOT drag the still-unsighted boss (two closed connector doors
// away) into the same initiative roster, or the whole dungeon rolls into
// one un-endable fight (design doc gap 5).
//
// rpg-api#694/#689 merge finding (2026-07-23): with the compact crypt
// dimensions, the entrance monster's deterministic anchor (near its own
// outgoing door — regionMonsterAnchor's doc) sits close enough to spawn
// that reaching it can need more than one turn's movement budget once
// combat is active — moveAlongPath replans persisted-geometry BFS routes
// after each real MoveEntity chunk (at most six adjacent destinations) and
// ends turns only when the turn-based movement budget requires it.
func (s *DungeonCryptSuite) TestEntranceSighting_StartsPocketScopedCombat() {
	encounterID, characterID, encData := s.startCryptDungeon("gate2", "alice")
	entranceIDs, bossIDs := archetypeMonsters(encData)
	s.Require().Len(entranceIDs, 1, "rpg-api#689: exactly one deterministic-anchor monster in the entrance region")
	s.Require().Len(bossIDs, 1, "rpg-api#689: exactly one deterministic-anchor boss")
	s.Require().Equal(refs.Monsters.Skeleton().String(), encData.Monsters[entranceIDs[0]].MonsterRef,
		"the entrance monster must be the plain skeleton, never a goblin")
	s.Require().Equal(refs.Monsters.SkeletonCaptain().String(), encData.Monsters[bossIDs[0]].MonsterRef,
		"the boss must be rpg-toolkit#816's non-wight skeleton captain")

	sightedMonster := entranceIDs[0]
	target := encData.Monsters[sightedMonster].Position

	err := s.moveAlongPath(encounterID, characterID, target, 10)
	s.Require().NoError(err, "moving onto a pre-vetted entrance-region monster hex must not be blocked")

	after, err := s.srv.EncRepoV2.Get(s.ctx, encounterID)
	s.Require().NoError(err)
	s.Require().Equal(core.ModeTurnBased, after.Mode,
		"sighting an entrance-region monster (at spawn or via an explicit Move) must start a combat pocket (TURN_BASED)")

	// Pocket-scoping (design doc: "initiative scoped to engaged (LoS-having)
	// monsters") means only the sighted monster joins initiative — the boss
	// (definitely unsighted — both connector doors are still closed) must be
	// asserted OUT, which is the direction that actually proves pocket-
	// scoping (gap 5: the whole dungeon must NOT roll into one un-endable
	// initiative).
	inInitiative := make(map[core.EntityID]bool, len(after.Initiative))
	for _, id := range after.Initiative {
		inInitiative[id] = true
	}
	s.Require().True(inInitiative[sightedMonster], "the sighted entrance-region monster %q must be in the pocket's initiative", sightedMonster)
	for _, id := range bossIDs {
		s.Require().False(inInitiative[id], "boss-region monster %q must NOT be dragged into the entrance pocket", id)
	}
}

// TestBossMonster_NoEntityAppeared_UntilBothDoorsOpenAndSightlinesForm is
// gate fact (4): the boss exists (seeded at StartEncounter) but emits no
// EntityAppeared to alice — neither in her connect-time replay, nor
// after either connector door opens individually, nor after BOTH open —
// until a sightline actually forms. Opening a door alone is not enough
// (Slice 1's progressive through-the-doorway reveal, not a whole-region
// reveal); this test specifically distinguishes "both doors opened" from
// "sightline formed" by checking for EntityAppeared after each step
// separately.
func (s *DungeonCryptSuite) TestBossMonster_NoEntityAppeared_UntilBothDoorsOpenAndSightlinesForm() {
	encounterID, characterID, encData := s.startCryptDungeon("gate3", "alice")
	_, bossIDs := archetypeMonsters(encData)
	s.Require().Len(bossIDs, 1, "rpg-api#689: exactly one deterministic-anchor boss")
	bossMonster := bossIDs[0]

	s.Require().Len(encData.Doors, 2, "the crypt spec's 3-region chain has exactly 2 connector doors")

	stream, err := s.srv.EncounterClientV2.StreamEncounter(s.authCtx("alice"),
		&encounterv2pb.StreamEncounterRequest{EncounterId: encounterID})
	s.Require().NoError(err)

	// ONE background reader for the stream's whole lifetime, drained at each
	// checkpoint below via drainAvailable — see streamEvents/drainAvailable's
	// own docs (shared with lobby_start_then_move_test.go's sibling suites in
	// this package) for why a fresh reader per checkpoint would race.
	events := s.streamEvents(stream)

	sawBossMonster := func(batch []*encounterv2pb.EncounterEvent) bool {
		for _, ev := range batch {
			if hkc := ev.GetHexKnowledgeChanged(); hkc != nil {
				for _, e := range hkc.GetEntities() {
					if e.GetId() == string(bossMonster) {
						return true
					}
				}
			}
		}
		return false
	}

	// Drain the connect-time replay: SnapshotDelivered + one HexKnowledgeChanged
	// carrying alice's own entity + her revealed hexes (rpg-api-protos#197
	// collapsed the old per-entity EntityAppeared + whole-set GeometryRevealed
	// burst into this single event — BuildReplayEvents' doc). The boss must
	// not be among the disclosed entities — both connector doors are closed,
	// and it is out of alice's entrance-region sight regardless.
	replay := drainAvailable(events, 500*time.Millisecond)
	s.Require().False(sawBossMonster(replay), "boss-region monster must not appear in the connect-time replay")

	// Open the entrance connector first. This alone must NOT reveal the boss
	// — it is still one closed door and a whole corridor away.
	//
	// rpg-toolkit#864: must be opened in PHYSICAL traversal order now
	// (entrance, then boss) rather than the old alphabetically-sorted
	// doorIDs[0]/doorIDs[1] — walkAdjacentToDoor's BFS (inside
	// openCryptDoor) cannot route through a still-closed door, and the
	// alphabetical sort happened to put the boss door first. Neither
	// assertion below cares WHICH door is "first", only that opening one
	// (then both) doesn't leak the boss — unaffected by the reordering.
	s.openCryptDoor(encounterID, characterID, cryptEntranceDoorID)
	afterFirstDoor := drainAvailable(events, 500*time.Millisecond)
	s.Require().False(sawBossMonster(afterFirstDoor),
		"opening one connector door alone must not reveal the boss-region monster")

	// Open the boss connector too. Both doors open STILL must not
	// reveal the boss by itself — only the doorway's own progressive
	// reveal (Slice 1), not a whole-region one; a sightline must still form.
	s.openCryptDoor(encounterID, characterID, cryptBossDoorID)
	afterSecondDoor := drainAvailable(events, 500*time.Millisecond)
	s.Require().False(sawBossMonster(afterSecondDoor),
		"opening both connector doors alone must not reveal the boss-region monster — a sightline must still form")

	// Now form a sightline using only adjacent destinations over the real
	// MoveEntity RPC path. Both open doors are traversable; no test state is
	// mutated to get through the dungeon.
	after, err := s.srv.EncRepoV2.Get(s.ctx, encounterID)
	s.Require().NoError(err)
	target := after.Monsters[bossMonster].Position

	err = s.moveAlongPath(encounterID, characterID, target, 10)
	s.Require().NoError(err, "moving through both open connector doors onto the boss monster's hex must not be blocked")

	postMove := drainAvailable(events, 1500*time.Millisecond)
	s.Require().True(sawBossMonster(postMove),
		"a sightline that actually forms (alice standing on the boss monster's hex) must emit EntityAppeared")
}

func (s *DungeonCryptSuite) TestStaticObstacles_ProjectOnlyWhenRevealedAndRemainExplored() {
	encounterID, characterID, encData := s.startCryptDungeon("obstacles", "alice")
	s.Require().NotEmpty(encData.Space.Obstacles, "#697 must persist canonical crypt obstacles")

	bossObstacleIDs := make(map[string]bool)
	for _, obstacle := range encData.Space.Obstacles {
		archetype, ok := regionArchetypeAt(encData.Space, obstacle.Position)
		if ok && archetype == tkenc.ArchetypeBoss {
			bossObstacleIDs[string(obstacle.ID)] = true
		}
	}
	s.Require().NotEmpty(bossObstacleIDs, "crypt must persist boss-region obstacle instances")

	initial, err := s.srv.EncounterClientV2.GetEncounter(s.authCtx("alice"), &encounterv2pb.GetEncounterRequest{EncounterId: encounterID})
	s.Require().NoError(err)
	initialObstacleIDs := map[string]bool{}
	for _, entity := range initial.GetEncounter().GetSpace().GetEntities() {
		if entity.GetType() == encounterv2pb.EntityType_ENTITY_TYPE_OBSTACLE {
			initialObstacleIDs[entity.GetId()] = true
		}
	}
	for id := range bossObstacleIDs {
		s.Require().False(initialObstacleIDs[id], "hidden boss obstacle %q must not leak in the initial snapshot", id)
	}

	stream, err := s.srv.EncounterClientV2.StreamEncounter(s.authCtx("alice"), &encounterv2pb.StreamEncounterRequest{EncounterId: encounterID})
	s.Require().NoError(err)
	events := s.streamEvents(stream)
	_ = drainAvailable(events, 500*time.Millisecond)

	s.Require().Len(encData.Doors, 2)
	// rpg-toolkit#864: physical traversal order (entrance, then boss), not
	// an alphabetical sort — see TestBossMonster_...'s identical note.
	s.openCryptDoor(encounterID, characterID, cryptEntranceDoorID)
	_ = drainAvailable(events, 500*time.Millisecond)
	s.openCryptDoor(encounterID, characterID, cryptBossDoorID)
	_ = drainAvailable(events, 500*time.Millisecond)

	current, err := s.srv.EncRepoV2.Get(s.ctx, encounterID)
	s.Require().NoError(err)
	var revealed tkenc.ObstacleData
	var target core.Hex
	for _, obstacle := range current.Space.Obstacles {
		if !bossObstacleIDs[string(obstacle.ID)] || obstacle.BlocksLoS {
			continue
		}
		for _, neighbor := range perception.HexNeighbors(obstacle.Position) {
			if _, pathErr := planWalk(current.Space, current.Doors, current.Players["alice"].View.Position, neighbor); pathErr == nil {
				revealed = obstacle
				target = neighbor
				break
			}
		}
		if revealed.ID != "" {
			break
		}
	}
	s.Require().NotEmpty(revealed.ID, "a boss obstacle with an adjacent walkable reveal target must exist")

	s.Require().NoError(s.moveAlongPath(encounterID, characterID, target, 10))
	postReveal := drainAvailable(events, 1500*time.Millisecond)
	appeared := 0
	for _, event := range postReveal {
		hkc := event.GetHexKnowledgeChanged()
		if hkc == nil {
			continue
		}
		var entity *encounterv2pb.Entity
		for _, e := range hkc.GetEntities() {
			if e.GetId() == string(revealed.ID) {
				entity = e
				break
			}
		}
		if entity == nil {
			continue
		}
		appeared++
		s.Require().Equal(encounterv2pb.EntityType_ENTITY_TYPE_OBSTACLE, entity.GetType())
		s.Require().Equal(string(revealed.ID), entity.GetId())
		// Position now lives on the HexRecord's Contents placement, never on
		// the Entity (rpg-api-protos#197) — the same HexKnowledgeChanged must
		// carry a hex record naming this entity.
		hex := hexRecordAt(hkc.GetHexes(), revealed.Position)
		s.Require().NotNil(hex, "the obstacle's own hex must be in the same HexKnowledgeChanged")
		var placed bool
		for _, p := range hex.GetContents() {
			if p.GetEntityId() == string(revealed.ID) {
				placed = true
			}
		}
		s.Require().True(placed, "the obstacle's hex record must list it in Contents")
		s.Require().Equal(revealed.Ref, entity.GetObstacle().GetObstacleRef().GetModule()+":"+entity.GetObstacle().GetObstacleRef().GetType()+":"+entity.GetObstacle().GetObstacleRef().GetId())
		s.Require().Equal(revealed.BlocksMovement, entity.GetObstacle().GetBlocksMovement())
		s.Require().Equal(revealed.BlocksLoS, entity.GetObstacle().GetBlocksLineOfSight())
	}
	// >=1, not ==1: rpg-api#737's playtest follow-up gave the mover's own
	// per-move HexKnowledgeChanged restatement (translateMoveEventWithData)
	// its own Entities disclosure, alongside the EntityAppeared-driven one
	// this test originally measured — moveAlongPath chunks a long walk into
	// several separate MoveEntity calls, and each step still within sight of
	// this obstacle legitimately re-discloses it in that step's OWN full
	// restatement (Hexes/Entities are both "restate everything now known",
	// never a diff — knownHexesToProto's doc). What must hold is that it is
	// disclosed at least once, in the SAME envelope that first places it —
	// asserted per-envelope by the loop above (hex/placed checks); a second
	// or third re-disclosure on a later step is harmless, matching how the
	// obstacle's Hexes record is itself redundantly resent every subsequent
	// move without complaint.
	s.Require().GreaterOrEqual(appeared, 1, "the obstacle must be disclosed at least once, in the same envelope that places it")

	reconnect, err := s.srv.EncounterClientV2.GetEncounter(s.authCtx("alice"), &encounterv2pb.GetEncounterRequest{EncounterId: encounterID})
	s.Require().NoError(err)
	var found bool
	for _, entity := range reconnect.GetEncounter().GetSpace().GetEntities() {
		if entity.GetId() == string(revealed.ID) {
			found = true
		}
	}
	s.Require().True(found, "a revealed static obstacle must remain in reconnect snapshots")
}

// TestSpaceZonesAndHexZoneId_ProjectRegionsOnTheWire is rpg-api#687's Done
// bar verified end to end through the REAL RPC surface (not a hand-seeded
// project_test.go fixture), ported here after rpg-api#693 retired the
// two-chamber generator/suite this test originally lived against
// (dungeon_two_chamber_test.go's TestSpaceZonesAndHexZoneId_
// ProjectRegionsOnTheWire): "a [dungeon] encounter arrives at the client
// with every revealed hex carrying the right zone_id, and Space.zones
// naming [every region]." Adapted to the crypt spec's 3 regions (entrance/
// corridor/boss) and 2 connector doors instead of the retired 2-region/1-
// door topology — region IDs are looked up by ARCHETYPE (regionArchetypeAt/
// archetypeMonsters' pattern, this file's established convention) rather
// than by literal ID string, since the crypt spec's region IDs are
// unexported lobby-package constants by design.
//
// Unlike the retired two-chamber suite's version of this test (which
// exercised the missing-Theme fallback — InitTwoChamberRoom never set
// Theme), the crypt spec DOES set Theme="crypt" (DungeonParams.Theme,
// rpg-toolkit#814's Approved Slice 3 corrections), so this version proves
// verbatim theme passthrough instead. The absent-metadata Theme/zones
// fallback remains covered by project_test.go's unit tests
// (TestProjectFor_NoSpace_ThemeEmpty_ZonesNil_NoError /
// TestProjectFor_SpaceWithNoRegions_ZonesNilThemeEmpty).
func (s *DungeonCryptSuite) TestSpaceZonesAndHexZoneId_ProjectRegionsOnTheWire() {
	encounterID, characterID, encData := s.startCryptDungeon("zones1", "alice")
	entranceIDs, bossIDs := archetypeMonsters(encData)
	s.Require().Len(entranceIDs, 1, "rpg-api#689: exactly one deterministic-anchor monster in the entrance region")
	s.Require().Len(bossIDs, 1, "rpg-api#689: exactly one deterministic-anchor boss")

	resp, err := s.srv.EncounterClientV2.GetEncounter(s.authCtx("alice"), &encounterv2pb.GetEncounterRequest{
		EncounterId: encounterID,
	})
	s.Require().NoError(err)
	space := resp.GetEncounter().GetSpace()

	// Legacy crypt Regions are structural seed metadata, not authored
	// SemanticRegions. AuthorizedZones intentionally exposes neither those
	// global regions nor an invented zone for root/unpainted cells. This is a
	// deliberate fog-contract update: the former expectation that all three
	// global crypt regions were disclosed at connect leaked hidden scope names.
	s.Require().Empty(space.GetZones(), "unpainted legacy crypt cells disclose no zones")

	// Verbatim theme passthrough: the crypt spec sets Theme="crypt".
	s.Require().Equal("crypt", space.GetTheme(),
		"InitDungeon's crypt spec sets Theme=\"crypt\"; the wire must carry it verbatim")

	// The provider's observation carries the legacy entrance ID verbatim, but
	// its name is not authorized through Space.zones. This test asserts only
	// that the API preserves that event/snapshot fact without deriving it.
	spawnHex := hexRecordAt(space.GetHexes(), encData.Space.Entrance)
	s.Require().NotNil(spawnHex, "alice's entrance spawn hex must be in her revealed set")
	s.Require().Equal("entrance", spawnHex.GetZoneId(),
		"the fixed CryptDungeonParams provider observation must pass through verbatim")

	// Now form a sightline into the boss region via the live stream and
	// prove the INCREMENTAL HexKnowledgeChanged event (not just the connect-
	// time snapshot above) also carries the right zone_id — #687's Done bar
	// covers hexes revealed mid-session, not only ones present at connect.
	// (rpg-api-protos#197 retired the dedicated GeometryRevealed message
	// this used to check; HexKnowledgeChanged is its sole successor.)
	stream, err := s.srv.EncounterClientV2.StreamEncounter(s.authCtx("alice"),
		&encounterv2pb.StreamEncounterRequest{EncounterId: encounterID})
	s.Require().NoError(err)
	events := s.streamEvents(stream)
	_ = drainAvailable(events, 500*time.Millisecond) // drain connect-time replay

	s.Require().Len(encData.Doors, 2, "the crypt spec's 3-region chain has exactly 2 connector doors")
	// rpg-toolkit#864: physical traversal order (entrance, then boss), not
	// an alphabetical sort — see TestBossMonster_...'s identical note.
	s.openCryptDoor(encounterID, characterID, cryptEntranceDoorID)
	_ = drainAvailable(events, 500*time.Millisecond)
	s.openCryptDoor(encounterID, characterID, cryptBossDoorID)
	_ = drainAvailable(events, 500*time.Millisecond)

	bossMonster := bossIDs[0]
	afterOpen, err := s.srv.EncRepoV2.Get(s.ctx, encounterID)
	s.Require().NoError(err)
	target := afterOpen.Monsters[bossMonster].Position

	// rpg-api#694/#689 merge finding: the entrance monster's own sightline
	// commonly starts a combat pocket before this point (see
	// TestEntranceSighting_StartsPocketScopedCombat's doc) — once that
	// happens, alice's movement is budget-gated per real turn, and this
	// target is far enough away (potentially multiple regions) that it can
	// take more than one turn to reach. moveAlongPath replans persisted-geometry
	// BFS routes after each real MoveEntity chunk (at most six adjacent
	// destinations) and uses the real EndTurn RPC when turn-based movement
	// requires another turn, never adjusting the target itself.
	err = s.moveAlongPath(encounterID, characterID, target, 10)
	s.Require().NoError(err, "moving onto the boss-region monster's hex must not be blocked once both doors are open")

	postMove := drainAvailable(events, 1500*time.Millisecond)
	var sawAny bool
	var sawZoneID string
	for _, ev := range postMove {
		hkc := ev.GetHexKnowledgeChanged()
		if hkc == nil {
			continue
		}
		if h := hexRecordAt(hkc.GetHexes(), target); h != nil {
			sawAny = true
			sawZoneID = h.GetZoneId()
		}
	}
	s.Require().True(sawAny, "the boss-region monster's hex must appear in a live HexKnowledgeChanged event")
	s.Require().Equal("boss", sawZoneID,
		"the fixed CryptDungeonParams provider observation must pass through verbatim")
}

// streamEvents starts the ONE background reader for stream's entire
// lifetime, forwarding every received event onto the returned channel until
// the stream closes. Deliberately NOT re-spun per checkpoint (unlike
// EncounterV2IntegrationSuite's collectStreamEvents, which is only ever
// called once per stream in encounter_v2_test.go — its own doc comment
// warns that calling it twice against the same stream races two goroutines
// on the same Recv()). Callers poll the returned channel via drainAvailable
// as many times as they like; there is exactly one producer for the
// stream's whole life, so repeated polling is safe.
func (s *DungeonCryptSuite) streamEvents(
	stream encounterv2pb.EncounterService_StreamEncounterClient,
) <-chan *encounterv2pb.EncounterEvent {
	ch := make(chan *encounterv2pb.EncounterEvent, 64)
	go func() {
		for {
			ev, err := stream.Recv()
			if err != nil {
				close(ch)
				return
			}
			ch <- ev
		}
	}()
	return ch
}

// drainAvailable pulls whatever events are ready on ch, resetting its idle
// timer on each receive so a burst of events (e.g. SnapshotDelivered + N
// EntityAppeared + GeometryRevealed, all published back-to-back) is
// collected in full rather than cut off by a fixed total deadline. Returns
// once idle for timeout or the stream closes.
func drainAvailable(ch <-chan *encounterv2pb.EncounterEvent, timeout time.Duration) []*encounterv2pb.EncounterEvent {
	var out []*encounterv2pb.EncounterEvent
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return out
			}
			out = append(out, ev)
			if !timer.Stop() {
				<-timer.C
			}
			timer.Reset(timeout)
		case <-timer.C:
			return out
		}
	}
}
