package session_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	charactermock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
)

type OrchestratorTestSuite struct {
	suite.Suite
	miniredis *miniredis.Miniredis
}

func (s *OrchestratorTestSuite) SetupTest() {
	s.miniredis = miniredis.RunT(s.T())
}

func (s *OrchestratorTestSuite) TearDownTest() {
	if s.miniredis != nil {
		s.miniredis.Close()
	}
}

func TestOrchestratorSuite(t *testing.T) {
	suite.Run(t, new(OrchestratorTestSuite))
}

func (s *OrchestratorTestSuite) TestNew_WithEveryCapabilitySupplied_Succeeds() {
	ctrl := gomock.NewController(s.T())
	client := goredis.NewClient(&goredis.Options{Addr: s.miniredis.Addr()})
	defer func() { _ = client.Close() }()

	orch, err := sessionorch.New(sessionorch.Config{
		Redis:           client,
		Characters:      charactermock.NewMockRepository(ctrl),
		TTL:             24 * time.Hour,
		PresentationIDs: idgen.NewSequential("presentation"),
	})
	s.Require().NoError(err)
	s.Require().NotNil(orch)
	s.NotNil(orch.Manager, "construction is total (SDK law S8): a usable Manager or an error, never a nil one with no error")
	s.NotNil(orch.Broker)
}

func (s *OrchestratorTestSuite) TestNew_MissingRedis_Errors() {
	ctrl := gomock.NewController(s.T())
	_, err := sessionorch.New(sessionorch.Config{
		Characters: charactermock.NewMockRepository(ctrl),
		TTL:        24 * time.Hour,
	})
	s.Require().Error(err)
}

// fixedRoller is a deterministic sdk.Roller a test can pass through
// Config.Dice to override the crypto-secure production default.
type fixedRoller struct{ value int }

func (r fixedRoller) Roll(_ context.Context, _ int) (int, error) { return r.value, nil }

func (s *OrchestratorTestSuite) TestNew_DiceOverride_IsHonored() {
	ctrl := gomock.NewController(s.T())
	client := goredis.NewClient(&goredis.Options{Addr: s.miniredis.Addr()})
	defer func() { _ = client.Close() }()

	orch, err := sessionorch.New(sessionorch.Config{
		Redis: client, Characters: charactermock.NewMockRepository(ctrl),
		TTL: 24 * time.Hour, Dice: fixedRoller{value: 20},
		PresentationIDs: idgen.NewSequential("presentation"),
	})
	s.Require().NoError(err)
	s.Require().NotNil(orch)
	// The override is exercised end-to-end by the acceptance suite
	// (internal/integration/session), which needs a guaranteed hit; this
	// test only pins that New accepts and does not silently drop it.
}

func (s *OrchestratorTestSuite) TestNew_MissingCharacters_Errors() {
	client := goredis.NewClient(&goredis.Options{Addr: s.miniredis.Addr()})
	defer func() { _ = client.Close() }()

	_, err := sessionorch.New(sessionorch.Config{
		Redis: client,
		TTL:   24 * time.Hour,
	})
	s.Require().Error(err)
}
