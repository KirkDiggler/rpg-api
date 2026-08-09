package lobby_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/dungeonregistry"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	authoringorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/authoring"
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
	rpgcore "github.com/KirkDiggler/rpg-toolkit/core"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
	"github.com/KirkDiggler/rpg-toolkit/encounter/perception"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/saves"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

func (s *LobbySuite) seedReadyLobby(id, host string, others ...string) {
	members := map[string]*lobbyrepo.Member{
		host: {PlayerID: host, CharacterID: "char-" + host, IsHost: true, IsReady: true},
	}
	order := make([]string, 0, 1+len(others))
	order = append(order, host)
	for _, p := range others {
		members[p] = &lobbyrepo.Member{PlayerID: p, CharacterID: "char-" + p, IsReady: true}
		order = append(order, p)
	}
	s.seedLobby(&lobbyrepo.Data{
		ID: id, HostPlayerID: host, Status: lobbyrepo.StatusWaiting,
		Members: members, MemberOrder: order,
	})
}

func (s *LobbySuite) TestStartEncounter_Success_ConstructsAndPersistsEncounter() {
	s.seedReadyLobby("lobby-s1", "alice", "bob")
	s.expectCharacter("char-alice", "alice", "Alice", 12, 12)
	s.expectCharacter("char-bob", "bob", "Bob", 10, 10)

	out, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-s1",
	})
	s.Require().NoError(err)
	s.Require().NotEmpty(out.EncounterID)

	encData, err := s.encRepo.Get(s.ctx, out.EncounterID)
	s.Require().NoError(err)
	s.Require().Len(encData.Players, 2, "both ready members become encounter players")
	s.Require().Contains(encData.Players, core.PlayerID("alice"))
	s.Require().Contains(encData.Players, core.PlayerID("bob"))
	s.Require().Equal(12, encData.Players[core.PlayerID("alice")].HP, "HP must be seeded from the character store")
	s.Require().Equal(10, encData.Players[core.PlayerID("bob")].HP)

	// rpg-api#632: an unseeded SightRange (0) reveals exactly one hex per
	// player, so nobody can see anybody else — the diagnosed bug. Assert each
	// member's cumulative reveal actually covers the other member's spawn hex,
	// not just their own.
	alice := encData.Players[core.PlayerID("alice")]
	bob := encData.Players[core.PlayerID("bob")]
	s.Require().NotZero(alice.View.SightRange, "SightRange must be seeded — a zero value reveals only the player's own hex")
	s.Require().True(alice.View.KnownHexSet().Has(bob.View.Position), "alice must be able to see bob's spawn hex")
	s.Require().True(bob.View.KnownHexSet().Has(alice.View.Position), "bob must be able to see alice's spawn hex")

	lobbyData, err := s.lobbyRepo.Get(s.ctx, "lobby-s1")
	s.Require().NoError(err)
	s.Require().Equal(lobbyrepo.StatusStarted, lobbyData.Status)
	s.Require().Equal(out.EncounterID, lobbyData.EncounterID)
}

// TestStartEncounter_CanvasPutDungeonSurvivesProductionReload proves the
// restart contract without rebuilding a registry in the test: PutDungeon writes
// the source, then a fresh production LoadContentRegistry discovers and compiles
// it, and a fresh runtime StartEncounter consumes that loaded entry.
func (s *LobbySuite) TestStartEncounter_CanvasPutDungeonSurvivesProductionReload() {
	const key = "canvas-production-reload"
	dir := s.T().TempDir()
	authoringRegistry := dungeonregistry.New(nil)
	authoring, err := authoringorch.New(&authoringorch.Config{
		Registry: authoringRegistry, ContentDir: dir, PartyStartSeatCount: lobbyorch.DefaultPartyCap,
	})
	s.Require().NoError(err)

	yaml := `version: 1
key: canvas-production-reload
name: Canvas Production Reload
height: 1
canvas: { width: 9, height: 2 }
rooms: []
start: [1, 1]
place:
  - { ref: dnd5e:props:altar, at: [1, 0], facing: W }
  - { ref: dnd5e:props:bookcase, at: [2, 0], offset: [0, 0, 0] }
  - { ref: dnd5e:props:pillar, at: [3, 0], offset: [-0.25, 1.5, 2.75] }
  - { ref: dnd5e:monsters:skeleton, at: [4, 0] }
  - { ref: dnd5e:monsters:skeleton, at: [5, 0], offset: [0, 0, 0] }
  - { ref: dnd5e:monsters:skeleton, at: [6, 0], offset: [0.125, -2.5, 3.75] }
walls:
  - { from: [0, 0], to: [0, 1], kind: solid }
  - { from: [1, 0], to: [1, 1], kind: door }
`
	put, err := authoring.PutDungeon(s.ctx, &authoringorch.PutDungeonInput{Key: key, YAML: yaml})
	s.Require().NoError(err)
	s.Require().True(put.Success, "field error: %s", put.FieldError)
	s.Require().FileExists(filepath.Join(dir, key+".yaml"))
	s.Require().Len(put.FloorPlan.Placements, 6)
	for index, placement := range put.FloorPlan.Placements {
		s.Require().Equal(fmt.Sprintf("place[%d]", index), placement.SourcePath)
	}
	s.Require().Nil(put.FloorPlan.Placements[0].Offset, "omitted canvas-prop offset must remain absent")
	s.Require().Equal(&authoringorch.PlacementOffset{}, put.FloorPlan.Placements[1].Offset,
		"explicit canvas-prop zero must remain present")
	s.Require().Equal(&authoringorch.PlacementOffset{X: -0.25, Y: 1.5, Z: 2.75}, put.FloorPlan.Placements[2].Offset)
	s.Require().Nil(put.FloorPlan.Placements[3].Offset, "omitted canvas-monster offset must remain absent")
	s.Require().Equal(&authoringorch.PlacementOffset{}, put.FloorPlan.Placements[4].Offset,
		"explicit canvas-monster zero must remain present")
	s.Require().Equal(&authoringorch.PlacementOffset{X: 0.125, Y: -2.5, Z: 3.75}, put.FloorPlan.Placements[5].Offset)

	// Replace the same authored key before the simulated restart. The fresh
	// loader below must reconstruct from this committed source, not from the
	// authoring process's in-memory registry or the first version's bytes.
	yaml = strings.Replace(yaml, "name: Canvas Production Reload", "name: Canvas Production Reload Updated", 1)
	put, err = authoring.PutDungeon(s.ctx, &authoringorch.PutDungeonInput{Key: key, YAML: yaml})
	s.Require().NoError(err)
	s.Require().True(put.Success)
	committed, err := os.ReadFile(filepath.Join(dir, key+".yaml"))
	s.Require().NoError(err)
	s.Require().Equal(yaml, string(committed))

	// This is the production startup loader, not dungeonspec.Load* or a
	// manually assembled registry. It has to discover the bytes PutDungeon wrote.
	s.T().Setenv("RPG_CONTENT_DIR", dir)
	reloadedRegistry, err := lobbyorch.LoadContentRegistry()
	s.Require().NoError(err)
	s.Require().NotSame(authoringRegistry, reloadedRegistry)
	reloadedEntry, ok := reloadedRegistry.Get(key)
	s.Require().True(ok, "fresh registry must discover PutDungeon's on-disk source")
	s.Require().NoError(reloadedEntry.Err)
	s.Require().Equal("Canvas Production Reload Updated", reloadedEntry.Name)

	runtime, err := s.newOrchestratorWithContentDir(dir)
	s.Require().NoError(err)
	s.seedReadyLobby("lobby-canvas-production-reload", "alice", "bob")
	s.expectCharacter("char-alice", "alice", "Alice", 12, 12)
	s.expectCharacter("char-bob", "bob", "Bob", 11, 11)
	out, err := runtime.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-canvas-production-reload", DungeonKey: key, RandomSeed: 42,
	})
	s.Require().NoError(err)

	data, err := s.encRepo.Get(s.ctx, out.EncounterID)
	s.Require().NoError(err)
	anchor := core.HexFromPosition(spatial.Position{X: 1, Y: 1})
	s.Require().Equal(tkenc.FloorSourceCanvas, data.Space.FloorSource)
	s.Require().Equal(9, data.Space.Width)
	s.Require().Equal(2, data.Space.Height)
	s.Require().Equal(anchor, data.Space.Entrance)
	s.Require().Len(data.Space.PartyStartPositions, lobbyorch.DefaultPartyCap)
	s.Require().Equal(anchor, data.Space.PartyStartPositions[0])
	s.Require().Equal(data.Space.PartyStartPositions[0], data.Players[core.PlayerID("alice")].View.Position)
	s.Require().Equal(data.Space.PartyStartPositions[1], data.Players[core.PlayerID("bob")].View.Position)

	s.Require().Len(data.Space.Obstacles, 3)
	propsByPosition := make(map[core.Hex]tkenc.ObstacleData, len(data.Space.Obstacles))
	for _, prop := range data.Space.Obstacles {
		propsByPosition[prop.Position] = prop
	}
	omittedProp := propsByPosition[core.HexFromPosition(spatial.Position{X: 1, Y: 0})]
	s.Require().Equal("dnd5e:props:altar", omittedProp.Ref)
	s.Require().NotNil(omittedProp.Facing)
	s.Require().Equal(uint32(3), *omittedProp.Facing, "W is authored facing, not inferred")
	s.Require().Nil(omittedProp.Offset, "omitted canvas-prop offset must remain absent")
	zeroProp := propsByPosition[core.HexFromPosition(spatial.Position{X: 2, Y: 0})]
	s.Require().Equal("dnd5e:props:bookcase", zeroProp.Ref)
	s.Require().Equal(&core.PlacementOffset{}, zeroProp.Offset)
	signedProp := propsByPosition[core.HexFromPosition(spatial.Position{X: 3, Y: 0})]
	s.Require().Equal("dnd5e:props:pillar", signedProp.Ref)
	s.Require().Equal(&core.PlacementOffset{-0.25, 1.5, 2.75}, signedProp.Offset)

	s.Require().Len(data.Monsters, 3)
	monstersByPosition := make(map[core.Hex]*tkenc.MonsterData, len(data.Monsters))
	for _, monster := range data.Monsters {
		s.Require().Equal("dnd5e:monsters:skeleton", monster.MonsterRef)
		monstersByPosition[monster.Position] = monster
	}
	omittedMonster := monstersByPosition[core.HexFromPosition(spatial.Position{X: 4, Y: 0})]
	s.Require().NotNil(omittedMonster)
	s.Require().Nil(omittedMonster.Offset, "omitted canvas-monster offset must survive as absent")
	zeroMonster := monstersByPosition[core.HexFromPosition(spatial.Position{X: 5, Y: 0})]
	s.Require().NotNil(zeroMonster)
	s.Require().Equal(&core.PlacementOffset{}, zeroMonster.Offset,
		"explicit canvas-monster zero must survive encounter creation and repository reload")
	signedMonster := monstersByPosition[core.HexFromPosition(spatial.Position{X: 6, Y: 0})]
	s.Require().NotNil(signedMonster)
	s.Require().Equal(&core.PlacementOffset{0.125, -2.5, 3.75}, signedMonster.Offset,
		"canvas-monster signed axes survive creation and repository reload")

	// A later complete-document source edit must not recompute the already
	// persisted encounter snapshot.
	editedYAML := strings.ReplaceAll(yaml, ", offset: [-0.25, 1.5, 2.75]", "")
	editedYAML = strings.ReplaceAll(editedYAML, "offset: [0.125, -2.5, 3.75]", "offset: [96, 95, 94]")
	edited, err := authoring.PutDungeon(s.ctx, &authoringorch.PutDungeonInput{Key: key, YAML: editedYAML})
	s.Require().NoError(err)
	s.Require().True(edited.Success)
	unchanged, err := s.encRepo.Get(s.ctx, out.EncounterID)
	s.Require().NoError(err)
	unchangedProps := make(map[core.Hex]tkenc.ObstacleData, len(unchanged.Space.Obstacles))
	for _, prop := range unchanged.Space.Obstacles {
		unchangedProps[prop.Position] = prop
	}
	s.Require().Equal(&core.PlacementOffset{-0.25, 1.5, 2.75},
		unchangedProps[core.HexFromPosition(spatial.Position{X: 3, Y: 0})].Offset,
		"existing encounter must retain creation-time authored truth after source removal")
	for _, monster := range unchanged.Monsters {
		if monster.Position == core.HexFromPosition(spatial.Position{X: 6, Y: 0}) {
			s.Require().Equal(&core.PlacementOffset{0.125, -2.5, 3.75}, monster.Offset,
				"existing canvas monster must not recompute after a source edit")
		}
	}

	s.Require().Equal([]tkenc.AuthoredEdge{
		{From: core.HexFromPosition(spatial.Position{X: 0, Y: 1}), To: core.HexFromPosition(spatial.Position{X: 0, Y: 0}), Kind: tkenc.GeneratedEdgeKindSolid},
		{From: core.HexFromPosition(spatial.Position{X: 1, Y: 1}), To: core.HexFromPosition(spatial.Position{X: 1, Y: 0}), Kind: tkenc.GeneratedEdgeKindDoor, DoorID: "canvas-production-reload-authored-door-1--2-1--1--1-0"},
	}, data.Space.AuthoredEdges)
	door, ok := data.Doors[core.EntityID("canvas-production-reload-authored-door-1--2-1--1--1-0")]
	s.Require().True(ok)
	s.Require().Equal(core.HexFromPosition(spatial.Position{X: 1, Y: 1}), door.Position)
}

