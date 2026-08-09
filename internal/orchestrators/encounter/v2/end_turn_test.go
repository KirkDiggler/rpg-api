package encounter_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	v2encounter "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v2/encounter"
	encounterorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/encounter/v2"
	encountersv2 "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/v2"
	"github.com/KirkDiggler/rpg-api/internal/testsupport/monsterfixture"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
)

// EndTurn orchestrator tests (#582 step 7 — the final verb carve). These
// exercise the orchestrator's load → EndTurn verb → NPC dispatch loop → persist
// core directly, proving:
//   - the shared load preconditions (membership + entity ownership) classify as
//     the orchestrator sentinels the handler maps to gRPC codes;
//   - toolkit EndTurn gate sentinels (ErrEncounterEnded, etc.) surface UNWRAPPED
//     so the handler maps each distinctly;
//   - the happy path advances initiative and persists;
//   - the NPC dispatch loop cycles past consecutive NPC turns (NPCAct +
//     EndTurn-the-NPC server-side) until a player is active.
//
// ErrNPCChainExhausted is a defensive guard not reachable through this
// player-gated entrypoint: the toolkit's first EndTurn requires the player to be
// the active actor (so the player is always in initiative), and the loop's
// roster-derived cap (len + margin) always re-reaches the player on wrap. It is
// only hit by a corrupt no-player snapshot, so it is documented (see its godoc)
// rather than unit-tested through a fabricated invalid fixture.
//
// The NPC pause-for-reaction handling is covered in two places: the
// prompt-bookkeeping helper branches (enforceSingleReactor + the skip paths of
// serializeNPCPendingReactions) are white-box unit-tested in
// end_turn_pause_test.go, and the full pause path — a genuine SDK pause that
// consults the injected MarshalAttackContext and persists AttackContextJSON — is
// driven end-to-end through Orchestrator.EndTurn in end_turn_npc_pause_test.go
// (rehydratable goblin + fake phased resolver).

const (
	etEncID     = "enc-end-turn"
	etPlayerBob = "bob"
	etEntityBob = "char-bob"
	etGoblinID  = "goblin-et"
)

type EndTurnSuite struct {
	suite.Suite

	ctx    context.Context
	broker *tkenc.Broker
	repo   encountersv2.Repository
	orch   *encounterorch.Orchestrator
}

