package session_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"

	authoringpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/authoring/v1alpha1"
	recordpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	authoringhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/authoring/v1alpha1"
	recordhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha2"
	"github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/sessionaccess"
	authoringorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/authoring"
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
	"github.com/KirkDiggler/rpg-api/internal/repositories/worlddungeons"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
	"github.com/KirkDiggler/rpg-api/internal/worldcontext"
)

// hashSiteKey is the small site this suite saves, launches and reads back.
const hashSiteKey = "creature-facts-walk"

// ContentHashSuite is rpg-project#1001's item: a saved run says whether it
// was a run of what is open in the studio. The hash an author's studio is
// handed (PutDungeon, GetDungeon) and the hash a launched run records on its
// `scene_content` beat are one value, through the real authoring handler,
// the real lobby launch and the real record read. One harness, one registry
// over a one-file shelf, for the whole suite.
type ContentHashSuite struct {
	suite.Suite

	h         *acceptanceHarness
	world     worlddungeons.Repository
	authoring *authoringhandler.Handler
	lobby     *lobbyorch.Orchestrator
	lobbies   lobbyrepo.Repository
	record    *recordhandler.Handler
	author    context.Context
	site      string
	launches  int
}

func TestContentHashSuite(t *testing.T) { suite.Run(t, new(ContentHashSuite)) }

func (s *ContentHashSuite) SetupSuite() {
	s.h = newAcceptanceHarnessWithDice(s.T(), alwaysTheLowestFace{})
	projector := dungeonstest.ProjectorFor(s.h.manager.Manager)

	raw, err := os.ReadFile(dungeonstest.SitePath(s.T(), hashSiteKey)) //nolint:gosec // repository fixture
	s.Require().NoError(err)
	s.site = string(raw)
	dir := s.T().TempDir()
	s.Require().NoError(os.WriteFile(filepath.Join(dir, dungeons.DefaultKey+".yaml"),
		dungeonstest.AsKey(s.T(), raw, hashSiteKey, dungeons.DefaultKey), 0o600))
	shipped, err := dungeons.NewFileRegistry(dir, projector)
	s.Require().NoError(err)
	s.world = dungeonstest.WorldStore(s.T())
	registry, err := dungeons.NewRegistry(&dungeons.RegistryConfig{
		Shipped: shipped, World: s.world, Authoring: true, Projector: projector,
	})
	s.Require().NoError(err)

	aOrch, err := authoringorch.New(&authoringorch.Config{Dungeons: registry})
	s.Require().NoError(err)
	s.authoring, err = authoringhandler.New(&authoringhandler.HandlerConfig{Orchestrator: aOrch})
	s.Require().NoError(err)

	s.lobbies = lobbyrepo.NewInMemory()
	s.lobby, err = lobbyorch.New(&lobbyorch.Config{
		LobbyRepo: s.lobbies, LobbyBroker: lobbyorch.NewBroker(), CharacterRepo: s.h.charRepo,
		LobbyIDGenerator: idgen.NewSequential("lobby"), JoinRefGenerator: idgen.NewSequential("ref"),
		EncounterIDGenerator: idgen.NewSequential("enc"), SessionManager: s.h.manager.Manager, Dungeons: registry,
	})
	s.Require().NoError(err)

	access, err := sessionaccess.New(s.h.charRepo, s.h.manager.Manager)
	s.Require().NoError(err)
	s.record, err = recordhandler.New(&recordhandler.HandlerConfig{
		Manager: s.h.manager.Manager, Broker: s.h.manager.RecordBroker, Gate: access, Enabled: true,
	})
	s.Require().NoError(err)

	s.author = worldcontext.With(auth.WithPlayerID(context.Background(), dungeonstest.Caller.PlayerID),
		worldcontext.Value{WorldID: dungeonstest.Caller.WorldID})
}

// mine is the small site as the author's own dungeon under key.
func (s *ContentHashSuite) mine(key string) string {
	return string(dungeonstest.AsKey(s.T(), []byte(s.site), hashSiteKey, key))
}

