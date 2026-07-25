package encounter_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	encounterorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/encounter/v2"
	encountersv2 "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/v2"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster"
)

// NPC-pause-persists-prompt test (#582 step 7). This drives a GENUINE SDK NPC
// reaction pause through Orchestrator.EndTurn and asserts the orchestrator's
// pause branch ran end-to-end:
//   - enforceSingleReactor + serializeNPCPendingReactions fire;
//   - the injected ReactionResume.MarshalAttackContext is consulted to fill the
//     persisted prompt's AttackContextJSON (the branch Copilot flagged as
//     uncovered);
//   - EndTurn returns success (the player must respond before the dispatch loop
//     continues) and the prompt is persisted on the saved snapshot.
//
// The pause is real: a rehydratable goblin (monster.NewGoblin().ToData()) takes
// its turn through the rulebook, publishes an AttackEvent against the adjacent
// player, and the wired PHASED combat resolver returns a player-reactor trigger
// from ResolveAttackHit — which the SDK partitions to the player, persists as a
// pending prompt, and signals via errNPCPausedForReaction. The fake resolver
// bypasses all combat math (it just returns the trigger), and the injected
// marshal is a stub returning known bytes, so this stays rulebook-light while
// exercising the orchestrator's real pause-handling path.

const (
	npEncID     = "enc-npc-pause"
	npPlayerWiz = "wendy"
	npEntityWiz = "char-wendy"
	npGoblinID  = "goblin-np"
	npShieldRef = "dnd5e:spells:shield"
)

// fakePhasedResolver implements tkenc.PhasedCombatResolver. ResolveAttackHit
// returns a single player-reactor trigger (so the SDK pauses the NPC turn);
// ApplyAttackOutcome is never reached on the pause path. ResolveAttack (the
// single-phase fallback) is present to satisfy the embedded CombatResolver but
// is not used because the SDK routes NPC attacks through the phased path when
// the resolver implements PhasedCombatResolver.
type fakePhasedResolver struct {
	reactorEntityID core.EntityID
}

func (r fakePhasedResolver) ResolveAttack(input tkenc.AttackInput) (*tkenc.AttackOutcome, error) {
	return &tkenc.AttackOutcome{Hit: false, TargetAC: input.TargetAC}, nil
}

func (r fakePhasedResolver) ResolveAttackHit(input tkenc.AttackInput) (*tkenc.PhasedAttackContext, []tkenc.ReactionTrigger, error) {
	ctx := &tkenc.PhasedAttackContext{
		// Rulebook is opaque to the SDK; the orchestrator never inspects it —
		// only the injected MarshalAttackContext does, and our stub ignores it.
		Rulebook:   map[string]any{"fake": "attack-context"},
		AttackerID: input.AttackerID,
		TargetID:   input.TargetID,
	}
	triggers := []tkenc.ReactionTrigger{{
		ReactorID:    r.reactorEntityID,
		ConditionRef: npShieldRef,
		TriggerKind:  "post_hit",
		SourceEntity: input.AttackerID,
	}}
	return ctx, triggers, nil
}

func (r fakePhasedResolver) ApplyAttackOutcome(_ *tkenc.PhasedAttackContext, _ []tkenc.ReactionModifier) (*tkenc.AttackOutcome, error) {
	return &tkenc.AttackOutcome{Hit: false}, nil
}

type EndTurnNPCPauseSuite struct {
	suite.Suite

	ctx          context.Context
	broker       *tkenc.Broker
	repo         encountersv2.Repository
	orch         *encounterorch.Orchestrator
	marshalCalls int
}

func (s *EndTurnNPCPauseSuite) SetupTest() {
	s.ctx = context.Background()
	s.broker = tkenc.NewBroker(tkenc.NewInMemoryTransport())
	s.repo = encountersv2.NewInMemory()
	s.marshalCalls = 0

	orch, err := encounterorch.New(&encounterorch.Config{
		Broker:                 s.broker,
		EncounterRepo:          s.repo,
		BuildCharacterResolver: constCharacterResolver(stubCharacterResolver{}),
		BuildCombatResolver: func(_ *tkenc.Data) encounterorch.CombatResolver {
			return fakePhasedResolver{reactorEntityID: npEntityWiz}
		},
		BuildMovementResolver: func(_ *tkenc.Data) tkenc.MovementResolver {
			return nil
		},
		ReactionResume: encounterorch.ReactionResume{
			// Stub marshal: records that the orchestrator consulted the injected
			// seam on the pause path and returns known bytes so the test can assert
			// they landed on the persisted prompt. The real adapter type-asserts the
			// rulebook *combat.AttackContext; this stub ignores the opaque payload.
			MarshalAttackContext: func(_ *tkenc.PhasedAttackContext) ([]byte, error) {
				s.marshalCalls++
				return []byte(`{"npc":"marshaled"}`), nil
			},
		},
		Now: func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) },
	})
	s.Require().NoError(err)
	s.orch = orch
}