func (s *EndTurnSuite) SetupTest() {
	s.ctx = context.Background()
	s.broker = tkenc.NewBroker(tkenc.NewInMemoryTransport())
	s.repo = encountersv2.NewInMemory()

	orch, err := encounterorch.New(&encounterorch.Config{
		Broker:                 s.broker,
		EncounterRepo:          s.repo,
		BuildCharacterResolver: constCharacterResolver(stubCharacterResolver{}),
		// Stand-in combat resolver: the NPC dispatch loop's goblin attack runs
		// through this (single-phase ResolveAttack), advancing initiative
		// deterministically without a rulebook. The resolver sources its stats
		// from the AttackInput snapshot (MonsterInput's own AttackBonus/
		// DamageDice fields), not from the rehydrated DataJSON, so it stays
		// single-phase with no reaction triggers regardless — isolating the
		// load → EndTurn → dispatch → persist contract.
		BuildCombatResolver: func(_ *tkenc.Data) encounterorch.CombatResolver {
			return v2encounter.NewStandInCombatResolver(nil)
		},
		BuildMovementResolver: func(_ *tkenc.Data) tkenc.MovementResolver {
			return nil
		},
		// MarshalAttackContext is wired for completeness; the scripted NPC path
		// never pauses, so it is never invoked in these tests.
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

// seedTurnBased persists a turn-based encounter with bob (player) and a goblin
// (monster), forcing the supplied initiative order + active index so the
// active-actor gate passes deterministically.
func (s *EndTurnSuite) seedTurnBased(encID string, initiative []core.EntityID, activeIdx int) {
	enc := tkenc.New(s.ctx, core.EncounterID(encID), s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID:   etPlayerBob,
		EntityID:   etEntityBob,
		Position:   core.Hex{Q: 0, R: 0, S: 0},
		SightRange: 10,
		// HP=50: comfortably above a goblin crit-max hit (1d6+2 -> 2d6+2=14 on
		// a nat 20), so the NPC dispatch loop below can never TPK bob mid-chain
		// (rpg-api#659: the v0.29.2 toolkit bump added TPK detection to
		// checkEncounterEnd, which clears Initiative on ModeEnded — this
		// suite's real-dice StandInCombatResolver occasionally hit that at the
		// old HP=14, flaking the NPC-dispatch assertions below).
		HP:          50,
		MaxHP:       50,
		AC:          14,
		AttackBonus: 5,
		DamageDice:  "1d12+3",
		DamageType:  "slashing",
	}))
	s.Require().NoError(enc.AddMonster(tkenc.MonsterInput{
		ID:          etGoblinID,
		Position:    core.Hex{Q: 1, R: 0, S: -1},
		HP:          7,
		MaxHP:       7,
		AC:          15,
		Speed:       6,
		MonsterRef:  "dnd5e:monsters:goblin",
		AttackBonus: 4,
		DamageDice:  "1d6+2",
		DamageType:  "slashing",
		DataJSON:    monsterfixture.GoblinDataJSON(s.T(), etGoblinID),
	}))
	// AddMonster inline-checks combat entry (rpg-toolkit#759): bob (sight
	// range 10, no room) already sees the goblin at (1,0,-1), so the
	// encounter self-transitions to TURN_BASED here. An explicit SetMode
	// would now be redundant and error ("mode is already TURN_BASED").
	data := enc.ToData()
	data.Initiative = initiative
	data.ActiveIdx = activeIdx
	s.Require().NoError(s.repo.Save(s.ctx, data))
}

func (s *EndTurnSuite) endTurn(encID, entity string) (*encounterorch.EndTurnOutput, error) {
	return s.orch.EndTurn(s.ctx, &encounterorch.EndTurnInput{
		EncounterID: encID,
		PlayerID:    etPlayerBob,
		EntityID:    core.EntityID(entity),
	})
}

// --- nil input ---

func (s *EndTurnSuite) TestEndTurn_NilInput_Error() {
	_, err := s.orch.EndTurn(s.ctx, nil)
	s.Require().Error(err)
}

// --- load preconditions ---

func (s *EndTurnSuite) TestEndTurn_MissingEncounter_ErrEncounterNotFound() {
	_, err := s.endTurn("nope", etEntityBob)
	s.Require().ErrorIs(err, encounterorch.ErrEncounterNotFound)
}

func (s *EndTurnSuite) TestEndTurn_PlayerNotInEncounter_ErrPlayerNotInEncounter() {
	enc := tkenc.New(s.ctx, "enc-no-player", s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "other", EntityID: "other-char", Position: core.Hex{Q: 0, R: 0, S: 0}, SightRange: 4,
	}))
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))

	_, err := s.endTurn("enc-no-player", etEntityBob)
	s.Require().ErrorIs(err, encounterorch.ErrPlayerNotInEncounter)
}

func (s *EndTurnSuite) TestEndTurn_EntityMismatch_ErrEntityOwnershipMismatch() {
	s.seedTurnBased("enc-mismatch", []core.EntityID{etEntityBob, etGoblinID}, 0)
	_, err := s.endTurn("enc-mismatch", "char-someone-else")
	s.Require().ErrorIs(err, encounterorch.ErrEntityOwnershipMismatch)
}

// --- toolkit gate sentinels surface UNWRAPPED so the handler maps distinctly ---

func (s *EndTurnSuite) TestEndTurn_EncounterEnded_ErrEncounterEnded() {
	s.seedTurnBased("enc-ended", []core.EntityID{etEntityBob, etGoblinID}, 0)
	data, err := s.repo.Get(s.ctx, "enc-ended")
	s.Require().NoError(err)
	data.Mode = core.ModeEnded
	s.Require().NoError(s.repo.Save(s.ctx, data))

	_, err = s.endTurn("enc-ended", etEntityBob)
	s.Require().ErrorIs(err, tkenc.ErrEncounterEnded)
}

func (s *EndTurnSuite) TestEndTurn_NotTurnBased_ErrNotTurnBased() {
	// Free-roam encounter: EndTurn gates on ModeTurnBased.
	enc := tkenc.New(s.ctx, "enc-free-roam", s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: etPlayerBob, EntityID: etEntityBob, Position: core.Hex{Q: 0, R: 0, S: 0}, SightRange: 4,
		HP: 14, MaxHP: 14, AC: 14,
	}))
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))

	_, err := s.endTurn("enc-free-roam", etEntityBob)
	s.Require().ErrorIs(err, tkenc.ErrNotTurnBased)
}