// TestStartEncounter_RoomPlacementOffsetMatrixUsesRuntimeCarriers proves each
// room-local runtime path preserves all three presence cases independently.
// The files are loaded through the production content registry, StartEncounter
// consumes only provider Params/Spawns, and the repository Get is a JSON reload.
func (s *LobbySuite) TestStartEncounter_RoomPlacementOffsetMatrixUsesRuntimeCarriers() {
	type offsetCase struct {
		name   string
		suffix string
		want   *core.PlacementOffset
	}
	cases := []offsetCase{
		{name: "omitted"},
		{name: "explicit-zero", suffix: ", offset: [0, 0, 0]", want: &core.PlacementOffset{}},
		{name: "signed", suffix: ", offset: [-0.25, 1.5, 2.75]", want: &core.PlacementOffset{-0.25, 1.5, 2.75}},
	}

	dir := s.T().TempDir()
	for _, tc := range cases {
		key := "room-offset-" + tc.name
		source := fmt.Sprintf(`version: 1
key: %s
name: Room Offset %s
height: 8
rooms:
  - id: entrance
    archetype: entrance
    width: 6
    place:
      - { ref: "dnd5e:props:bookcase", at: [1, 1]%s }
  - id: boss
    archetype: boss
    width: 8
    boss: { ref: "dnd5e:monsters:skeleton-captain", at: [4, 2]%s }
    place:
      - { ref: "dnd5e:monsters:skeleton", at: [2, 2]%s }
connectors:
  - { from: entrance, to: boss }
`, key, tc.name, tc.suffix, tc.suffix, tc.suffix)
		s.Require().NoError(os.WriteFile(filepath.Join(dir, key+".yaml"), []byte(source), 0o600))
	}

	runtime, err := s.newOrchestratorWithContentDir(dir)
	s.Require().NoError(err)
	for _, tc := range cases {
		s.Run(tc.name, func() {
			key := "room-offset-" + tc.name
			lobbyID := "lobby-" + key
			s.seedReadyLobby(lobbyID, "alice")
			s.expectCharacter("char-alice", "alice", "Alice", 12, 12)
			out, startErr := runtime.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
				PlayerID: "alice", LobbyID: lobbyID, DungeonKey: lobbyorch.DungeonKey(key), RandomSeed: 42,
			})
			s.Require().NoError(startErr)

			data, getErr := s.encRepo.Get(s.ctx, out.EncounterID)
			s.Require().NoError(getErr)
			s.Require().Len(data.Space.Obstacles, 1)
			s.Require().Equal(core.HexFromPosition(spatial.Position{X: 1, Y: 1}), data.Space.Obstacles[0].Position)
			s.Require().Equal(tc.want, data.Space.Obstacles[0].Offset, "room prop")

			monstersByRef := make(map[string]*tkenc.MonsterData, len(data.Monsters))
			for _, monster := range data.Monsters {
				monstersByRef[monster.MonsterRef] = monster
			}
			roomMonster := monstersByRef["dnd5e:monsters:skeleton"]
			s.Require().NotNil(roomMonster)
			s.Require().Equal(core.HexFromPosition(spatial.Position{X: 9, Y: 2}), roomMonster.Position)
			s.Require().Equal(tc.want, roomMonster.Offset, "room monster")
			boss := monstersByRef["dnd5e:monsters:skeleton-captain"]
			s.Require().NotNil(boss)
			s.Require().Equal(core.HexFromPosition(spatial.Position{X: 11, Y: 2}), boss.Position)
			s.Require().Equal(tc.want, boss.Offset, "room boss")
		})
	}
}

// TestStartEncounter_AuthoredStartMapsToolkitSeatsToOrderedRoster proves the
// StartEncounter seam without duplicating toolkit placement: an authored
// absolute start on the boss room's legal door row becomes Space.Entrance, and
// every ordered member receives the corresponding toolkit-resolved seat.
func (s *LobbySuite) TestStartEncounter_AuthoredStartMapsToolkitSeatsToOrderedRoster() {
	const key = "authored-start"
	dir := s.T().TempDir()
	const yaml = `version: 1
key: authored-start
name: Authored Start
height: 8
start: [12, 4]
rooms:
  - id: entrance
    archetype: entrance
    width: 6
    place:
      - { ref: "dnd5e:props:bookcase", at: [1, 1] }
  - id: boss
    archetype: boss
    width: 8
    boss: { ref: "dnd5e:monsters:skeleton-captain", at: [4, 2], offset: [-0.25, 1.5, 2.75] }
    place:
      - { ref: "dnd5e:monsters:skeleton", at: [2, 2], offset: [0, 0, 0] }
connectors:
  - { from: entrance, to: boss }
`
	s.Require().NoError(os.WriteFile(filepath.Join(dir, key+".yaml"), []byte(yaml), 0o600))
	orch, err := s.newOrchestratorWithContentDir(dir)
	s.Require().NoError(err)

	s.seedReadyLobby("lobby-authored-start", "alice", "bob", "carol", "dave")
	s.expectCharacter("char-alice", "alice", "Alice", 12, 12)
	s.expectCharacter("char-bob", "bob", "Bob", 11, 11)
	s.expectCharacter("char-carol", "carol", "Carol", 10, 10)
	s.expectCharacter("char-dave", "dave", "Dave", 9, 9)

	out, err := orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-authored-start", DungeonKey: key, RandomSeed: 42,
	})
	s.Require().NoError(err)

	data, err := s.encRepo.Get(s.ctx, out.EncounterID)
	s.Require().NoError(err)

	// The room-local runtime carriers are independent from the authoring
	// sidecar: StartEncounter consumes only provider Params/Spawns, and the
	// repository JSON round trip preserves omission, explicit zero, and signed axes.
	s.Require().Len(data.Space.Obstacles, 1)
	s.Require().Equal("dnd5e:props:bookcase", data.Space.Obstacles[0].Ref)
	s.Require().Equal(core.HexFromPosition(spatial.Position{X: 1, Y: 1}), data.Space.Obstacles[0].Position)
	s.Require().Nil(data.Space.Obstacles[0].Offset, "omitted room-prop offset must remain absent")

	monstersByRef := make(map[string]*tkenc.MonsterData, len(data.Monsters))
	for _, monster := range data.Monsters {
		monstersByRef[monster.MonsterRef] = monster
	}
	roomMonster := monstersByRef["dnd5e:monsters:skeleton"]
	s.Require().NotNil(roomMonster)
	s.Require().Equal(core.HexFromPosition(spatial.Position{X: 9, Y: 2}), roomMonster.Position)
	s.Require().Equal(&core.PlacementOffset{}, roomMonster.Offset,
		"explicit room-monster zero must remain present")
	boss := monstersByRef["dnd5e:monsters:skeleton-captain"]
	s.Require().NotNil(boss)
	s.Require().Equal(core.HexFromPosition(spatial.Position{X: 11, Y: 2}), boss.Position)
	s.Require().Equal(&core.PlacementOffset{-0.25, 1.5, 2.75}, boss.Offset,
		"room-boss signed offset must survive StartEncounter and repository JSON")

	wantAnchor := core.HexFromPosition(spatial.Position{X: 12, Y: 4})
	s.Require().Equal(wantAnchor, data.Space.Entrance,
		"the toolkit-resolved authored anchor must persist as Space.Entrance")
	s.Require().Len(data.Space.PartyStartPositions, 4,
		"normal product capacity must reserve four toolkit-owned seats")

	seen := make(map[core.Hex]struct{}, len(data.Space.PartyStartPositions))
	for i, playerID := range []core.PlayerID{"alice", "bob", "carol", "dave"} {
		position := data.Players[playerID].View.Position
		s.Require().Equal(data.Space.PartyStartPositions[i], position,
			"member %q must receive toolkit seat %d verbatim", playerID, i)
		seen[position] = struct{}{}
	}
	s.Require().Len(seen, 4, "the toolkit's four returned seats must be distinct")
}

