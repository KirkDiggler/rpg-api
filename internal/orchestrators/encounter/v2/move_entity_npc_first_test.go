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

// MoveEntity NPC-first combat-entry regression (rpg-api#659): a sighted Move
// that flips FREE_ROAM -> TURN_BASED with a monster winning initiative must
// drive that monster's turn ITSELF — it must not depend on a later
// StreamEncounter/GetEncounter connect providing the #636 self-heal kick
// (DriveStalledNPCTurn) to un-stall it. Before the fix, MoveEntity persisted
// right after enc.Move() without checking whether the mode transition it just
// triggered left an NPC active, so the encounter would sit there until some
// client (re)connected — exactly the issue's repro ("goblin wins initiative,
// don't refresh, watch it hang").
//
// This exercises the REAL trigger path the toolkit uses for combat entry
// (Move -> checkCombatEntry -> SetMode -> rollInitiative), not a hand-seeded
// post-roll Initiative/ActiveIdx (which would bypass the very code path this
// bug lives in). A forced-constant dice.Roller (Config.Roller, rpg-api#659)
// makes every rollInitiative d20 call tie, so the sort's ascending-id
// tiebreak deterministically puts the goblin first on every run — no retries,
// no flake.

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
		Broker:        s.broker,
		EncounterRepo: s.repo,
		Resolver:      stubCharacterResolver{},
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

// seedFreeRoamOutOfSight persists a FREE_ROAM encounter with zoe far enough
// from the goblin (hex distance 20 > sight range 10) that combat has NOT yet
// started at seed time — the encounter only flips to TURN_BASED once zoe's
// own Move closes the distance below her sight range, exercising
// checkCombatEntry for real rather than asserting against hand-seeded state.
func (s *MoveEntityNPCFirstSuite) seedFreeRoamOutOfSight() {
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
	s.Require().NoError(enc.AddMonster(tkenc.MonsterInput{
		ID:          mfGoblinID,
		Position:    core.Hex{Q: 20, R: 0, S: -20},
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
	s.seedFreeRoamOutOfSight()

	// Move zoe to within sight range (hex distance 9 <= sightRange 10) of the
	// goblin. This is the exact repro shape from rpg-api#659: a sighted Move
	// that itself triggers combat entry, with the monster winning initiative.
	out, err := s.orch.MoveEntity(s.ctx, &encounterorch.MoveEntityInput{
		EncounterID: mfEncID,
		PlayerID:    mfPlayerZoe,
		EntityID:    mfEntityZoe,
		Path: []core.Hex{
			{Q: 0, R: 0, S: 0},
			{Q: 11, R: 0, S: -11},
		},
	})
	s.Require().NoError(err, "the sighted move that triggers combat entry must succeed")
	s.Require().NotNil(out)

	loaded, getErr := s.repo.Get(s.ctx, mfEncID)
	s.Require().NoError(getErr)
	s.Require().Equal(core.ModeTurnBased, loaded.Mode, "the move must have triggered combat entry")
	s.Require().NotEmpty(loaded.Initiative, "initiative must have rolled")
	s.Require().Equal(core.EntityID(mfGoblinID), loaded.Initiative[0],
		"sanity check on the test setup: the constant roller's tiebreak must have put the goblin first")

	// The headline assertion: MoveEntity alone — no DriveStalledNPCTurn call,
	// no StreamEncounter/GetEncounter connect anywhere in this test — must have
	// already driven the goblin's turn to completion and advanced the active
	// actor to zoe. Before the #659 fix, the active actor would still be the
	// goblin here, stalled forever until some client reconnected.
	s.Equal(core.EntityID(mfEntityZoe), loaded.Initiative[loaded.ActiveIdx],
		"MoveEntity itself must drive the goblin's turn — the active actor must already be the player, with zero reconnects")
}

func TestMoveEntityNPCFirstSuite(t *testing.T) {
	suite.Run(t, new(MoveEntityNPCFirstSuite))
}
