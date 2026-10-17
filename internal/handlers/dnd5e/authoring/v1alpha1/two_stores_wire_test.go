package authoringv1alpha1_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authoringpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/authoring/v1alpha1"
	lobbyv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/lobby/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	authoringhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/authoring/v1alpha1"
	lobbyhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/lobby/v1alpha1"
	authoringorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/authoring"
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
	"github.com/KirkDiggler/rpg-api/internal/repositories/worlddungeons"
	"github.com/KirkDiggler/rpg-api/internal/worldcontext"
)

const (
	wireKitchen  = dungeonstest.CastleKitchenKey
	wireMyCastle = "my-castle"
	wireMyKeep   = "my-keep"
)

// TwoStoresWireSuite is S7-A5's seam: the real authoring and lobby handlers
// over one real two-store registry (a temp copy of content/ with the castle
// kitchen fixture staged in as a reference, and a world
// store over miniredis), with contexts carrying the world and the player as
// the auth boundary installs them. Alice and bob share w1; carol is in w2.
type TwoStoresWireSuite struct {
	suite.Suite

	dir       string
	authoring *authoringhandler.Handler
	lobby     *lobbyhandler.Handler
	kitchen   []byte

	alice, bob, carol context.Context
}

func TestTwoStoresWireSuite(t *testing.T) { suite.Run(t, new(TwoStoresWireSuite)) }

func memberOf(worldID, playerID string) context.Context {
	return worldcontext.With(auth.WithPlayerID(context.Background(), playerID), worldcontext.Value{WorldID: worldID})
}

func (s *TwoStoresWireSuite) SetupTest() {
	s.alice, s.bob, s.carol = memberOf("w1", "alice"), memberOf("w1", "bob"), memberOf("w2", "carol")

	mr := miniredis.RunT(s.T())
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	s.T().Cleanup(func() { _ = client.Close() })
	chars, err := characterrepo.NewRedis(&characterrepo.RedisConfig{Client: client})
	s.Require().NoError(err)
	sessOrch, err := sessionorch.New(sessionorch.Config{
		Redis: client, Characters: chars, TTL: time.Hour, PresentationIDs: idgen.NewSequential("presentation"),
	})
	s.Require().NoError(err)
	projector := dungeonstest.ProjectorFor(sessOrch.Manager)

	s.dir = dungeonstest.ScratchDir(s.T())
	s.kitchen = dungeonstest.StageCastleKitchen(s.T(), s.dir)
	shipped, err := dungeons.NewFileRegistry(s.dir, projector)
	s.Require().NoError(err)
	world, err := worlddungeons.NewRedis(&worlddungeons.RedisConfig{Client: client})
	s.Require().NoError(err)
	registry, err := dungeons.NewRegistry(&dungeons.RegistryConfig{
		Shipped: shipped, World: world, Authoring: true, Projector: projector,
	})
	s.Require().NoError(err)

	aOrch, err := authoringorch.New(&authoringorch.Config{Dungeons: registry})
	s.Require().NoError(err)
	s.authoring, err = authoringhandler.New(&authoringhandler.HandlerConfig{Orchestrator: aOrch})
	s.Require().NoError(err)

	broker := lobbyorch.NewBroker()
	lOrch, err := lobbyorch.New(&lobbyorch.Config{
		LobbyRepo: lobbyrepo.NewInMemory(), LobbyBroker: broker, CharacterRepo: chars,
		LobbyIDGenerator: idgen.NewSequential("lobby"), JoinRefGenerator: idgen.NewSequential("ref"),
		EncounterIDGenerator: idgen.NewSequential("enc"), SessionManager: sessOrch.Manager, Dungeons: registry,
	})
	s.Require().NoError(err)
	s.lobby, err = lobbyhandler.New(&lobbyhandler.HandlerConfig{Orchestrator: lOrch, Broker: broker})
	s.Require().NoError(err)
}

func (s *TwoStoresWireSuite) file(key string) string {
	return string(dungeonstest.AsKey(s.T(), s.kitchen, wireKitchen, key))
}

func (s *TwoStoresWireSuite) requireStatus(err error, code codes.Code, message string) {
	s.T().Helper()
	st, ok := status.FromError(err)
	s.Require().True(ok, "not a status error: %v", err)
	s.Equal(code, st.Code(), st.Message())
	s.Equal(message, st.Message())
}

