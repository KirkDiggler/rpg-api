// Package integration_test's dungeon_two_chamber_test.go is the rpg-api#676
// gate: The Dungeon wave 2 Slice 2 (api leg) end-to-end through the REAL
// LobbyService.StartEncounter + v1alpha2 EncounterService RPC surface, not
// a hand-seeded tkenc.New/AddPlayer fixture. It proves the issue's four gate
// facts: (1) the party spawns at the chamber-1 entrance, not a room center;
// (2) the connecting door projects on the wire as a DOOR_CLOSED Wall
// carrying an id; (3) a Move that sights a chamber-1 goblin starts a
// COMBAT POCKET scoped to chamber 1 — chamber-2 monsters are absent from
// initiative; (4) chamber-2 goblins exist but emit no EntityAppeared until
// the door opens AND a sightline actually forms (opening alone doesn't
// reveal them).
package integration_test

import (
	"context"
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
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
)

// dungeonGateHP is deliberately generous (not the usual 12 HP other
// fixtures use): these tests deliberately walk the lone party member onto
// a goblin's own hex to force a sighted Move, which can flip the encounter
// to TURN_BASED and let the toolkit's driveNPCChain resolve a real (random)
// goblin attack synchronously inside the same MoveEntity call
// (move_entity.go's doc). A low-HP single-player fixture risks a flaky TPK
// (ModeEnded instead of the TURN_BASED pocket this suite asserts) purely
// from unlucky dice; 100 HP absorbs worst-case goblin damage (1d6+2, at
// most a couple of swings) with room to spare.
const dungeonGateHP = 100

type DungeonTwoChamberSuite struct {
	suite.Suite
	ctx    context.Context
	cancel context.CancelFunc
	srv    *harness.TestServer
}

func TestDungeonTwoChamberSuite(t *testing.T) {
	suite.Run(t, new(DungeonTwoChamberSuite))
}

func (s *DungeonTwoChamberSuite) SetupTest() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 2*time.Minute)
	var err error
	s.srv, err = harness.New(s.ctx, nil)
	s.Require().NoError(err, "failed to create test server")
}

func (s *DungeonTwoChamberSuite) TearDownTest() {
	if s.srv != nil {
		s.srv.Close()
	}
	if s.cancel != nil {
		s.cancel()
	}
}

func (s *DungeonTwoChamberSuite) authCtx(playerID string) context.Context {
	return metadata.AppendToOutgoingContext(s.ctx, "authorization", "Dev "+playerID)
}