// TestStartEncounter_PartySpawnCapacityErrorPropagates verifies the API does
// not derive a fifth coordinate or partially seat a roster beyond the normal
// four-seat toolkit reservation.
func (s *LobbySuite) TestStartEncounter_PartySpawnCapacityErrorPropagates() {
	s.seedReadyLobby("lobby-party-over-capacity", "alice", "bob", "carol", "dave", "erin")

	_, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-party-over-capacity",
	})
	s.Require().Error(err)
	var capacityErr *tkenc.PartySpawnCapacityError
	s.Require().ErrorAs(err, &capacityErr)
	s.Require().Equal(5, capacityErr.Requested)
	s.Require().Equal(4, capacityErr.Available)

	lobbyData, getErr := s.lobbyRepo.Get(s.ctx, "lobby-party-over-capacity")
	s.Require().NoError(getErr)
	s.Require().Equal(lobbyrepo.StatusWaiting, lobbyData.Status)
	s.Require().Empty(lobbyData.EncounterID, "capacity failure must happen before any partial encounter persists")
}

// TestStartEncounter_SeedsHonestCombatSnapshot proves the rpg-api#634 fix:
// AC is seeded from the stored character (a real field, copied verbatim),
// while AttackBonus/DamageDice/DamageType stay zero — a character carries no
// precomputed value for them, and rpg-api must not compute one (that's rules
// math the toolkit owns). isPlayerCombatant instead treats a hydrated seat
// as combat-ready; that hydration comes from the v2 encounter orchestrator's
// characterData.Attach cascade on the first combat-capable RPC, not from
// StartEncounter, so DataJSON is correctly absent here.
func (s *LobbySuite) TestStartEncounter_SeedsHonestCombatSnapshot() {
	s.seedReadyLobby("lobby-s7", "alice")
	s.expectCharacterWithAC("char-alice", "alice", "Alice", 12, 12, 15)

	out, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-s7",
	})
	s.Require().NoError(err)

	encData, err := s.encRepo.Get(s.ctx, out.EncounterID)
	s.Require().NoError(err)
	alice := encData.Players[core.PlayerID("alice")]
	s.Require().Equal(15, alice.AC, "AC must be seeded honestly from the stored character")
	s.Require().Zero(alice.AttackBonus, "no stored field to honestly derive an attack bonus from")
	s.Require().Empty(alice.DamageDice, "no stored field to honestly derive damage dice from")
	s.Require().Empty(alice.DamageType, "no stored field to honestly derive a damage type from")
	s.Require().Empty(alice.DataJSON, "hydration is the v2 orchestrator's characterData.Attach cascade's job, not StartEncounter's")
}

// TestStartEncounter_ClearsStaleActionEconomy is the wave-close playtest
// blocker regression test (rpg-api#644): a character carrying a non-nil
// ActionEconomy left over from a PRIOR encounter (character.ActionEconomy
// has no encounter scoping — see clearStaleActionEconomy's doc) must have
// it cleared by StartEncounter, or the toolkit's Move() budget gate
// (InCombat() == ActionEconomy != nil) rejects every move on the brand-new
// FREE_ROAM encounter with "insufficient movement remaining", even though
// nothing about the new encounter has happened yet — reproduced live on the
// dev stack with alice's real character data (movement_remaining: 0,
// turn_number: 1, carried over from an earlier playtest's combat).
func (s *LobbySuite) TestStartEncounter_ClearsStaleActionEconomy() {
	s.seedReadyLobby("lobby-stale-economy", "alice")

	staleEconomy := &toolkitchar.ActionEconomyData{
		TurnNumber: 1, ActionsRemaining: 0, BonusActionsRemaining: 0,
		ReactionsRemaining: 0, MovementRemaining: 0,
	}
	s.charRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-alice"}).
		Return(&characterrepo.GetOutput{
			Character: &entities.Character{
				Data: &toolkitchar.Data{
					ID: "char-alice", PlayerID: "alice", Name: "Alice",
					HitPoints: 12, MaxHitPoints: 12,
					ActionEconomy: staleEconomy,
				},
			},
		}, nil)
	var persisted *toolkitchar.Data
	s.charRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ interface{}, input characterrepo.UpdateInput) (*characterrepo.UpdateOutput, error) {
			persisted = input.Character.Data
			return &characterrepo.UpdateOutput{Character: input.Character}, nil
		})

	out, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-stale-economy",
	})
	s.Require().NoError(err, "a stale action economy must not block StartEncounter")
	s.Require().NotEmpty(out.EncounterID)

	s.Require().NotNil(persisted, "the cleared character must be persisted back to the character store")
	s.Require().Nil(persisted.ActionEconomy, "ActionEconomy must be cleared (ExitCombat) so the fresh encounter's Move() is not budget-gated")

	// HP/MaxHP still seed correctly onto the new encounter — clearing the
	// stale economy must not disturb the honest combat-snapshot seed.
	encData, err := s.encRepo.Get(s.ctx, out.EncounterID)
	s.Require().NoError(err)
	s.Require().Equal(12, encData.Players[core.PlayerID("alice")].HP)
}

// TestStartEncounter_DeadCharacter_ArcadeRecoveryRestoresAndPersists is the
// rpg-api#670 regression: a character carrying 0 HP from a PRIOR encounter
// (a confirmed death or an unresolved TPK snapshot) must be seated ALIVE at
// full HP in a BRAND NEW encounter — rpg-toolkit's arcade recovery
// (character.RestoreForNewEncounter, toolkit#785/#786) exists for exactly
// this, but only fires where a caller actually invokes it. StartEncounter's
// AddPlayer call deliberately carries no DataJSON (hydration is the v2
// orchestrator's job — see TestStartEncounter_SeedsHonestCombatSnapshot
// above), so the toolkit's OWN DataJSON-gated restoreForNewSeat path inside
// AddPlayer never fires for a real lobby-started encounter. character.go's
// restoreForNewEncounter step is what actually triggers the toolkit's
// restore rule here, and must persist the result back to the character
// store — otherwise the NEXT StartEncounter for this character would read
// the same pre-restore hp=0 record right back out of the store.
func (s *LobbySuite) TestStartEncounter_DeadCharacter_ArcadeRecoveryRestoresAndPersists() {
	s.seedReadyLobby("lobby-dead-alice", "alice")

	unconsciousBlob, err := json.Marshal(struct {
		Ref *rpgcore.Ref `json:"ref"`
	}{Ref: refs.Conditions.Unconscious()})
	s.Require().NoError(err)

	s.charRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-alice"}).
		Return(&characterrepo.GetOutput{
			Character: &entities.Character{
				Data: &toolkitchar.Data{
					ID: "char-alice", PlayerID: "alice", Name: "Alice",
					HitPoints: 0, MaxHitPoints: 20,
					DeathSaveState: &saves.DeathSaveState{Failures: 3},
					Conditions:     []json.RawMessage{unconsciousBlob},
				},
			},
		}, nil)

	var persisted *toolkitchar.Data
	s.charRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ interface{}, input characterrepo.UpdateInput) (*characterrepo.UpdateOutput, error) {
			persisted = input.Character.Data
			return &characterrepo.UpdateOutput{Character: input.Character}, nil
		})

	out, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-dead-alice",
	})
	s.Require().NoError(err)

	// Alive on the encounter wire: full HP, not the dead hp=0 the store had.
	encData, err := s.encRepo.Get(s.ctx, out.EncounterID)
	s.Require().NoError(err)
	s.Require().Equal(20, encData.Players[core.PlayerID("alice")].HP,
		"a dead character must be seated at full HP in a NEW encounter (arcade recovery)")
	s.Require().Equal(20, encData.Players[core.PlayerID("alice")].MaxHP)

	// Persisted back to the character store: the NEXT StartEncounter must not
	// read the same pre-restore hp=0 record again.
	s.Require().NotNil(persisted, "the restored character must be persisted back to the character store")
	s.Require().Equal(20, persisted.HitPoints, "persisted record must carry the restored HP")
	s.Require().Nil(persisted.DeathSaveState, "death-save state must be cleared")
	s.Require().Empty(persisted.Conditions, "the Unconscious condition must be stripped")
}

// TestStartEncounter_LivingCharacter_ArcadeRecoveryDoesNotFire proves the
// restore is death-scoped, not a free heal: a character already above 0 HP
// must be seated unchanged, with no extra Update call to the character
// store beyond StartEncounter's other necessary writes.
func (s *LobbySuite) TestStartEncounter_LivingCharacter_ArcadeRecoveryDoesNotFire() {
	s.seedReadyLobby("lobby-alive-alice", "alice")
	s.expectCharacter("char-alice", "alice", "Alice", 7, 20)
	// No Update EXPECT() is armed: gomock fails the test if StartEncounter
	// calls charRepo.Update for a character that never needed restoring.

	out, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-alive-alice",
	})
	s.Require().NoError(err)

	encData, err := s.encRepo.Get(s.ctx, out.EncounterID)
	s.Require().NoError(err)
	s.Require().Equal(7, encData.Players[core.PlayerID("alice")].HP,
		"a living character's HP must not be touched by arcade recovery")
}

func (s *LobbySuite) TestStartEncounter_CharacterNotFound_SeedsZeroHP_NotFatal() {
	s.seedReadyLobby("lobby-s2", "alice")
	s.charRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-alice"}).
		Return(nil, apierr.NotFound("character not found"))

	out, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-s2",
	})
	s.Require().NoError(err, "a missing character seeds 0/0 HP rather than failing StartEncounter")

	encData, err := s.encRepo.Get(s.ctx, out.EncounterID)
	s.Require().NoError(err)
	s.Require().Equal(0, encData.Players[core.PlayerID("alice")].HP)
	s.Require().Equal(0, encData.Players[core.PlayerID("alice")].MaxHP)
}

func (s *LobbySuite) TestStartEncounter_NotHost_PermissionDenied() {
	s.seedReadyLobby("lobby-s3", "alice", "bob")

	_, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "bob", LobbyID: "lobby-s3",
	})
	s.Require().ErrorIs(err, lobbyorch.ErrNotHost)
}

func (s *LobbySuite) TestStartEncounter_NotAllReady_FailedPrecondition() {
	s.seedLobby(&lobbyrepo.Data{
		ID: "lobby-s4", HostPlayerID: "alice", Status: lobbyrepo.StatusWaiting,
		Members: map[string]*lobbyrepo.Member{
			"alice": {PlayerID: "alice", IsHost: true, IsReady: true},
			"bob":   {PlayerID: "bob", IsReady: false},
		},
		MemberOrder: []string{"alice", "bob"},
	})

	_, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-s4",
	})
	s.Require().ErrorIs(err, lobbyorch.ErrNotAllReady)
}

