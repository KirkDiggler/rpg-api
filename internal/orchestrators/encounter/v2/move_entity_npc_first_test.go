package encounter_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	v2encounter "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v2/encounter"
	encounterorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/encounter/v2"
	encountersv2 "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/v2"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
)

// MoveEntity NPC-first combat-entry regression (rpg-api#659): when the active
// actor in a TURN_BASED encounter is an NPC, MoveEntity must drive that NPC's
// turn ITSELF — it must not depend on a later StreamEncounter/GetEncounter
// connect providing the #636 self-heal kick (DriveStalledNPCTurn) to un-stall
// it. Before the fix, MoveEntity persisted right after enc.Move() without
// checking whether the encounter's active actor was an NPC, so the encounter
// would sit there until some client (re)connected — exactly the issue's
// repro ("goblin wins initiative, don't refresh, watch it hang").
//
// rpg-toolkit#864/#867 rebase note: the ORIGINAL version of this test
// constructed the goblin-wins-initiative state via a Move that itself
// triggered checkCombatEntry (zoe walking into the goblin's sight range).
// rpg-toolkit#867 forces the entity whose OWN action triggers a
// FreeRoam->TurnBased transition into initiative slot 0 regardless of its
// roll — so a player's Move can no longer produce "the monster won
// initiative" outcome; the trigger (the mover) is always first by design now.
// That vector is gone for good, not flaky — #867 is an explicit
// creative-director ruling (toolkit#865), not a bug.
//
// The #659 regression this test guards against is still very much
// reachable, just via a DIFFERENT combat-entry vector #867 deliberately
// leaves untouched: AddMonster (a monster becoming visible/added has no
// acting player to privilege into slot 0, so it stays plain roll order,
// ascending-id tiebreak, exactly as before #867). This test's setup now
// constructs the stalled state directly via an AddMonster-triggered entry
// (the goblin is added already within the player's sight range, so
// AddMonster's own inline checkCombatEntry call fires immediately,
// unattributed, and the goblin wins the tie) — then exercises MoveEntity
// with an ORDINARY subsequent move (one that doesn't itself need to trigger
// anything, since combat is already active) to prove MoveEntity's post-Move
// check still drives whatever NPC currently holds the active slot, not just
// one a Move JUST revealed. This preserves the original bug's shape (a
// stalled NPC-first turn, un-stalled without a reconnect) through the one
// vector #867 didn't close off.
//
// A forced-constant dice.Roller (Config.Roller, rpg-api#659) makes every
// rollInitiative d20 call tie, so the sort's ascending-id tiebreak
// deterministically puts the goblin first on every run — no retries, no
// flake.

const (
	mfEncID     = "enc-move-npc-first"
	mfPlayerZoe = "zoe"
	mfEntityZoe = "zed-char-zoe"  // sorts AFTER mfGoblinID on an initiative tie
	mfGoblinID  = "ann-goblin-mf" // sorts BEFORE mfEntityZoe on an initiative tie
)

// constantRoller always returns the same value (capped to the die size), so
// rollInitiative's per-combatant d20 calls tie every time, making the
// ascending-id tiebreak in the toolkit's rollInitiative (combat.go) the sole,
// deterministic decider of turn order — see this file's doc comment.
type constantRoller struct{ value int }

func (c constantRoller) Roll(_ context.Context, size int) (int, error) {
	if size <= 0 {
		return 0, fmt.Errorf("constantRoller: invalid die size %d", size)
	}
	if c.value > size {
		return size, nil
	}
	return c.value, nil
}

func (c constantRoller) RollN(ctx context.Context, count, size int) ([]int, error) {
	out := make([]int, count)
	for i := range out {
		v, err := c.Roll(ctx, size)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

type MoveEntityNPCFirstSuite struct {
	suite.Suite

	ctx    context.Context
	broker *tkenc.Broker
	repo   encountersv2.Repository
	orch   *encounterorch.Orchestrator
}

func (s *MoveEntityNPCFirstSuite) SetupTest() {
	s.ctx = context.Background()
	s.broker = tkenc.NewBroker(tkenc.NewInMemoryTransport())
	s.repo = encountersv2.NewInMemory()

	orch, err := encounterorch.New(&encounterorch.Config{
		Broker:                 s.broker,
		EncounterRepo:          s.repo,
		BuildCharacterResolver: constCharacterResolver(stubCharacterResolver{}),
		// Stand-in combat resolver, same choice as DriveStalledNPCTurnSuite: no
		// DataJSON is seeded on the goblin, so NPCAct takes the scripted
		// single-phase path — this test asserts WHETHER/WHEN the goblin's turn
		// was driven, not its combat outcome.
		BuildCombatResolver: func(_ *tkenc.Data) encounterorch.CombatResolver {
			return v2encounter.NewStandInCombatResolver(nil)
		},
		// nil movement resolver: the toolkit Move takes the legacy single-jump
		// branch (no per-step chain, no OAs) — isolates the combat-entry kick
		// from movement-resolver mechanics, matching MoveEntitySuite's own choice.
		BuildMovementResolver: func(_ *tkenc.Data) tkenc.MovementResolver {
			return nil
		},
		ReactionResume: encounterorch.ReactionResume{
			MarshalAttackContext: func(_ *tkenc.PhasedAttackContext) ([]byte, error) {
				return []byte(`{}`), nil
			},
		},
		Now: func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) },
		// Forces every rollInitiative d20 to tie so the goblin (lower id) wins
		// the ascending-id tiebreak deterministically — see constantRoller.
		Roller: constantRoller{value: 10},
	})
	s.Require().NoError(err)
	s.orch = orch
}

