// Package integration_test's lobby_start_then_move_test.go is the rpg-api#656
// regression gate: MoveEntity must actually move the entity in an encounter
// created via the REAL RPC path (LobbyService.StartEncounter, not a direct
// tkenc.New/AddPlayer seed). This is exactly the class of coverage hole
// named in #656 — every existing integration test seeds encounters directly
// via the toolkit SDK (tkenc.New + AddPlayer + repo.Save), never through
// StartEncounter's own room-init + spawn combination (originally InitRoom's
// corner anchor; as of rpg-api#676, InitTwoChamberRoom's entrance anchor,
// and rpg-api#688, InitDungeon's — see hexOutOfBounds' doc below), so none
// of them could have caught a movement-direction bug specific to that
// spawn position.
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
	core "github.com/KirkDiggler/rpg-toolkit/encounter/core"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
)

type LobbyStartThenMoveSuite struct {
	suite.Suite
	ctx     context.Context
	cancel  context.CancelFunc
	srv     *harness.TestServer
	release func()
}

func TestLobbyStartThenMoveSuite(t *testing.T) {
	suite.Run(t, new(LobbyStartThenMoveSuite))
}

func (s *LobbyStartThenMoveSuite) SetupTest() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 2*time.Minute)

	// Lease the package's shared Redis container (rpg-api#699) — see
	// main_test.go. Released in TearDownTest.
	s.release = sharedRedis.Lease()

	var err error
	s.srv, err = harness.NewWithRedis(s.ctx, nil, sharedRedis.Addr)
	s.Require().NoError(err, "failed to create test server")
	s.Require().NoError(s.srv.FlushRedis(s.ctx), "failed to flush shared redis")
}