func (s *LobbySuite) TestStartEncounter_AlreadyStarted_FailedPrecondition() {
	s.seedLobby(&lobbyrepo.Data{
		ID: "lobby-s5", HostPlayerID: "alice", Status: lobbyrepo.StatusStarted,
		EncounterID: "enc-existing",
		Members:     map[string]*lobbyrepo.Member{"alice": {PlayerID: "alice", IsHost: true, IsReady: true}},
		MemberOrder: []string{"alice"},
	})

	_, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-s5",
	})
	s.Require().ErrorIs(err, lobbyorch.ErrLobbyAlreadyStarted)
}

func (s *LobbySuite) TestStartEncounter_LobbyNotFound() {
	_, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "no-such-lobby",
	})
	s.Require().ErrorIs(err, lobbyorch.ErrLobbyNotFound)
}

// TestStartEncounter_PublishesEncounterStarted_AfterPersist proves the
// persist-then-emit ordering (lobby-surface.md): by the time the broadcast
// event lands in a subscriber's channel, the encounter is already readable
// from the encounter repo.
func (s *LobbySuite) TestStartEncounter_PublishesEncounterStarted_AfterPersist() {
	s.seedReadyLobby("lobby-s6", "alice")
	s.expectCharacter("char-alice", "alice", "Alice", 12, 12)

	sub, err := s.broker.Subscribe("lobby-s6")
	s.Require().NoError(err)
	defer func() { _ = sub.Close() }()

	out, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-s6",
	})
	s.Require().NoError(err)

	evt := <-sub.Events()
	s.Require().Equal(lobbyorch.EventKindEncounterStarted, evt.Kind)
	s.Require().Equal(out.EncounterID, evt.EncounterStarted.EncounterID)

	// The encounter must already be persisted by the time this event is
	// observable — that ordering is what makes the event safe to act on.
	_, err = s.encRepo.Get(s.ctx, out.EncounterID)
	s.Require().NoError(err)
}

// regionByArchetype finds the single region tagged with the given
// archetype in space.Regions. Returns ok=false for BOTH zero matches and
// more than one match — a duplicate archetype is exactly as wrong as a
// missing one for the crypt spec's "exactly one entrance/corridor/boss"
// invariant, and silently returning the first of several duplicates would
// let that invariant break without any test noticing (Copilot review
// catch on this PR).
func regionByArchetype(space *tkenc.SpaceData, archetype tkenc.RegionArchetype) (tkenc.RegionData, bool) {
	var found tkenc.RegionData
	count := 0
	for _, r := range space.Regions {
		if r.Archetype == archetype {
			found = r
			count++
		}
	}
	return found, count == 1
}

// TestRegionByArchetype_DuplicateArchetype_ReturnsNotOK is the Copilot
// review regression pin (PR #693): a naive first-match implementation
// would report ok=true even with two regions sharing an archetype,
// silently masking the exact "InitDungeon accidentally produced two boss
// regions" failure mode the crypt-spec tests above rely on this helper to
// catch.
func TestRegionByArchetype_DuplicateArchetype_ReturnsNotOK(t *testing.T) {
	space := &tkenc.SpaceData{
		Regions: []tkenc.RegionData{
			{ID: "boss-1", Archetype: tkenc.ArchetypeBoss},
			{ID: "boss-2", Archetype: tkenc.ArchetypeBoss},
		},
	}
	_, ok := regionByArchetype(space, tkenc.ArchetypeBoss)
	require.False(t, ok, "two regions sharing an archetype must not resolve as a single match")
}

func TestRegionByArchetype_NoMatch_ReturnsNotOK(t *testing.T) {
	space := &tkenc.SpaceData{Regions: []tkenc.RegionData{{ID: "entrance", Archetype: tkenc.ArchetypeEntrance}}}
	_, ok := regionByArchetype(space, tkenc.ArchetypeBoss)
	require.False(t, ok)
}

// regionParamsByArchetype is regionByArchetype's sibling over
// tkenc.DungeonParams.Regions (the toolkit CONSTRUCTOR's input params,
// pre-generation) rather than over a persisted tkenc.SpaceData.Regions
// (post-generation output) — used by rpg-api#694's tests to read expected
// per-region Width/Obstacles directly off a fresh tkenc.CryptDungeonParams
// call instead of hardcoding a number that could silently drift from the
// toolkit's own crypt template.
func regionParamsByArchetype(params tkenc.DungeonParams, archetype tkenc.RegionArchetype) (tkenc.DungeonRegionParams, bool) {
	var found tkenc.DungeonRegionParams
	count := 0
	for _, r := range params.Regions {
		if r.Archetype == archetype {
			found = r
			count++
		}
	}
	return found, count == 1
}

func TestRegionByArchetype_ExactlyOneMatch_ReturnsIt(t *testing.T) {
	boss := tkenc.RegionData{ID: "boss", Archetype: tkenc.ArchetypeBoss}
	space := &tkenc.SpaceData{Regions: []tkenc.RegionData{{ID: "entrance", Archetype: tkenc.ArchetypeEntrance}, boss}}
	got, ok := regionByArchetype(space, tkenc.ArchetypeBoss)
	require.True(t, ok)
	require.Equal(t, boss, got)
}

// regionArchetypeAt returns the RegionArchetype of the region containing
// hex, and whether one was found — the archetype-keyed sibling of
// SpaceData.RegionAt (which only returns the region ID), used throughout
// these tests so assertions key off the toolkit's fixed generic-role
// vocabulary instead of this package's own spec-specific region ID
// strings (cryptRegionIDEntrance etc. are unexported — these tests are
// black-box, package lobby_test).
func regionArchetypeAt(space *tkenc.SpaceData, hex core.Hex) (tkenc.RegionArchetype, bool) {
	for _, r := range space.Regions {
		if r.Hexes.Has(hex) {
			return r.Archetype, true
		}
	}
	return "", false
}

// regionBoundingWidth returns a region's offset-coordinate column span
// (max X - min X + 1) — the same "width" tkenc.DungeonRegionParams.Width
// configures, read back purely from the persisted hex membership rather
// than any rpg-api-side constant, so assertions using this pin the
// toolkit's OWN generated geometry, not a duplicated expectation.
func regionBoundingWidth(region tkenc.RegionData) int {
	minX, maxX := math.MaxInt, math.MinInt
	for h := range region.Hexes {
		x := int(h.ToPosition().X)
		if x < minX {
			minX = x
		}
		if x > maxX {
			maxX = x
		}
	}
	return maxX - minX + 1
}

// TestStartEncounter_CryptDungeon_ThreeRegionsArchetypesThemeScaleAndEntrance
// is the rpg-api#688 headline proof (updated by rpg-api#694 to read
// expected dimensions off the toolkit constructor itself, never a
// hardcoded literal): StartEncounter now builds the toolkit's generic
// InitDungeon N-region linear chain selected by the crypt key
// (rpg-toolkit#814's Approved Slice 3 corrections, rpg-toolkit#826's
// CryptDungeonParams) instead of the retired two-chamber constants —
// exactly 3 regions (entrance -> corridor -> boss), each carrying its own
// RegionArchetype, Space.Theme == "crypt" passed through opaque and
// unbranched, the boss region's scale invariant (primary playable axis >
// 6 hex steps, enforced by the toolkit's own validateDungeonParams at
// generation time — not eyeballed here), and the party spawning inside
// the entrance region at its designated anchor.
//
// wantParams is built by calling tkenc.CryptDungeonParams directly with
// the SAME seed StartEncounter is given below — the door IDs passed here
// are throwaway placeholders (rpg-api#694: Width/Height/Theme/Regions/
// Obstacles never depend on which door ID string a caller picks), used
// only so this black-box test never hardcodes a region width/height that
// could silently drift from the toolkit's own crypt template.
func (s *LobbySuite) TestStartEncounter_CryptDungeon_ThreeRegionsArchetypesThemeScaleAndEntrance() {
	const seed = 42
	wantParams := tkenc.CryptDungeonParams(seed, "want-door-1", "want-door-2")

	s.seedReadyLobby("lobby-crypt1", "alice", "bob")
	s.expectCharacter("char-alice", "alice", "Alice", 12, 12)
	s.expectCharacter("char-bob", "bob", "Bob", 10, 10)

	out, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-crypt1",
		DungeonKey: lobbyorch.DungeonKeyCrypt, RandomSeed: seed,
	})
	s.Require().NoError(err)

	encData, err := s.encRepo.Get(s.ctx, out.EncounterID)
	s.Require().NoError(err)
	s.Require().NotNil(encData.Space, "StartEncounter must create the encounter with an InitDungeon space")
	s.Require().Equal(wantParams.Theme, encData.Space.Theme, "the crypt spec's opaque theme must pass through verbatim")
	s.Require().Equal(wantParams.Height, encData.Space.Height)
	wantWidth := 0
	for _, r := range wantParams.Regions {
		wantWidth += r.Width + 1
	}
	wantWidth-- // no trailing boundary column after the last region
	s.Require().Equal(wantWidth, encData.Space.Width,
		"total width = sum(region width + 1 door-column) - 1 trailing column, entirely toolkit-derived")
	s.Require().Len(encData.Space.Regions, 3, "the crypt spec is a 3-region linear chain: entrance -> corridor -> boss")

	entrance, entranceOK := regionByArchetype(encData.Space, tkenc.ArchetypeEntrance)
	s.Require().True(entranceOK, "exactly one entrance-archetype region")
	corridor, corridorOK := regionByArchetype(encData.Space, tkenc.ArchetypeCorridor)
	s.Require().True(corridorOK, "exactly one corridor-archetype region")
	boss, bossOK := regionByArchetype(encData.Space, tkenc.ArchetypeBoss)
	s.Require().True(bossOK, "exactly one boss-archetype region")

	entranceParams, _ := regionParamsByArchetype(wantParams, tkenc.ArchetypeEntrance)
	corridorParams, _ := regionParamsByArchetype(wantParams, tkenc.ArchetypeCorridor)
	bossParams, _ := regionParamsByArchetype(wantParams, tkenc.ArchetypeBoss)
	s.Require().Equal(entranceParams.Width, regionBoundingWidth(entrance))
	s.Require().Equal(corridorParams.Width, regionBoundingWidth(corridor))
	s.Require().Equal(bossParams.Width, regionBoundingWidth(boss))

	// rpg-toolkit#814's Approved Slice 3 corrections' scale invariant: the
	// boss region's primary playable axis (min(width, shared height)) must
	// exceed 6 hex steps — enforced by the toolkit's own
	// validateDungeonParams at generation time, not eyeballed here.
	bossAxis := regionBoundingWidth(boss)
	if encData.Space.Height < bossAxis {
		bossAxis = encData.Space.Height
	}
	s.Require().Greater(bossAxis, 6, "boss chamber primary playable axis must exceed 6 hex steps")

	// Exactly 2 toolkit-configured connector doors join the 3-region chain.
	// The entrance remains plain; the canonical crypt constructor owns the
	// locked boss connector's DC/ability/tool configuration.
	s.Require().Len(encData.Doors, 2, "a 3-region chain has exactly 2 connectors")
	var entranceDoor, bossDoor *tkenc.DoorData
	for _, door := range encData.Doors {
		if door.Locked {
			bossDoor = door
		} else {
			entranceDoor = door
		}
	}
	s.Require().NotNil(entranceDoor)
	s.Require().NotNil(bossDoor)
	s.Require().False(entranceDoor.Open)
	s.Require().False(entranceDoor.Locked)
	s.Require().False(bossDoor.Open)
	s.Require().True(bossDoor.Locked)
	s.Require().Equal(12, bossDoor.LockDC)
	s.Require().Equal("dex", bossDoor.LockAbility)
	s.Require().Empty(bossDoor.LockTool)

	// Entrance-anchored spawn (rpg-api#648/#676, preserved): every seated
	// member lands inside the entrance region, and the first member sits
	// exactly at the designated Space.Entrance.
	alice := encData.Players[core.PlayerID("alice")]
	bob := encData.Players[core.PlayerID("bob")]
	s.Require().NotNil(alice)
	s.Require().Equal(encData.Space.Entrance, alice.View.Position,
		"the first member must spawn exactly at the designated entrance")
	aliceArchetype, aliceOK := regionArchetypeAt(encData.Space, alice.View.Position)
	s.Require().True(aliceOK)
	s.Require().Equal(tkenc.ArchetypeEntrance, aliceArchetype)
	bobArchetype, bobOK := regionArchetypeAt(encData.Space, bob.View.Position)
	s.Require().True(bobOK)
	s.Require().Equal(tkenc.ArchetypeEntrance, bobArchetype, "every member must spawn inside the entrance region")

	// No FreeRoam/TurnBased assertion here (rpg-api#689, 2026-07-23): this
	// test predates the deterministic-anchor monster composition merged in
	// alongside rpg-api#694. The retired out-of-sight goblin search
	// guaranteed FreeRoam at spawn; #689's FixedPositions anchors carry no
	// such guarantee and the revised issue explicitly allows the entrance
	// monster to be visible and trigger combat immediately — see
	// TestStartEncounter_CryptMonsters_OneEntranceSkeletonZeroCorridorOneBoss's
	// own doc for the same reasoning. Whatever the real toolkit LoS produces
	// is what StartEncounter must honor, not an rpg-api-side guarantee.
}

