package encounter_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/suite"

	v2encounter "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v2/encounter"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	encountercore "github.com/KirkDiggler/rpg-toolkit/encounter/core"
	"github.com/KirkDiggler/rpg-toolkit/events"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/combat"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/conditions"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
)

// Dnd5eCombatResolverTestSuite tests the Dnd5eCombatResolver — the real
// rulebook-backed CombatResolver.
//
// #689: the resolver reads the SDK-held attacker/defender from
// AttackInput.Attacker/Defender and MUST NOT re-load from a character store. So
// these tests hydrate the held entities themselves (mirroring what the SDK's
// LoadFromData cascade does) and pass them on the AttackInput. There is no
// character-repo mock anymore — the resolver never touches it on the attack path.
//
// They verify:
//   - Held attacker + held defender → the real combat.ResolveAttack chain runs
//     and the outcome translates (AttackRoll in [1,20], TargetAC reflected).
//   - A nil held attacker → fall back to StandInCombatResolver (stat snapshots).
//   - A nil held defender but held attacker → the attacker's chain still runs
//     against a snapshot defender stub (no all-or-nothing stand-in degrade).
type Dnd5eCombatResolverTestSuite struct {
	suite.Suite
	ctx context.Context
}

func TestDnd5eCombatResolverTestSuite(t *testing.T) {
	suite.Run(t, new(Dnd5eCombatResolverTestSuite))
}

func (s *Dnd5eCombatResolverTestSuite) SetupTest() {
	s.ctx = context.Background()
}

// ---------------------------------------------------------------------------
// Fallback behavior — no held attacker
// ---------------------------------------------------------------------------

// TestResolveAttack_NilEncounterData_FallsBackToStandIn verifies that with no
// held entities (e.g. CreateEncounter path), the resolver falls through to
// StandInCombatResolver and still returns a valid AttackOutcome.
func (s *Dnd5eCombatResolverTestSuite) TestResolveAttack_NilEncounterData_FallsBackToStandIn() {
	resolver := v2encounter.NewDnd5eCombatResolverForData(
		v2encounter.Dnd5eCombatResolverConfig{},
		nil, // nil data — stand-in path
	)

	outcome, err := resolver.ResolveAttack(tkenc.AttackInput{
		AttackerID:          "char-alice",
		TargetID:            "goblin-1",
		AttackerAttackBonus: 4,
		AttackerDamageDice:  "1d6+2",
		AttackerDamageType:  "slashing",
		TargetAC:            10,
		// No held Attacker/Defender → stand-in.
	})

	s.Require().NoError(err)
	s.Require().NotNil(outcome, "no held entities must still return a valid outcome via stand-in")
}

// TestResolveAttack_NilHeldAttacker_FallsBackToStandIn verifies that a nil held
// attacker degrades to stand-in math (the attacker's chain cannot run without
// its hydrated conditions).
func (s *Dnd5eCombatResolverTestSuite) TestResolveAttack_NilHeldAttacker_FallsBackToStandIn() {
	data := s.buildEncounterData("char-alice", "goblin-1")

	resolver := v2encounter.NewDnd5eCombatResolverForData(
		v2encounter.Dnd5eCombatResolverConfig{},
		data,
	)

	outcome, err := resolver.ResolveAttack(tkenc.AttackInput{
		AttackerID:          "char-alice",
		TargetID:            "goblin-1",
		AttackerAttackBonus: 4,
		AttackerDamageDice:  "1d6+2",
		AttackerDamageType:  "slashing",
		TargetAC:            15,
		// Attacker nil → stand-in.
	})

	s.Require().NoError(err)
	s.Require().NotNil(outcome, "nil held attacker must degrade to stand-in, not error")
}

// ---------------------------------------------------------------------------
// Monster-side attacks (held monster attacker)
// ---------------------------------------------------------------------------