// seedGoblinAdjacentToWizard persists a turn-based encounter with wendy (player)
// and a rehydratable goblin positioned adjacent to her, with the goblin active.
// Ending wendy's turn would advance to the goblin, but the seed makes the goblin
// active and wendy second, so the FIRST toolkit EndTurn (ending wendy's prior
// turn) is not what we call — instead wendy ends her own turn (she is active),
// the loop reaches the goblin, and the goblin's scimitar attack against adjacent
// wendy pauses for her Shield reaction via the fake phased resolver.
func (s *EndTurnNPCPauseSuite) seedGoblinAdjacentToWizard() {
	enc := tkenc.New(s.ctx, npEncID, s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID:   npPlayerWiz,
		EntityID:   npEntityWiz,
		Position:   core.Hex{Q: 0, R: 0, S: 0},
		SightRange: 10,
		HP:         12,
		MaxHP:      12,
		AC:         12,
		// Combat fields so wendy is a valid combatant (EndTurn / NPC targeting).
		AttackBonus: 2,
		DamageDice:  "1d4",
		DamageType:  "bludgeoning",
	}))
	// Rehydratable goblin adjacent to wendy (axial distance 1) so its scimitar
	// (5ft reach) can attack without needing a long move. DataJSON makes NPCAct
	// take the rulebook TakeTurn path (not the scripted single-phase fallback),
	// so the attack routes through the wired PHASED resolver.
	goblinData := monster.NewGoblin(npGoblinID).ToData()
	rawData, err := json.Marshal(goblinData)
	s.Require().NoError(err)
	s.Require().NoError(enc.AddMonster(tkenc.MonsterInput{
		ID:          npGoblinID,
		Position:    core.Hex{Q: 1, R: 0, S: -1},
		HP:          7,
		MaxHP:       7,
		AC:          15,
		Speed:       6,
		MonsterRef:  "dnd5e:monsters:goblin",
		AttackBonus: 4,
		DamageDice:  "1d6+2",
		DamageType:  "slashing",
		DataJSON:    rawData,
	}))
	// AddMonster inline-checks combat entry (rpg-toolkit#759): wendy (sight
	// range 10, no room) already sees the goblin at (1,0,-1), so the
	// encounter self-transitions to TURN_BASED here. An explicit SetMode
	// would now be redundant and error ("mode is already TURN_BASED").
	data := enc.ToData()
	// wendy active (idx 0), goblin next (idx 1): ending wendy's turn advances to
	// the goblin and the NPC dispatch loop runs the goblin's turn.
	data.Initiative = []core.EntityID{npEntityWiz, npGoblinID}
	data.ActiveIdx = 0
	s.Require().NoError(s.repo.Save(s.ctx, data))
}

func (s *EndTurnNPCPauseSuite) TestEndTurn_NPCAttackPauses_PersistsPromptWithMarshaledContext() {
	s.seedGoblinAdjacentToWizard()

	out, err := s.orch.EndTurn(s.ctx, &encounterorch.EndTurnInput{
		EncounterID: npEncID,
		PlayerID:    npPlayerWiz,
		EntityID:    npEntityWiz,
	})
	s.Require().NoError(err, "EndTurn must succeed (return) when the NPC turn pauses for a player reaction")
	s.Require().NotNil(out)

	// The injected marshal seam must have been consulted on the pause path —
	// this is the branch Copilot flagged as previously uncovered.
	s.Require().Positive(s.marshalCalls,
		"the orchestrator must consult the injected MarshalAttackContext on the NPC pause path")

	loaded, getErr := s.repo.Get(s.ctx, npEncID)
	s.Require().NoError(getErr)
	prompt, ok := loaded.PendingReactionPrompts[core.PlayerID(npPlayerWiz)]
	s.Require().True(ok, "wendy's pending reaction prompt must be persisted on the snapshot")
	s.Require().NotNil(prompt)
	s.Equal([]byte(`{"npc":"marshaled"}`), prompt.AttackContextJSON,
		"the prompt's AttackContextJSON must carry the bytes from the injected marshal so SubmitReactionCheck can rebuild phase 2")
	s.Equal(npShieldRef, prompt.ConditionRef, "the persisted prompt must carry the reaction's condition ref")
}

func TestEndTurnNPCPauseSuite(t *testing.T) {
	suite.Run(t, new(EndTurnNPCPauseSuite))
}
