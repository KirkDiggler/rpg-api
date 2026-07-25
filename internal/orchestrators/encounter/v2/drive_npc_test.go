package encounter_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	v2encounter "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v2/encounter"
	encounterorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/encounter/v2"
	encountersv2 "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/v2"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
)

// DriveStalledNPCTurn orchestrator tests (rpg-api#636). These prove the
// combat-entry kick — the seam that fixes "TURN_BASED begins with an NPC as
// the FIRST active actor and nothing ever drives it, because the NPC dispatch
// loop only ever ran as a tail of EndTurn":
//   - the active actor being an NPC drives exactly one NPC turn (and persists);
//   - a chain of consecutive NPCs cascades all the way to the first player,
//     exactly like EndTurn's own dispatch loop (driveNPCChain is shared code);
//   - the active actor already being a player is a true no-op (Kicked=false,
//     no NPCAct dispatched);
//   - concurrent kicks against the SAME encounter single-flight: only ONE of N
//     concurrent callers actually drives the NPC turn, proving the keyed
//     mutex — not the toolkit's own in-memory ActiveActor guard, which cannot
//     protect across independently loaded snapshots — prevents the same NPC
//     turn from being dispatched more than once.

const (
	dnEncID     = "enc-drive-npc"
	dnPlayerBob = "bob"
	dnEntityBob = "char-bob-dn"
	dnGoblinID  = "goblin-dn"
	dnGoblin2ID = "goblin-dn-2"
)

type DriveStalledNPCTurnSuite struct {
	suite.Suite

	ctx    context.Context
	broker *tkenc.Broker
	repo   encountersv2.Repository
	orch   *encounterorch.Orchestrator
}