// obstaclesByRef tallies a region's placed tkenc.ObstacleData by Ref, and
// records the (BlocksMovement, BlocksLoS) pair seen for each Ref — used by
// the obstacle tests below to assert exact counts and blocking flags
// without hardcoding a region's full obstacle list inline per test. Fails
// fast (via require.Equal) if the SAME Ref ever shows two different
// blocking-flag pairs across its placed instances — Copilot review catch
// on this PR: a naive last-one-wins map assignment would silently accept
// inconsistent data and still pass every per-ref blocking assertion below,
// masking exactly the kind of bad data these tests exist to catch.
type obstacleBlocking struct {
	blocksMovement bool
	blocksLoS      bool
}

func obstaclesByRef(
	t require.TestingT, obstacles []tkenc.ObstacleData,
) (counts map[string]int, blocking map[string]obstacleBlocking) {
	counts = map[string]int{}
	blocking = map[string]obstacleBlocking{}
	for _, o := range obstacles {
		counts[o.Ref]++
		got := obstacleBlocking{blocksMovement: o.BlocksMovement, blocksLoS: o.BlocksLoS}
		if existing, seen := blocking[o.Ref]; seen {
			require.Equal(t, existing, got,
				"obstacle ref %q must carry consistent blocking flags across every placed instance", o.Ref)
		}
		blocking[o.Ref] = got
	}
	return counts, blocking
}

// TestStartEncounter_CryptObstacles_ExactRefsCountsAndBlockingByRegion is
// rpg-api#694's headline proof: a REAL StartEncounter call persists a
// non-empty SpaceData.Obstacles list whose refs/counts/blocking flags are
// the EXACT canonical set rpg-toolkit#826's CryptDungeonParams places —
// entrance (1 obelisk + 2 pillars), corridor (1 pillar), boss (1 coffin +
// 1 altar + 1 statue-reaper + 1 statue-knight-hooded) — read off the
// toolkit's own exported Ref constants, never a string this package
// invents. rpg-api never computed this list itself; it is entirely the
// toolkit constructor's output, InitDungeon's placement, and this
// package's job is only to have asked for it via CryptDungeonParams.
func (s *LobbySuite) TestStartEncounter_CryptObstacles_ExactRefsCountsAndBlockingByRegion() {
	s.seedReadyLobby("lobby-crypt-obstacles1", "alice")
	s.expectCharacter("char-alice", "alice", "Alice", 12, 12)

	out, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-crypt-obstacles1",
		DungeonKey: lobbyorch.DungeonKeyCrypt, RandomSeed: 300,
	})
	s.Require().NoError(err)

	encData, err := s.encRepo.Get(s.ctx, out.EncounterID)
	s.Require().NoError(err)
	s.Require().NotEmpty(encData.Space.Obstacles, "a real StartEncounter call must persist a non-empty obstacle list")

	byRegion := map[tkenc.RegionArchetype][]tkenc.ObstacleData{}
	seenIDs := make(map[core.EntityID]bool, len(encData.Space.Obstacles))
	for _, o := range encData.Space.Obstacles {
		s.Require().False(seenIDs[o.ID], "obstacle IDs must be unique: %q", o.ID)
		seenIDs[o.ID] = true
		archetype, ok := regionArchetypeAt(encData.Space, o.Position)
		s.Require().True(ok, "obstacle %q must be placed inside a tagged region", o.ID)
		byRegion[archetype] = append(byRegion[archetype], o)
	}

	entranceCounts, entranceBlocking := obstaclesByRef(s.T(), byRegion[tkenc.ArchetypeEntrance])
	s.Require().Equal(map[string]int{
		tkenc.CryptObstacleRefObelisk:  1,
		tkenc.CryptObstacleRefPillar:   2,
		tkenc.CryptObstacleRefBrazier:  2,
		tkenc.CryptObstacleRefBonePile: 2,
	}, entranceCounts, "entrance region: exactly 1 obelisk + 2 pillars + 2 braziers + 2 bone-piles "+
		"(rpg-toolkit#839 depth-pass dressing)")
	s.Require().Equal(obstacleBlocking{blocksMovement: true, blocksLoS: true}, entranceBlocking[tkenc.CryptObstacleRefObelisk])
	s.Require().Equal(obstacleBlocking{blocksMovement: true, blocksLoS: true}, entranceBlocking[tkenc.CryptObstacleRefPillar])
	s.Require().Equal(obstacleBlocking{blocksMovement: true, blocksLoS: false}, entranceBlocking[tkenc.CryptObstacleRefBrazier],
		"a brazier blocks movement but not line of sight -- you can see over the flame")
	s.Require().Equal(obstacleBlocking{blocksMovement: false, blocksLoS: false}, entranceBlocking[tkenc.CryptObstacleRefBonePile],
		"a bone-pile is walkable-past floor dressing -- blocks neither movement nor line of sight")

	corridorCounts, corridorBlocking := obstaclesByRef(s.T(), byRegion[tkenc.ArchetypeCorridor])
	s.Require().Equal(map[string]int{
		tkenc.CryptObstacleRefPillar:      1,
		tkenc.CryptObstacleRefTorchOrnate: 1,
	}, corridorCounts, "corridor region: exactly 1 sparse pillar + 1 torch-ornate light anchor "+
		"(rpg-toolkit#839), still no others")
	s.Require().Equal(obstacleBlocking{blocksMovement: true, blocksLoS: false},
		corridorBlocking[tkenc.CryptObstacleRefTorchOrnate],
		"a torch-ornate blocks movement but not line of sight -- same shape as a brazier")

	bossCounts, bossBlocking := obstaclesByRef(s.T(), byRegion[tkenc.ArchetypeBoss])
	s.Require().Equal(map[string]int{
		tkenc.CryptObstacleRefCoffin:             1,
		tkenc.CryptObstacleRefAltar:              1,
		tkenc.CryptObstacleRefStatueReaper:       1,
		tkenc.CryptObstacleRefStatueKnightHooded: 1,
		tkenc.CryptObstacleRefCandles:            2,
		tkenc.CryptObstacleRefBrazier:            2,
		tkenc.CryptObstacleRefChain:              1,
		tkenc.CryptObstacleRefSkeletonRemains:    1,
	}, bossCounts, "boss region: coffin + altar + one of each statue variant, plus rpg-toolkit#839's "+
		"2 candles + 2 braziers + 1 chain + 1 skeleton-remains")
	s.Require().Equal(obstacleBlocking{blocksMovement: true, blocksLoS: false}, bossBlocking[tkenc.CryptObstacleRefCoffin],
		"the coffin/tomb blocks movement but not line of sight -- walk around, see over")
	s.Require().Equal(obstacleBlocking{blocksMovement: true, blocksLoS: true}, bossBlocking[tkenc.CryptObstacleRefAltar])
	s.Require().Equal(obstacleBlocking{blocksMovement: true, blocksLoS: true}, bossBlocking[tkenc.CryptObstacleRefStatueReaper])
	s.Require().Equal(obstacleBlocking{blocksMovement: true, blocksLoS: true},
		bossBlocking[tkenc.CryptObstacleRefStatueKnightHooded])
	s.Require().Equal(obstacleBlocking{blocksMovement: true, blocksLoS: false}, bossBlocking[tkenc.CryptObstacleRefBrazier],
		"a brazier blocks movement but not line of sight -- you can see over the flame")
	s.Require().Equal(obstacleBlocking{blocksMovement: false, blocksLoS: false}, bossBlocking[tkenc.CryptObstacleRefCandles],
		"candles are walkable-past floor dressing -- block neither movement nor line of sight")
	s.Require().Equal(obstacleBlocking{blocksMovement: false, blocksLoS: false}, bossBlocking[tkenc.CryptObstacleRefChain],
		"a chain is walkable-past floor dressing -- blocks neither movement nor line of sight")
	s.Require().Equal(obstacleBlocking{blocksMovement: false, blocksLoS: false},
		bossBlocking[tkenc.CryptObstacleRefSkeletonRemains],
		"skeleton-remains is walkable-past floor dressing -- blocks neither movement nor line of sight")
}

