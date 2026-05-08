package encounter_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	v2encounter "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v2/encounter"
	encountersv2 "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/v2"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	core "github.com/KirkDiggler/rpg-toolkit/encounter/core"
)

type HandlerSuite struct {
	suite.Suite
	ctx    context.Context
	cancel context.CancelFunc // canceled in TearDownTest to stop streaming goroutines

	broker  *tkenc.Broker
	repo    encountersv2.Repository
	handler *v2encounter.Handler
	fixed   time.Time
}

func (s *HandlerSuite) SetupTest() {
	s.fixed = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	base, cancel := context.WithCancel(context.Background())
	s.cancel = cancel
	s.ctx = auth.WithPlayerID(base, "player-A")
	s.broker = tkenc.NewBroker(tkenc.NewInMemoryTransport())
	s.repo = encountersv2.NewInMemory()
	// Capture s.fixed by value (not pointer) to avoid a data race: the streaming
	// goroutines spawned in tests call h.now() concurrently with the next test's
	// SetupTest overwriting s.fixed.
	fixedNow := s.fixed
	h, err := v2encounter.New(&v2encounter.HandlerConfig{
		Broker: s.broker, Repo: s.repo, Now: func() time.Time { return fixedNow },
	})
	s.Require().NoError(err)
	s.handler = h
}

// TearDownTest cancels the test context, which causes any streaming goroutines
// started by the test to exit their select loop and return.
func (s *HandlerSuite) TearDownTest() {
	s.cancel()
}

func (s *HandlerSuite) TestCreateEncounter_NoAuth_Unauthenticated() {
	ctx := context.Background() // no auth
	_, err := s.handler.CreateEncounter(ctx, &encounterv2pb.CreateEncounterRequest{
		CampaignId: "campaign-1",
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.Unauthenticated, st.Code())
}

func (s *HandlerSuite) TestCreateEncounter_EmptyCampaignID_InvalidArgument() {
	_, err := s.handler.CreateEncounter(s.ctx, &encounterv2pb.CreateEncounterRequest{
		CampaignId: "",
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.InvalidArgument, st.Code())
}

func (s *HandlerSuite) TestCreateEncounter_TurnBasedMode_InvalidArgument() {
	_, err := s.handler.CreateEncounter(s.ctx, &encounterv2pb.CreateEncounterRequest{
		CampaignId:  "campaign-1",
		InitialMode: encounterv2pb.EncounterMode_ENCOUNTER_MODE_TURN_BASED,
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.InvalidArgument, st.Code())
}

func (s *HandlerSuite) TestCreateEncounter_UnspecifiedMode_TreatedAsFreeRoam() {
	resp, err := s.handler.CreateEncounter(s.ctx, &encounterv2pb.CreateEncounterRequest{
		CampaignId: "campaign-1",
		// InitialMode omitted -> ENCOUNTER_MODE_UNSPECIFIED
	})
	s.Require().NoError(err)
	s.Require().NotNil(resp.GetEncounter())
	s.Require().NotEmpty(resp.GetEncounter().GetId())
}

func (s *HandlerSuite) TestCreateEncounter_Success_ReturnsEncounterWithID() {
	resp, err := s.handler.CreateEncounter(s.ctx, &encounterv2pb.CreateEncounterRequest{
		CampaignId:  "campaign-1",
		InitialMode: encounterv2pb.EncounterMode_ENCOUNTER_MODE_FREE_ROAM,
	})
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Require().NotNil(resp.GetEncounter())
	s.Require().NotEmpty(resp.GetEncounter().GetId())
}

func (s *HandlerSuite) TestCreateEncounter_Success_PersistsToRepo() {
	resp, err := s.handler.CreateEncounter(s.ctx, &encounterv2pb.CreateEncounterRequest{
		CampaignId:  "campaign-1",
		InitialMode: encounterv2pb.EncounterMode_ENCOUNTER_MODE_FREE_ROAM,
	})
	s.Require().NoError(err)
	s.Require().NotNil(resp.GetEncounter())

	encID := resp.GetEncounter().GetId()
	data, err := s.repo.Get(s.ctx, encID)
	s.Require().NoError(err)
	s.Require().NotNil(data)
	s.Require().Equal(encID, string(data.ID))
}

func (s *HandlerSuite) TestMoveEntity_HappyPath_LoadsCallsMoveSaves() {
	// Seed encounter with player-A controlling char-A at (0,0,0).
	enc := tkenc.New("enc-1", s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "player-A", EntityID: "char-A", Position: core.Hex{Q: 0, R: 0, S: 0},
	}))
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))

	_, err := s.handler.MoveEntity(s.ctx, &encounterv2pb.MoveEntityRequest{
		EncounterId:  "enc-1",
		EntityId:     "char-A",
		ProposedPath: []*encounterv2pb.Position{{X: 0, Y: 0, Z: 0}, {X: 1, Y: -1, Z: 0}},
	})
	s.Require().NoError(err)

	// Verify the encounter was saved post-move and that player-A's position
	// reflects the move destination (0,0,0) → (1,-1,0).
	loaded, err := s.repo.Get(s.ctx, "enc-1")
	s.Require().NoError(err)
	s.Require().NotNil(loaded)
	s.Require().Equal(core.EncounterID("enc-1"), loaded.ID)
	s.Require().NotNil(loaded.Players)
	s.Require().Contains(loaded.Players, core.PlayerID("player-A"))
	s.Require().Equal(core.Hex{Q: 1, R: -1, S: 0}, loaded.Players["player-A"].View.Position)
}