func (s *DriveStalledNPCTurnSuite) SetupTest() {
	s.ctx = context.Background()
	s.broker = tkenc.NewBroker(tkenc.NewInMemoryTransport())
	s.repo = encountersv2.NewInMemory()

	orch, err := encounterorch.New(&encounterorch.Config{
		Broker:                 s.broker,
		EncounterRepo:          s.repo,
		BuildCharacterResolver: constCharacterResolver(stubCharacterResolver{}),
		// Stand-in combat resolver, same choice as EndTurnSuite: no DataJSON is
		// seeded on the goblins, so NPCAct takes the scripted single-phase path —
		// isolating the load -> drive -> persist contract from rulebook combat
		// math (a real d20 roll decides hit/miss, but every assertion here is
		// about WHETHER/HOW OFTEN a turn was dispatched, not its outcome).
		BuildCombatResolver: func(_ *tkenc.Data) encounterorch.CombatResolver {
			return v2encounter.NewStandInCombatResolver(nil)
		},
		BuildMovementResolver: func(_ *tkenc.Data) tkenc.MovementResolver {
			return nil
		},
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

// seedTurnBased persists a turn-based encounter with bob (player) and the
// given monster IDs, forcing the initiative order + active index so the
// active-actor state is deterministic regardless of the toolkit's own
// initiative roll. Mirrors EndTurnSuite.seedTurnBased in end_turn_test.go
// (duplicated rather than shared across suite types for a ~30-line helper —
// same call this package already made).
func (s *DriveStalledNPCTurnSuite) seedTurnBased(encID string, monsterIDs []core.EntityID, initiative []core.EntityID, activeIdx int) {
	enc := tkenc.New(s.ctx, core.EncounterID(encID), s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID:   dnPlayerBob,
		EntityID:   dnEntityBob,
		Position:   core.Hex{Q: 0, R: 0, S: 0},
		SightRange: 10,
		// HP=50: comfortably above any realistic damage a chained goblin
		// NPCAct dispatch can deal here (rpg-api#659 toolkit bump to
		// v0.29.2 added TPK detection to checkEncounterEnd, which clears
		// Initiative on transition to ModeEnded — a real crit-max 1d6+2
		// goblin hit is 2d6+2=14, exactly lethal at the old HP=14 and
		// occasionally flaked this suite's real-dice StandInCombatResolver
		// into a TPK mid-chain). This margin makes every test below
		// structurally immune to that outcome regardless of roll luck, for
		// up to several chained goblins.
		HP:          50,
		MaxHP:       50,
		AC:          14,
		AttackBonus: 5,
		DamageDice:  "1d12+3",
		DamageType:  "slashing",
	}))
	for i, mid := range monsterIDs {
		s.Require().NoError(enc.AddMonster(tkenc.MonsterInput{
			ID:          mid,
			Position:    core.Hex{Q: i + 1, R: 0, S: -(i + 1)},
			HP:          7,
			MaxHP:       7,
			AC:          15,
			Speed:       6,
			MonsterRef:  "dnd5e:monsters:goblin",
			AttackBonus: 4,
			DamageDice:  "1d6+2",
			DamageType:  "slashing",
		}))
	}
	// AddMonster inline-checks combat entry (rpg-toolkit#759): bob (sight
	// range 10, no room) already sees the first monster added, so the
	// encounter self-transitions to TURN_BASED here. An explicit SetMode
	// would now be redundant and error ("mode is already TURN_BASED").
	data := enc.ToData()
	data.Initiative = initiative
	data.ActiveIdx = activeIdx
	s.Require().NoError(s.repo.Save(s.ctx, data))
}

func (s *DriveStalledNPCTurnSuite) kick(encID string) (*encounterorch.DriveStalledNPCTurnOutput, error) {
	return s.orch.DriveStalledNPCTurn(s.ctx, &encounterorch.DriveStalledNPCTurnInput{
		EncounterID: encID,
		PlayerID:    dnPlayerBob,
	})
}

// --- nil input ---

func (s *DriveStalledNPCTurnSuite) TestDriveStalledNPCTurn_NilInput_Error() {
	_, err := s.orch.DriveStalledNPCTurn(s.ctx, nil)
	s.Require().Error(err)
}

// --- load preconditions surface the same sentinels as every other verb ---

func (s *DriveStalledNPCTurnSuite) TestDriveStalledNPCTurn_MissingEncounter_ErrEncounterNotFound() {
	_, err := s.kick("nope")
	s.Require().ErrorIs(err, encounterorch.ErrEncounterNotFound)
}

func (s *DriveStalledNPCTurnSuite) TestDriveStalledNPCTurn_PlayerNotInEncounter_ErrPlayerNotInEncounter() {
	enc := tkenc.New(s.ctx, "enc-dn-no-player", s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "other", EntityID: "other-char", Position: core.Hex{Q: 0, R: 0, S: 0}, SightRange: 4,
	}))
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))

	_, err := s.kick("enc-dn-no-player")
	s.Require().ErrorIs(err, encounterorch.ErrPlayerNotInEncounter)
}

// --- no-op cases: nothing to drive, no persist implied ---

func (s *DriveStalledNPCTurnSuite) TestDriveStalledNPCTurn_PlayerAlreadyActive_NoOp() {
	s.seedTurnBased(dnEncID, []core.EntityID{dnGoblinID}, []core.EntityID{dnEntityBob, dnGoblinID}, 0)

	out, err := s.kick(dnEncID)
	s.Require().NoError(err)
	s.Require().NotNil(out)
	s.False(out.Kicked, "the active actor is already a player — there is nothing to drive")

	loaded, getErr := s.repo.Get(s.ctx, dnEncID)
	s.Require().NoError(getErr)
	s.Equal(core.EntityID(dnEntityBob), loaded.Initiative[loaded.ActiveIdx],
		"a no-op kick must not advance the turn")
}

func (s *DriveStalledNPCTurnSuite) TestDriveStalledNPCTurn_NotTurnBased_NoOp() {
	enc := tkenc.New(s.ctx, "enc-dn-free-roam", s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: dnPlayerBob, EntityID: dnEntityBob, Position: core.Hex{Q: 0, R: 0, S: 0}, SightRange: 4,
		HP: 14, MaxHP: 14, AC: 14,
	}))
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))

	out, err := s.kick("enc-dn-free-roam")
	s.Require().NoError(err)
	s.False(out.Kicked, "a free-roam encounter has no active actor to drive")
}

// --- the headline fix: NPC-first drives exactly once, no preceding EndTurn ---

