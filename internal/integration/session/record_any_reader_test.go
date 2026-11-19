package session_test

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"

	recordpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	recordhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha2"
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
)

// readerPlayer is signed in and has never been seated in any run.
const readerPlayer = "player-onlooker"

// RecordAnyReaderSuite is rpg-project#1001 item 14, as ruled: "in dev mode
// we can play any record". Through a real gRPC server and client, a run's
// whole record reads in development mode for a caller who was never seated,
// for a member after the run ended and the party left, and while it is live;
// outside development mode every one of those reads is refused as before.
type RecordAnyReaderSuite struct {
	suite.Suite

	h       *acceptanceHarness
	dev     recordpb.RecordServiceClient
	release recordpb.RecordServiceClient
	lobby   *lobbyorch.Orchestrator
	lobbies lobbyrepo.Repository
	run     string
	member  string
	host    string
}

func TestRecordAnyReaderSuite(t *testing.T) { suite.Run(t, new(RecordAnyReaderSuite)) }

func (s *RecordAnyReaderSuite) SetupSuite() {
	s.h = newAcceptanceHarnessWithDice(s.T(), alwaysTheLowestFace{})
	projector := dungeonstest.ProjectorFor(s.h.manager.Manager)
	raw, err := os.ReadFile(dungeonstest.SitePath(s.T(), hashSiteKey)) //nolint:gosec // repository fixture
	s.Require().NoError(err)
	dir := s.T().TempDir()
	s.Require().NoError(os.WriteFile(filepath.Join(dir, dungeons.DefaultKey+".yaml"),
		dungeonstest.AsKey(s.T(), raw, hashSiteKey, dungeons.DefaultKey), 0o600))
	shipped, err := dungeons.NewFileRegistry(dir, projector)
	s.Require().NoError(err)

	s.lobbies = lobbyrepo.NewInMemory()
	s.lobby, err = lobbyorch.New(&lobbyorch.Config{
		LobbyRepo: s.lobbies, LobbyBroker: lobbyorch.NewBroker(), CharacterRepo: s.h.charRepo,
		LobbyIDGenerator: idgen.NewSequential("lobby"), JoinRefGenerator: idgen.NewSequential("ref"),
		EncounterIDGenerator: idgen.NewSequential("enc"), SessionManager: s.h.manager.Manager,
		Dungeons: dungeonstest.Join(s.T(), shipped, false),
	})
	s.Require().NoError(err)

	s.dev = s.serve(true)
	s.release = s.serve(false)

	s.member, s.host = "reader-alice", "player-reader-alice"
	_, err = s.h.charRepo.Create(context.Background(), characterrepo.CreateInput{
		Character: &entities.Character{Data: armedFighter(s.member, s.host)},
	})
	s.Require().NoError(err)
	s.Require().NoError(s.lobbies.Save(context.Background(), &lobbyrepo.Data{
		ID: "lobby-reader", HostPlayerID: s.host, Status: lobbyrepo.StatusWaiting,
		Members:     map[string]*lobbyrepo.Member{s.host: {PlayerID: s.host, CharacterID: s.member, IsHost: true, IsReady: true}},
		MemberOrder: []string{s.host},
	}))
	out, err := s.lobby.StartEncounter(context.Background(), &lobbyorch.StartEncounterInput{
		WorldID: dungeonstest.Caller.WorldID, PlayerID: s.host, LobbyID: "lobby-reader",
	})
	s.Require().NoError(err)
	s.run = out.EncounterID
}

// serve is a real gRPC server for the record service, development mode on
// or off, behind a stand-in for the auth boundary.
func (s *RecordAnyReaderSuite) serve(development bool) recordpb.RecordServiceClient {
	handler, err := recordhandler.New(&recordhandler.HandlerConfig{
		Manager: s.h.manager.Manager, Broker: s.h.manager.RecordBroker, Enabled: development,
	})
	s.Require().NoError(err)
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer(grpc.UnaryInterceptor(signedInPlayer), grpc.StreamInterceptor(signedInPlayerStream))
	recordpb.RegisterRecordServiceServer(srv, handler)
	go func() { _ = srv.Serve(lis) }()
	s.T().Cleanup(srv.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = conn.Close() })
	return recordpb.NewRecordServiceClient(conn)
}

