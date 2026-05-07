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

func TestHandlerSuite(t *testing.T) {
	suite.Run(t, new(HandlerSuite))
}