func (s *DriveStalledNPCTurnSuite) TestDriveStalledNPCTurn_NPCFirst_DrivesToPlayer() {
	// Goblin active at idx 0 with NO preceding EndTurn call — this is exactly
	// the rpg-api#636 repro: devcombat.Inject (or any future in-process
	// combat-entry trigger) rolled the goblin first, and nothing has ended a
	// turn yet, so EndTurn's own dispatch loop was never reached.
	s.seedTurnBased(dnEncID, []core.EntityID{dnGoblinID}, []core.EntityID{dnGoblinID, dnEntityBob}, 0)

	out, err := s.kick(dnEncID)
	s.Require().NoError(err, "kicking a stalled NPC-first encounter must succeed")
	s.Require().NotNil(out)
	s.True(out.Kicked, "the goblin's turn must have been driven")

	loaded, getErr := s.repo.Get(s.ctx, dnEncID)
	s.Require().NoError(getErr)
	s.Equal(core.EntityID(dnEntityBob), loaded.Initiative[loaded.ActiveIdx],
		"after the kick, the active actor must be the player — the goblin's turn ran and ended server-side")
}

// --- chained NPC-first+NPC-second cascades to the first player ---

func (s *DriveStalledNPCTurnSuite) TestDriveStalledNPCTurn_ChainedNPCFirst_CascadesToFirstPlayer() {
	s.seedTurnBased(dnEncID, []core.EntityID{dnGoblinID, dnGoblin2ID},
		[]core.EntityID{dnGoblinID, dnGoblin2ID, dnEntityBob}, 0)

	out, err := s.kick(dnEncID)
	s.Require().NoError(err, "kicking a stalled chained-NPC-first encounter must succeed")
	s.True(out.Kicked)

	loaded, getErr := s.repo.Get(s.ctx, dnEncID)
	s.Require().NoError(getErr)
	s.Equal(core.EntityID(dnEntityBob), loaded.Initiative[loaded.ActiveIdx],
		"the dispatch loop must cascade past BOTH goblins to the player, exactly like EndTurn's own chain")
}

// --- single-flight: concurrent kicks against the same encounter must not
// double-dispatch the same NPC turn ---
//
// N goroutines call DriveStalledNPCTurn concurrently against the SAME
// NPC-first encounter. Without the per-encounter keyed mutex, each would
// independently load its own copy of the NPC-active snapshot (the toolkit's
// NPCAct "am I still the active actor" guard only protects the in-memory copy
// the CALLER loaded, not across separately loaded copies) and all would
// dispatch the goblin's turn — resolving the same attack multiple times. With
// the lock, only the first caller to acquire it sees the NPC active; every
// later caller (once unblocked) re-loads the ALREADY-persisted post-drive
// state and finds a player active, so it no-ops. Exactly one Kicked=true
// among N concurrent callers is the deterministic (roll-independent) proof.
func (s *DriveStalledNPCTurnSuite) TestDriveStalledNPCTurn_ConcurrentKicks_SingleFlight() {
	const n = 8
	s.seedTurnBased(dnEncID, []core.EntityID{dnGoblinID}, []core.EntityID{dnGoblinID, dnEntityBob}, 0)

	var wg sync.WaitGroup
	results := make([]*encounterorch.DriveStalledNPCTurnOutput, n)
	errs := make([]error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			out, err := s.kick(dnEncID)
			results[idx] = out
			errs[idx] = err
		}(i)
	}
	wg.Wait()

	kickedCount := 0
	for i := 0; i < n; i++ {
		s.Require().NoError(errs[i], "no concurrent caller should error")
		s.Require().NotNil(results[i])
		if results[i].Kicked {
			kickedCount++
		}
	}
	s.Equal(1, kickedCount, "exactly one of the concurrent callers must have driven the NPC turn — the rest must find it already advanced")

	loaded, getErr := s.repo.Get(s.ctx, dnEncID)
	s.Require().NoError(getErr)
	s.Equal(core.EntityID(dnEntityBob), loaded.Initiative[loaded.ActiveIdx],
		"after all concurrent kicks settle, the active actor must be the player")
}

func TestDriveStalledNPCTurnSuite(t *testing.T) {
	suite.Run(t, new(DriveStalledNPCTurnSuite))
}