func playerFrom(ctx context.Context) context.Context {
	md, _ := metadata.FromIncomingContext(ctx)
	if p := md.Get("player"); len(p) == 1 {
		return auth.WithPlayerID(ctx, p[0])
	}
	return ctx
}

func signedInPlayer(ctx context.Context, req any, _ *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
	return next(playerFrom(ctx), req)
}

type playerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (p *playerStream) Context() context.Context { return p.ctx }

func signedInPlayerStream(srv any, ss grpc.ServerStream, _ *grpc.StreamServerInfo, next grpc.StreamHandler) error {
	return next(srv, &playerStream{ServerStream: ss, ctx: playerFrom(ss.Context())})
}

func as(player string) context.Context {
	return metadata.AppendToOutgoingContext(context.Background(), "player", player)
}

// requireWhole asserts the page is the run's whole record: a beat told to
// nobody (scene_content) and another member's private beat (a guard's own
// sighting) are both in it.
func (s *RecordAnyReaderSuite) requireWhole(events []*recordpb.RecordEvent) {
	s.T().Helper()
	scene, guardsOwn := false, false
	for _, e := range events {
		switch e.GetBeat() {
		case encounter.BeatSceneContent:
			scene = true
		case encounter.BeatSighted:
			var seen struct{ Observer string }
			s.Require().NoError(json.Unmarshal(e.GetFieldsJson(), &seen))
			if seen.Observer == "guard-1" {
				guardsOwn = true
			}
		}
	}
	s.True(scene, "a beat told to nobody is in it")
	s.True(guardsOwn, "another member's private beat is in it")
}

// TestALiveRunReadsForACallerNeverSeated, by page and by stream.
func (s *RecordAnyReaderSuite) TestALiveRunReadsForACallerNeverSeated() {
	page, err := s.dev.GetRecord(as(readerPlayer), &recordpb.GetRecordRequest{Session: s.run})
	s.Require().NoError(err)
	s.requireWhole(page.GetEvents())

	ctx, cancel := context.WithCancel(as(readerPlayer))
	defer cancel()
	stream, err := s.dev.StreamRecord(ctx, &recordpb.StreamRecordRequest{Session: s.run})
	s.Require().NoError(err)
	first, err := stream.Recv()
	s.Require().NoError(err)
	s.Equal(page.GetEvents()[0].GetId(), first.GetEvent().GetId(), "the stream replays the same record")
}

// TestOutsideDevelopmentModeEveryReadIsRefused: the same calls, refused as
// before, for a stranger and for the run's own member alike.
func (s *RecordAnyReaderSuite) TestOutsideDevelopmentModeEveryReadIsRefused() {
	for _, player := range []string{readerPlayer, s.host} {
		_, err := s.release.GetRecord(as(player), &recordpb.GetRecordRequest{Session: s.run, Member: s.member})
		s.Equal(codes.PermissionDenied, status.Code(err), player)
		stream, err := s.release.StreamRecord(as(player), &recordpb.StreamRecordRequest{Session: s.run, Member: s.member})
		s.Require().NoError(err)
		_, err = stream.Recv()
		s.Equal(codes.PermissionDenied, status.Code(err), player)
	}
}

// TestAnAnonymousCallerIsRefused: no seat is asked, but a signed-in caller is.
func (s *RecordAnyReaderSuite) TestAnAnonymousCallerIsRefused() {
	_, err := s.dev.GetRecord(context.Background(), &recordpb.GetRecordRequest{Session: s.run})
	s.Equal(codes.Unauthenticated, status.Code(err))
}

// TestZZAnEndedRunReadsForTheMemberWhoLeft runs last (it ends the suite's
// run): the host abandons it, the party leaves, and the record still reads
// whole, for the member who left and for a caller never seated.
func (s *RecordAnyReaderSuite) TestZZAnEndedRunReadsForTheMemberWhoLeft() {
	_, err := s.lobby.AbandonEncounter(context.Background(), &lobbyorch.AbandonEncounterInput{
		PlayerID: s.host, LobbyID: "lobby-reader",
	})
	s.Require().NoError(err)
	for _, player := range []string{s.host, readerPlayer} {
		page, err := s.dev.GetRecord(as(player), &recordpb.GetRecordRequest{Session: s.run, Member: s.member})
		s.Require().NoError(err, player)
		s.requireWhole(page.GetEvents())
	}
}
