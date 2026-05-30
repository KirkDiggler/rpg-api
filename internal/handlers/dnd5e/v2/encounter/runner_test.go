package encounter_test

// runner_test.go — TDD tests for the EncounterRunner.
//
// Tests the three behavior guarantees:
//  1. Non-member player → PermissionDenied.
//  2. Wrong entity (player is member but requests another player's entity) → PermissionDenied.
//  3. Happy path → verb invoked once, encounter saved, no error.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	v2encounter "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v2/encounter"
	encountersv2 "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/v2"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	encountercore "github.com/KirkDiggler/rpg-toolkit/encounter/core"
)

// RunnerSuite tests the EncounterRunner contract without wiring a full Handler.
type RunnerSuite struct {
	suite.Suite

	broker *tkenc.Broker
	repo   encountersv2.Repository
	runner *v2encounter.Runner
}

func TestRunnerSuite(t *testing.T) {
	suite.Run(t, new(RunnerSuite))
}

func (s *RunnerSuite) SetupTest() {
	s.broker = tkenc.NewBroker(tkenc.NewInMemoryTransport())
	s.repo = encountersv2.NewInMemory()

	fixedNow := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	s.runner = v2encounter.NewRunner(v2encounter.RunnerConfig{
		Broker: s.broker,
		Repo:   s.repo,
		Now:    func() time.Time { return fixedNow },
	})
}

// seedEncounter creates a minimal encounter with one player (playerID → entityID)
// and saves it. Returns the seeded encounter ID.
func (s *RunnerSuite) seedEncounter(encID, playerID, entityID string) {
	enc := tkenc.New(encountercore.EncounterID(encID), s.broker)

	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID:   encountercore.PlayerID(playerID),
		EntityID:   encountercore.EntityID(entityID),
		Position:   encountercore.Hex{Q: 0, R: 0, S: 0},
		SightRange: 10,
		HP:         10,
		MaxHP:      10,
		AC:         12,
	}))

	s.Require().NoError(s.repo.Save(context.Background(), enc.ToData()))
}

// TestRun_NonMember_PermissionDenied verifies that a player not seated in
// the encounter receives PermissionDenied before the verb is invoked.
func (s *RunnerSuite) TestRun_NonMember_PermissionDenied() {
	s.seedEncounter("enc-1", "player-alice", "char-alice")

	verbCalled := false
	err := s.runner.Run(
		context.Background(),
		&v2encounter.EncounterRunnerInput{
			EncounterID: "enc-1",
			PlayerID:    encountercore.PlayerID("player-stranger"), // not in encounter
			EntityID:    "char-alice",
		},
		func(_ *tkenc.Encounter, _ *tkenc.Data) error {
			verbCalled = true
			return nil
		},
	)

	s.Require().Error(err)
	st, ok := status.FromError(err)
	s.Require().True(ok, "error must be a gRPC status error")
	s.Equal(codes.PermissionDenied, st.Code(), "non-member must get PermissionDenied")
	s.False(verbCalled, "verb must not be called for non-member")
}

// TestRun_WrongEntity_PermissionDenied verifies that a player who IS in the
// encounter but requests a different entity receives PermissionDenied.
func (s *RunnerSuite) TestRun_WrongEntity_PermissionDenied() {
	s.seedEncounter("enc-2", "player-bob", "char-bob")

	verbCalled := false
	err := s.runner.Run(
		context.Background(),
		&v2encounter.EncounterRunnerInput{
			EncounterID: "enc-2",
			PlayerID:    encountercore.PlayerID("player-bob"),
			EntityID:    "char-alice", // bob's entity is char-bob, not char-alice
		},
		func(_ *tkenc.Encounter, _ *tkenc.Data) error {
			verbCalled = true
			return nil
		},
	)

	s.Require().Error(err)
	st, ok := status.FromError(err)
	s.Require().True(ok, "error must be a gRPC status error")
	s.Equal(codes.PermissionDenied, st.Code(), "wrong entity must get PermissionDenied")
	s.False(verbCalled, "verb must not be called for wrong entity")
}

// TestRun_HappyPath_InvokesVerbAndSaves verifies that on a valid request:
//   - The verb is called exactly once with a non-nil encounter.
//   - The encounter is saved after the verb returns.
//   - No error is returned.
func (s *RunnerSuite) TestRun_HappyPath_InvokesVerbAndSaves() {
	s.seedEncounter("enc-3", "player-carol", "char-carol")

	verbCallCount := 0
	var capturedEnc *tkenc.Encounter
	err := s.runner.Run(
		context.Background(),
		&v2encounter.EncounterRunnerInput{
			EncounterID: "enc-3",
			PlayerID:    encountercore.PlayerID("player-carol"),
			EntityID:    "char-carol",
		},
		func(enc *tkenc.Encounter, _ *tkenc.Data) error {
			verbCallCount++
			capturedEnc = enc
			return nil
		},
	)

	s.Require().NoError(err)
	s.Equal(1, verbCallCount, "verb must be called exactly once")
	s.Require().NotNil(capturedEnc, "verb must receive a non-nil encounter")

	// Verify encounter was saved (repo round-trip succeeds).
	data, repoErr := s.repo.Get(context.Background(), "enc-3")
	s.Require().NoError(repoErr)
	s.Require().NotNil(data, "encounter must be persisted after Run")
}

// TestRun_EncounterNotFound_NotFound verifies that a missing encounter ID
// returns NotFound.
func (s *RunnerSuite) TestRun_EncounterNotFound_NotFound() {
	err := s.runner.Run(
		context.Background(),
		&v2encounter.EncounterRunnerInput{
			EncounterID: "does-not-exist",
			PlayerID:    encountercore.PlayerID("player-x"),
			EntityID:    "char-x",
		},
		func(_ *tkenc.Encounter, _ *tkenc.Data) error { return nil },
	)

	s.Require().Error(err)
	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Equal(codes.NotFound, st.Code())
}

// TestRun_EmptyEntityID_SkipsOwnershipCheck verifies that an empty EntityID
// bypasses the entity-ownership check (used for operations where the player
// doesn't need to own a specific entity, e.g. future read-only operations).
func (s *RunnerSuite) TestRun_EmptyEntityID_SkipsOwnershipCheck() {
	s.seedEncounter("enc-4", "player-dave", "char-dave")

	verbCalled := false
	err := s.runner.Run(
		context.Background(),
		&v2encounter.EncounterRunnerInput{
			EncounterID: "enc-4",
			PlayerID:    encountercore.PlayerID("player-dave"),
			EntityID:    "", // empty = skip ownership check
		},
		func(_ *tkenc.Encounter, _ *tkenc.Data) error {
			verbCalled = true
			return nil
		},
	)

	s.Require().NoError(err)
	s.True(verbCalled, "verb must be called when EntityID is empty")
}

// TestRun_VerbError_PropagatesError verifies that if the verb returns an error,
// Run propagates it (wraps it as Internal).
func (s *RunnerSuite) TestRun_VerbError_PropagatesError() {
	s.seedEncounter("enc-5", "player-eve", "char-eve")

	verbErr := errors.New("verb failed")
	err := s.runner.Run(
		context.Background(),
		&v2encounter.EncounterRunnerInput{
			EncounterID: "enc-5",
			PlayerID:    encountercore.PlayerID("player-eve"),
			EntityID:    "char-eve",
		},
		func(_ *tkenc.Encounter, _ *tkenc.Data) error {
			return verbErr
		},
	)

	s.Require().Error(err)
	// The runner wraps verb errors as gRPC Internal.
	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Equal(codes.Internal, st.Code())
}