// startTwoChamberDungeon creates a character + lobby, readies it, and starts
// the encounter via the REAL LobbyService.StartEncounter RPC — the same
// path a real client drives, not a hand-seeded tkenc fixture. Returns the
// encounter id, the seated character id, and the freshly persisted
// encounter data for direct inspection.
func (s *DungeonTwoChamberSuite) startTwoChamberDungeon(suffix, playerID string) (encounterID, characterID string, encData *tkenc.Data) {
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

// chamberGoblins splits encData.Monsters by their SpaceData region tag.
func chamberGoblins(encData *tkenc.Data) (chamber1, chamber2 []core.EntityID) {
	for id, m := range encData.Monsters {
		region, ok := encData.Space.RegionAt(m.Position)
		if !ok {
			continue
		}
		switch region {
		case tkenc.RegionChamber1:
			chamber1 = append(chamber1, id)
		case tkenc.RegionChamber2:
			chamber2 = append(chamber2, id)
		}
	}
	return chamber1, chamber2
}

// moveOnto issues the real MoveEntity RPC with a two-waypoint path (current
// position, target) — mirroring lobby_start_then_move_test.go's shape. A
// two-waypoint path is the RIGHT choice here, not just the simple one: the
// toolkit's truncateAtWall checks only the waypoints actually IN the given
// path for blocking (space.go), so a coarse [from, to] jump is immune to
// tripping over an interior RandomPattern wall that a naively-interpolated
// multi-hex line might cross — load-bearing for a target many hexes away in
// an entropy-seeded chamber, which a full step-by-step path is not safe
// against. The orchestrator passes the path straight through to the
// toolkit's Move verb with no rpg-api-side pathing/budget logic
// (move_entity.go's doc), so this direct jump onto a pre-vetted non-wall
// floor hex (a goblin's own cell — monster.Monster carries no
// spatial.Placeable blocking, per seedGoblins/start_encounter_test.go's
// established pattern) is a valid, unblocked move.
func (s *DungeonTwoChamberSuite) moveOnto(encounterID, characterID string, from, to core.Hex) error {
	_, err := s.srv.EncounterClientV2.MoveEntity(s.authCtx("alice"), &encounterv2pb.MoveEntityRequest{
		EncounterId: encounterID,
		EntityId:    characterID,
		ProposedPath: []*encounterv2pb.Position{
			{X: int32(from.Q), Y: int32(from.R), Z: int32(from.S)},
			{X: int32(to.Q), Y: int32(to.R), Z: int32(to.S)},
		},
	})
	return err
}

// TestSpaceZonesAndHexZoneId_ProjectRegionsOnTheWire is rpg-api#687's Done
// bar verified end to end through the REAL RPC surface (not a hand-seeded
// project_test.go fixture): "a two-chamber encounter arrives at the client
// with every revealed hex carrying the right zone_id, and Space.zones
// naming both chambers." Also covers the missing-metadata fallback in the
// same real path: InitTwoChamberRoom (still the live generator on main,
// pre-rpg-api#688) never sets DungeonParams.Theme, so Space.theme must be
// "" — an absent-metadata fallback, not an invented default — right next
// to the populated zones/archetypes in the SAME response.
func (s *DungeonTwoChamberSuite) TestSpaceZonesAndHexZoneId_ProjectRegionsOnTheWire() {
	encounterID, characterID, encData := s.startTwoChamberDungeon("zones1", "alice")
	chamber1IDs, chamber2IDs := chamberGoblins(encData)
	s.Require().NotEmpty(chamber1IDs, "chamber 1 must have goblins seeded")
	s.Require().NotEmpty(chamber2IDs, "chamber 2 must have goblins seeded")

	resp, err := s.srv.EncounterClientV2.GetEncounter(s.authCtx("alice"), &encounterv2pb.GetEncounterRequest{
		EncounterId: encounterID,
	})
	s.Require().NoError(err)
	space := resp.GetEncounter().GetSpace()

	// Space.zones names both chambers with their toolkit-assigned archetype
	// (InitTwoChamberRoom tags chamber-1 entrance, chamber-2 chamber — see
	// rpg-toolkit's two_chamber.go).
	zones := space.GetZones()
	s.Require().Len(zones, 2, "both chambers must be named as zones")
	byID := make(map[string]string, 2) // zone id -> archetype
	for _, z := range zones {
		byID[z.GetId()] = z.GetArchetype()
	}
	s.Require().Equal("entrance", byID[tkenc.RegionChamber1])
	s.Require().Equal("chamber", byID[tkenc.RegionChamber2])

	// Missing-metadata fallback, same response: InitTwoChamberRoom never
	// sets Theme, so it must come across as "" — not invented.
	s.Require().Empty(space.GetTheme(), "InitTwoChamberRoom sets no Theme; the wire must not invent one")

	// Alice spawns at the entrance (chamber-1) — her own revealed hex set
	// must include that hex tagged with chamber-1's zone id.
	var spawnHex *encounterv2pb.Hex
	for _, h := range space.GetHexes() {
		if h.GetPosition().GetX() == int32(encData.Space.Entrance.Q) &&
			h.GetPosition().GetY() == int32(encData.Space.Entrance.R) &&
			h.GetPosition().GetZ() == int32(encData.Space.Entrance.S) {
			spawnHex = h
			break
		}
	}
	s.Require().NotNil(spawnHex, "alice's entrance spawn hex must be in her revealed set")
	s.Require().Equal(string(tkenc.RegionChamber1), spawnHex.GetZoneId(),
		"the entrance hex must carry chamber-1's zone id")

	// Now form a sightline into chamber 2 via the live stream and prove the
	// INCREMENTAL GeometryRevealed event (not just the connect-time
	// snapshot above) also carries the right zone_id — #687's Done bar
	// covers hexes revealed mid-session, not only ones present at connect.
	stream, err := s.srv.EncounterClientV2.StreamEncounter(s.authCtx("alice"),
		&encounterv2pb.StreamEncounterRequest{EncounterId: encounterID})
	s.Require().NoError(err)
	events := s.streamEvents(stream)
	_ = drainAvailable(events, 500*time.Millisecond) // drain connect-time replay

	var doorID core.EntityID
	for id := range encData.Doors {
		doorID = id
	}
	s.Require().NotEmpty(doorID, "exactly one door must connect the two chambers")
	_, err = s.srv.EncounterClientV2.Interact(s.authCtx("alice"), &encounterv2pb.InteractRequest{
		EncounterId: encounterID, TargetEntityId: string(doorID),
	})
	s.Require().NoError(err)
	_ = drainAvailable(events, 500*time.Millisecond) // drain door-opened effects

	chamber2Goblin := chamber2IDs[0]
	afterOpen, err := s.srv.EncRepoV2.Get(s.ctx, encounterID)
	s.Require().NoError(err)
	alicePD := afterOpen.Players[core.PlayerID("alice")]
	target := afterOpen.Monsters[chamber2Goblin].Position

	err = s.moveOnto(encounterID, characterID, alicePD.View.Position, target)
	s.Require().NoError(err, "moving onto the chamber-2 goblin's hex must not be blocked once the door is open")

	postMove := drainAvailable(events, 1500*time.Millisecond)
	var sawChamber2ZoneID string
	var sawAny bool
	for _, ev := range postMove {
		gr := ev.GetGeometryRevealed()
		if gr == nil {
			continue
		}
		for _, h := range gr.GetHexes() {
			if h.GetPosition().GetX() == int32(target.Q) &&
				h.GetPosition().GetY() == int32(target.R) &&
				h.GetPosition().GetZ() == int32(target.S) {
				sawAny = true
				sawChamber2ZoneID = h.GetZoneId()
			}
		}
	}
	s.Require().True(sawAny, "the chamber-2 goblin's hex must appear in a live GeometryRevealed event")
	s.Require().Equal(string(tkenc.RegionChamber2), sawChamber2ZoneID,
		"a hex revealed mid-session via the live stream must carry its zone_id too, not just connect-time hexes")
}

// TestStartEncounter_SpawnsAtEntrance_DoorProjectsClosedWithId is gate facts
// (1) and (2): the party spawns at SpaceData.Entrance (chamber 1's
// designated anchor, NOT a room-center placeholder — roomCenterHex() is
// gone, rpg-api#648/#676), and the connecting door projects on the wire as
// a WALL_KIND_DOOR_CLOSED Wall carrying a non-empty id (rpg-api-protos#186's
// Wall.id — the click->Interact bridge).
func (s *DungeonTwoChamberSuite) TestStartEncounter_SpawnsAtEntrance_DoorProjectsClosedWithId() {
	encounterID, _, encData := s.startTwoChamberDungeon("gate1", "alice")

	alicePD, ok := encData.Players[core.PlayerID("alice")]
	s.Require().True(ok, "alice must be seated in the encounter")
	s.Require().NotNil(alicePD.View)
	s.Require().Equal(encData.Space.Entrance, alicePD.View.Position,
		"StartEncounter must spawn the party at chamber 1's entrance, not a room center")

	resp, err := s.srv.EncounterClientV2.GetEncounter(s.authCtx("alice"), &encounterv2pb.GetEncounterRequest{
		EncounterId: encounterID,
	})
	s.Require().NoError(err)

	var doorWall *encounterv2pb.Wall
	for _, w := range resp.GetEncounter().GetSpace().GetWalls() {
		if w.GetId() != "" {
			doorWall = w
			break
		}
	}
	s.Require().NotNil(doorWall, "the connecting door must project onto the wire as a Wall carrying an id")
	s.Require().Equal(encounterv2pb.WallKind_WALL_KIND_DOOR_CLOSED, doorWall.GetKind(),
		"the door must start closed on the wire")
}

// TestChamber1Sighting_StartsPocketScopedCombat is gate fact (3): a Move
// that brings a chamber-1 goblin into sight starts a combat pocket scoped
// to chamber 1 — the toolkit's LoS-scoped rollInitiative (rpg-toolkit#796,
// Slice 1b) must NOT drag chamber-2's still-unsighted goblins into the
// same initiative roster, or the whole dungeon rolls into one un-endable
// fight (design doc gap 5).
func (s *DungeonTwoChamberSuite) TestChamber1Sighting_StartsPocketScopedCombat() {
	encounterID, characterID, encData := s.startTwoChamberDungeon("gate2", "alice")
	chamber1IDs, chamber2IDs := chamberGoblins(encData)
	s.Require().NotEmpty(chamber1IDs, "chamber 1 must have goblins seeded")
	s.Require().NotEmpty(chamber2IDs, "chamber 2 must have goblins seeded")

	alicePD := encData.Players[core.PlayerID("alice")]
	sightedGoblin := chamber1IDs[0]
	target := encData.Monsters[sightedGoblin].Position

	err := s.moveOnto(encounterID, characterID, alicePD.View.Position, target)
	s.Require().NoError(err, "moving onto a pre-vetted chamber-1 goblin hex must not be blocked")

	after, err := s.srv.EncRepoV2.Get(s.ctx, encounterID)
	s.Require().NoError(err)
	s.Require().Equal(core.ModeTurnBased, after.Mode,
		"a Move that sights a chamber-1 goblin must start a combat pocket (TURN_BASED)")

	// Pocket-scoping (design doc: "initiative scoped to engaged (LoS-having)
	// monsters") means only goblins actually WITHIN SIGHT at combat-entry join
	// initiative — not every goblin sharing chamber 1's region tag. Chamber 1's
	// OTHER goblin may not be sighted from this exact spot, so only the one
	// alice walked onto is asserted IN; every chamber-2 goblin (definitely
	// unsighted — the door is still closed) must be asserted OUT, which is the
	// direction that actually proves pocket-scoping (gap 5: the whole dungeon
	// must NOT roll into one un-endable initiative).
	inInitiative := make(map[core.EntityID]bool, len(after.Initiative))
	for _, id := range after.Initiative {
		inInitiative[id] = true
	}
	s.Require().True(inInitiative[sightedGoblin], "the sighted chamber-1 goblin %q must be in the pocket's initiative", sightedGoblin)
	for _, id := range chamber2IDs {
		s.Require().False(inInitiative[id], "chamber-2 goblin %q must NOT be dragged into chamber 1's pocket", id)
	}
}

// TestChamber2Goblins_NoEntityAppeared_UntilDoorOpensAndSightlinesForm is
// gate fact (4): chamber-2 goblins exist (seeded at StartEncounter) but
// emit no EntityAppeared to alice — neither in her connect-time replay nor
// immediately after the door opens — until a sightline actually forms.
// Opening the door alone is not enough (Slice 1's progressive
// through-the-doorway reveal, not a whole-chamber reveal); this test
// specifically distinguishes "door opened" from "sightline formed" by
// checking for EntityAppeared after each step separately.
func (s *DungeonTwoChamberSuite) TestChamber2Goblins_NoEntityAppeared_UntilDoorOpensAndSightlinesForm() {
	encounterID, characterID, encData := s.startTwoChamberDungeon("gate3", "alice")
	_, chamber2IDs := chamberGoblins(encData)
	s.Require().NotEmpty(chamber2IDs, "chamber 2 must have goblins seeded")
	chamber2Goblin := chamber2IDs[0]

	var doorID core.EntityID
	for id := range encData.Doors {
		doorID = id
	}
	s.Require().NotEmpty(doorID, "exactly one door must connect the two chambers")

	stream, err := s.srv.EncounterClientV2.StreamEncounter(s.authCtx("alice"),
		&encounterv2pb.StreamEncounterRequest{EncounterId: encounterID})
	s.Require().NoError(err)

	// ONE background reader for the stream's whole lifetime, drained at each
	// checkpoint below via drainAvailable. encounter_v2_test.go's own
	// collectStreamEvents explicitly warns against spinning a NEW reader
	// goroutine per checkpoint on the SAME stream: an earlier call's timeout
	// firing does not stop its goroutine (only stream closure does), so a
	// second concurrent Recv() races it — and can silently steal the very
	// event a later checkpoint is waiting for. drainAvailable has exactly one
	// producer for the stream's entire life, so checkpoints are safe to poll
	// repeatedly.
	events := s.streamEvents(stream)

	sawChamber2Goblin := func(batch []*encounterv2pb.EncounterEvent) bool {
		for _, ev := range batch {
			if a := ev.GetEntityAppeared(); a != nil && a.GetEntity().GetId() == string(chamber2Goblin) {
				return true
			}
		}
		return false
	}

	// Drain the connect-time replay: SnapshotDelivered + EntityAppeared(alice)
	// + GeometryRevealed. The chamber-2 goblin must not be among them — the
	// door is closed, and it is out of alice's chamber-1 sight regardless.
	replay := drainAvailable(events, 500*time.Millisecond)
	s.Require().False(sawChamber2Goblin(replay), "chamber-2 goblin must not appear in the connect-time replay")

	// Open the door. This alone must NOT reveal the chamber-2 goblin — only
	// the doorway's own progressive reveal (Slice 1), not a whole-chamber one.
	_, err = s.srv.EncounterClientV2.Interact(s.authCtx("alice"), &encounterv2pb.InteractRequest{
		EncounterId: encounterID, TargetEntityId: string(doorID),
	})
	s.Require().NoError(err)

	postOpen := drainAvailable(events, 500*time.Millisecond)
	s.Require().False(sawChamber2Goblin(postOpen),
		"opening the door alone must not reveal the chamber-2 goblin — a sightline must still form")

	// Now form a sightline: move alice directly onto the chamber-2 goblin's
	// hex (the door no longer blocks movement/LoS once open — rpg-toolkit#790).
	after, err := s.srv.EncRepoV2.Get(s.ctx, encounterID)
	s.Require().NoError(err)
	alicePD := after.Players[core.PlayerID("alice")]
	target := after.Monsters[chamber2Goblin].Position

	err = s.moveOnto(encounterID, characterID, alicePD.View.Position, target)
	s.Require().NoError(err, "moving through the open door onto the chamber-2 goblin's hex must not be blocked")

	postMove := drainAvailable(events, 1500*time.Millisecond)
	s.Require().True(sawChamber2Goblin(postMove),
		"a sightline that actually forms (alice standing on the goblin's hex) must emit EntityAppeared")
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
func (s *DungeonTwoChamberSuite) streamEvents(
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