func (s *HandlerSuite) TestMoveEntity_NoPlayerID_Unauthenticated() {
	ctx := context.Background() // no auth
	_, err := s.handler.MoveEntity(ctx, &encounterv2pb.MoveEntityRequest{EncounterId: "enc-1"})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.Unauthenticated, st.Code())
}

func (s *HandlerSuite) TestMoveEntity_MissingEncounter_NotFound() {
	_, err := s.handler.MoveEntity(s.ctx, &encounterv2pb.MoveEntityRequest{
		EncounterId: "missing", EntityId: "char-A",
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.NotFound, st.Code())
}

func (s *HandlerSuite) TestMoveEntity_PlayerNotInEncounter_PermissionDenied() {
	// Seed encounter with no players; auth as player-A (the suite default).
	enc := tkenc.New("enc-noplayer", s.broker)
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))

	_, err := s.handler.MoveEntity(s.ctx, &encounterv2pb.MoveEntityRequest{
		EncounterId: "enc-noplayer", EntityId: "char-A",
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.PermissionDenied, st.Code())
}

func (s *HandlerSuite) TestMoveEntity_EntityIDMismatch_PermissionDenied() {
	enc := tkenc.New("enc-2", s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "player-A", EntityID: "char-A", Position: core.Hex{Q: 0, R: 0, S: 0},
	}))
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))

	_, err := s.handler.MoveEntity(s.ctx, &encounterv2pb.MoveEntityRequest{
		EncounterId: "enc-2", EntityId: "char-IMPOSTER",
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.PermissionDenied, st.Code())
}

func (s *HandlerSuite) TestMoveEntity_EmptyPath_InvalidArgument() {
	enc := tkenc.New("enc-3", s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "player-A", EntityID: "char-A", Position: core.Hex{Q: 0, R: 0, S: 0},
	}))
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))

	_, err := s.handler.MoveEntity(s.ctx, &encounterv2pb.MoveEntityRequest{
		EncounterId: "enc-3", EntityId: "char-A",
		ProposedPath: nil, // empty path is the one true argument-shaped error
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.InvalidArgument, st.Code())
}

func (s *HandlerSuite) TestStreamEncounter_SendsSnapshotFirst() {
	enc := tkenc.New("enc-1", s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "player-A", EntityID: "char-A", Position: core.Hex{Q: 0, R: 0, S: 0},
	}))
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))

	stream := newCapturingStream(s.ctx)
	go func() {
		_ = s.handler.StreamEncounter(&encounterv2pb.StreamEncounterRequest{
			EncounterId: "enc-1",
		}, stream)
	}()

	first := stream.WaitForSend(s.T(), 2*time.Second)
	s.Require().NotNil(first.GetSnapshotDelivered(), "first event should be SnapshotDelivered")
}

