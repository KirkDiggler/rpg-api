package encounter_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
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
	ctx     context.Context
	broker  *tkenc.Broker
	repo    encountersv2.Repository
	handler *v2encounter.Handler
	fixed   time.Time
}

func (s *HandlerSuite) SetupTest() {
	s.fixed = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.ctx = auth.WithPlayerID(context.Background(), "player-A")
	s.broker = tkenc.NewBroker(tkenc.NewInMemoryTransport())
	s.repo = encountersv2.NewInMemory()
	h, err := v2encounter.New(&v2encounter.HandlerConfig{
		Broker: s.broker, Repo: s.repo, Now: func() time.Time { return s.fixed },
	})
	s.Require().NoError(err)
	s.handler = h
}

func (s *HandlerSuite) TestCreateEncounter_ReturnsUnimplemented() {
	_, err := s.handler.CreateEncounter(s.ctx, &encounterv2pb.CreateEncounterRequest{})
	s.Require().Error(err)
	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Require().Equal(codes.Unimplemented, st.Code())
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

	// Verify the encounter was saved post-move.
	loaded, err := s.repo.Get(s.ctx, "enc-1")
	s.Require().NoError(err)
	s.Require().NotNil(loaded)
	s.Require().Equal(core.EncounterID("enc-1"), loaded.ID)
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

func TestHandlerSuite(t *testing.T) {
	suite.Run(t, new(HandlerSuite))
}