// TestStartEncounter_CryptObstacles_ExplicitSeed_DeterministicPositions
// proves obstacle PLACEMENT (not just refs/counts) is deterministic under
// an explicit seed, mirroring TestStartEncounter_ExplicitSeed_
// ReproducibleDungeonLayout's whole-Space proof but calling out obstacles
// specifically since they are this issue's headline addition.
func (s *LobbySuite) TestStartEncounter_CryptObstacles_ExplicitSeed_DeterministicPositions() {
	s.seedReadyLobby("lobby-crypt-obstacles2a", "alice")
	s.expectCharacter("char-alice", "alice", "Alice", 12, 12)
	out1, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-crypt-obstacles2a", RandomSeed: 271,
	})
	s.Require().NoError(err)
	data1, err := s.encRepo.Get(s.ctx, out1.EncounterID)
	s.Require().NoError(err)

	s.seedReadyLobby("lobby-crypt-obstacles2b", "bob")
	s.expectCharacter("char-bob", "bob", "Bob", 10, 10)
	out2, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "bob", LobbyID: "lobby-crypt-obstacles2b", RandomSeed: 271,
	})
	s.Require().NoError(err)
	data2, err := s.encRepo.Get(s.ctx, out2.EncounterID)
	s.Require().NoError(err)

	s.Require().NotEmpty(data1.Space.Obstacles)
	s.Require().Equal(data1.Space.Obstacles, data2.Space.Obstacles,
		"the same explicit RandomSeed must reproduce byte-identical obstacle placement")
}

// TestStartEncounter_CryptMonsters_OneEntranceSkeletonZeroCorridorOneBoss
// is the rpg-api#689 headline proof, superseding the retired goblin test of
// the same rough shape (2026-07-23 approved first-swing simplification):
// exactly ONE skeleton in the entrance region, ZERO in the corridor, and
// exactly ONE non-wight skeleton-captain boss (rpg-toolkit#816) in the
// boss region — no goblins, no PositionOracle search. Concealment (if any)
// comes from real door/wall LoS geometry, not a placement predicate — this
// test does not assert either FreeRoam or TurnBased at spawn, since the
// revised issue explicitly allows the entrance monster to be visible and
// trigger combat immediately; whatever the real toolkit LoS produces here
// is what StartEncounter must honor, not an rpg-api-side guarantee.
func (s *LobbySuite) TestStartEncounter_CryptMonsters_OneEntranceSkeletonZeroCorridorOneBoss() {
	s.seedReadyLobby("lobby-crypt2", "alice")
	s.expectCharacter("char-alice", "alice", "Alice", 12, 12)

	out, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-crypt2",
	})
	s.Require().NoError(err)

	encData, err := s.encRepo.Get(s.ctx, out.EncounterID)
	s.Require().NoError(err)

	s.Require().Len(encData.Monsters, 2, "exactly one entrance skeleton + one boss skeleton-captain, no goblins")
	archetypeCounts := map[tkenc.RegionArchetype]int{}
	for id, m := range encData.Monsters {
		archetype, ok := regionArchetypeAt(encData.Space, m.Position)
		s.Require().True(ok, "monster %q must be placed inside a tagged region", id)
		archetypeCounts[archetype]++
		s.Require().Positive(m.HP, "monster %q must have positive HP", id)
		s.Require().NotEmpty(m.DataJSON, "monster %q must carry hydration DataJSON", id)
		switch archetype {
		case tkenc.ArchetypeEntrance:
			s.Require().Equal(refs.Monsters.Skeleton().String(), m.MonsterRef,
				"the entrance monster must be the plain skeleton, never a goblin")
		case tkenc.ArchetypeBoss:
			s.Require().Equal(refs.Monsters.SkeletonCaptain().String(), m.MonsterRef,
				"the boss must be rpg-toolkit#816's non-wight skeleton captain")
		default:
			s.Fail("unexpected monster region archetype", "archetype=%q", archetype)
		}
	}
	s.Require().Equal(1, archetypeCounts[tkenc.ArchetypeEntrance], "exactly one entrance monster")
	s.Require().Equal(1, archetypeCounts[tkenc.ArchetypeBoss], "exactly one boss monster")
	s.Require().Zero(archetypeCounts[tkenc.ArchetypeCorridor], "the interior corridor region must get no monsters")

	// The boss must be concealed by the closed connector door's LoS
	// blocking, not by a placement search: rehydrate and directly verify
	// no seated player can currently see the boss.
	enc, err := tkenc.LoadFromData(s.ctx, encData, s.encBroker)
	s.Require().NoError(err)
	var bossPos core.Hex
	for _, m := range encData.Monsters {
		if archetype, _ := regionArchetypeAt(encData.Space, m.Position); archetype == tkenc.ArchetypeBoss {
			bossPos = m.Position
		}
	}
	for _, p := range enc.ToData().Players {
		s.Require().False(p.View != nil && perception.CanSeeAt(p.View, bossPos, enc.Room()),
			"the boss must stay concealed behind its closed connector door at spawn")
	}
}

// TestStartEncounter_CryptMonsters_SameSeedByteIdenticalPositions proves
// rpg-api#689's determinism done bar end-to-end through the real
// StartEncounter/repo path (in-memory here; real-Redis coverage lives in
// internal/integration/lobby_crypt_monster_seed_test.go): the same
// explicit seed on two independent encounters produces byte-identical
// monster positions.
func (s *LobbySuite) TestStartEncounter_CryptMonsters_SameSeedByteIdenticalPositions() {
	s.seedReadyLobby("lobby-crypt-seed-a", "alice")
	s.expectCharacter("char-alice", "alice", "Alice", 12, 12)
	s.seedReadyLobby("lobby-crypt-seed-b", "bob")
	s.expectCharacter("char-bob", "bob", "Bob", 10, 10)

	outA, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-crypt-seed-a", RandomSeed: 2026,
	})
	s.Require().NoError(err)
	outB, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "bob", LobbyID: "lobby-crypt-seed-b", RandomSeed: 2026,
	})
	s.Require().NoError(err)

	dataA, err := s.encRepo.Get(s.ctx, outA.EncounterID)
	s.Require().NoError(err)
	dataB, err := s.encRepo.Get(s.ctx, outB.EncounterID)
	s.Require().NoError(err)

	positionsByArchetype := func(data *tkenc.Data) map[tkenc.RegionArchetype]core.Hex {
		out := map[tkenc.RegionArchetype]core.Hex{}
		for _, m := range data.Monsters {
			archetype, ok := regionArchetypeAt(data.Space, m.Position)
			s.Require().True(ok)
			out[archetype] = m.Position
		}
		return out
	}
	s.Require().Equal(positionsByArchetype(dataA), positionsByArchetype(dataB),
		"the same explicit seed must produce byte-identical monster positions across independent encounters")
}

// TestStartEncounter_DefaultDungeonKey_MatchesExplicitCryptKey proves
// rpg-api#688's Scope-section default: StartEncounterInput.DungeonKey's
// zero value must resolve to the exact same spec as explicitly passing
// DungeonKeyCrypt — not merely "some dungeon," the same literal geometry
// given the same seed.
func (s *LobbySuite) TestStartEncounter_DefaultDungeonKey_MatchesExplicitCryptKey() {
	s.seedReadyLobby("lobby-crypt3a", "alice")
	s.expectCharacter("char-alice", "alice", "Alice", 12, 12)
	outDefault, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-crypt3a", RandomSeed: 7,
	})
	s.Require().NoError(err)
	defaultData, err := s.encRepo.Get(s.ctx, outDefault.EncounterID)
	s.Require().NoError(err)

	s.seedReadyLobby("lobby-crypt3b", "carol")
	s.expectCharacter("char-carol", "carol", "Carol", 12, 12)
	outExplicit, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "carol", LobbyID: "lobby-crypt3b", RandomSeed: 7, DungeonKey: lobbyorch.DungeonKeyCrypt,
	})
	s.Require().NoError(err)
	explicitData, err := s.encRepo.Get(s.ctx, outExplicit.EncounterID)
	s.Require().NoError(err)

	s.Require().Equal(defaultData.Space, explicitData.Space,
		"the zero-value DungeonKey must resolve to the same spec as an explicit DungeonKeyCrypt")
}

// TestStartEncounter_UnknownDungeonKey_ErrorsAndLobbyStaysWaiting proves
// rpg-api#688's boundary: an unrecognized key fails LOUDLY — rpg-api never
// invents geometry for a key it doesn't recognize — and fails BEFORE any
// state transition (no encounter persisted, lobby stays WAITING).
func (s *LobbySuite) TestStartEncounter_UnknownDungeonKey_ErrorsAndLobbyStaysWaiting() {
	s.seedReadyLobby("lobby-crypt4", "alice")
	// No expectCharacter armed: resolving the dungeon key fails before
	// StartEncounter ever reaches character-snapshot seeding — gomock would
	// fail this test outright if that assumption were wrong.

	_, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-crypt4", DungeonKey: lobbyorch.DungeonKey("no-such-key"),
	})
	s.Require().Error(err)
	s.Require().ErrorIs(err, lobbyorch.ErrUnknownDungeonKey)

	lobbyData, err := s.lobbyRepo.Get(s.ctx, "lobby-crypt4")
	s.Require().NoError(err)
	s.Require().Equal(lobbyrepo.StatusWaiting, lobbyData.Status, "an unknown dungeon key must fail before any state transition")
	s.Require().Empty(lobbyData.EncounterID)
}

// TestStartEncounter_ExplicitSeed_ReproducibleDungeonLayout proves seed
// semantics survive the InitDungeon migration: the same explicit
// RandomSeed passed through tkenc.DungeonParams.RandomSeed must reproduce
// byte-identical dungeon geometry across two entirely independent
// encounters.
func (s *LobbySuite) TestStartEncounter_ExplicitSeed_ReproducibleDungeonLayout() {
	s.seedReadyLobby("lobby-crypt5a", "alice")
	s.expectCharacter("char-alice", "alice", "Alice", 12, 12)
	out1, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-crypt5a", RandomSeed: 999,
	})
	s.Require().NoError(err)
	data1, err := s.encRepo.Get(s.ctx, out1.EncounterID)
	s.Require().NoError(err)

	s.seedReadyLobby("lobby-crypt5b", "bob")
	s.expectCharacter("char-bob", "bob", "Bob", 10, 10)
	out2, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "bob", LobbyID: "lobby-crypt5b", RandomSeed: 999,
	})
	s.Require().NoError(err)
	data2, err := s.encRepo.Get(s.ctx, out2.EncounterID)
	s.Require().NoError(err)

	s.Require().Equal(data1.Space, data2.Space, "the same explicit RandomSeed must reproduce the identical dungeon layout")
}

