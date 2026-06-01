package encounter

// White-box tests (package encounter) for the EndTurn NPC pause-for-reaction
// handling helpers: enforceSingleReactor + serializeNPCPendingReactions.
//
// These are white-box (same package) because the helpers are unexported
// orchestrator methods and because the pending PHASED attack context they read
// (enc.PendingPhasedAttackContext) can only be populated by the SDK's unexported
// cachePhasedAttackContext — set from inside NPCAct on a genuine reaction pause,
// which needs full rulebook monster DataJSON rehydration. So these tests cover
// the orchestrator-owned logic reachable without that machinery:
//   - enforceSingleReactor drops all-but-one prompt (the #538 C single-reactor
//     enforcement) — fully exercisable via the public SetPendingReactionPrompt;
//   - serializeNPCPendingReactions's skip branches: it skips prompts that are
//     already serialized (player-attack path filled them in) or carry no cached
//     phased context — neither consults the injected marshal seam.
//
// The consult-the-marshal branch (a genuine SDK pause that caches a phased
// context, then the injected marshal fills AttackContextJSON) is driven
// end-to-end through Orchestrator.EndTurn in end_turn_npc_pause_test.go.
//
// The end-to-end NPC-attack reaction path (a genuine SDK pause populating the
// phased context, then the injected marshal filling AttackContextJSON) is
// exercised through the handler by the Shield integration suite.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	encountersv2 "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/v2"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
)

// pauseStubResolver is a zero-modifier CharacterResolver for orchestrator
// construction in these helper tests (no skill checks run).
type pauseStubCharResolver struct{}

func (pauseStubCharResolver) AbilityModifier(_ core.PlayerID, _ string) (int, bool) { return 0, true }
func (pauseStubCharResolver) ToolProficiencyBonus(_ core.PlayerID, _ string) (int, bool) {
	return 0, true
}

type EndTurnPauseSuite struct {
	suite.Suite

	ctx    context.Context
	broker *tkenc.Broker
}

func (s *EndTurnPauseSuite) SetupTest() {
	s.ctx = context.Background()
	s.broker = tkenc.NewBroker(tkenc.NewInMemoryTransport())
}

// newOrch builds an Orchestrator with the supplied ReactionResume. Construction
// is in-package so the tests can call the unexported helpers directly.
func (s *EndTurnPauseSuite) newOrch(rr ReactionResume) *Orchestrator {
	o, err := New(&Config{
		Broker:        s.broker,
		EncounterRepo: encountersv2.NewInMemory(),
		Resolver:      pauseStubCharResolver{},
		BuildCombatResolver: func(_ *tkenc.Data) CombatResolver {
			return nil
		},
		BuildMovementResolver: func(_ *tkenc.Data) tkenc.MovementResolver {
			return nil
		},
		ReactionResume: rr,
	})
	s.Require().NoError(err)
	return o
}

// encWithPrompts builds a fresh encounter with the named players and sets a
// pending reaction prompt (with the supplied pre-filled AttackContextJSON) for
// each. Used to drive the prompt-bookkeeping helpers.
func (s *EndTurnPauseSuite) encWithPrompts(prompts map[core.PlayerID][]byte) *tkenc.Encounter {
	enc := tkenc.New(s.ctx, "enc-pause", s.broker)
	for pid, ctxJSON := range prompts {
		entityID := core.EntityID("char-" + string(pid))
		s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
			PlayerID: pid, EntityID: entityID, Position: core.Hex{Q: 0, R: 0, S: 0}, SightRange: 10,
			HP: 10, MaxHP: 10, AC: 12,
		}))
		enc.SetPendingReactionPrompt(pid, &tkenc.PendingReactionPrompt{
			ReactorEntityID:   entityID,
			ConditionRef:      "dnd5e:spells:shield",
			TriggerKind:       "post_hit",
			SourceEntity:      "goblin-x",
			AttackContextJSON: ctxJSON,
		})
	}
	return enc
}

// --- enforceSingleReactor ---

func (s *EndTurnPauseSuite) TestEnforceSingleReactor_DropsAllButOne() {
	o := s.newOrch(ReactionResume{})
	enc := s.encWithPrompts(map[core.PlayerID][]byte{
		"amy": []byte(`{"a":1}`),
		"bob": []byte(`{"b":2}`),
		"cat": []byte(`{"c":3}`),
	})

	o.enforceSingleReactor(enc)

	prompts := enc.ToData().PendingReactionPrompts
	s.Require().Len(prompts, 1, "single-reactor enforcement must drop all but one prompt")
}

func (s *EndTurnPauseSuite) TestEnforceSingleReactor_SinglePrompt_NoOp() {
	o := s.newOrch(ReactionResume{})
	enc := s.encWithPrompts(map[core.PlayerID][]byte{
		"amy": []byte(`{"a":1}`),
	})

	o.enforceSingleReactor(enc)

	prompts := enc.ToData().PendingReactionPrompts
	s.Require().Len(prompts, 1, "a single prompt must be left untouched")
	s.Require().Contains(prompts, core.PlayerID("amy"))
}

func (s *EndTurnPauseSuite) TestEnforceSingleReactor_NoPrompts_NoOp() {
	o := s.newOrch(ReactionResume{})
	enc := tkenc.New(s.ctx, "enc-empty", s.broker)

	o.enforceSingleReactor(enc) // must not panic

	s.Require().Empty(enc.ToData().PendingReactionPrompts)
}

// --- serializeNPCPendingReactions ---

// A prompt that already carries AttackContextJSON (the player-attack path filled
// it in) is skipped — the marshal seam is never consulted, so even a nil marshal
// is fine.
func (s *EndTurnPauseSuite) TestSerializeNPCPendingReactions_AlreadySerialized_Skips() {
	o := s.newOrch(ReactionResume{MarshalAttackContext: nil})
	enc := s.encWithPrompts(map[core.PlayerID][]byte{
		"amy": []byte(`{"already":"serialized"}`),
	})

	err := o.serializeNPCPendingReactions(enc)
	s.Require().NoError(err, "an already-serialized prompt must be skipped without consulting the marshal seam")

	s.Require().Equal([]byte(`{"already":"serialized"}`),
		enc.ToData().PendingReactionPrompts["amy"].AttackContextJSON,
		"the pre-filled AttackContextJSON must be left untouched")
}

// A prompt with empty JSON but NO cached phased context (PendingPhasedAttackContext
// returns nil) is skipped — there is nothing to marshal. Proves the helper does
// not error on a prompt the SDK pause did not cache a context for.
func (s *EndTurnPauseSuite) TestSerializeNPCPendingReactions_NoCachedContext_Skips() {
	marshalCalled := false
	o := s.newOrch(ReactionResume{
		MarshalAttackContext: func(_ *tkenc.PhasedAttackContext) ([]byte, error) {
			marshalCalled = true
			return []byte(`{}`), nil
		},
	})
	enc := s.encWithPrompts(map[core.PlayerID][]byte{
		"amy": nil, // empty JSON, but no cached phased context exists for amy
	})

	err := o.serializeNPCPendingReactions(enc)
	s.Require().NoError(err, "a prompt with no cached phased context must be skipped")
	s.Require().False(marshalCalled,
		"the injected marshal must not be consulted when there is no cached phased context")
	s.Require().Empty(enc.ToData().PendingReactionPrompts["amy"].AttackContextJSON,
		"the prompt's AttackContextJSON must stay empty when there is nothing to marshal")
}

func TestEndTurnPauseSuite(t *testing.T) {
	suite.Run(t, new(EndTurnPauseSuite))
}
