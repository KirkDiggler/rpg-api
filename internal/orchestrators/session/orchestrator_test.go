package session_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

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

func (s *OrchestratorTestSuite) TestNew_InvalidStaleTargetPolicyFailsConstruction() {
	ctrl := gomock.NewController(s.T())
	client := goredis.NewClient(&goredis.Options{Addr: s.miniredis.Addr()})
	defer func() { _ = client.Close() }()
	orch, err := sessionorch.New(sessionorch.Config{
		Redis: client, Characters: charactermock.NewMockRepository(ctrl), StaleTargetPolicy: "typo",
	})
	s.Nil(orch)
	s.ErrorIs(err, sdk.ErrIncompleteConfig)
	s.ErrorContains(err, "StaleTargetPolicy")
}

// passDriver is an every-session driver a test can hand Config.TurnDriver.
type passDriver struct{}

func (passDriver) Act(sdk.MonsterView) (sdk.TurnIntent, error) { return sdk.Pass{}, nil }

// Exactly one of the SDK's two driver doors is taken, and which one depends on
// whether a driver was supplied.
//
// THE SDK IS THE ASSERTION HERE, which is what makes these two rows worth
// having rather than "New returns no error" twice. sdk.NewManager refuses a
// config with both TurnDriver and TurnDrivers wired (ErrAmbiguousConfig,
// rpg-toolkit#1734) and refuses one with neither (ErrIncompleteConfig). So a
// Manager that exists at all is proof this package wired exactly one — and
// wiring the per-session cache alongside a supplied driver, or forgetting to
// wire either, fails right here rather than in a walk.
func (s *OrchestratorTestSuite) TestNew_TakesExactlyOneDriverDoor() {
	ctrl := gomock.NewController(s.T())
	client := goredis.NewClient(&goredis.Options{Addr: s.miniredis.Addr()})
	defer func() { _ = client.Close() }()

	base := func() sessionorch.Config {
		return sessionorch.Config{
			Redis: client, Characters: charactermock.NewMockRepository(ctrl),
			TTL: 24 * time.Hour, PresentationIDs: idgen.NewSequential("presentation"),
		}
	}

	s.Run("no driver supplied takes the per-session door", func() {
		orch, err := sessionorch.New(base())
		s.Require().NoError(err)
		s.Require().NotNil(orch)
		s.NotNil(orch.Manager)
	})

	s.Run("a supplied driver takes the every-session door", func() {
		cfg := base()
		cfg.TurnDriver = passDriver{}

		orch, err := sessionorch.New(cfg)
		s.Require().NoError(err)
		s.Require().NotNil(orch)
		s.NotNil(orch.Manager)
	})
}
