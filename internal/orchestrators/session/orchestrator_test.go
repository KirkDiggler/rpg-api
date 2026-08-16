package session_test

import (
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
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
		Redis:      client,
		Characters: charactermock.NewMockRepository(ctrl),
		TTL:        24 * time.Hour,
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

func (s *OrchestratorTestSuite) TestNew_MissingCharacters_Errors() {
	client := goredis.NewClient(&goredis.Options{Addr: s.miniredis.Addr()})
	defer func() { _ = client.Close() }()

	_, err := sessionorch.New(sessionorch.Config{
		Redis: client,
		TTL:   24 * time.Hour,
	})
	s.Require().Error(err)
}
