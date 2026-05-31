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

type SubmitCheckSuite struct {
	suite.Suite

	ctx    context.Context
	broker *tkenc.Broker
	repo   encountersv2.Repository
	orch   *encounterorch.Orchestrator
}

func (s *SubmitCheckSuite) SetupTest() {
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
		BuildMovementResolver: func(_ *tkenc.Data) tkenc.MovementResolver {
			return nil
		},
		// Reaction funcs supplied so SubmitReactionCheck reaches the load path in
		// the reaction tests below (decode/build are only invoked once a prompt
		// is present, which the deterministic tests below do not reach).
		ReactionResume: encounterorch.ReactionResume{
			DecodeAttackContext: func(_ []byte) (*tkenc.PhasedAttackContext, error) {
				return &tkenc.PhasedAttackContext{}, nil
			},
			BuildReactionModifiers: func(_ *tkenc.PendingReactionPrompt, _ bool) []tkenc.ReactionModifier {
				return nil
			},
			IsOneShotReaction: func(_ string) bool { return false },
		},
		Now: func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) },
	})
	s.Require().NoError(err)
	s.orch = orch
}

// seedLockedDoor persists an encounter with player-A (controlling char-A)
// adjacent to a locked door, then issues the per-player skill-check prompt via
// Interact so SubmitCheck has a pending prompt to resolve.
func (s *SubmitCheckSuite) seedLockedDoorWithPrompt(encID, doorID string, dc int) {
	enc := tkenc.New(s.ctx, core.EncounterID(encID), s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID:   "player-A",
		EntityID:   "char-A",
		Position:   core.Hex{Q: 0, R: 0, S: 0},
		SightRange: 4,
	}))
	enc.AddDoor(core.EntityID(doorID), core.Hex{Q: 1, R: 0, S: -1}, false)
	data := enc.ToData()
	door := data.Doors[core.EntityID(doorID)]
	door.Locked = true
	door.LockDC = dc
	door.LockAbility = "DEX"
	s.Require().NoError(s.repo.Save(s.ctx, data))

	// Issue the prompt via Interact (the carved orchestrator door path).
	_, err := s.orch.Interact(s.ctx, &encounterorch.InteractInput{
		EncounterID:    encID,
		PlayerID:       "player-A",
		TargetEntityID: core.EntityID(doorID),
	})
	s.Require().NoError(err)
}

func (s *SubmitCheckSuite) submitCheck(encID, entity string, roll int) (*encounterorch.SubmitCheckOutput, error) {
	return s.orch.SubmitCheck(s.ctx, &encounterorch.SubmitCheckInput{
		EncounterID: encID,
		PlayerID:    "player-A",
		EntityID:    entity,
		Roll:        roll,
	})
}

// --- nil input ---

func (s *SubmitCheckSuite) TestSubmitCheck_NilInput_Error() {
	_, err := s.orch.SubmitCheck(s.ctx, nil)
	s.Require().Error(err)
}

// --- load preconditions ---

func (s *SubmitCheckSuite) TestSubmitCheck_MissingEncounter_ErrEncounterNotFound() {
	_, err := s.submitCheck("nope", "char-A", 10)
	s.Require().ErrorIs(err, encounterorch.ErrEncounterNotFound)
}

func (s *SubmitCheckSuite) TestSubmitCheck_PlayerNotInEncounter_ErrPlayerNotInEncounter() {
	enc := tkenc.New(s.ctx, "enc-no-player", s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "other", EntityID: "other-char", Position: core.Hex{Q: 0, R: 0, S: 0}, SightRange: 4,
	}))
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))

	_, err := s.submitCheck("enc-no-player", "char-A", 10)
	s.Require().ErrorIs(err, encounterorch.ErrPlayerNotInEncounter)
}

func (s *SubmitCheckSuite) TestSubmitCheck_EntityMismatch_ErrEntityOwnershipMismatch() {
	s.seedLockedDoorWithPrompt("enc-mismatch", "door-east", 10)
	_, err := s.submitCheck("enc-mismatch", "char-someone-else", 10)
	s.Require().ErrorIs(err, encounterorch.ErrEntityOwnershipMismatch)
}

// --- no pending prompt ---

func (s *SubmitCheckSuite) TestSubmitCheck_NoPendingPrompt_ErrNoPendingPrompt() {
	enc := tkenc.New(s.ctx, "enc-no-prompt", s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "player-A", EntityID: "char-A", Position: core.Hex{Q: 0, R: 0, S: 0}, SightRange: 4,
	}))
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))

	_, err := s.submitCheck("enc-no-prompt", "char-A", 15)
	s.Require().ErrorIs(err, tkenc.ErrNoPendingPrompt)
}

// --- success path: total >= DC unlocks + opens the door, prompt cleared ---