// seedTurnBasedGoblinFirstStalled persists a TURN_BASED encounter where the
// goblin already won initiative and nothing has driven its turn yet — the
// exact stalled state rpg-api#659 guards against, constructed via an
// AddMonster-triggered combat entry (see this file's doc comment for why:
// rpg-toolkit#867 makes a Move-triggered "monster wins initiative" outcome
// impossible by design, but AddMonster-triggered entry stays plain roll
// order/ascending-id tiebreak, unaffected by #867's trigger-attribution
// rule). The goblin is added ALREADY within zoe's sight range, so
// AddMonster's own inline checkCombatEntry call fires immediately as part of
// this seed step, with no acting player to attribute — the goblin's
// constant-rolled tie is broken by ascending id, and it wins.
func (s *MoveEntityNPCFirstSuite) seedTurnBasedGoblinFirstStalled() {
	enc := tkenc.New(s.ctx, core.EncounterID(mfEncID), s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID:   mfPlayerZoe,
		EntityID:   mfEntityZoe,
		Position:   core.Hex{Q: 0, R: 0, S: 0},
		SightRange: 10,
		// HP=50, not a lower value: the goblin's scripted turn runs as part of
		// this test's own driveNPCChain dispatch. A goblin crit-max hit
		// (1d6+2 -> 2d6+2=14 on a nat 20) must never be able to reduce zoe to
		// 0 HP and trigger the toolkit's TPK-end path (checkEncounterEnd,
		// v0.29.2), which would clear Initiative and break this test's
		// core assertion on an unrelated roll of the dice.
		HP:    50,
		MaxHP: 50,
		AC:    14,
	}))
	// Within sight range (hex distance 5 <= sightRange 10) — AddMonster's own
	// inline combat-entry check (no trigger entity, per rpg-toolkit#867's doc
	// comment on AddMonster) fires here, at seed time, plain roll order.
	s.Require().NoError(enc.AddMonster(tkenc.MonsterInput{
		ID:          mfGoblinID,
		Position:    core.Hex{Q: 5, R: 0, S: -5},
		HP:          7,
		MaxHP:       7,
		AC:          15,
		Speed:       6,
		MonsterRef:  "dnd5e:monsters:goblin",
		AttackBonus: 4,
		DamageDice:  "1d6+2",
		DamageType:  "slashing",
	}))
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))
}

// --- the headline fix ---

func (s *MoveEntityNPCFirstSuite) TestMoveEntity_NPCFirstCombatEntry_DrivesGoblinTurnWithoutReconnect() {
	s.seedTurnBasedGoblinFirstStalled()

	preLoad, getErr := s.repo.Get(s.ctx, mfEncID)
	s.Require().NoError(getErr)
	s.Require().Equal(core.ModeTurnBased, preLoad.Mode,
		"test premise: AddMonster's inline combat-entry check must have already flipped to TURN_BASED")
	s.Require().NotEmpty(preLoad.Initiative, "initiative must have rolled")
	s.Require().Equal(core.EntityID(mfGoblinID), preLoad.Initiative[0],
		"sanity check on the test setup: the constant roller's tiebreak must have put the goblin first")
	s.Require().Equal(core.EntityID(mfGoblinID), preLoad.Initiative[preLoad.ActiveIdx],
		"test premise: the goblin's turn is stalled — nothing has driven it yet")

	// zoe takes an ORDINARY move — nothing about this move needs to reveal or
	// trigger anything new (combat is already active from the seed step).
	// Move has no turn-order gate of its own (unlike TakeAction/EndTurn), and
	// zoe carries no hydrated character/economy in this fixture, so this
	// succeeds regardless of whose turn it nominally is. The point being
	// proven: MoveEntity's post-Move check (o.driveNPCChain, keyed on
	// whatever enc.ActiveActor() is AFTER Move, not on whether THIS Move
	// caused a transition) drives the already-stalled goblin turn.
	out, err := s.orch.MoveEntity(s.ctx, &encounterorch.MoveEntityInput{
		EncounterID: mfEncID,
		PlayerID:    mfPlayerZoe,
		EntityID:    mfEntityZoe,
		Path: []core.Hex{
			{Q: 1, R: 0, S: -1},
		},
	})
	s.Require().NoError(err, "the ordinary move must succeed")
	s.Require().NotNil(out)

	// The headline assertion: MoveEntity alone — no DriveStalledNPCTurn call,
	// no StreamEncounter/GetEncounter connect anywhere in this test — must have
	// already driven the goblin's turn to completion and advanced the active
	// actor to zoe. Before the #659 fix, the active actor would still be the
	// goblin here, stalled forever until some client reconnected.
	loaded, getErr := s.repo.Get(s.ctx, mfEncID)
	s.Require().NoError(getErr)
	s.Equal(core.EntityID(mfEntityZoe), loaded.Initiative[loaded.ActiveIdx],
		"MoveEntity itself must drive the goblin's turn — the active actor must already be the player, with zero reconnects")
}

func TestMoveEntityNPCFirstSuite(t *testing.T) {
	suite.Run(t, new(MoveEntityNPCFirstSuite))
}
