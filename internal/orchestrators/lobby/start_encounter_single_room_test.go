package lobby_test

import (
	"bytes"
	"encoding/json"
	"time"

	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	sessionv1alpha1 "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1"
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
)

// TestStartEncounter_PlaysCompleteSingleRoom is the public launch regression
// for one authored World Builder room: the full v3 workshop (a grouped raised
// prop with explicit declarations, supported lit decor, fractional/negative
// source coordinates, two existing monster refs under their authored stable
// ids — one of them on a NEGATIVE ODD axial row — and the party start) is Put
// into a real scratch registry, launched through the real StartEncounter, and
// read back through the real session Manager and the real GetAtlas handler.
// Every assertion below compares DECODED SOURCE VALUES against the
// independently spelled expected graph (dungeonstest.WorkshopRoomScene and
// the authored axial cells), never two outputs of one converter.
func (s *SessionStackSuite) TestStartEncounter_PlaysCompleteSingleRoom() {
	registry, _ := dungeonstest.Scratch(s.T())
	raw := dungeonstest.WorkshopRoomYAML(s.T())
	put, err := registry.Put(s.ctx, &dungeons.PutInput{Key: dungeonstest.WorkshopRoomKey, YAML: raw})
	s.Require().NoError(err)
	s.Require().Empty(put.Errors)

	orch := s.lobbyOver(registry)
	s.seedCharacter("char-alice", "alice", "Alice")
	s.seedReadyLobby("lobby-1", "alice")
	out, err := orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-1", DungeonKey: lobbyorch.DungeonKey(dungeonstest.WorkshopRoomKey),
	})
	s.Require().NoError(err)

	// THE MEMBER ATLAS, through the real SDK manager — the same read
	// GetAtlas serves.
	atlas, err := s.sessOrch.Manager.Atlas(s.ctx, &sdk.AtlasInput{Session: out.EncounterID, Member: "char-alice"})
	s.Require().NoError(err)
	s.NotEmpty(atlas.RoomSceneJSON, "a v3 room's canonical scene reaches the wire")
	s.Equal(put.Entry.Atlas.RoomSceneJSON, atlas.RoomSceneJSON,
		"PutDungeon's atlas and the started session's GetAtlas carry the same scene -- one producer")
	s.Equal(dungeonstest.WorkshopRoomScene(), *s.decodeRoomScene(atlas.RoomSceneJSON),
		"the full authored source graph arrived, value by value")
	// The distinctive leaves, spelled directly so a decode regression names
	// the fact it lost: the supported lit decor and the fractional negative
	// transforms are presentation doubles nobody may narrow.
	scene := s.decodeRoomScene(atlas.RoomSceneJSON)
	s.Require().Len(scene.Scene.Items, 2)
	candles := scene.Scene.Items[1]
	s.Equal("table", candles.SupportID, "the decor stands on the raised prop")
	s.Require().NotNil(candles.PointLight)
	s.Equal("#ff9d52", candles.PointLight.Color)
	s.InDelta(1.1, candles.PointLight.Intensity, 1e-9)
	s.InDelta(-2.25, scene.Scene.Items[0].Transform.X, 1e-9)
	s.Require().NotNil(scene.Scene.Items[0].HeightScale)
	s.InDelta(1.5, *scene.Scene.Items[0].HeightScale, 1e-9)

	// The floor: one implicit region owning exactly the authored walkable
	// hexes, in dungeon-absolute axial space.
	s.Require().Len(atlas.Regions, 1)
	s.Equal("room-1-region", atlas.Regions[0].ID)
	s.Equal("Workshop", atlas.Regions[0].Name)
	s.Equal("crypt", atlas.Regions[0].Archetype)
	s.InDelta(1.0, atlas.Regions[0].Lighting.Intensity, 1e-9, "bright is intensity 1")
	// Authored axial cells, asserted as authored: (2,0) and (1,-1) stand on
	// the page exactly as the author wrote them.
	s.ElementsMatch([]spatial.Position{
		{X: 0, Y: 0}, {X: 1, Y: 0}, {X: 2, Y: 0}, {X: 0, Y: 1},
		{X: -1, Y: 1}, {X: -1, Y: 0}, {X: 0, Y: -1}, {X: 1, Y: -1},
	}, atlas.Cells, "the floor is the authored walkable hexes, axial, negatives included")
	s.Require().NotNil(atlas.Start)
	s.Equal(spatial.Position{X: 0, Y: 0}, atlas.Start.At, "the way in is the authored party start")

	// THE ACTORS: real Spawn/Join members at their authored axial cells,
	// asked per actor through the same verb a client uses for a token's cell.
	roster, err := s.sessOrch.Manager.Roster(s.ctx, &sdk.RosterInput{
		Session: out.EncounterID, Player: "alice",
	})
	s.Require().NoError(err)
	kinds := make(map[string]sdk.MemberKind, len(roster.Members))
	for _, member := range roster.Members {
		kinds[member.ID] = member.Kind
	}
	s.Require().Len(roster.Members, 3, "the seated player and the two authored skeletons")
	s.Equal(sdk.KindPlayer, kinds["char-alice"])
	s.Equal(sdk.KindMonster, kinds["skeleton-a"])
	s.Equal(sdk.KindMonster, kinds["skeleton-b"])
	where := func(member string) spatial.Position {
		at, werr := s.sessOrch.Manager.Where(s.ctx, &sdk.WhereInput{Session: out.EncounterID, Member: member})
		s.Require().NoErrorf(werr, "%s should be standing in the room", member)
		return at.Position
	}
	s.Equal(spatial.Position{X: 0, Y: 0}, where("char-alice"), "the party start cell, axial")
	s.Equal(spatial.Position{X: 2, Y: 0}, where("skeleton-a"), "the authored cell, axial")
	// THE NEGATIVE ODD ROW, pinned through the API: the authored cell
	// {q: 1, r: -1} passes through the one offset conversion sessionworld
	// owns and the member STANDS on the axial cell the author wrote.
	s.Equal(sdk.KindMonster, kinds["skeleton-b"])
	s.Equal(spatial.Position{X: 1, Y: -1}, where("skeleton-b"),
		"authored axial (q=1, r=-1) is the member's cell — a negative odd row survives the API")

	// THE SDK'S OWN DEFAULTS: the authored placement named only id and ref,
	// so the spawned sheet is the stat block's own, arms included.
	//
	// AND NOT A MIND: a sheet no longer names one (rpg-project#465). What a
	// skeleton does is the rulebook's default TABLE for its kind, laid on
	// inside Spawn and held with the member, so there is nothing about
	// behavior on this record to assert here any more.
	sessions := sessionorch.NewSessionRepository(s.redisClient, time.Hour)
	stored, err := sessions.GetSession(s.ctx, out.EncounterID)
	s.Require().NoError(err)
	sheets := make(map[string]monster.Data, len(stored.NPCs))
	for _, npc := range stored.NPCs {
		sheets[npc.ID] = npc
	}
	s.Require().Contains(sheets, "skeleton-a", "the launch spawned the authored garrison")
	skeleton := sheets["skeleton-a"]
	s.Require().NotNil(skeleton.Ref)
	s.True(skeleton.Ref.Equals(refs.Monsters.Skeleton()),
		"the member is the ref the author named")
	s.Equal(13, skeleton.HitPoints, "the stat block's own HP, not a default this side invented")
	s.Equal(13, skeleton.MaxHitPoints)
	s.Require().Len(skeleton.Actions, 2, "the stat block's own arms")
	s.True(skeleton.Actions[0].Ref.Equals(refs.Weapons.Shortsword()),
		"melee first, as the definition arms it")
	s.True(skeleton.Actions[1].Ref.Equals(refs.Weapons.Shortbow()),
		"and the bow behind it")

	// And the real GetAtlas handler serves the same scene on the wire —
	// AtlasToProto's one carriage, exercised end to end.
	handler, err := sessionv1alpha1.New(&sessionv1alpha1.HandlerConfig{
		Manager: s.sessOrch.Manager, Broker: s.sessOrch.Broker, Characters: s.charRepo,
	})
	s.Require().NoError(err)
	resp, err := handler.GetAtlas(auth.WithPlayerID(s.ctx, "alice"),
		&sessionpb.GetAtlasRequest{Session: out.EncounterID, Member: "char-alice"})
	s.Require().NoError(err)
	s.Equal(atlas.RoomSceneJSON, resp.GetRoomSceneJson(),
		"the handler carries the exact scene string, doubles intact")
}

