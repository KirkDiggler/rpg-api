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
// Wall carrying an id; (3) a Move that sights an entrance-region goblin
// starts a COMBAT POCKET scoped to the entrance region — boss-region
// monsters are absent from initiative; (4) boss-region goblins exist but
// emit no EntityAppeared until BOTH connector doors open AND a sightline
// actually forms (opening alone doesn't reveal them).
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

type DungeonCryptSuite struct {
	suite.Suite
	ctx    context.Context
	cancel context.CancelFunc
	srv    *harness.TestServer
}

func TestDungeonCryptSuite(t *testing.T) {
	suite.Run(t, new(DungeonCryptSuite))
}

func (s *DungeonCryptSuite) SetupTest() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 2*time.Minute)
	var err error
	s.srv, err = harness.New(s.ctx, nil)
	s.Require().NoError(err, "failed to create test server")
}

func (s *DungeonCryptSuite) TearDownTest() {
	if s.srv != nil {
		s.srv.Close()
	}
	if s.cancel != nil {
		s.cancel()
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

// archetypeGoblins splits encData.Monsters by their SpaceData region
// archetype.
func archetypeGoblins(encData *tkenc.Data) (entrance, boss []core.EntityID) {
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

// moveOnto issues the real MoveEntity RPC with a two-waypoint path (current
// position, target) — a coarse [from, to] jump is immune to tripping over
// an interior RandomPattern wall that a naively-interpolated multi-hex line
// might cross — load-bearing for a target many hexes away in an
// entropy-seeded region, which a full step-by-step path is not safe
// against. The orchestrator passes the path straight through to the
// toolkit's Move verb with no rpg-api-side pathing/budget logic
// (move_entity.go's doc), so this direct jump onto a pre-vetted non-wall
// floor hex (a goblin's own cell — monster.Monster carries no
// spatial.Placeable blocking) is a valid move, UNLESS a closed door still
// sits on the path — closed doors block movement (rpg-toolkit's
// TestClosedDoor_BlocksMovement), so callers reaching through a connector
// must open it first.
func (s *DungeonCryptSuite) moveOnto(encounterID, characterID string, from, to core.Hex) error {
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

// openDoor issues the real Interact RPC against doorID.
func (s *DungeonCryptSuite) openDoor(encounterID, doorID string) {
	_, err := s.srv.EncounterClientV2.Interact(s.authCtx("alice"), &encounterv2pb.InteractRequest{
		EncounterId: encounterID, TargetEntityId: doorID,
	})
	s.Require().NoError(err, "opening door %q must succeed", doorID)
}

// TestStartEncounter_SpawnsAtEntrance_DoorProjectsClosedWithId is gate facts
// (1) and (2): the party spawns at SpaceData.Entrance (the entrance
// region's designated anchor, NOT a room-center placeholder —
// roomCenterHex() is gone, rpg-api#648/#676/#688), and a connector door
// projects on the wire as a WALL_KIND_DOOR_CLOSED Wall carrying a
// non-empty id (rpg-api-protos#186's Wall.id — the click->Interact bridge).
func (s *DungeonCryptSuite) TestStartEncounter_SpawnsAtEntrance_DoorProjectsClosedWithId() {
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

	var doorWall *encounterv2pb.Wall
	for _, w := range resp.GetEncounter().GetSpace().GetWalls() {
		if w.GetId() != "" {
			doorWall = w
			break
		}
	}
	s.Require().NotNil(doorWall, "a connector door must project onto the wire as a Wall carrying an id")
	s.Require().Equal(encounterv2pb.WallKind_WALL_KIND_DOOR_CLOSED, doorWall.GetKind(),
		"the door must start closed on the wire")
}

// TestEntranceSighting_StartsPocketScopedCombat is gate fact (3): a Move
// that brings an entrance-region goblin into sight starts a combat pocket
// scoped to the entrance region — the toolkit's LoS-scoped rollInitiative
// (rpg-toolkit#796, Slice 1b) must NOT drag the boss region's still-
// unsighted goblins (two closed connector doors away) into the same
// initiative roster, or the whole dungeon rolls into one un-endable fight
// (design doc gap 5).
func (s *DungeonCryptSuite) TestEntranceSighting_StartsPocketScopedCombat() {
	encounterID, characterID, encData := s.startCryptDungeon("gate2", "alice")
	entranceIDs, bossIDs := archetypeGoblins(encData)
	s.Require().NotEmpty(entranceIDs, "the entrance region must have goblins seeded")
	s.Require().NotEmpty(bossIDs, "the boss region must have goblins seeded")

	alicePD := encData.Players[core.PlayerID("alice")]
	sightedGoblin := entranceIDs[0]
	target := encData.Monsters[sightedGoblin].Position

	err := s.moveOnto(encounterID, characterID, alicePD.View.Position, target)
	s.Require().NoError(err, "moving onto a pre-vetted entrance-region goblin hex must not be blocked")

	after, err := s.srv.EncRepoV2.Get(s.ctx, encounterID)
	s.Require().NoError(err)
	s.Require().Equal(core.ModeTurnBased, after.Mode,
		"a Move that sights an entrance-region goblin must start a combat pocket (TURN_BASED)")

	// Pocket-scoping (design doc: "initiative scoped to engaged (LoS-having)
	// monsters") means only goblins actually WITHIN SIGHT at combat-entry join
	// initiative — not every goblin sharing the entrance region's tag. The
	// entrance region's OTHER goblin may not be sighted from this exact spot,
	// so only the one alice walked onto is asserted IN; every boss-region
	// goblin (definitely unsighted — both connector doors are still closed)
	// must be asserted OUT, which is the direction that actually proves
	// pocket-scoping (gap 5: the whole dungeon must NOT roll into one
	// un-endable initiative).
	inInitiative := make(map[core.EntityID]bool, len(after.Initiative))
	for _, id := range after.Initiative {
		inInitiative[id] = true
	}
	s.Require().True(inInitiative[sightedGoblin], "the sighted entrance-region goblin %q must be in the pocket's initiative", sightedGoblin)
	for _, id := range bossIDs {
		s.Require().False(inInitiative[id], "boss-region goblin %q must NOT be dragged into the entrance pocket", id)
	}
}

// TestBossGoblins_NoEntityAppeared_UntilBothDoorsOpenAndSightlinesForm is
// gate fact (4): boss-region goblins exist (seeded at StartEncounter) but
// emit no EntityAppeared to alice — neither in her connect-time replay, nor
// after either connector door opens individually, nor after BOTH open —
// until a sightline actually forms. Opening a door alone is not enough
// (Slice 1's progressive through-the-doorway reveal, not a whole-region
// reveal); this test specifically distinguishes "both doors opened" from
// "sightline formed" by checking for EntityAppeared after each step
// separately.
func (s *DungeonCryptSuite) TestBossGoblins_NoEntityAppeared_UntilBothDoorsOpenAndSightlinesForm() {
	encounterID, characterID, encData := s.startCryptDungeon("gate3", "alice")
	_, bossIDs := archetypeGoblins(encData)
	s.Require().NotEmpty(bossIDs, "the boss region must have goblins seeded")
	bossGoblin := bossIDs[0]

	s.Require().Len(encData.Doors, 2, "the crypt spec's 3-region chain has exactly 2 connector doors")
	doorIDs := make([]string, 0, 2)
	for id := range encData.Doors {
		doorIDs = append(doorIDs, string(id))
	}

	stream, err := s.srv.EncounterClientV2.StreamEncounter(s.authCtx("alice"),
		&encounterv2pb.StreamEncounterRequest{EncounterId: encounterID})
	s.Require().NoError(err)

	// ONE background reader for the stream's whole lifetime, drained at each
	// checkpoint below via drainAvailable — see streamEvents/drainAvailable's
	// own docs (shared with lobby_start_then_move_test.go's sibling suites in
	// this package) for why a fresh reader per checkpoint would race.
	events := s.streamEvents(stream)

	sawBossGoblin := func(batch []*encounterv2pb.EncounterEvent) bool {
		for _, ev := range batch {
			if a := ev.GetEntityAppeared(); a != nil && a.GetEntity().GetId() == string(bossGoblin) {
				return true
			}
		}
		return false
	}

	// Drain the connect-time replay: SnapshotDelivered + EntityAppeared(alice)
	// + GeometryRevealed. The boss goblin must not be among them — both
	// connector doors are closed, and it is out of alice's entrance-region
	// sight regardless.
	replay := drainAvailable(events, 500*time.Millisecond)
	s.Require().False(sawBossGoblin(replay), "boss-region goblin must not appear in the connect-time replay")

	// Open the first connector door. This alone must NOT reveal the boss
	// goblin — it is still one closed door and a whole corridor away.
	s.openDoor(encounterID, doorIDs[0])
	afterFirstDoor := drainAvailable(events, 500*time.Millisecond)
	s.Require().False(sawBossGoblin(afterFirstDoor),
		"opening one connector door alone must not reveal the boss-region goblin")

	// Open the second connector door too. Both doors open STILL must not
	// reveal the boss goblin by itself — only the doorway's own progressive
	// reveal (Slice 1), not a whole-region one; a sightline must still form.
	s.openDoor(encounterID, doorIDs[1])
	afterSecondDoor := drainAvailable(events, 500*time.Millisecond)
	s.Require().False(sawBossGoblin(afterSecondDoor),
		"opening both connector doors alone must not reveal the boss-region goblin — a sightline must still form")

	// Now form a sightline: move alice directly onto the boss goblin's hex
	// (neither door blocks movement/LoS once open — rpg-toolkit#790).
	after, err := s.srv.EncRepoV2.Get(s.ctx, encounterID)
	s.Require().NoError(err)
	alicePD := after.Players[core.PlayerID("alice")]
	target := after.Monsters[bossGoblin].Position

	err = s.moveOnto(encounterID, characterID, alicePD.View.Position, target)
	s.Require().NoError(err, "moving through both open connector doors onto the boss goblin's hex must not be blocked")

	postMove := drainAvailable(events, 1500*time.Millisecond)
	s.Require().True(sawBossGoblin(postMove),
		"a sightline that actually forms (alice standing on the goblin's hex) must emit EntityAppeared")
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
// archetypeGoblins' pattern, this file's established convention) rather
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
	entranceIDs, bossIDs := archetypeGoblins(encData)
	s.Require().NotEmpty(entranceIDs, "the entrance region must have goblins seeded")
	s.Require().NotEmpty(bossIDs, "the boss region must have goblins seeded")

	resp, err := s.srv.EncounterClientV2.GetEncounter(s.authCtx("alice"), &encounterv2pb.GetEncounterRequest{
		EncounterId: encounterID,
	})
	s.Require().NoError(err)
	space := resp.GetEncounter().GetSpace()

	// Space.zones names all 3 crypt regions with their toolkit-assigned
	// archetype.
	zones := space.GetZones()
	s.Require().Len(zones, 3, "all 3 crypt regions must be named as zones")
	byArchetype := make(map[string]string, 3) // archetype -> zone id
	for _, z := range zones {
		byArchetype[z.GetArchetype()] = z.GetId()
	}
	s.Require().Contains(byArchetype, "entrance")
	s.Require().Contains(byArchetype, "corridor")
	s.Require().Contains(byArchetype, "boss")

	// Verbatim theme passthrough: the crypt spec sets Theme="crypt".
	s.Require().Equal("crypt", space.GetTheme(),
		"InitDungeon's crypt spec sets Theme=\"crypt\"; the wire must carry it verbatim")

	// Alice spawns at the entrance — her own revealed hex set must include
	// that hex tagged with the entrance region's zone id.
	entranceZoneID := byArchetype["entrance"]
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
	s.Require().Equal(entranceZoneID, spawnHex.GetZoneId(),
		"the entrance hex must carry the entrance region's zone id")

	// Now form a sightline into the boss region via the live stream and
	// prove the INCREMENTAL GeometryRevealed event (not just the connect-
	// time snapshot above) also carries the right zone_id — #687's Done bar
	// covers hexes revealed mid-session, not only ones present at connect.
	stream, err := s.srv.EncounterClientV2.StreamEncounter(s.authCtx("alice"),
		&encounterv2pb.StreamEncounterRequest{EncounterId: encounterID})
	s.Require().NoError(err)
	events := s.streamEvents(stream)
	_ = drainAvailable(events, 500*time.Millisecond) // drain connect-time replay

	s.Require().Len(encData.Doors, 2, "the crypt spec's 3-region chain has exactly 2 connector doors")
	doorIDs := make([]string, 0, 2)
	for id := range encData.Doors {
		doorIDs = append(doorIDs, string(id))
	}
	s.openDoor(encounterID, doorIDs[0])
	_ = drainAvailable(events, 500*time.Millisecond)
	s.openDoor(encounterID, doorIDs[1])
	_ = drainAvailable(events, 500*time.Millisecond)

	bossGoblin := bossIDs[0]
	afterOpen, err := s.srv.EncRepoV2.Get(s.ctx, encounterID)
	s.Require().NoError(err)
	alicePD := afterOpen.Players[core.PlayerID("alice")]
	target := afterOpen.Monsters[bossGoblin].Position

	err = s.moveOnto(encounterID, characterID, alicePD.View.Position, target)
	s.Require().NoError(err, "moving onto the boss-region goblin's hex must not be blocked once both doors are open")

	postMove := drainAvailable(events, 1500*time.Millisecond)
	bossZoneID := byArchetype["boss"]
	var sawAny bool
	var sawZoneID string
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
				sawZoneID = h.GetZoneId()
			}
		}
	}
	s.Require().True(sawAny, "the boss-region goblin's hex must appear in a live GeometryRevealed event")
	s.Require().Equal(bossZoneID, sawZoneID,
		"a hex revealed mid-session via the live stream must carry its zone_id too, not just connect-time hexes")
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