func (s *HandlerSuite) TestStreamEncounter_ForwardsBrokerEvents() {
	enc := tkenc.New("enc-1", s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "player-A", EntityID: "char-A", Position: core.Hex{Q: 0, R: 0, S: 0},
	}))
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))

	stream := newCapturingStream(s.ctx)
	go func() {
		_ = s.handler.StreamEncounter(&encounterv2pb.StreamEncounterRequest{
			EncounterId: "enc-1",
		}, stream)
	}()

	// Drain the snapshot.
	_ = stream.WaitForSend(s.T(), 2*time.Second)

	// Move via the handler — broker emits MoveEvent.
	_, err := s.handler.MoveEntity(s.ctx, &encounterv2pb.MoveEntityRequest{
		EncounterId:  "enc-1",
		EntityId:     "char-A",
		ProposedPath: []*encounterv2pb.Position{{X: 0, Y: 0, Z: 0}, {X: 1, Y: -1, Z: 0}},
	})
	s.Require().NoError(err)

	// Stream should receive an EntityMoved.
	got := stream.WaitForSend(s.T(), 2*time.Second)
	s.Require().NotNil(got.GetEntityMoved())
}

func (s *HandlerSuite) TestGetEncounter_NoAuth_Unauthenticated() {
	ctx := context.Background() // no auth
	_, err := s.handler.GetEncounter(ctx, &encounterv2pb.GetEncounterRequest{
		EncounterId: "enc-1",
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.Unauthenticated, st.Code())
}

func (s *HandlerSuite) TestGetEncounter_EmptyEncounterID_InvalidArgument() {
	_, err := s.handler.GetEncounter(s.ctx, &encounterv2pb.GetEncounterRequest{
		EncounterId: "",
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.InvalidArgument, st.Code())
}

func (s *HandlerSuite) TestGetEncounter_UnknownID_NotFound() {
	_, err := s.handler.GetEncounter(s.ctx, &encounterv2pb.GetEncounterRequest{
		EncounterId: "does-not-exist",
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.NotFound, st.Code())
}

func (s *HandlerSuite) TestGetEncounter_NonMember_PermissionDenied() {
	// Encounter seeded with player-B only; auth context is player-A.
	enc := tkenc.New("enc-nonmember", s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "player-B", EntityID: "char-B", Position: core.Hex{Q: 0, R: 0, S: 0},
	}))
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))

	_, err := s.handler.GetEncounter(s.ctx, &encounterv2pb.GetEncounterRequest{
		EncounterId: "enc-nonmember",
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.PermissionDenied, st.Code())
}

func (s *HandlerSuite) TestGetEncounter_Success_ReturnsEncounterWithID() {
	enc := tkenc.New("enc-get-1", s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "player-A", EntityID: "char-A", Position: core.Hex{Q: 0, R: 0, S: 0},
	}))
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))

	resp, err := s.handler.GetEncounter(s.ctx, &encounterv2pb.GetEncounterRequest{
		EncounterId: "enc-get-1",
	})
	s.Require().NoError(err)
	s.Require().NotNil(resp.GetEncounter())
	s.Require().Equal("enc-get-1", resp.GetEncounter().GetId())
}

func TestHandlerSuite(t *testing.T) {
	suite.Run(t, new(HandlerSuite))
}

// capturingStream satisfies grpc.ServerStreamingServer[encounterv2pb.EncounterEvent]
// (i.e., encounterv2pb.EncounterService_StreamEncounterServer) for unit tests.
// It records Send calls in a buffered channel so tests can assert on them.
type capturingStream struct {
	grpc.ServerStream // embed for unused methods (SetHeader, SendHeader, SetTrailer, etc.)
	ctx               context.Context
	sent              chan *encounterv2pb.EncounterEvent
}

func newCapturingStream(ctx context.Context) *capturingStream {
	return &capturingStream{ctx: ctx, sent: make(chan *encounterv2pb.EncounterEvent, 16)}
}

// Context returns the stream's context; satisfies grpc.ServerStream.
func (s *capturingStream) Context() context.Context { return s.ctx }

// Send records the event for later assertion. Non-blocking: if the
// 16-slot buffer fills (more events than the test drains), Send returns
// an error rather than deadlocking the streaming goroutine.
func (s *capturingStream) Send(evt *encounterv2pb.EncounterEvent) error {
	select {
	case s.sent <- evt:
		return nil
	default:
		return fmt.Errorf("capturingStream buffer full (16 events undrained); test should drain or grow buffer")
	}
}

// WaitForSend blocks until an event arrives or timeout expires.
func (s *capturingStream) WaitForSend(t *testing.T, timeout time.Duration) *encounterv2pb.EncounterEvent {
	t.Helper()
	select {
	case evt := <-s.sent:
		return evt
	case <-time.After(timeout):
		t.Fatalf("no event received within %s", timeout)
		return nil
	}
}