func (s *ContentHashSuite) put(key, yaml string, validateOnly bool) *authoringpb.PutDungeonResponse {
	s.T().Helper()
	resp, err := s.authoring.PutDungeon(s.author, &authoringpb.PutDungeonRequest{Key: key, Yaml: yaml, ValidateOnly: validateOnly})
	s.Require().NoError(err)
	return resp
}

func (s *ContentHashSuite) get(key string) *authoringpb.GetDungeonResponse {
	s.T().Helper()
	resp, err := s.authoring.GetDungeon(s.author, &authoringpb.GetDungeonRequest{Key: key})
	s.Require().NoError(err)
	return resp
}

// launch starts key through the lobby's own StartEncounter with one fresh
// fighter, and returns the run and its member.
func (s *ContentHashSuite) launch(key string) (string, string) {
	s.T().Helper()
	s.launches++
	member := "hasher-" + string(rune('a'+s.launches))
	player := "player-" + member
	_, err := s.h.charRepo.Create(context.Background(), characterrepo.CreateInput{
		Character: &entities.Character{Data: armedFighter(member, player)},
	})
	s.Require().NoError(err)
	lobbyID := "lobby-" + member
	s.Require().NoError(s.lobbies.Save(context.Background(), &lobbyrepo.Data{
		ID: lobbyID, HostPlayerID: player, Status: lobbyrepo.StatusWaiting,
		Members:     map[string]*lobbyrepo.Member{player: {PlayerID: player, CharacterID: member, IsHost: true, IsReady: true}},
		MemberOrder: []string{player},
	}))
	out, err := s.lobby.StartEncounter(context.Background(), &lobbyorch.StartEncounterInput{
		WorldID: dungeonstest.Caller.WorldID, PlayerID: player, LobbyID: lobbyID,
		DungeonKey: lobbyorch.DungeonKey(key),
	})
	s.Require().NoError(err)
	return out.EncounterID, member
}

// recordOf is the run's whole record as the watch reads it, and the
// member's own context.
func (s *ContentHashSuite) recordOf(run, member string) []*recordpb.RecordEvent {
	s.T().Helper()
	ctx := auth.WithPlayerID(context.Background(), "player-"+member)
	page, err := s.record.GetRecord(ctx, &recordpb.GetRecordRequest{Session: run, Member: member, Limit: 500})
	s.Require().NoError(err)
	s.False(page.GetMore())
	return page.GetEvents()
}

type sceneContent struct {
	Key         string `json:"key"`
	SiteHash    string `json:"site_hash"`
	ContentHash string `json:"content_hash"`
	Engine      string `json:"engine"`
}

func (s *ContentHashSuite) sceneContentOf(events []*recordpb.RecordEvent) (sceneContent, []string) {
	s.T().Helper()
	var scenes []sceneContent
	var joined []string
	for _, e := range events {
		switch e.GetBeat() {
		case encounter.BeatSceneContent:
			var sc sceneContent
			s.Require().NoError(json.Unmarshal(e.GetFieldsJson(), &sc))
			scenes = append(scenes, sc)
		case encounter.BeatJoinedAt:
			var j struct {
				Member string `json:"member"`
			}
			s.Require().NoError(json.Unmarshal(e.GetFieldsJson(), &j))
			joined = append(joined, j.Member)
		}
	}
	s.Require().Len(scenes, 1, "one scene_content per run")
	return scenes[0], joined
}