func (s *TwoStoresWireSuite) save(ctx context.Context, key, yaml string) {
	s.T().Helper()
	resp, err := s.authoring.PutDungeon(ctx, &authoringpb.PutDungeonRequest{Key: key, Yaml: yaml})
	s.Require().NoError(err)
	s.Require().Empty(resp.GetErrors())
}

func (s *TwoStoresWireSuite) worldRow(list []*lobbyv1alpha1.DungeonSummary, key string) *lobbyv1alpha1.DungeonSummary {
	for _, d := range list {
		if d.GetKey() == key {
			return d
		}
	}
	return nil
}

func (s *TwoStoresWireSuite) TestAMembersEncounterIsTheWorldsAndOnlyTheirs() {
	aliceYAML := s.file(wireMyCastle)
	s.save(s.alice, wireMyCastle, aliceYAML)

	for _, ctx := range []context.Context{s.alice, s.bob} {
		listed, err := s.authoring.ListDungeons(ctx, &authoringpb.ListDungeonsRequest{})
		s.Require().NoError(err)
		row := s.worldRow(listed.GetDungeons(), wireMyCastle)
		s.Require().NotNil(row)
		s.Equal(lobbyv1alpha1.DungeonSource_DUNGEON_SOURCE_WORLD, row.GetSource())
		s.Equal("alice", row.GetOwner())
	}

	got, err := s.authoring.GetDungeon(s.bob, &authoringpb.GetDungeonRequest{Key: wireMyCastle})
	s.Require().NoError(err)
	s.Equal(aliceYAML, got.GetYaml(), "bob reads alice's bytes")

	_, err = s.authoring.PutDungeon(s.bob, &authoringpb.PutDungeonRequest{Key: wireMyCastle, Yaml: aliceYAML + "# bob\n"})
	s.requireStatus(err, codes.PermissionDenied, "that key belongs to another member of this world")

	_, err = s.authoring.GetDungeon(s.carol, &authoringpb.GetDungeonRequest{Key: wireMyCastle})
	s.requireStatus(err, codes.NotFound, "dungeon not found")
	s.save(s.carol, wireMyCastle, aliceYAML+"# carol\n")
	got, err = s.authoring.GetDungeon(s.alice, &authoringpb.GetDungeonRequest{Key: wireMyCastle})
	s.Require().NoError(err)
	s.Equal(aliceYAML, got.GetYaml(), "two worlds hold the same key independently")
}

func (s *TwoStoresWireSuite) TestAReferenceKeyIsRefusedAndTheFileUntouched() {
	for _, validateOnly := range []bool{true, false} {
		_, err := s.authoring.PutDungeon(s.alice, &authoringpb.PutDungeonRequest{
			Key: wireKitchen, Yaml: string(s.kitchen) + "# edited\n", ValidateOnly: validateOnly,
		})
		s.requireStatus(err, codes.PermissionDenied, "that key is a reference; save it under your own key")
	}
	onDisk, err := os.ReadFile(filepath.Join(s.dir, wireKitchen+".yaml")) //nolint:gosec // dir is t.TempDir()
	s.Require().NoError(err)
	s.Equal(s.kitchen, onDisk, "the content file is byte-identical")
}

func (s *TwoStoresWireSuite) TestRenameThenDelete() {
	s.save(s.alice, wireMyCastle, s.file(wireMyCastle))

	_, err := s.authoring.RenameDungeon(s.alice, &authoringpb.RenameDungeonRequest{Key: wireMyCastle, NewKey: wireMyKeep})
	s.Require().NoError(err)
	got, err := s.authoring.GetDungeon(s.alice, &authoringpb.GetDungeonRequest{Key: wireMyKeep})
	s.Require().NoError(err)
	s.Contains(got.GetYaml(), "\nkey: my-keep\n")
	s.NotContains(got.GetYaml(), "key: my-castle")

	_, err = s.authoring.DeleteDungeon(s.bob, &authoringpb.DeleteDungeonRequest{Key: wireMyKeep})
	s.requireStatus(err, codes.PermissionDenied, "that key belongs to another member of this world")

	_, err = s.authoring.DeleteDungeon(s.alice, &authoringpb.DeleteDungeonRequest{Key: wireMyKeep})
	s.Require().NoError(err)
	listed, err := s.lobby.ListDungeons(s.alice, &lobbyv1alpha1.ListDungeonsRequest{})
	s.Require().NoError(err)
	s.Nil(s.worldRow(listed.GetDungeons(), wireMyKeep), "the lobby list lacks it")
	s.NotNil(s.worldRow(listed.GetDungeons(), wireKitchen), "and still lists the references")
}