func (s *EndTurnSuite) TestEndTurn_NotActiveActor_ErrNotYourTurn() {
	// Goblin is active (idx 1), bob tries to end his turn — not the active actor.
	s.seedTurnBased("enc-not-turn", []core.EntityID{etGoblinID, etEntityBob}, 0)
	_, err := s.endTurn("enc-not-turn", etEntityBob)
	s.Require().ErrorIs(err, tkenc.ErrNotYourTurn)
}

// --- happy path: advances initiative + persists ---
//
// bob is active (idx 0) with the goblin AFTER him in initiative. Ending bob's
// turn advances to the goblin; the NPC dispatch loop runs the goblin's scripted
// turn (StandIn resolver) and ends it, cycling back to bob. The post-state must
// be persisted and the active actor must be a player (bob).
func (s *EndTurnSuite) TestEndTurn_HappyPath_AdvancesAndPersists() {
	s.seedTurnBased(etEncID, []core.EntityID{etEntityBob, etGoblinID}, 0)

	out, err := s.endTurn(etEncID, etEntityBob)
	s.Require().NoError(err, "ending the active player's turn must succeed via the toolkit verb")
	s.Require().NotNil(out)

	loaded, getErr := s.repo.Get(s.ctx, etEncID)
	s.Require().NoError(getErr)
	s.Require().NotNil(loaded)

	postActive := loaded.Initiative[loaded.ActiveIdx]
	// After the NPC dispatch loop the active actor must be the player again
	// (the goblin's turn was run + ended server-side).
	s.Equal(core.EntityID(etEntityBob), postActive,
		"after EndTurn + NPC dispatch loop, active actor must be the player")
}

// --- NPC dispatch loop cycles past a consecutive NPC ---
//
// Two goblins between bob and his next turn. Ending bob's turn must dispatch
// both goblins' scripted turns server-side, cycling initiative all the way back
// to bob without a second EndTurn call.
func (s *EndTurnSuite) TestEndTurn_NPCDispatchLoop_CyclesPastConsecutiveNPCs() {
	const goblin2 = "goblin-et-2"
	enc := tkenc.New(s.ctx, "enc-npc-chain", s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: etPlayerBob, EntityID: etEntityBob, Position: core.Hex{Q: 0, R: 0, S: 0}, SightRange: 10,
		// HP=50, not 14: TWO goblins chain their scripted attacks below; see
		// seedTurnBased's doc comment above for why (rpg-api#659, v0.29.2 TPK
		// detection).
		HP: 50, MaxHP: 50, AC: 14, AttackBonus: 5, DamageDice: "1d12+3", DamageType: "slashing",
	}))
	s.Require().NoError(enc.AddMonster(tkenc.MonsterInput{
		ID: etGoblinID, Position: core.Hex{Q: 1, R: 0, S: -1}, HP: 7, MaxHP: 7, AC: 15, Speed: 6,
		MonsterRef: "dnd5e:monsters:goblin", AttackBonus: 4, DamageDice: "1d6+2", DamageType: "slashing",
		DataJSON: monsterfixture.GoblinDataJSON(s.T(), etGoblinID),
	}))
	s.Require().NoError(enc.AddMonster(tkenc.MonsterInput{
		ID: goblin2, Position: core.Hex{Q: 2, R: 0, S: -2}, HP: 7, MaxHP: 7, AC: 15, Speed: 6,
		MonsterRef: "dnd5e:monsters:goblin", AttackBonus: 4, DamageDice: "1d6+2", DamageType: "slashing",
		DataJSON: monsterfixture.GoblinDataJSON(s.T(), goblin2),
	}))
	// AddMonster inline-checks combat entry (rpg-toolkit#759): the first
	// goblin add already self-transitions to TURN_BASED, so an explicit
	// SetMode here would be redundant and error.
	data := enc.ToData()
	data.Initiative = []core.EntityID{etEntityBob, etGoblinID, goblin2}
	data.ActiveIdx = 0
	s.Require().NoError(s.repo.Save(s.ctx, data))

	_, err := s.endTurn("enc-npc-chain", etEntityBob)
	s.Require().NoError(err, "ending bob's turn must dispatch both goblins server-side")

	loaded, getErr := s.repo.Get(s.ctx, "enc-npc-chain")
	s.Require().NoError(getErr)
	postActive := loaded.Initiative[loaded.ActiveIdx]
	s.Equal(core.EntityID(etEntityBob), postActive,
		"NPC dispatch loop must cycle past both goblins back to the player")
}

func TestEndTurnSuite(t *testing.T) {
	suite.Run(t, new(EndTurnSuite))
}
