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

// TakeAction orchestrator tests (#582 step 5). These exercise the orchestrator's
// load → TakeActionPhased verb → persist core directly, proving:
//   - the shared load preconditions (membership + entity ownership) classify as
//     the orchestrator sentinels the handler maps to gRPC codes;
//   - toolkit verb gates (unsupported action, not-your-turn, unknown target,
//     non-turn-based) surface UNWRAPPED so the handler maps each distinctly;
//   - the inline-resolution happy path (no readied reactions) resolves the
//     attack, applies damage to the target, and persists the snapshot.
//
// The phased reaction-pause path (player reactor → pending prompt persisted +
// InputRequiredDelivered published) is covered end-to-end through the handler by
// the Shield + barbarian-rage integration suites; these unit tests use a
// non-phased stand-in CombatResolver so TakeActionPhased takes the inline
// Resolved=true branch, isolating the orchestrator's load/persist contract.

const (
	taEncID     = "enc-take-action"
	taPlayerBob = "bob"
	taEntityBob = "char-bob"
	taGoblinID  = "goblin-ta"
	taAttackRef = "attack"
)

// stubCombatResolver is a non-phased CombatResolver that returns a fixed hit for
// a deterministic damage amount. Because it does NOT implement
// PhasedCombatResolver, TakeActionPhased runs the inline single-phase branch
// (Resolved=true) — no reaction triggers, the cleanest path to prove the
// orchestrator's load → verb → persist contract without a rulebook.
type stubCombatResolver struct {
	damage int
}

func (r stubCombatResolver) ResolveAttack(input tkenc.AttackInput) (*tkenc.AttackOutcome, error) {
	return &tkenc.AttackOutcome{
		Hit:        true,
		AttackRoll: 15,
		TargetAC:   input.TargetAC,
		Damage:     r.damage,
		DamageType: input.AttackerDamageType,
	}, nil
}

type TakeActionSuite struct {
	suite.Suite

	ctx    context.Context
	broker *tkenc.Broker
	repo   encountersv2.Repository
	orch   *encounterorch.Orchestrator
}

func (s *TakeActionSuite) SetupTest() {
	s.ctx = context.Background()
	s.broker = tkenc.NewBroker(tkenc.NewInMemoryTransport())
	s.repo = encountersv2.NewInMemory()

	orch, err := encounterorch.New(&encounterorch.Config{
		Broker:        s.broker,
		EncounterRepo: s.repo,
		Resolver:      stubCharacterResolver{},
		// Stand-in combat resolver: fixed 7-damage hit, non-phased so the inline
		// resolution branch runs (no reaction prompts in these unit tests).
		BuildCombatResolver: func(_ *tkenc.Data) encounterorch.CombatResolver {
			return stubCombatResolver{damage: 7}
		},
		BuildMovementResolver: func(_ *tkenc.Data) tkenc.MovementResolver {
			return nil
		},
		// MarshalAttackContext is required by TakeAction even though the
		// non-phased stand-in never produces a reaction to marshal.
		ReactionResume: encounterorch.ReactionResume{
			MarshalAttackContext: func(_ *tkenc.PhasedAttackContext) ([]byte, error) {
				return []byte(`{}`), nil
			},
		},
		Now: func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) },
	})
	s.Require().NoError(err)
	s.orch = orch
}

// seedCombat persists a turn-based encounter with bob (player, combatant) and a
// goblin (monster), then forces bob to be the active actor by overriding the
// randomly-rolled initiative on the saved snapshot. Deterministic so TakeAction
// always passes the active-actor gate for bob.
func (s *TakeActionSuite) seedCombat(encID string, goblinHP int) {
	enc := tkenc.New(s.ctx, core.EncounterID(encID), s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID:    taPlayerBob,
		EntityID:    taEntityBob,
		Position:    core.Hex{Q: 0, R: 0, S: 0},
		SightRange:  10,
		HP:          14,
		MaxHP:       14,
		AC:          14,
		AttackBonus: 5,
		DamageDice:  "1d12+3",
		DamageType:  "slashing",
	}))
	s.Require().NoError(enc.AddMonster(tkenc.MonsterInput{
		ID:          taGoblinID,
		Position:    core.Hex{Q: 1, R: 0, S: -1},
		HP:          goblinHP,
		MaxHP:       goblinHP,
		AC:          15,
		Speed:       6,
		MonsterRef:  "dnd5e:monsters:goblin",
		AttackBonus: 4,
		DamageDice:  "1d6+2",
		DamageType:  "slashing",
	}))
	s.Require().NoError(enc.SetMode(core.ModeTurnBased))
	data := enc.ToData()
	// Force bob active so the active-actor gate passes deterministically.
	data.Initiative = []core.EntityID{taEntityBob, taGoblinID}
	data.ActiveIdx = 0
	s.Require().NoError(s.repo.Save(s.ctx, data))
}

func (s *TakeActionSuite) takeAttack(encID, actor, target string) (*encounterorch.TakeActionOutput, error) {
	return s.orch.TakeAction(s.ctx, &encounterorch.TakeActionInput{
		EncounterID:    encID,
		PlayerID:       taPlayerBob,
		ActorID:        core.EntityID(actor),
		ActionRef:      tkenc.ActionRef{Module: "dnd5e", Type: "action", ID: taAttackRef},
		TargetEntityID: core.EntityID(target),
	})
}