func (s *LobbyStartThenMoveSuite) TearDownTest() {
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

func (s *LobbyStartThenMoveSuite) authCtx(playerID string) context.Context {
	return metadata.AppendToOutgoingContext(s.ctx, "authorization", "Dev "+playerID)
}

// sixHexDirections is every unit step in cube-hex space (Q+R+S=0 preserved).
// rpg-api#656's own bug report only exercised ONE of these (Q+1,S-1) — "one
// direction green was exactly how this bug hid for three days" (gate note on
// PR #657). A corner-anchored spawn breaks roughly half of these (any
// direction that decreases offset X or Y); a center-anchored spawn must
// clear all six, so this test asserts all six rather than re-pinning just
// the one direction the original report happened to catch.
var sixHexDirections = []core.Hex{
	{Q: 1, R: -1, S: 0},
	{Q: 1, R: 0, S: -1},
	{Q: 0, R: 1, S: -1},
	{Q: -1, R: 1, S: 0},
	{Q: -1, R: 0, S: 1},
	{Q: 0, R: -1, S: 1},
}

// TestMoveEntity_AfterStartEncounter_ActuallyMoves is the #656 regression
// gate: StartEncounter's InitRoom + player-seeding combination must produce
// an encounter where movement is HONEST — either an entity actually arrives
// at its destination, or a real wall cell on the requested path explains why
// it didn't. Each direction gets its own fresh lobby/character/encounter
// (via s.Run sub-tests) rather than chaining moves on one encounter, so a
// failure in one direction is independently visible in test output and
// isn't masked by, or dependent on, any other direction's outcome.
//
// Retired assumption (rpg-api#669, toolkit#793): this test used to assert
// EVERY direction succeeds unconditionally. That held back when walls were
// rare — StartEncounter's roomCenterHex() spawn (see that function's doc)
// was written assuming "PatternRandom's default wall density is sparse," and
// under rpg-toolkit v0.4.3 an unseeded 20x20 room had walls only ~30% of the
// time. toolkit#793 fixed RandomPattern's empty-room-fallback rate (its own
// fix target: 30%→96% non-empty at 20x20) — which means walls near spawn are
// now the NORM, not a rare edge case.
//
// Root cause, confirmed by reading rpg-toolkit encounter/space.go's
// truncateAtWall: it checks EVERY hex in the requested path, INCLUDING THE
// STARTING HEX — not just the destination. roomCenterHex() (start_encounter.
// go) places the party at a fixed formula position with no check that the
// cell itself is clear, so the spawn cell itself can land ON a
// movement-blocking wall (confirmed empirically: captured wall snapshots
// where the spawn hex appears in Space.Walls). When that happens,
// truncateAtWall's very first path element is already blocked, so the
// requested path truncates to empty and NO direction can move — not because
// that one destination is walled, but because the mover's own tile is. This
// is a sharper, more consequential version of "a wall near spawn is normal
// now": it's not merely "spawn ended up wall-adjacent," it's "spawn can be
// ON a wall," which stalls movement in every direction. That's still real
// dungeon behavior given #793's fix and NOT the #656 regression this test
// guards (#656 was a SILENT no-op with no wall anywhere to blame) — but it's
// exactly the class of gap Slice 2's entrance-anchored spawn (rpg-project
// ideas/the-dungeon/wave2-design.md) fixes by replacing roomCenterHex() with
// a toolkit-provided, necessarily-clear spawn cell. Until then, this test
// asserts honesty (moved, or blocked by a real wall on the path) rather than
// universal success.
//
// Slice 2 update (rpg-api#676): roomCenterHex() is now gone — StartEncounter
// spawns the party at SpaceData.Entrance, InitTwoChamberRoom's designated
// anchor. Unlike the old room-center placeholder, the entrance sits at
// chamber 1's own EDGE column (offset X=0 — see two_chamber.go's
// generateTwoChamberLayout doc: "entrance sits at chamber 1's far edge,
// column 0"), by design, not by accident: a dungeon entrance has nothing
// behind it. So roughly half the six directions now walk directly off the
// room's grid, not into a wall — assertMoveHonestOutcome's "honest outcome"
// check below (hexBlocksMovement) needed a matching hexOutOfBounds check, or
// it would wrongly expect a wall-free out-of-bounds hex to be reachable.
//
// rpg-api#688 update: StartEncounter now calls the generalized InitDungeon
// (selected by DungeonKey, default "crypt") instead of InitTwoChamberRoom
// directly, but the entrance-edge-column property this test depends on is
// unchanged — InitDungeon's own generateDungeonLayout places the entrance
// at region 0's far edge (column 0), exactly like the retired two-chamber
// generator it generalizes. This test reads the spawn position dynamically
// (pd.View.Position) rather than assuming any specific region shape, so it
// needed no logic change, only this doc update.
func (s *LobbyStartThenMoveSuite) TestMoveEntity_AfterStartEncounter_ActuallyMoves() {
	anyMoved := false
	for i, dir := range sixHexDirections {
		s.Run(fmt.Sprintf("direction_%d_%+v", i, dir), func() {
			if s.assertMoveHonestOutcome(fmt.Sprintf("656-%d", i), dir) {
				anyMoved = true
			}
		})
	}
	s.True(anyMoved, "at least one of the six directions must succeed — toolkit#793's path-safety "+
		"validation guarantees walkable connectivity from any point in the room, so a spawn boxed in on "+
		"all six sides (or spawned ON a wall) in EVERY one of six independent fresh rooms would itself be "+
		"a suspiciously unlucky sample, not a legitimate wall layout")
}

// assertMoveHonestOutcome creates a fresh lobby, starts an encounter via the
// real RPC path, reads the player's actual spawn position (no hardcoded hex
// — this pins "movement is honest from wherever StartEncounter spawns the
// party," not a specific fix shape), moves one step in dir via the real
// MoveEntity RPC, and asserts the outcome matches what the persisted room
// snapshot says SHOULD happen per rpg-toolkit's own truncateAtWall
// semantics: truncateAtWall checks the WHOLE requested path (here: [start,
// dest]) in order and stops at the first blocked hex, so movement succeeds
// only when BOTH start and dest are clear — either one being a wall cell
// truncates to zero movement. Returns whether the entity actually moved.
func (s *LobbyStartThenMoveSuite) assertMoveHonestOutcome(suffix string, dir core.Hex) bool {
	playerID := "alice-" + suffix
	characterID := "char-alice-" + suffix

	_, err := s.srv.CharacterRepo.Create(s.ctx, characterrepo.CreateInput{
		Character: &entities.Character{
			Data: &toolkitchar.Data{
				ID: characterID, PlayerID: playerID, Name: "Alice",
				HitPoints: 12, MaxHitPoints: 12,
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
	encounterID := startResp.GetEncounterId()
	s.Require().NotEmpty(encounterID)

	before, err := s.srv.EncRepoV2.Get(s.ctx, encounterID)
	s.Require().NoError(err)
	pd, ok := before.Players[core.PlayerID(playerID)]
	s.Require().True(ok, "%q must be seated in the encounter", playerID)
	s.Require().NotNil(pd.View, "%q's view must be set", playerID)
	startHex := pd.View.Position

	destHex := core.Hex{Q: startHex.Q + dir.Q, R: startHex.R + dir.R, S: startHex.S + dir.S}
	// truncateAtWall walks [start, dest] in order and stops at the first
	// blocked hex — so a wall (or the grid boundary, rpg-api#676's
	// entrance-edge case — see hexOutOfBounds) at EITHER position truncates
	// movement to nothing. Checking only destHex would miss the case where
	// the spawn cell itself is walled (see the parent test's doc for how
	// that happens), which blocks every direction identically, not just
	// moves toward a walled neighbor or off the map.
	pathBlocked := hexBlocksMovement(before.Space, startHex) || hexBlocksMovement(before.Space, destHex) ||
		hexOutOfBounds(before.Space, startHex) || hexOutOfBounds(before.Space, destHex)

	_, err = s.srv.EncounterClientV2.MoveEntity(s.authCtx(playerID), &encounterv2pb.MoveEntityRequest{
		EncounterId: encounterID,
		EntityId:    characterID,
		ProposedPath: []*encounterv2pb.Position{
			{X: int32(startHex.Q), Y: int32(startHex.R), Z: int32(startHex.S)},
			{X: int32(destHex.Q), Y: int32(destHex.R), Z: int32(destHex.S)},
		},
	})
	s.Require().NoError(err, "MoveEntity must succeed via the real RPC path")

	after, err := s.srv.EncRepoV2.Get(s.ctx, encounterID)
	s.Require().NoError(err)
	afterPD, ok := after.Players[core.PlayerID(playerID)]
	s.Require().True(ok)
	finalPos := afterPD.View.Position

	if pathBlocked {
		s.Equal(startHex, finalPos,
			"%q must stay at spawn moving in direction %+v — startHex %+v or destHex %+v is a real "+
				"movement-blocking wall cell (or outside the room's grid entirely — rpg-api#676's "+
				"entrance-edge case) in the persisted room snapshot, so a truncated move here is "+
				"correct dungeon behavior, not the #656 regression", playerID, dir, startHex, destHex)
		return false
	}

	s.Equal(destHex, finalPos,
		"%q must actually be at the destination hex after moving in direction %+v — neither startHex %+v "+
			"nor destHex %+v is a wall cell or out of the room's bounds in the persisted room snapshot, "+
			"so a silently-truncated (no-op) move here IS the #656 regression", playerID, dir, startHex, destHex)
	return finalPos == destHex
}

// hexOutOfBounds reports whether hex falls outside the persisted room's
// offset-coordinate grid (SpaceData.Width/Height's doc: "offset-coordinate
// bounds... without these, a reconstructed room would... silently accepting
// or rejecting edge positions wrong"). rpg-toolkit's truncateAtWall
// (encounter/space.go) rejects an out-of-bounds hex via room.CanPlaceEntity
// -> the grid's own IsValidPosition — exactly like a movement-blocking wall
// cell, just for a different reason — so a step that would leave the grid
// must be treated the same way hexBlocksMovement treats a wall: expected to
// truncate, not a silent-no-op bug.
//
// This matters for Slice 2's entrance-anchored spawn (rpg-api#676):
// InitTwoChamberRoom's entrance sits at chamber 1's own edge column (offset
// X=0 — two_chamber.go's generateTwoChamberLayout doc), so roughly half the
// six unit directions from spawn are genuinely off the map, not walled.
func hexOutOfBounds(space *tkenc.SpaceData, hex core.Hex) bool {
	if space == nil {
		return false
	}
	pos := hex.ToPosition()
	return pos.X < 0 || pos.X >= float64(space.Width) || pos.Y < 0 || pos.Y >= float64(space.Height)
}

// hexBlocksMovement reports whether hex is a movement-blocking cell in the
// persisted room snapshot — either a wall (tkenc.SpaceData.Walls doc: one
// degenerate Start==End segment per discretized wall hex — see rpg-toolkit
// encounter/data.go and tools/environments' WallSegmentData) OR, as of
// rpg-api#694 (the crypt key now asks the toolkit for CryptDungeonParams'
// physical set-piece obstacles — entrance: obelisk + 2 pillars — instead of
// leaving every region's Obstacles unset), a placed ObstacleData whose own
// BlocksMovement is true. Before #694 this function only needed to check
// Walls: SpaceData.Obstacles was always empty for every dungeon key, so a
// destHex landing on one was never possible. It is now a real, legitimate
// truncation cause this suite's own truncateAtWall-mirroring logic must
// account for, or a party stepping onto a pillar/obelisk/coffin/altar/statue
// would be misclassified as the #656 regression instead of correct
// obstacle-blocking behavior.
func hexBlocksMovement(space *tkenc.SpaceData, hex core.Hex) bool {
	if space == nil {
		return false
	}
	for _, w := range space.Walls {
		if w.BlocksMovement && core.HexFromCube(w.Start) == hex {
			return true
		}
	}
	for _, o := range space.Obstacles {
		if o.BlocksMovement && o.Position == hex {
			return true
		}
	}
	return false
}