// TestStartEncounter_SingleRoomSurvivesReloadAndAuthorEdit pins the snapshot
// rule (single-room-play.md §4): each run persists its compiled presentation,
// so a fresh read path over the STORED records serves the same scene, and a
// room published AFTER the launch reaches only future launches — the already
// running session keeps its original snapshot.
func (s *SessionStackSuite) TestStartEncounter_SingleRoomSurvivesReloadAndAuthorEdit() {
	registry, _ := dungeonstest.Scratch(s.T())
	raw := dungeonstest.WorkshopRoomYAML(s.T())
	put, err := registry.Put(s.ctx, &dungeons.PutInput{Key: dungeonstest.WorkshopRoomKey, YAML: raw})
	s.Require().NoError(err)
	s.Require().Empty(put.Errors)

	orch := s.lobbyOver(registry)
	s.seedCharacter("char-alice", "alice", "Alice")
	s.seedReadyLobby("lobby-1", "alice")
	out, err := orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-1", DungeonKey: lobbyorch.DungeonKey(dungeonstest.WorkshopRoomKey),
	})
	s.Require().NoError(err)

	atlas, err := s.sessOrch.Manager.Atlas(s.ctx, &sdk.AtlasInput{Session: out.EncounterID, Member: "char-alice"})
	s.Require().NoError(err)
	original := atlas.RoomSceneJSON
	s.NotEmpty(original)
	s.Equal(dungeonstest.WorkshopRoomScene(), *s.decodeRoomScene(original))

	// A FRESH read path: a new orchestrator over the same stored records —
	// nothing in memory — reconstitutes the session and serves the same
	// scene. This is the reload half, through the SDK's own persisted-record
	// load, not a cached answer.
	fresh, err := sessionorch.New(sessionorch.Config{
		Redis: s.redisClient, Characters: s.charRepo, TTL: 24 * time.Hour,
		PresentationIDs: idgen.NewSequential("presentation"),
	})
	s.Require().NoError(err)
	reloaded, err := fresh.Manager.Atlas(s.ctx, &sdk.AtlasInput{Session: out.EncounterID, Member: "char-alice"})
	s.Require().NoError(err)
	s.Equal(original, reloaded.RoomSceneJSON, "the persisted snapshot survives a fresh read path")

	// The author publishes a CHANGED room under the same key after the
	// launch: a different label compiles to a different scene.
	edited := bytes.Replace(raw, []byte("label: Table"), []byte("label: Table II"), 1)
	s.Require().NotEqual(raw, edited, "the fixture's table label must be where this test expects it")
	republished, err := registry.Put(s.ctx, &dungeons.PutInput{Key: dungeonstest.WorkshopRoomKey, YAML: edited})
	s.Require().NoError(err)
	s.Require().Empty(republished.Errors)
	s.NotEqual(original, republished.Entry.Atlas.RoomSceneJSON,
		"the newly authored room is genuinely different")

	// The already running session keeps its ORIGINAL snapshot — through the
	// running manager and through the fresh read path alike.
	after, err := s.sessOrch.Manager.Atlas(s.ctx, &sdk.AtlasInput{Session: out.EncounterID, Member: "char-alice"})
	s.Require().NoError(err)
	s.Equal(original, after.RoomSceneJSON, "the running session did not see the edit")
	afterReload, err := fresh.Manager.Atlas(s.ctx, &sdk.AtlasInput{Session: out.EncounterID, Member: "char-alice"})
	s.Require().NoError(err)
	s.Equal(original, afterReload.RoomSceneJSON, "and neither does its persisted snapshot")

	// A FUTURE launch gets the edit: a second lobby starting on the same key
	// plays the republished scene.
	s.seedCharacter("char-bob", "bob", "Bob")
	s.seedReadyLobby("lobby-2", "bob")
	next, err := orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "bob", LobbyID: "lobby-2", DungeonKey: lobbyorch.DungeonKey(dungeonstest.WorkshopRoomKey),
	})
	s.Require().NoError(err)
	nextAtlas, err := s.sessOrch.Manager.Atlas(s.ctx, &sdk.AtlasInput{Session: next.EncounterID, Member: "char-bob"})
	s.Require().NoError(err)
	s.Equal(republished.Entry.Atlas.RoomSceneJSON, nextAtlas.RoomSceneJSON,
		"the published edit reaches the next launch")
}