// --- nil input ---

func (s *TakeActionSuite) TestTakeAction_NilInput_Error() {
	_, err := s.orch.TakeAction(s.ctx, nil)
	s.Require().Error(err)
}

// --- load preconditions ---

func (s *TakeActionSuite) TestTakeAction_MissingEncounter_ErrEncounterNotFound() {
	_, err := s.takeAttack("nope", taEntityBob, taGoblinID)
	s.Require().ErrorIs(err, encounterorch.ErrEncounterNotFound)
}

func (s *TakeActionSuite) TestTakeAction_PlayerNotInEncounter_ErrPlayerNotInEncounter() {
	enc := tkenc.New(s.ctx, "enc-no-player", s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "other", EntityID: "other-char", Position: core.Hex{Q: 0, R: 0, S: 0}, SightRange: 4,
	}))
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))

	_, err := s.takeAttack("enc-no-player", taEntityBob, taGoblinID)
	s.Require().ErrorIs(err, encounterorch.ErrPlayerNotInEncounter)
}

func (s *TakeActionSuite) TestTakeAction_EntityMismatch_ErrEntityOwnershipMismatch() {
	s.seedCombat("enc-mismatch", 100)
	_, err := s.takeAttack("enc-mismatch", "char-someone-else", taGoblinID)
	s.Require().ErrorIs(err, encounterorch.ErrEntityOwnershipMismatch)
}

// --- toolkit gate sentinels surface UNWRAPPED so the handler maps distinctly ---

func (s *TakeActionSuite) TestTakeAction_UnsupportedAction_ErrUnsupportedAction() {
	s.seedCombat("enc-unsupported", 100)
	_, err := s.orch.TakeAction(s.ctx, &encounterorch.TakeActionInput{
		EncounterID:    "enc-unsupported",
		PlayerID:       taPlayerBob,
		ActorID:        taEntityBob,
		ActionRef:      tkenc.ActionRef{Module: "dnd5e", Type: "action", ID: "dodge"},
		TargetEntityID: taGoblinID,
	})
	s.Require().ErrorIs(err, tkenc.ErrUnsupportedAction)
}

func (s *TakeActionSuite) TestTakeAction_UnknownTarget_ErrUnknownTarget() {
	s.seedCombat("enc-unknown-target", 100)
	_, err := s.takeAttack("enc-unknown-target", taEntityBob, "no-such-monster")
	s.Require().ErrorIs(err, tkenc.ErrUnknownTarget)
}

func (s *TakeActionSuite) TestTakeAction_NotTurnBased_ErrNotTurnBased() {
	// Free-roam encounter: TakeActionPhased gates on ModeTurnBased.
	enc := tkenc.New(s.ctx, "enc-free-roam", s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: taPlayerBob, EntityID: taEntityBob, Position: core.Hex{Q: 0, R: 0, S: 0}, SightRange: 4,
		HP: 14, MaxHP: 14, AC: 14, AttackBonus: 5, DamageDice: "1d12+3", DamageType: "slashing",
	}))
	s.Require().NoError(enc.AddMonster(tkenc.MonsterInput{
		ID: taGoblinID, Position: core.Hex{Q: 1, R: 0, S: -1}, HP: 100, MaxHP: 100, AC: 15, Speed: 6,
		MonsterRef: "dnd5e:monsters:goblin", AttackBonus: 4, DamageDice: "1d6+2", DamageType: "slashing",
	}))
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))

	_, err := s.takeAttack("enc-free-roam", taEntityBob, taGoblinID)
	s.Require().ErrorIs(err, tkenc.ErrNotTurnBased)
}

// --- happy path: inline resolution applies damage + persists snapshot ---
//
// Proves the orchestrator's load → TakeActionPhased(inline) → persist contract:
// the stand-in resolver's 7-damage hit lands, the goblin's HP drops 100 → 93,
// and the snapshot is persisted (the next Get sees the reduced HP). No reaction
// prompt is set because the non-phased resolver never triggers one.
func (s *TakeActionSuite) TestTakeAction_InlineResolution_AppliesDamageAndPersists() {
	const goblinStartHP = 100
	s.seedCombat(taEncID, goblinStartHP)

	out, err := s.takeAttack(taEncID, taEntityBob, taGoblinID)
	s.Require().NoError(err, "inline attack resolution must succeed via the toolkit verb")
	s.Require().NotNil(out)

	loaded, getErr := s.repo.Get(s.ctx, taEncID)
	s.Require().NoError(getErr)
	s.Require().NotNil(loaded)

	goblin, ok := loaded.Monsters[taGoblinID]
	s.Require().True(ok, "goblin must still be in the persisted snapshot")
	s.Equal(goblinStartHP-7, goblin.HP, "goblin HP must drop by the resolver's 7 damage and persist")

	// No reaction prompt on the inline (non-phased) path.
	s.Empty(loaded.PendingReactionPrompts, "inline resolution must not persist a pending reaction prompt")
}

func TestTakeActionSuite(t *testing.T) {
	suite.Run(t, new(TakeActionSuite))
}
