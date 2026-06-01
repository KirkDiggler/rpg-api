package encounter_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	encounterorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/encounter/v2"
	encountersv2 "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/v2"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
)

// MoveEntity orchestrator tests (#582 step 6). These exercise the orchestrator's
// load → Move verb → persist core directly, proving:
//   - the shared load preconditions (membership + entity ownership) classify as
//     the orchestrator sentinels the handler maps to gRPC codes;
//   - empty-path classifies as ErrEmptyPath AFTER load (so the auth sentinels
//     keep precedence over the argument-shaped error);
//   - the happy path moves the player to the path destination and persists.
//
// The opportunity-attack path (a fleeing combatant leaving reach → resolver
// triggers an OA) is covered end-to-end through the handler by the movement-OA
// integration suite; these unit tests wire a nil movement resolver so the
// toolkit Move takes the legacy single-jump branch (no OAs), isolating the
// orchestrator's load → verb → persist contract.

const (
	meEncID     = "enc-move-entity"
	mePlayerAmy = "amy"
	meEntityAmy = "char-amy"
)

type MoveEntitySuite struct {
	suite.Suite

	ctx    context.Context
	broker *tkenc.Broker
	repo   encountersv2.Repository
	orch   *encounterorch.Orchestrator
}

func (s *MoveEntitySuite) SetupTest() {
	s.ctx = context.Background()
	s.broker = tkenc.NewBroker(tkenc.NewInMemoryTransport())
	s.repo = encountersv2.NewInMemory()

	orch, err := encounterorch.New(&encounterorch.Config{
		Broker:        s.broker,
		EncounterRepo: s.repo,
		Resolver:      stubCharacterResolver{},
		BuildCombatResolver: func(_ *tkenc.Data) encounterorch.CombatResolver {
			return nil
		},
		// nil movement resolver: the toolkit Move takes the legacy single-jump
		// branch (no per-step chain, no OAs) — the cleanest path to prove the
		// orchestrator's load → Move → persist contract without a rulebook.
		BuildMovementResolver: func(_ *tkenc.Data) tkenc.MovementResolver {
			return nil
		},
		Now: func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) },
	})
	s.Require().NoError(err)
	s.orch = orch
}

// seedMover persists a free-roam encounter with amy controlling char-amy at the
// origin. Move has no turn-order gate (slice scope), so no initiative override
// is needed.
func (s *MoveEntitySuite) seedMover(encID string) {
	enc := tkenc.New(s.ctx, core.EncounterID(encID), s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID:   mePlayerAmy,
		EntityID:   meEntityAmy,
		Position:   core.Hex{Q: 0, R: 0, S: 0},
		SightRange: 10,
		HP:         14,
		MaxHP:      14,
		AC:         14,
	}))
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))
}

func (s *MoveEntitySuite) move(encID, entity string, path []core.Hex) (*encounterorch.MoveEntityOutput, error) {
	return s.orch.MoveEntity(s.ctx, &encounterorch.MoveEntityInput{
		EncounterID: encID,
		PlayerID:    mePlayerAmy,
		EntityID:    core.EntityID(entity),
		Path:        path,
	})
}

// --- nil input ---

func (s *MoveEntitySuite) TestMoveEntity_NilInput_Error() {
	_, err := s.orch.MoveEntity(s.ctx, nil)
	s.Require().Error(err)
}

// --- load preconditions ---

func (s *MoveEntitySuite) TestMoveEntity_MissingEncounter_ErrEncounterNotFound() {
	_, err := s.move("nope", meEntityAmy, []core.Hex{{Q: 1, R: -1, S: 0}})
	s.Require().ErrorIs(err, encounterorch.ErrEncounterNotFound)
}

func (s *MoveEntitySuite) TestMoveEntity_PlayerNotInEncounter_ErrPlayerNotInEncounter() {
	enc := tkenc.New(s.ctx, "enc-no-player", s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "other", EntityID: "other-char", Position: core.Hex{Q: 0, R: 0, S: 0}, SightRange: 4,
	}))
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))

	_, err := s.move("enc-no-player", meEntityAmy, []core.Hex{{Q: 1, R: -1, S: 0}})
	s.Require().ErrorIs(err, encounterorch.ErrPlayerNotInEncounter)
}

func (s *MoveEntitySuite) TestMoveEntity_EntityMismatch_ErrEntityOwnershipMismatch() {
	s.seedMover("enc-mismatch")
	_, err := s.move("enc-mismatch", "char-someone-else", []core.Hex{{Q: 1, R: -1, S: 0}})
	s.Require().ErrorIs(err, encounterorch.ErrEntityOwnershipMismatch)
}

// --- empty path classified AFTER load (auth sentinels keep precedence) ---

func (s *MoveEntitySuite) TestMoveEntity_EmptyPath_ErrEmptyPath() {
	s.seedMover("enc-empty-path")
	_, err := s.move("enc-empty-path", meEntityAmy, nil)
	s.Require().ErrorIs(err, encounterorch.ErrEmptyPath)
}

// Empty path on a MISSING encounter must classify as the load sentinel, NOT
// ErrEmptyPath — proving the empty-path check runs after load.
func (s *MoveEntitySuite) TestMoveEntity_EmptyPathMissingEncounter_LoadSentinelWins() {
	_, err := s.move("nope", meEntityAmy, nil)
	s.Require().ErrorIs(err, encounterorch.ErrEncounterNotFound)
	s.Require().NotErrorIs(err, encounterorch.ErrEmptyPath)
}

// --- happy path: moves to destination + persists snapshot ---

func (s *MoveEntitySuite) TestMoveEntity_HappyPath_MovesAndPersists() {
	s.seedMover(meEncID)

	out, err := s.move(meEncID, meEntityAmy, []core.Hex{
		{Q: 0, R: 0, S: 0},
		{Q: 1, R: -1, S: 0},
	})
	s.Require().NoError(err, "move must succeed via the toolkit verb")
	s.Require().NotNil(out)

	loaded, getErr := s.repo.Get(s.ctx, meEncID)
	s.Require().NoError(getErr)
	s.Require().NotNil(loaded)
	s.Require().Contains(loaded.Players, core.PlayerID(mePlayerAmy))
	s.Equal(core.Hex{Q: 1, R: -1, S: 0}, loaded.Players[mePlayerAmy].View.Position,
		"mover must land at the path destination and persist")
}

func TestMoveEntitySuite(t *testing.T) {
	suite.Run(t, new(MoveEntitySuite))
}