// TestStartEncounter_SingleRoomInsufficientSeatsWritesNothing pins the launch
// gate: a valid one-seat room and two ready members are refused by the
// existing seat-capacity guard BEFORE anything is written — no session, no
// encounter world, no character save, no EncounterStarted event. The unknown
// monster source is refused even earlier, at Put, so it is not in the
// registry at all and launching that key names the refusal without creating
// a partial launch entry either.
func (s *SessionStackSuite) TestStartEncounter_SingleRoomInsufficientSeatsWritesNothing() {
	registry, _ := dungeonstest.Scratch(s.T())
	oneSeat := dungeonstest.WorkshopOneSeatYAML(s.T())
	put, err := registry.Put(s.ctx, &dungeons.PutInput{Key: dungeonstest.WorkshopOneSeatKey, YAML: oneSeat})
	s.Require().NoError(err)
	s.Require().Empty(put.Errors, "the one-seat room is valid authored content")
	s.Require().NotNil(put.Entry.Dungeon)
	s.Len(put.Entry.Dungeon.PartySeats, 1, "exactly one seat is derived")

	orch := s.lobbyOver(registry)
	s.seedCharacter("char-alice", "alice", "Alice")
	s.seedCharacter("char-bob", "bob", "Bob")
	s.seedReadyLobby("lobby-1", "alice", "bob")

	beforeAlice, err := s.redisClient.Get(s.ctx, "character:char-alice").Bytes()
	s.Require().NoError(err)
	beforeBob, err := s.redisClient.Get(s.ctx, "character:char-bob").Bytes()
	s.Require().NoError(err)

	sub, err := s.broker.Subscribe("lobby-1")
	s.Require().NoError(err)
	defer func() { _ = sub.Close() }()

	_, err = orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-1", DungeonKey: lobbyorch.DungeonKey(dungeonstest.WorkshopOneSeatKey),
	})
	s.Require().Error(err)
	s.Contains(err.Error(), "has 2 members and the dungeon seats 1",
		"the existing seat-capacity guard is the refusal")

	// NOTHING WAS WRITTEN. No session or encounter record exists, the lobby
	// is where it was, the characters' stored bytes are untouched, and no
	// EncounterStarted event was published.
	sessionKeys, err := s.redisClient.Keys(s.ctx, "session:v1alpha1:*").Result()
	s.Require().NoError(err)
	s.Empty(sessionKeys, "no session record was written")
	encounterKeys, err := s.redisClient.Keys(s.ctx, "session-enc:v1alpha1:*").Result()
	s.Require().NoError(err)
	s.Empty(encounterKeys, "no encounter world was written")

	data, err := s.lobbyRepo.Get(s.ctx, "lobby-1")
	s.Require().NoError(err)
	s.Equal(lobbyrepo.StatusWaiting, data.Status, "the lobby is where it was")
	s.Empty(data.EncounterID)

	afterAlice, err := s.redisClient.Get(s.ctx, "character:char-alice").Bytes()
	s.Require().NoError(err)
	s.Equal(beforeAlice, afterAlice, "alice's stored sheet is untouched")
	afterBob, err := s.redisClient.Get(s.ctx, "character:char-bob").Bytes()
	s.Require().NoError(err)
	s.Equal(beforeBob, afterBob, "and so is bob's")

	select {
	case evt := <-sub.Events():
		s.Failf("event", "the refusal published %v", evt.Kind)
	case <-time.After(150 * time.Millisecond):
	}

	// The unknown-monster source never became an entry in the first place:
	// its Put is refused, so the launch names a dungeon that does not exist
	// and still writes nothing.
	full := dungeonstest.WorkshopRoomYAML(s.T())
	broken := bytes.Replace(full, []byte("key: workshop-room"), []byte("key: broken-room"), 1)
	broken = bytes.Replace(broken, []byte("dnd5e:monsters:skeleton"), []byte("dnd5e:monsters:not-real"), 1)
	s.Require().NotEqual(full, broken)
	res, err := registry.Put(s.ctx, &dungeons.PutInput{Key: "broken-room", YAML: broken})
	s.Require().NoError(err)
	s.Require().NotEmpty(res.Errors, "the unknown monster ref refuses the save")

	_, err = orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-1", DungeonKey: lobbyorch.DungeonKey("broken-room"),
	})
	s.Require().ErrorIs(err, lobbyorch.ErrDungeonNotFound,
		"an unaccepted key is a dungeon the registry does not have")

	stillThere, err := s.lobbyRepo.Get(s.ctx, "lobby-1")
	s.Require().NoError(err)
	s.Equal(lobbyrepo.StatusWaiting, stillThere.Status, "and the lobby is STILL where it was")
	s.Empty(stillThere.EncounterID)
	sessionKeys, err = s.redisClient.Keys(s.ctx, "session:v1alpha1:*").Result()
	s.Require().NoError(err)
	s.Empty(sessionKeys, "and still no session record")
}

// decodeRoomScene decodes one RoomSceneJSON into the toolkit's presentation
// type, so scene assertions compare decoded source values rather than two
// strings of the same converter.
func (s *SessionStackSuite) decodeRoomScene(raw string) *tkencounter.RoomScenePresentation {
	s.T().Helper()
	var scene tkencounter.RoomScenePresentation
	s.Require().NoError(json.Unmarshal([]byte(raw), &scene))
	return &scene
}
