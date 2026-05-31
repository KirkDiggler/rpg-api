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

// shieldReactionRef is the canonical readiness-map key the handler assembles
// from the proto Ref for the Shield reaction.
const shieldReactionRef = "dnd5e:conditions:shield"

type SetReactionReadySuite struct {
	suite.Suite

	ctx    context.Context
	broker *tkenc.Broker
	repo   encountersv2.Repository
	orch   *encounterorch.Orchestrator
}

func (s *SetReactionReadySuite) SetupTest() {
	s.ctx = context.Background()
	s.broker = tkenc.NewBroker(tkenc.NewInMemoryTransport())
	s.repo = encountersv2.NewInMemory()

	orch, err := encounterorch.New(&encounterorch.Config{
		Broker:        s.broker,
		EncounterRepo: s.repo,
		Resolver:      stubCharacterResolver{},
		// The readiness toggle never invokes the combat / movement resolvers, but
		// the orchestrator requires the builders to be present.
		BuildCombatResolver: func(_ *tkenc.Data) encounterorch.CombatResolver {
			return nil
		},
		BuildMovementResolver: func(_ *tkenc.Data) tkenc.MovementResolver {
			return nil
		},
		Now: func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) },
	})
	s.Require().NoError(err)
	s.orch = orch
}

// seedPlayer persists an encounter with player-A controlling char-A.
func (s *SetReactionReadySuite) seedPlayer(encID string) {
	enc := tkenc.New(s.ctx, core.EncounterID(encID), s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID:   "player-A",
		EntityID:   "char-A",
		Position:   core.Hex{Q: 0, R: 0, S: 0},
		SightRange: 4,
	}))
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))
}

func (s *SetReactionReadySuite) setReady(encID, entity, ref string, ready bool) (*encounterorch.SetReactionReadyOutput, error) {
	return s.orch.SetReactionReady(s.ctx, &encounterorch.SetReactionReadyInput{
		EncounterID: encID,
		PlayerID:    "player-A",
		EntityID:    core.EntityID(entity),
		ReactionRef: ref,
		Ready:       ready,
	})
}

// --- nil input ---

func (s *SetReactionReadySuite) TestSetReactionReady_NilInput_Error() {
	_, err := s.orch.SetReactionReady(s.ctx, nil)
	s.Require().Error(err)
}

// --- load preconditions ---

func (s *SetReactionReadySuite) TestSetReactionReady_MissingEncounter_ErrEncounterNotFound() {
	_, err := s.setReady("nope", "char-A", shieldReactionRef, true)
	s.Require().ErrorIs(err, encounterorch.ErrEncounterNotFound)
}

func (s *SetReactionReadySuite) TestSetReactionReady_PlayerNotInEncounter_ErrPlayerNotInEncounter() {
	enc := tkenc.New(s.ctx, "enc-no-player", s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "other", EntityID: "other-char", Position: core.Hex{Q: 0, R: 0, S: 0}, SightRange: 4,
	}))
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))

	_, err := s.setReady("enc-no-player", "char-A", shieldReactionRef, true)
	s.Require().ErrorIs(err, encounterorch.ErrPlayerNotInEncounter)
}

func (s *SetReactionReadySuite) TestSetReactionReady_EntityMismatch_ErrEntityOwnershipMismatch() {
	s.seedPlayer("enc-mismatch")
	_, err := s.setReady("enc-mismatch", "char-someone-else", shieldReactionRef, true)
	s.Require().ErrorIs(err, encounterorch.ErrEntityOwnershipMismatch)
}

// --- verb refusal wrapped in the sentinel ---
//
// The toolkit returns plain errors (not sentinels) for its SetReactionReady
// refusals; the orchestrator joins them with ErrSetReactionReadyRefused so the
// handler maps to FailedPrecondition without string matching. The
// entity-not-in-encounter case isn't reachable through the public surface (the
// load ownership check passes only when EntityID is the player's seat, and the
// toolkit re-validates that same charID), so we drive the empty-ref refusal —
// normally pre-validated handler-side — straight at the orchestrator to prove
// the wrapping.
func (s *SetReactionReadySuite) TestSetReactionReady_EmptyRef_ErrSetReactionReadyRefused() {
	s.seedPlayer("enc-empty-ref")
	_, err := s.setReady("enc-empty-ref", "char-A", "", true)
	s.Require().ErrorIs(err, encounterorch.ErrSetReactionReadyRefused)
}

// --- happy path: toggle on, persisted ---

func (s *SetReactionReadySuite) TestSetReactionReady_True_PersistsReadyFlag() {
	s.seedPlayer("enc-ready-on")

	out, err := s.setReady("enc-ready-on", "char-A", shieldReactionRef, true)
	s.Require().NoError(err)
	s.Require().NotNil(out)

	loaded, err := s.repo.Get(s.ctx, "enc-ready-on")
	s.Require().NoError(err)
	s.Require().Contains(loaded.ReactionReadiness, core.EntityID("char-A"),
		"readiness map must carry the entity after a toggle")
	s.Require().True(loaded.ReactionReadiness[core.EntityID("char-A")][shieldReactionRef],
		"ready=true must persist as true")
}

// --- toggle off: clears the flag, persisted ---

func (s *SetReactionReadySuite) TestSetReactionReady_False_PersistsClearedFlag() {
	s.seedPlayer("enc-ready-off")

	// Toggle on, then off — the off write must persist as false.
	_, err := s.setReady("enc-ready-off", "char-A", shieldReactionRef, true)
	s.Require().NoError(err)
	_, err = s.setReady("enc-ready-off", "char-A", shieldReactionRef, false)
	s.Require().NoError(err)

	loaded, err := s.repo.Get(s.ctx, "enc-ready-off")
	s.Require().NoError(err)
	s.Require().False(loaded.ReactionReadiness[core.EntityID("char-A")][shieldReactionRef],
		"ready=false must persist as false")
}

func TestSetReactionReadySuite(t *testing.T) {
	suite.Run(t, new(SetReactionReadySuite))
}
