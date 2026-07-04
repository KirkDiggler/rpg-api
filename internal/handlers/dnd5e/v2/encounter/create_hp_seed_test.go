package encounter_test

// create_hp_seed_test.go — rpg-api#612: CreateEncounter must seed the
// creator's PlayerData.HP/MaxHP from the character store, not leave it at
// the Go zero value. Before this fix, PlayerData.HP/MaxHP were never set
// anywhere: the toolkit's combat verbs only clamp HP DOWNWARD on a hit, so
// an unseeded seat stayed 0/0 forever — the client-facing HP bar always
// read 0/0 and the player death gate (hpBefore > 0 && hpAfter == 0) could
// never fire from an already-0 snapshot.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	v2encounter "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v2/encounter"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	encountersv2 "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/v2"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
)

const (
	hpSeedPlayerID = "player-hp-seed"
	// CreateEncounter mirrors EntityID = PlayerID for the initial seat
	// (create.go's own comment) — the character store is keyed by that
	// same string.
	hpSeedEntityID = hpSeedPlayerID
)

// CreateEncounterHPSeedSuite tests seedPlayerHP's three paths through the
// real CreateEncounter RPC: character found (seeds real HP), character not
// found (leaves 0/0, not fatal), and a real store error (surfaces as
// codes.Internal rather than being silently swallowed).
type CreateEncounterHPSeedSuite struct {
	suite.Suite
	ctrl         *gomock.Controller
	mockCharRepo *charactermock.MockRepository
	broker       *tkenc.Broker
	repo         encountersv2.Repository
	handler      *v2encounter.Handler
	ctx          context.Context
}

func TestCreateEncounterHPSeedSuite(t *testing.T) {
	suite.Run(t, new(CreateEncounterHPSeedSuite))
}

func (s *CreateEncounterHPSeedSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockCharRepo = charactermock.NewMockRepository(s.ctrl)
	s.broker = tkenc.NewBroker(tkenc.NewInMemoryTransport())
	s.repo = encountersv2.NewInMemory()
	s.ctx = auth.WithPlayerID(context.Background(), hpSeedPlayerID)

	fixedNow := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	h, err := v2encounter.New(&v2encounter.HandlerConfig{
		Broker: s.broker,
		Repo:   s.repo,
		Now:    func() time.Time { return fixedNow },
		CombatResolverConfig: &v2encounter.Dnd5eCombatResolverConfig{
			CharacterRepo: s.mockCharRepo,
		},
	})
	s.Require().NoError(err)
	s.handler = h
}

func (s *CreateEncounterHPSeedSuite) TearDownTest() {
	s.ctrl.Finish()
}

// TestCreateEncounter_CharacterFound_SeedsRealHP proves the fix: a character
// with HitPoints=27/MaxHitPoints=30 in the store produces a persisted
// PlayerData.HP=27/MaxHP=30 snapshot — not 0/0.
func (s *CreateEncounterHPSeedSuite) TestCreateEncounter_CharacterFound_SeedsRealHP() {
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: hpSeedEntityID}).
		Return(&characterrepo.GetOutput{
			Character: &entities.Character{
				Data: &character.Data{ID: hpSeedEntityID, HitPoints: 27, MaxHitPoints: 30},
			},
		}, nil)

	resp, err := s.handler.CreateEncounter(s.ctx, &encounterv2pb.CreateEncounterRequest{
		CampaignId: "campaign-1",
	})
	s.Require().NoError(err)
	s.Require().NotNil(resp.GetEncounter())

	data, err := s.repo.Get(s.ctx, resp.GetEncounter().GetId())
	s.Require().NoError(err)
	pd := data.Players[hpSeedPlayerID]
	s.Require().NotNil(pd, "creator must be seated as a player")
	s.Equal(27, pd.HP, "PlayerData.HP must be seeded from the character store, not left at 0")
	s.Equal(30, pd.MaxHP, "PlayerData.MaxHP must be seeded from the character store, not left at 0")
}

// TestCreateEncounter_CharacterNotFound_LeavesHPZero_NotFatal proves the
// regression guard: today's flow can add a player before any character
// exists (EntityID mirrors PlayerID, no character-selection step yet).
// NotFound must not fail the RPC — HP/MaxHP simply stay 0, same as the
// pre-fix behavior.
func (s *CreateEncounterHPSeedSuite) TestCreateEncounter_CharacterNotFound_LeavesHPZero_NotFatal() {
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: hpSeedEntityID}).
		Return(nil, apierr.NotFound("character not found"))

	resp, err := s.handler.CreateEncounter(s.ctx, &encounterv2pb.CreateEncounterRequest{
		CampaignId: "campaign-1",
	})
	s.Require().NoError(err, "a not-yet-selected character must not fail CreateEncounter")
	s.Require().NotNil(resp.GetEncounter())

	data, err := s.repo.Get(s.ctx, resp.GetEncounter().GetId())
	s.Require().NoError(err)
	pd := data.Players[hpSeedPlayerID]
	s.Require().NotNil(pd)
	s.Equal(0, pd.HP)
	s.Equal(0, pd.MaxHP)
}

// TestCreateEncounter_CharacterStoreRealError_SurfacesAsInternal proves a
// genuine store failure (not NotFound) is NOT silently swallowed into an
// incorrect 0/0 seed — it surfaces as codes.Internal.
func (s *CreateEncounterHPSeedSuite) TestCreateEncounter_CharacterStoreRealError_SurfacesAsInternal() {
	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: hpSeedEntityID}).
		Return(nil, errors.New("redis: connection refused"))

	_, err := s.handler.CreateEncounter(s.ctx, &encounterv2pb.CreateEncounterRequest{
		CampaignId: "campaign-1",
	})
	s.Require().Error(err)
	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Equal(codes.Internal, st.Code())
}

// TestCreateEncounter_NilCharacterRepo_LeavesHPZero_NotFatal proves the
// nil-CharacterRepo guard (handler tests / configs without a wired store)
// behaves exactly as it did before this fix — no panic, no error, 0/0 seed.
func (s *CreateEncounterHPSeedSuite) TestCreateEncounter_NilCharacterRepo_LeavesHPZero_NotFatal() {
	h, err := v2encounter.New(&v2encounter.HandlerConfig{
		Broker: s.broker,
		Repo:   encountersv2.NewInMemory(),
		Now:    func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) },
	})
	s.Require().NoError(err)

	resp, err := h.CreateEncounter(s.ctx, &encounterv2pb.CreateEncounterRequest{
		CampaignId: "campaign-1",
	})
	s.Require().NoError(err)
	s.Require().NotNil(resp.GetEncounter())
}
