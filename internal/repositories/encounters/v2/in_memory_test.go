package encounters_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"

	encountersv2 "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/v2"
)

type InMemorySuite struct {
	suite.Suite
	ctx  context.Context
	repo encountersv2.Repository
}

func (s *InMemorySuite) SetupTest() {
	s.ctx = context.Background()
	s.repo = encountersv2.NewInMemory()
}

func (s *InMemorySuite) TestGet_ReturnsErrNotFound_ForMissingID() {
	_, err := s.repo.Get(s.ctx, "missing")
	s.Require().Error(err)
	s.Require().True(errors.Is(err, encountersv2.ErrNotFound))
}

func (s *InMemorySuite) TestSaveGet_RoundTrip_PreservesData() {
	enc := encounter.New("enc-1", encounter.NewBroker(encounter.NewInMemoryTransport()))
	// AddPlayer + minimal setup so ToData has something interesting.
	s.Require().NoError(enc.AddPlayer(encounter.PlayerInput{
		PlayerID: "player-A",
		EntityID: "char-A",
		Position: core.Hex{Q: 0, R: 0, S: 0},
	}))
	original := enc.ToData()

	s.Require().NoError(s.repo.Save(s.ctx, original))

	loaded, err := s.repo.Get(s.ctx, string(original.ID))
	s.Require().NoError(err)
	s.Require().Equal(original.ID, loaded.ID)
	// Round-trip via the toolkit serializer to prove ToData/LoadFromData survives storage.
	roundTripped, err := encounter.LoadFromData(loaded, encounter.NewBroker(encounter.NewInMemoryTransport()))
	s.Require().NoError(err)
	s.Require().Equal(original.ID, roundTripped.ID())
}

func TestInMemorySuite(t *testing.T) {
	suite.Run(t, new(InMemorySuite))
}