// TestStartEncounter_DungeonGeometryIndependentOfPartySize is the API-
// boundary proof: given the SAME key+seed, dungeon geometry (Space and
// connector door positions) must be IDENTICAL regardless of party size —
// InitDungeon runs before any AddPlayer call and takes no party
// information as input, so a runtime request shape difference (1 member
// vs 2) can never leak into rpg-api-side geometry derivation. Monster
// placement is deliberately NOT compared here — it legitimately depends
// on which players are seated (out-of-sight-of-every-player-view), a
// separate, pre-existing mechanism this issue doesn't touch.
func (s *LobbySuite) TestStartEncounter_DungeonGeometryIndependentOfPartySize() {
	s.seedReadyLobby("lobby-crypt6a", "alice")
	s.expectCharacter("char-alice", "alice", "Alice", 12, 12)
	out1, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-crypt6a", RandomSeed: 555,
	})
	s.Require().NoError(err)
	data1, err := s.encRepo.Get(s.ctx, out1.EncounterID)
	s.Require().NoError(err)

	s.seedReadyLobby("lobby-crypt6b", "carol", "dave")
	s.expectCharacter("char-carol", "carol", "Carol", 14, 14)
	s.expectCharacter("char-dave", "dave", "Dave", 11, 11)
	out2, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "carol", LobbyID: "lobby-crypt6b", RandomSeed: 555,
	})
	s.Require().NoError(err)
	data2, err := s.encRepo.Get(s.ctx, out2.EncounterID)
	s.Require().NoError(err)

	s.Require().Equal(data1.Space, data2.Space,
		"dungeon geometry must be identical regardless of party size — rpg-api never derives geometry "+
			"from runtime request shape, only the (key, seed) pair passed verbatim to the toolkit")

	doorPositions := func(d *tkenc.Data) map[core.EntityID]core.Hex {
		out := make(map[core.EntityID]core.Hex, len(d.Doors))
		for id, door := range d.Doors {
			out[id] = door.Position
		}
		return out
	}
	s.Require().Equal(doorPositions(data1), doorPositions(data2), "connector door positions are geometry too")
}

// --- Task E2: content-backed dungeon keys ---

// placedThing is one placed monster or prop's ref, owning region, and
// absolute cell — the comparison unit for pinning reference-tomb's exact
// compiled layout (see TestStartEncounter_ContentBackedKey_ReferenceTomb).
type placedThing struct {
	ref       string
	region    string
	at        core.Hex
	blocksLoS bool // only meaningful for obstacles; zero value for monsters
}

func sortPlacedThings(things []placedThing) {
	sort.Slice(things, func(i, j int) bool {
		if things[i].ref != things[j].ref {
			return things[i].ref < things[j].ref
		}
		if things[i].at.Q != things[j].at.Q {
			return things[i].at.Q < things[j].at.Q
		}
		if things[i].at.R != things[j].at.R {
			return things[i].at.R < things[j].at.R
		}
		return things[i].at.S < things[j].at.S
	})
}

// TestStartEncounter_ContentBackedKey_ReferenceTomb is the M1 acceptance
// proof at the orchestrator level: StartEncounter("reference-tomb")
// builds the content-hosted spec (internal/content/dungeons/
// reference-tomb.yaml, shipped embedded — no RPG_CONTENT_DIR override
// needed) via the SAME toolkit InitDungeon + SeedMonsters path a real
// content-authored dungeon would use, and the EXISTING crypt-path tests
// (TestStartEncounter_CryptDungeon_..., TestStartEncounter_CryptMonsters_...,
// TestStartEncounter_CryptObstacles_...) stay green completely unperturbed
// by this branch's addition.
//
// reference-tomb became a 3-room spec (entrance -> hall -> tomb, Kirk's
// live-authored draft, approved in-game 2026-07-25) — every want* value
// below was MEASURED directly off the real compiler + engine (a throwaway
// program: dungeonspec.Load -> InitDungeon(seed 42) -> SeedMonsters,
// reading back encData.Monsters/Space.Obstacles/Doors), never hand-derived.
// Every position in the spec is placed (place:/boss.at), never rolled, so
// these are seed-independent, stable absolute hexes.
func (s *LobbySuite) TestStartEncounter_ContentBackedKey_ReferenceTomb() {
	s.seedReadyLobby("lobby-tomb1", "alice")
	s.expectCharacter("char-alice", "alice", "Alice", 12, 12)

	out, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-tomb1",
		DungeonKey: lobbyorch.DungeonKey("reference-tomb"), RandomSeed: 42,
	})
	s.Require().NoError(err)

	encData, err := s.encRepo.Get(s.ctx, out.EncounterID)
	s.Require().NoError(err)
	s.Require().NotNil(encData.Space)
	s.Require().Equal("crypt", encData.Space.Theme)
	s.Require().Len(encData.Space.Regions, 3, "reference-tomb is now a three-room spec: entrance -> hall -> tomb")

	_, entranceOK := regionByArchetype(encData.Space, tkenc.ArchetypeEntrance)
	s.Require().True(entranceOK, "exactly one entrance-archetype region")
	hall, hallOK := regionByArchetype(encData.Space, tkenc.ArchetypeChamber)
	s.Require().True(hallOK, "exactly one chamber-archetype region (the hall)")
	tomb, tombOK := regionByArchetype(encData.Space, tkenc.ArchetypeBoss)
	s.Require().True(tombOK, "exactly one boss-archetype region (the tomb)")

	// The hall->tomb connector is locked (DC 12, dex) — the entrance->hall
	// connector is not. Exactly one locked door, with the spec's exact
	// lock parameters surviving compile -> InitDungeon -> persisted DoorData.
	var lockedDoors int
	for _, d := range encData.Doors {
		if !d.Locked {
			continue
		}
		lockedDoors++
		s.Assert().Equal(12, d.LockDC)
		s.Assert().Equal("dex", d.LockAbility)
	}
	s.Require().Equal(1, lockedDoors, "exactly the hall->tomb connector is locked")

	// Three monster spawns: two placed skeletons in the hall, the pinned
	// boss (skeleton-captain) in the tomb.
	wantMonsters := []placedThing{
		{ref: "dnd5e:monsters:skeleton", region: hall.ID, at: core.Hex{Q: 12, R: -9, S: -3}},
		{ref: "dnd5e:monsters:skeleton", region: hall.ID, at: core.Hex{Q: 14, R: -12, S: -2}},
		{ref: "dnd5e:monsters:skeleton-captain", region: tomb.ID, at: core.Hex{Q: 25, R: -18, S: -7}},
	}
	gotMonsters := make([]placedThing, 0, len(encData.Monsters))
	for _, m := range encData.Monsters {
		regionID, ok := encData.Space.RegionAt(m.Position)
		s.Require().True(ok, "every seeded monster must land on a tagged region hex")
		gotMonsters = append(gotMonsters, placedThing{ref: m.MonsterRef, region: regionID, at: m.Position})
	}
	sortPlacedThings(wantMonsters)
	sortPlacedThings(gotMonsters)
	s.Require().Equal(wantMonsters, gotMonsters, "every placed monster must land at its exact compiled cell, in its declared room")

	// Fifteen placed props across all three rooms, with the spec's
	// blocks_los override (coffin: false) and the toolkit's true default
	// (every other prop) both surviving compile -> InitDungeon ->
	// persisted ObstacleData.
	wantObstacles := []placedThing{
		{ref: "dnd5e:props:altar", region: tomb.ID, at: core.Hex{Q: 27, R: -17, S: -10}, blocksLoS: true},
		{ref: "dnd5e:props:bone-pile", region: hall.ID, at: core.Hex{Q: 15, R: -14, S: -1}, blocksLoS: true},
		{ref: "dnd5e:props:brazier", region: "entrance", at: core.Hex{Q: 1, R: -2, S: 1}, blocksLoS: true},
		{ref: "dnd5e:props:brazier", region: "entrance", at: core.Hex{Q: 1, R: -7, S: 6}, blocksLoS: true},
		{ref: "dnd5e:props:brazier", region: tomb.ID, at: core.Hex{Q: 21, R: -12, S: -9}, blocksLoS: true},
		{ref: "dnd5e:props:brazier", region: tomb.ID, at: core.Hex{Q: 21, R: -17, S: -4}, blocksLoS: true},
		{ref: "dnd5e:props:candles", region: tomb.ID, at: core.Hex{Q: 28, R: -19, S: -9}, blocksLoS: true},
		{ref: "dnd5e:props:coffin", region: tomb.ID, at: core.Hex{Q: 24, R: -15, S: -9}, blocksLoS: false},
		{ref: "dnd5e:props:pillar", region: hall.ID, at: core.Hex{Q: 9, R: -10, S: 1}, blocksLoS: true},
		{ref: "dnd5e:props:pillar", region: hall.ID, at: core.Hex{Q: 9, R: -7, S: -2}, blocksLoS: true},
		{ref: "dnd5e:props:pillar", region: hall.ID, at: core.Hex{Q: 13, R: -9, S: -4}, blocksLoS: true},
		{ref: "dnd5e:props:pillar", region: hall.ID, at: core.Hex{Q: 13, R: -12, S: -1}, blocksLoS: true},
		{ref: "dnd5e:props:statue-knight-hooded", region: tomb.ID, at: core.Hex{Q: 19, R: -16, S: -3}, blocksLoS: true},
		{ref: "dnd5e:props:statue-reaper", region: tomb.ID, at: core.Hex{Q: 19, R: -11, S: -8}, blocksLoS: true},
		{ref: "dnd5e:props:torch-ornate", region: hall.ID, at: core.Hex{Q: 11, R: -7, S: -4}, blocksLoS: true},
	}
	gotObstacles := make([]placedThing, 0, len(encData.Space.Obstacles))
	for _, obstacle := range encData.Space.Obstacles {
		regionID, ok := encData.Space.RegionAt(obstacle.Position)
		s.Require().True(ok, "every placed obstacle must land on a tagged region hex")
		gotObstacles = append(gotObstacles, placedThing{
			ref: obstacle.Ref, region: regionID, at: obstacle.Position, blocksLoS: obstacle.BlocksLoS,
		})
	}
	sortPlacedThings(wantObstacles)
	sortPlacedThings(gotObstacles)
	s.Require().Equal(wantObstacles, gotObstacles, "every placed prop must land at its exact compiled cell, in its declared room, with its exact blocks_los")
}