// TestResolveAttack_HeldMonsterAttacker_UsesRealChain verifies that with a held
// monster attacker (as the SDK cascade would hand it) and a held character
// defender, the resolver runs the real combat chain and translates the outcome.
func (s *Dnd5eCombatResolverTestSuite) TestResolveAttack_HeldMonsterAttacker_UsesRealChain() {
	goblin := s.heldGoblin("goblin-1")
	alice := s.heldCharacter("char-alice")

	encounterData := s.buildEncounterDataWithMonsterDataJSON("goblin-1", s.goblinDataJSON("goblin-1"), "char-alice")

	resolver := v2encounter.NewDnd5eCombatResolverForData(
		v2encounter.Dnd5eCombatResolverConfig{},
		encounterData,
	)

	outcome, err := resolver.ResolveAttack(tkenc.AttackInput{
		AttackerID:          "goblin-1",
		TargetID:            "char-alice",
		AttackerAttackBonus: 4,
		AttackerDamageDice:  "1d6+2",
		AttackerDamageType:  "slashing",
		TargetAC:            14,
		Attacker:            goblin,
		Defender:            alice,
	})

	s.Require().NoError(err)
	s.Require().NotNil(outcome)
	s.GreaterOrEqual(outcome.AttackRoll, 1)
	s.LessOrEqual(outcome.AttackRoll, 20)
}

// TestResolveAttack_DefenderDodging_DisadvantageCopiedToOutcome proves the
// Beat 2 (rpg-api#613) fix end to end through the REAL rulebook chain, not
// just a field-copy shape check: a live DodgingCondition subscribed to the
// shared attack bus grants disadvantage to attacks against the dodging
// character, and the resolver must surface that on AttackOutcome rather than
// silently dropping it (the bug this PR fixes).
func (s *Dnd5eCombatResolverTestSuite) TestResolveAttack_DefenderDodging_DisadvantageCopiedToOutcome() {
	bus := events.NewEventBus()
	alice, err := character.LoadFromData(s.ctx, s.buildCharacterData("char-alice"), bus)
	s.Require().NoError(err)
	goblin := s.heldGoblin("goblin-1")

	// Alice is Dodging: attacks against her carry disadvantage until her next
	// turn. Applied on the SAME bus the attack chain runs on below — this is
	// what makes the condition's AttackChain subscription actually fire.
	dodging := conditions.NewDodgingCondition("char-alice")
	s.Require().NoError(dodging.Apply(s.ctx, bus))

	encounterData := s.buildEncounterDataWithMonsterDataJSON("goblin-1", s.goblinDataJSON("goblin-1"), "char-alice")

	resolver := v2encounter.NewDnd5eCombatResolverForData(
		v2encounter.Dnd5eCombatResolverConfig{},
		encounterData,
	)

	outcome, err := resolver.ResolveAttack(tkenc.AttackInput{
		AttackerID:          "goblin-1",
		TargetID:            "char-alice",
		AttackerAttackBonus: 4,
		AttackerDamageDice:  "1d6+2",
		AttackerDamageType:  "slashing",
		TargetAC:            14,
		Attacker:            goblin,
		Defender:            alice,
		EventBus:            bus,
	})

	s.Require().NoError(err)
	s.Require().NotNil(outcome)
	s.True(outcome.HasDisadvantage, "attack against a Dodging target must carry disadvantage")
	s.False(outcome.HasAdvantage)
	s.Require().NotEmpty(outcome.DisadvantageSources, "the granting condition must be named for narration")
	found := false
	for _, ref := range outcome.DisadvantageSources {
		if ref != nil && ref.ID == "dodging" {
			found = true
		}
	}
	s.True(found, "disadvantage source should name the Dodging condition, got %+v", outcome.DisadvantageSources)
}

// ---------------------------------------------------------------------------
// Player-side attacks (held character attacker)
// ---------------------------------------------------------------------------

// TestResolveAttack_HeldPlayerAttacker_UsesRealChain verifies that a held
// character attacker runs the real chain and produces a valid outcome. The
// defender is a held monster.
func (s *Dnd5eCombatResolverTestSuite) TestResolveAttack_HeldPlayerAttacker_UsesRealChain() {
	alice := s.heldCharacter("char-alice")
	goblin := s.heldGoblin("goblin-1")

	encounterData := s.buildEncounterData("char-alice", "goblin-1")

	resolver := v2encounter.NewDnd5eCombatResolverForData(
		v2encounter.Dnd5eCombatResolverConfig{},
		encounterData,
	)

	outcome, err := resolver.ResolveAttack(tkenc.AttackInput{
		AttackerID:          "char-alice",
		TargetID:            "goblin-1",
		AttackerAttackBonus: 4,
		AttackerDamageDice:  "1d8+2",
		AttackerDamageType:  "slashing",
		TargetAC:            15,
		Attacker:            alice,
		Defender:            goblin,
	})

	s.Require().NoError(err)
	s.Require().NotNil(outcome)
	s.GreaterOrEqual(outcome.AttackRoll, 1)
	s.LessOrEqual(outcome.AttackRoll, 20)
}