// TestTheRunRecordsTheHashTheStudioWasHanded is the item, end to end: the
// hash a save, a validate-only check and a reopen hand the studio is the
// hash the launched run records, under the dungeon's key and this engine's
// release; every member's join is recorded; no player's stream carries
// either beat. A one-word edit saved under another key hashes differently,
// and that run records the new hash.
func (s *ContentHashSuite) TestTheRunRecordsTheHashTheStudioWasHanded() {
	const key = "my-watch-post"
	saved := s.put(key, s.mine(key), false)
	s.Require().Empty(saved.GetErrors())
	hash := saved.GetContentHash()
	s.Regexp(`^[0-9a-f]{64}$`, hash, "a lower-case hex SHA-256")
	s.Equal(hash, s.put(key, s.mine(key), true).GetContentHash(), "a validate-only check hashes what a save hashes")
	s.Equal(hash, s.get(key).GetContentHash(), "a reopen hashes the stored file alike")
	compiled, err := sessionworld.Compile([]byte(s.mine(key)))
	s.Require().NoError(err)
	direct, err := compiled.Spec.ContentHash()
	s.Require().NoError(err)
	s.Equal(string(direct), hash, "the engine's own ContentHash() of the compiled file")

	words, err := s.authoring.GetMachineWords(s.author, &authoringpb.GetMachineWordsRequest{})
	s.Require().NoError(err)

	run, member := s.launch(key)
	scene, joined := s.sceneContentOf(s.recordOf(run, member))
	s.Equal(hash, scene.ContentHash, "the run records the hash the studio was handed")
	s.Equal(key, scene.Key, "under the dungeon's key")
	s.Equal(words.GetEncounterVersion(), scene.Engine, "and this server's engine release")
	for _, m := range []string{member, "guard-1", "guard-2", "wolf-1"} {
		s.Contains(joined, m, "every member's join is recorded")
	}

	ctx := auth.WithPlayerID(context.Background(), "player-"+member)
	for _, act := range storyOf(ctx, s.T(), s.h.story, run, member) {
		text := protojson.Format(act)
		s.NotContains(text, encounter.BeatSceneContent, "no player's stream carries scene_content")
		s.NotContains(text, encounter.BeatJoinedAt, "nor joined_at")
	}

	// One word of a line changes: another hash, recorded by its own run.
	const edited = "my-watch-post-edited"
	source := strings.Replace(s.mine(edited), "until: {round: 3}", "until: {round: 4}", 1)
	s.Require().NotEqual(s.mine(edited), source, "the site no longer holds `until: {round: 3}`")
	resaved := s.put(edited, source, false)
	s.Require().Empty(resaved.GetErrors())
	other := resaved.GetContentHash()
	s.NotEqual(hash, other)
	s.Equal(other, s.get(edited).GetContentHash())
	run2, member2 := s.launch(edited)
	scene2, _ := s.sceneContentOf(s.recordOf(run2, member2))
	s.Equal(other, scene2.ContentHash)
	s.Equal(edited, scene2.Key)
}

// TestARefusedFileHasNoHash: a check of a file that does not compile carries
// no hash, and a stored file that no longer compiles opens with none.
func (s *ContentHashSuite) TestARefusedFileHasNoHash() {
	const key = "my-broken-post"
	broken := strings.Replace(s.mine(key), "faction: watch", "faction: wach", 1)
	s.Require().NotEqual(s.mine(key), broken)
	resp := s.put(key, broken, true)
	s.Require().NotEmpty(resp.GetErrors())
	s.Empty(resp.GetContentHash())

	_, err := s.world.Put(context.Background(), &worlddungeons.PutInput{
		WorldID: dungeonstest.Caller.WorldID, Owner: dungeonstest.Caller.PlayerID, Key: key, Name: "broken", YAML: []byte(broken),
	})
	s.Require().NoError(err)
	opened := s.get(key)
	s.Equal(broken, opened.GetYaml(), "it still opens")
	s.Empty(opened.GetContentHash(), "with no hash")
}

// TestAShippedReferenceOpensWithItsHeldHash: a shipped file's hash is the
// one taken when it compiled at boot.
func (s *ContentHashSuite) TestAShippedReferenceOpensWithItsHeldHash() {
	s.Regexp(`^[0-9a-f]{64}$`, s.get(dungeons.DefaultKey).GetContentHash())
}