// TestStartEncounter_DisabledContentKey_ErrorsBeforeLobbyLock proves the
// "found but disabled" branch (Task E2's three-state contract): a
// content-backed key whose file fails dungeonspec.Load at startup must
// return a *lobbyorch.DisabledDungeonKeyError, before touching the lobby
// lock — same zero-side-effects posture as an unknown legacy key
// (TestStartEncounter_UnknownDungeonKey_ErrorsAndLobbyStaysWaiting above).
// Uses a dedicated Orchestrator (newOrchestratorWithContentDir) since
// loadContentSpecs only reads RPG_CONTENT_DIR once, at construction.
func (s *LobbySuite) TestStartEncounter_DisabledContentKey_ErrorsBeforeLobbyLock() {
	dir := s.T().TempDir()
	require.NoError(s.T(), os.WriteFile(
		filepath.Join(dir, "broken-dungeon.yaml"),
		[]byte("version: 1\nkey: broken-dungeon\nname: Broken\nheight: 8\nrooms:\n"+
			"  - id: only-one\n    archetype: entrance\n    width: 6\n"),
		0o600,
	))
	orch, err := s.newOrchestratorWithContentDir(dir)
	s.Require().NoError(err, "a schema-invalid content FILE must not fail construction — only an unreadable path does")

	s.seedReadyLobby("lobby-broken1", "alice")
	// No expectCharacter armed: resolving the content spec fails before
	// StartEncounter ever reaches character-snapshot seeding.

	_, err = orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-broken1",
		DungeonKey: lobbyorch.DungeonKey("broken-dungeon"),
	})
	s.Require().Error(err)
	var disabledErr *lobbyorch.DisabledDungeonKeyError
	s.Require().True(errors.As(err, &disabledErr), "must be a *DisabledDungeonKeyError, not a generic error")
	s.Assert().Equal(lobbyorch.DungeonKey("broken-dungeon"), disabledErr.Key)

	lobbyData, err := s.lobbyRepo.Get(s.ctx, "lobby-broken1")
	s.Require().NoError(err)
	s.Require().Equal(lobbyrepo.StatusWaiting, lobbyData.Status, "a disabled content key must fail before any state transition")
	s.Require().Empty(lobbyData.EncounterID)
}

// scatteredSeedTestYAML is a content-backed spec whose ONLY seed-varying
// geometry lives in the entrance room (pattern: scattered, no place:
// entries — design.md's delta forbids place:/boss.at together with a
// scattered pattern). The tomb room's pinned boss.at (M1's own
// restriction requires a boss room to pin its boss) compiles its pattern
// to "empty" (verified directly against the compiler, not assumed) — a
// room with placed/pinned content never gets rolled interior walls, so
// only the entrance's own random walls can prove RandomSeed wiring here.
// entrance's width (20) is deliberately generous: verified empirically
// that a width as small as 6 rolls IDENTICAL walls for every seed 1-50
// (the margin heuristic leaves too few interior candidate cells to vary)
// — width 20 was checked to produce 47 distinct wall layouts across the
// same 50-seed sweep, and seeds 111/222 specifically (this test's values)
// were confirmed to differ before being hardcoded here.
const scatteredSeedTestYAML = `
version: 1
key: scattered-seed-test
name: Scattered Seed Test
theme: crypt
height: 8
rooms:
  - id: entrance
    archetype: entrance
    width: 20
    pattern: scattered
  - id: tomb
    archetype: boss
    width: 12
    boss: { ref: "dnd5e:monsters:skeleton-captain", at: [7, 5] }
connectors:
  - { from: entrance, to: tomb }
`

// TestStartEncounter_ContentBackedKey_SeedWiring proves compiled.Params.
// RandomSeed = in.RandomSeed (start_encounter.go) actually reaches
// InitDungeon: the same seed must reproduce byte-identical scattered-
// pattern geometry across two independent encounters, and a different
// seed must roll different geometry — the content-backed-branch
// equivalent of TestStartEncounter_ExplicitSeed_ReproducibleDungeonLayout
// for the legacy crypt path.
func (s *LobbySuite) TestStartEncounter_ContentBackedKey_SeedWiring() {
	dir := s.T().TempDir()
	require.NoError(s.T(), os.WriteFile(
		filepath.Join(dir, "scattered-seed-test.yaml"), []byte(scatteredSeedTestYAML), 0o600,
	))
	orch, err := s.newOrchestratorWithContentDir(dir)
	s.Require().NoError(err)

	s.seedReadyLobby("lobby-seed-a", "alice")
	s.expectCharacter("char-alice", "alice", "Alice", 12, 12)
	outA, err := orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-seed-a",
		DungeonKey: lobbyorch.DungeonKey("scattered-seed-test"), RandomSeed: 111,
	})
	s.Require().NoError(err)
	dataA, err := s.encRepo.Get(s.ctx, outA.EncounterID)
	s.Require().NoError(err)

	s.seedReadyLobby("lobby-seed-b", "bob")
	s.expectCharacter("char-bob", "bob", "Bob", 12, 12)
	outB, err := orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "bob", LobbyID: "lobby-seed-b",
		DungeonKey: lobbyorch.DungeonKey("scattered-seed-test"), RandomSeed: 111,
	})
	s.Require().NoError(err)
	dataB, err := s.encRepo.Get(s.ctx, outB.EncounterID)
	s.Require().NoError(err)
	s.Require().Equal(dataA.Space, dataB.Space, "the same seed must reproduce identical content-backed dungeon geometry")

	s.seedReadyLobby("lobby-seed-c", "carol")
	s.expectCharacter("char-carol", "carol", "Carol", 12, 12)
	outC, err := orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "carol", LobbyID: "lobby-seed-c",
		DungeonKey: lobbyorch.DungeonKey("scattered-seed-test"), RandomSeed: 222,
	})
	s.Require().NoError(err)
	dataC, err := s.encRepo.Get(s.ctx, outC.EncounterID)
	s.Require().NoError(err)
	s.Require().NotEqual(dataA.Space, dataC.Space, "a different seed must roll different content-backed dungeon geometry")
}

// --- Task E2b: RPG_DUNGEON_KEY override selects a dungeon end to end ---

// TestStartEncounter_DungeonKeyOverride_SelectsReferenceTomb is the M1
// manual-walkthrough mechanism proven at the orchestrator level: with
// Config.DungeonKeyOverride set to "reference-tomb" and the caller
// supplying NO DungeonKey at all (today's exact shape — no real proto
// surface sets one), StartEncounter must build the content-backed
// reference-tomb dungeon, not the legacy crypt default.
//
// Both specs now compile to 3 regions (reference-tomb's Kirk-authored
// entrance/hall/tomb draft; the legacy crypt's entrance/corridor/boss), so
// region COUNT alone can no longer distinguish them (it could when
// reference-tomb was 2 rooms). The crypt template never emits an
// ArchetypeChamber region (verified against tkenc's own crypt_dungeon.go:
// entrance/corridor/boss only) — reference-tomb's "hall" is the one
// structural signal that's actually distinguishing.
func (s *LobbySuite) TestStartEncounter_DungeonKeyOverride_SelectsReferenceTomb() {
	orch := s.newOrchestratorWithDungeonKeyOverride("reference-tomb")

	s.seedReadyLobby("lobby-override1", "alice")
	s.expectCharacter("char-alice", "alice", "Alice", 12, 12)

	out, err := orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-override1",
		// DungeonKey deliberately left unset -- the override must fill it in.
	})
	s.Require().NoError(err)

	encData, err := s.encRepo.Get(s.ctx, out.EncounterID)
	s.Require().NoError(err)
	hall, hallOK := regionByArchetype(encData.Space, tkenc.ArchetypeChamber)
	s.Require().True(hallOK, "reference-tomb (not the crypt default, which has no chamber-archetype region) must have been selected")
	s.Assert().Equal("hall", hall.ID)
	_, tombOK := regionByArchetype(encData.Space, tkenc.ArchetypeBoss)
	s.Require().True(tombOK)
}

// TestStartEncounter_DungeonKeyOverride_SelectsFogLab is the fog-of-war
// playtest fixture (rpg-project#733 follow-up) proven end to end the same
// way TestStartEncounter_DungeonKeyOverride_SelectsReferenceTomb proves
// reference-tomb: with Config.DungeonKeyOverride set to "fog-lab" and no
// caller-supplied DungeonKey, StartEncounter must build the content-backed
// fog-lab dungeon, not the legacy crypt default (which is what motivated
// this fixture in the first place — the procedural crypt is uncontrollable
// for isolating geometry-fog from entity-fog).
//
// fog-lab is uniquely a two-room spec (every other registered key — crypt,
// reference-tomb — compiles to three regions), so region count alone
// already distinguishes it; this also pins the two room ids/archetypes and
// that exactly one monster (the skeleton filling the mandatory boss slot)
// got seeded, proving SeedMonsters actually ran against the content-compiled
// spawn plan, not just that InitDungeon built the right geometry.
func (s *LobbySuite) TestStartEncounter_DungeonKeyOverride_SelectsFogLab() {
	orch := s.newOrchestratorWithDungeonKeyOverride("fog-lab")

	s.seedReadyLobby("lobby-foglab", "alice")
	s.expectCharacter("char-alice", "alice", "Alice", 12, 12)

	out, err := orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-foglab",
		// DungeonKey deliberately left unset -- the override must fill it in.
	})
	s.Require().NoError(err)

	encData, err := s.encRepo.Get(s.ctx, out.EncounterID)
	s.Require().NoError(err)

	s.Require().Len(encData.Space.Regions, 2, "fog-lab is a two-room spec; the crypt/reference-tomb defaults are both three")
	pillars, pillarsOK := regionByArchetype(encData.Space, tkenc.ArchetypeEntrance)
	s.Require().True(pillarsOK)
	s.Assert().Equal("pillars", pillars.ID)
	sentry, sentryOK := regionByArchetype(encData.Space, tkenc.ArchetypeBoss)
	s.Require().True(sentryOK)
	s.Assert().Equal("sentry", sentry.ID)

	s.Require().Len(encData.Monsters, 1, "exactly one monster: the skeleton filling fog-lab's mandatory boss slot")
	for _, m := range encData.Monsters {
		s.Assert().Equal("dnd5e:monsters:skeleton", m.MonsterRef)
	}
}

// TestStartEncounter_DungeonKeyOverride_ExplicitCallerKeyWins proves the
// substitution's ONLY-when-empty guard: a caller-supplied DungeonKey
// (crypt) must win over a configured override (reference-tomb) — the
// override is a fallback for "no key supplied," never a hijack of an
// explicit one.
func (s *LobbySuite) TestStartEncounter_DungeonKeyOverride_ExplicitCallerKeyWins() {
	orch := s.newOrchestratorWithDungeonKeyOverride("reference-tomb")

	s.seedReadyLobby("lobby-override2", "alice")
	s.expectCharacter("char-alice", "alice", "Alice", 12, 12)

	out, err := orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-override2",
		DungeonKey: lobbyorch.DungeonKeyCrypt,
	})
	s.Require().NoError(err)

	encData, err := s.encRepo.Get(s.ctx, out.EncounterID)
	s.Require().NoError(err)
	s.Require().Len(encData.Space.Regions, 3, "the explicit crypt key must win over the configured override")
}