func (s *SubmitCheckSuite) TestSubmitCheck_Success_OpensDoorPromptCleared() {
	s.seedLockedDoorWithPrompt("enc-success", "door-east", 10)

	// Roll 20 vs DC 10 with zero-modifier stub resolver — success.
	out, err := s.submitCheck("enc-success", "char-A", 20)
	s.Require().NoError(err)
	s.Require().True(out.Success)
	s.Require().Equal(20, out.Total)

	loaded, err := s.repo.Get(s.ctx, "enc-success")
	s.Require().NoError(err)
	door := loaded.Doors[core.EntityID("door-east")]
	s.Require().True(door.Open, "successful unlock opens the door")
	s.Require().False(door.Locked, "successful unlock clears Locked")
	s.Require().Empty(loaded.PendingPrompts, "prompt cleared after success")
}

// --- failure path: total < DC leaves the door locked, prompt cleared ---

func (s *SubmitCheckSuite) TestSubmitCheck_Failure_DoorStaysLockedPromptCleared() {
	s.seedLockedDoorWithPrompt("enc-fail", "door-east", 30)

	// Roll 1 vs DC 30 — failure.
	out, err := s.submitCheck("enc-fail", "char-A", 1)
	s.Require().NoError(err)
	s.Require().False(out.Success)
	s.Require().Equal(1, out.Total)

	loaded, err := s.repo.Get(s.ctx, "enc-fail")
	s.Require().NoError(err)
	door := loaded.Doors[core.EntityID("door-east")]
	s.Require().False(door.Open, "failed unlock leaves door closed")
	s.Require().True(door.Locked, "failed unlock leaves door locked")
	s.Require().Empty(loaded.PendingPrompts, "prompt cleared even on failure")
}

// --- reaction branch: load preconditions + no-prompt sentinel ---
//
// The take_reaction CompleteTakeAction happy path needs a paused two-phase
// attack, which the toolkit cannot resume cross-RPC at v0.17.0 (documented gap:
// ApplyAttackOutcome lacks a held-entity accessor). These tests cover the
// deterministically-reachable reaction surface: load auth + the
// no-pending-reaction-prompt sentinel.

func (s *SubmitCheckSuite) reactionCheck(encID, entity string, take bool) (*encounterorch.SubmitReactionCheckOutput, error) {
	return s.orch.SubmitReactionCheck(s.ctx, &encounterorch.SubmitReactionCheckInput{
		EncounterID: encID,
		PlayerID:    "player-A",
		EntityID:    entity,
		Take:        take,
	})
}

func (s *SubmitCheckSuite) TestSubmitReactionCheck_NilInput_Error() {
	_, err := s.orch.SubmitReactionCheck(s.ctx, nil)
	s.Require().Error(err)
}

func (s *SubmitCheckSuite) TestSubmitReactionCheck_MissingEncounter_ErrEncounterNotFound() {
	_, err := s.reactionCheck("nope", "char-A", true)
	s.Require().ErrorIs(err, encounterorch.ErrEncounterNotFound)
}

func (s *SubmitCheckSuite) TestSubmitReactionCheck_EntityMismatch_ErrEntityOwnershipMismatch() {
	enc := tkenc.New(s.ctx, "enc-react-mismatch", s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "player-A", EntityID: "char-A", Position: core.Hex{Q: 0, R: 0, S: 0}, SightRange: 4,
	}))
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))

	_, err := s.reactionCheck("enc-react-mismatch", "char-someone-else", true)
	s.Require().ErrorIs(err, encounterorch.ErrEntityOwnershipMismatch)
}

func (s *SubmitCheckSuite) TestSubmitReactionCheck_NoPendingPrompt_ErrNoPendingReactionPrompt() {
	enc := tkenc.New(s.ctx, "enc-react-no-prompt", s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "player-A", EntityID: "char-A", Position: core.Hex{Q: 0, R: 0, S: 0}, SightRange: 4,
	}))
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))

	_, err := s.reactionCheck("enc-react-no-prompt", "char-A", true)
	s.Require().ErrorIs(err, encounterorch.ErrNoPendingReactionPrompt)
}

func (s *SubmitCheckSuite) TestSubmitReactionCheck_MissingFuncs_Error() {
	// An orchestrator without the ReactionResume funcs must refuse the reaction
	// branch up front rather than nil-panic.
	orch, err := encounterorch.New(&encounterorch.Config{
		Broker:        s.broker,
		EncounterRepo: s.repo,
		Resolver:      stubCharacterResolver{},
		BuildCombatResolver: func(_ *tkenc.Data) encounterorch.CombatResolver {
			return nil
		},
		BuildMovementResolver: func(_ *tkenc.Data) tkenc.MovementResolver {
			return nil
		},
	})
	s.Require().NoError(err)

	_, err = orch.SubmitReactionCheck(s.ctx, &encounterorch.SubmitReactionCheckInput{
		EncounterID: "enc-x", PlayerID: "player-A", EntityID: "char-A", Take: true,
	})
	s.Require().Error(err)
}

func TestSubmitCheckSuite(t *testing.T) {
	suite.Run(t, new(SubmitCheckSuite))
}