// TestResolveAttack_HeldAttacker_NilDefender_RunsAgainstSnapshotStub verifies
// that a held attacker still runs the real chain when the defender is NOT held
// (no DataJSON) — the resolver builds a snapshot stub for the defender from
// TargetAC rather than degrading the whole attack to the stand-in.
func (s *Dnd5eCombatResolverTestSuite) TestResolveAttack_HeldAttacker_NilDefender_RunsAgainstSnapshotStub() {
	alice := s.heldCharacter("char-alice")

	encounterData := s.buildEncounterData("char-alice", "goblin-1")

	resolver := v2encounter.NewDnd5eCombatResolverForData(
		v2encounter.Dnd5eCombatResolverConfig{},
		encounterData,
	)

	outcome, err := resolver.ResolveAttack(tkenc.AttackInput{
		AttackerID:          "char-alice",
		TargetID:            "goblin-1",
		AttackerAttackBonus: 4,
		AttackerDamageDice:  "1d8+2",
		AttackerDamageType:  "slashing",
		TargetAC:            15,
		Attacker:            alice,
		// Defender nil — resolver builds a snapshot stub from TargetAC.
	})

	s.Require().NoError(err)
	s.Require().NotNil(outcome)
	s.GreaterOrEqual(outcome.AttackRoll, 1)
	s.LessOrEqual(outcome.AttackRoll, 20)
	s.Equal(15, outcome.TargetAC, "snapshot-stub defender must carry the AttackInput TargetAC")
}

// ---------------------------------------------------------------------------
// Dice-notation robustness
// ---------------------------------------------------------------------------

// TestResolveAttack_DamageDiceVariants_ParseWithoutError exercises a held
// monster attacker across several damage-dice notations; none must error.
func (s *Dnd5eCombatResolverTestSuite) TestResolveAttack_DamageDiceVariants_ParseWithoutError() {
	cases := []struct {
		name       string
		damageDice string
	}{
		{"modifier suffix", "1d6+2"},
		{"negative modifier", "2d4-1"},
		{"no modifier", "1d8"},
		{"empty", ""},
		{"multi-die", "3d6+3"},
	}

	for _, tc := range cases {
		s.Run(tc.name, func() {
			goblin := s.heldGoblin("goblin-1")
			alice := s.heldCharacter("char-alice")
			encounterData := s.buildEncounterDataWithMonsterDataJSON("goblin-1", s.goblinDataJSON("goblin-1"), "char-alice")

			resolver := v2encounter.NewDnd5eCombatResolverForData(
				v2encounter.Dnd5eCombatResolverConfig{},
				encounterData,
			)

			outcome, err := resolver.ResolveAttack(tkenc.AttackInput{
				AttackerID:         "goblin-1",
				TargetID:           "char-alice",
				AttackerDamageDice: tc.damageDice,
				AttackerDamageType: "slashing",
				TargetAC:           14,
				Attacker:           goblin,
				Defender:           alice,
			})

			s.Require().NoError(err, "dice notation %q must not cause an error", tc.damageDice)
			s.Require().NotNil(outcome)
		})
	}
}

// ---------------------------------------------------------------------------
// Outcome translation
// ---------------------------------------------------------------------------

// TestResolveAttack_OutcomeTranslation_HitSetCorrectly verifies the
// tkenc.AttackOutcome fields are populated from combat.AttackResult.
func (s *Dnd5eCombatResolverTestSuite) TestResolveAttack_OutcomeTranslation_HitSetCorrectly() {
	alice := s.heldCharacter("char-alice")
	goblin := s.heldGoblin("goblin-1")

	encounterData := s.buildEncounterData("char-alice", "goblin-1")

	resolver := v2encounter.NewDnd5eCombatResolverForData(
		v2encounter.Dnd5eCombatResolverConfig{},
		encounterData,
	)

	outcome, err := resolver.ResolveAttack(tkenc.AttackInput{
		AttackerID:          "char-alice",
		TargetID:            "goblin-1",
		AttackerAttackBonus: 4,
		AttackerDamageDice:  "1d8+2",
		AttackerDamageType:  "slashing",
		TargetAC:            15,
		Attacker:            alice,
		Defender:            goblin,
	})

	s.Require().NoError(err)
	s.Require().NotNil(outcome)

	s.GreaterOrEqual(outcome.AttackRoll, 1)
	s.LessOrEqual(outcome.AttackRoll, 20)

	if outcome.Hit {
		s.GreaterOrEqual(outcome.Damage, 0)
		s.NotEmpty(outcome.DamageType)
	} else {
		s.Equal(0, outcome.Damage, "no damage on a miss")
	}

	// Beat 2 (rpg-api#613): no condition is active on either side of this
	// attack, so the advantage/disadvantage fields the resolver now copies
	// from combat.AttackResult must reflect that — not silently hardcoded
	// true, and not left as a stale zero-value that happens to match by
	// coincidence.
	s.False(outcome.HasAdvantage, "no Dodging/Hidden condition is active — no advantage expected")
	s.False(outcome.HasDisadvantage, "no Dodging/Hidden condition is active — no disadvantage expected")
	s.Empty(outcome.AdvantageSources)
	s.Empty(outcome.DisadvantageSources)
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// heldCharacter hydrates a *character.Character on a throwaway bus, mirroring
// what the SDK's LoadFromData cascade hands the resolver as AttackInput.Attacker
// / Defender. A throwaway bus is fine for unit tests: we assert outcome shape,
// not cross-attack condition persistence.
func (s *Dnd5eCombatResolverTestSuite) heldCharacter(id string) *character.Character {
	char, err := character.LoadFromData(s.ctx, s.buildCharacterData(id), events.NewEventBus())
	s.Require().NoError(err)
	return char
}

// heldGoblin hydrates a *monster.Monster from a goblin blob on a throwaway bus.
func (s *Dnd5eCombatResolverTestSuite) heldGoblin(id string) *monster.Monster {
	var data monster.Data
	s.Require().NoError(json.Unmarshal(s.goblinDataJSON(id), &data))
	mon, err := monster.LoadFromData(s.ctx, &data, events.NewEventBus())
	s.Require().NoError(err)
	return mon
}

func (s *Dnd5eCombatResolverTestSuite) goblinDataJSON(id string) []byte {
	g := monster.NewGoblin(id)
	blob, err := json.Marshal(g.ToData())
	s.Require().NoError(err)
	return blob
}

// buildCharacterData returns a minimal character.Data suitable for LoadFromData.
func (s *Dnd5eCombatResolverTestSuite) buildCharacterData(id string) *character.Data {
	return &character.Data{
		ID:               id,
		Name:             "Test Character",
		Level:            1,
		ProficiencyBonus: 2,
		HitPoints:        12,
		MaxHitPoints:     12,
		ArmorClass:       14,
		AbilityScores: shared.AbilityScores{
			abilities.STR: 16,
			abilities.DEX: 12,
			abilities.CON: 14,
			abilities.INT: 10,
			abilities.WIS: 10,
			abilities.CHA: 8,
		},
	}
}

// buildEncounterData returns encounter data containing one player
// (playerEntityID) and one monster (monsterID, no DataJSON).
func (s *Dnd5eCombatResolverTestSuite) buildEncounterData(playerEntityID, monsterID string) *tkenc.Data {
	data := tkenc.NewData("enc-test")
	data.Players[encountercore.PlayerID(playerEntityID)] = &tkenc.PlayerData{
		ID:       encountercore.PlayerID(playerEntityID),
		EntityID: encountercore.EntityID(playerEntityID),
		HP:       12,
		MaxHP:    12,
		AC:       14,
	}
	data.Monsters[encountercore.EntityID(monsterID)] = &tkenc.MonsterData{
		ID:          encountercore.EntityID(monsterID),
		HP:          7,
		MaxHP:       7,
		AC:          15,
		AttackBonus: 4,
		DamageDice:  "1d6+2",
		DamageType:  "slashing",
		MonsterRef:  "dnd5e:monsters:goblin",
	}
	return data
}

// buildEncounterDataWithMonsterDataJSON returns encounter data where the
// monster has a full DataJSON blob (for the rehydration path).
func (s *Dnd5eCombatResolverTestSuite) buildEncounterDataWithMonsterDataJSON(
	monsterID string,
	dataJSON []byte,
	playerEntityID string,
) *tkenc.Data {
	data := s.buildEncounterData(playerEntityID, monsterID)
	data.Monsters[encountercore.EntityID(monsterID)].DataJSON = dataJSON
	return data
}

// compile guard: keep combat import referenced if a future assertion drops the
// only use (the suite asserts via tkenc types today).
var _ = combat.AttackHandMain
