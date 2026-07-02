package encounter

// White-box tests for the strike-ActionRef -> weapon resolution wired in
// rpg-toolkit#712. These live in package encounter (not encounter_test) so
// they can call the unexported resolveWeapon directly and assert on the
// concrete weaponResolution — the crispest proof that the resolver now honors
// AttackInput.ActionRef instead of switching on the equipped slot / AttackHand.
//
// The behavioral contract (owned by the toolkit's
// Character.WeaponForActionRef, exercised here through the resolver call-site):
//   - unarmed_strike by a weapon-holding character resolves the canonical
//     unarmed weapon, NOT the equipped weapon.
//   - off_hand_strike resolves the equipped off-hand weapon, AttackHandOff,
//     and populates a TwoWeaponContext (so the rulebook's off-hand validation
//     does not hard-fail).
//   - a normal strike is unchanged: the equipped main-hand weapon, main hand,
//     no TwoWeaponContext.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"

	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/events"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/combat"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/weapons"
)

type ResolveWeaponTestSuite struct {
	suite.Suite
	ctx      context.Context
	resolver *Dnd5eCombatResolver
}

func TestResolveWeaponTestSuite(t *testing.T) {
	suite.Run(t, new(ResolveWeaponTestSuite))
}

func (s *ResolveWeaponTestSuite) SetupTest() {
	s.ctx = context.Background()
	s.resolver = NewDnd5eCombatResolverForData(Dnd5eCombatResolverConfig{}, nil)
}

// weaponHoldingMonk hydrates a Monk with a shortsword equipped in the main
// hand — the exact case #712 broke: a weapon-holding Monk's unarmed_strike
// swung the equipped shortsword instead of resolving unarmed.
func (s *ResolveWeaponTestSuite) weaponHoldingMonk(id string) *character.Character {
	data := &character.Data{
		ID:               id,
		Name:             "Test Monk",
		Level:            1,
		ProficiencyBonus: 2,
		ClassID:          classes.Monk,
		HitPoints:        10,
		MaxHitPoints:     10,
		ArmorClass:       13,
		AbilityScores: shared.AbilityScores{
			abilities.STR: 12, abilities.DEX: 16, abilities.CON: 14,
			abilities.INT: 10, abilities.WIS: 14, abilities.CHA: 8,
		},
		Inventory: []character.InventoryItemData{
			{Type: shared.EquipmentTypeWeapon, ID: weapons.Shortsword, Quantity: 1},
		},
		EquipmentSlots: character.EquipmentSlots{
			character.SlotMainHand: weapons.Shortsword,
		},
	}
	char, err := character.LoadFromData(s.ctx, data, events.NewEventBus())
	s.Require().NoError(err)
	// Guard: the shortsword really is equipped, so a "resolves unarmed" assertion
	// is meaningful (it must ignore a present weapon, not an empty slot).
	s.Require().NotNil(char.GetEquippedSlot(character.SlotMainHand))
	s.Require().Equal(weapons.Shortsword, char.GetEquippedSlot(character.SlotMainHand).AsWeapon().ID)
	return char
}

// dualWielder hydrates a Fighter with a shortsword main-hand + dagger off-hand
// (both light) so off_hand_strike has a real off-hand weapon to resolve.
func (s *ResolveWeaponTestSuite) dualWielder(id string) *character.Character {
	data := &character.Data{
		ID:               id,
		Name:             "Test Dual Wielder",
		Level:            1,
		ProficiencyBonus: 2,
		ClassID:          classes.Fighter,
		HitPoints:        12,
		MaxHitPoints:     12,
		ArmorClass:       15,
		AbilityScores: shared.AbilityScores{
			abilities.STR: 14, abilities.DEX: 16, abilities.CON: 14,
			abilities.INT: 10, abilities.WIS: 10, abilities.CHA: 8,
		},
		Inventory: []character.InventoryItemData{
			{Type: shared.EquipmentTypeWeapon, ID: weapons.Shortsword, Quantity: 1},
			{Type: shared.EquipmentTypeWeapon, ID: weapons.Dagger, Quantity: 1},
		},
		EquipmentSlots: character.EquipmentSlots{
			character.SlotMainHand: weapons.Shortsword,
			character.SlotOffHand:  weapons.Dagger,
		},
	}
	char, err := character.LoadFromData(s.ctx, data, events.NewEventBus())
	s.Require().NoError(err)
	return char
}

// TestUnarmedStrike_WeaponHoldingMonk_ResolvesUnarmed proves the resolver now
// honors the ActionRef: a Monk holding a shortsword who submits unarmed_strike
// resolves the canonical unarmed weapon, not the equipped shortsword. Before
// #712 the resolver picked by equipped slot and swung the shortsword.
func (s *ResolveWeaponTestSuite) TestUnarmedStrike_WeaponHoldingMonk_ResolvesUnarmed() {
	monk := s.weaponHoldingMonk("char-monk")

	res, err := s.resolver.resolveWeapon(monk, nil, tkenc.AttackInput{
		AttackerID: "char-monk",
		ActionRef:  *refs.Actions.UnarmedStrike(),
	})

	s.Require().NoError(err)
	s.Require().NotNil(res)
	s.Require().NotNil(res.weapon)
	s.Equal(weapons.UnarmedStrike, res.weapon.ID,
		"unarmed_strike must resolve the canonical unarmed weapon, not the equipped shortsword")
	s.NotEqual(weapons.Shortsword, res.weapon.ID)
	s.Equal(combat.AttackHandMain, res.attackHand)
	s.Nil(res.twoWeapon, "a main-hand unarmed strike carries no two-weapon context")
}

// TestFlurryStrike_WeaponHoldingMonk_ResolvesUnarmed: flurry_strike is also a
// Monk unarmed strike — same ignore-the-equipped-weapon contract.
func (s *ResolveWeaponTestSuite) TestFlurryStrike_WeaponHoldingMonk_ResolvesUnarmed() {
	monk := s.weaponHoldingMonk("char-monk")

	res, err := s.resolver.resolveWeapon(monk, nil, tkenc.AttackInput{
		AttackerID: "char-monk",
		ActionRef:  *refs.Actions.FlurryStrike(),
	})

	s.Require().NoError(err)
	s.Require().NotNil(res)
	s.Equal(weapons.UnarmedStrike, res.weapon.ID,
		"flurry_strike must resolve unarmed, not the equipped shortsword")
	s.Equal(combat.AttackHandMain, res.attackHand)
	s.Nil(res.twoWeapon)
}

// TestOffHandStrike_SelectsOffHandWeaponWithTwoWeaponContext proves
// off_hand_strike selects the off-hand weapon (dagger), sets AttackHandOff,
// and populates a TwoWeaponContext so the rulebook's off-hand validation does
// not hard-fail — the exact failure #712 called out for AttackHand="off".
func (s *ResolveWeaponTestSuite) TestOffHandStrike_SelectsOffHandWeaponWithTwoWeaponContext() {
	tw := s.dualWielder("char-twf")

	res, err := s.resolver.resolveWeapon(tw, nil, tkenc.AttackInput{
		AttackerID: "char-twf",
		ActionRef:  *refs.Actions.OffHandStrike(),
	})

	s.Require().NoError(err)
	s.Require().NotNil(res)
	s.Require().NotNil(res.weapon)
	s.Equal(weapons.Dagger, res.weapon.ID,
		"off_hand_strike must resolve the off-hand dagger, not the main-hand shortsword")
	s.Equal(combat.AttackHandOff, res.attackHand)
	s.Require().NotNil(res.twoWeapon,
		"off_hand_strike must populate a TwoWeaponContext or the rulebook off-hand validation hard-fails")

	// The context reports the character's REAL equipped weapons.
	mainHand := res.twoWeapon.GetMainHandWeapon("char-twf")
	offHand := res.twoWeapon.GetOffHandWeapon("char-twf")
	s.Require().NotNil(mainHand)
	s.Require().NotNil(offHand)
	s.Equal(weapons.Shortsword, mainHand.WeaponID)
	s.Equal(weapons.Dagger, offHand.WeaponID)
}

// TestNormalStrike_ResolvesEquippedMainHand: the normal attack path is
// unchanged — a strike resolves the equipped main-hand weapon, main hand, no
// two-weapon context.
func (s *ResolveWeaponTestSuite) TestNormalStrike_ResolvesEquippedMainHand() {
	tw := s.dualWielder("char-twf")

	res, err := s.resolver.resolveWeapon(tw, nil, tkenc.AttackInput{
		AttackerID: "char-twf",
		ActionRef:  *refs.Actions.Strike(),
	})

	s.Require().NoError(err)
	s.Require().NotNil(res)
	s.Equal(weapons.Shortsword, res.weapon.ID,
		"a normal strike resolves the equipped main-hand weapon")
	s.Equal(combat.AttackHandMain, res.attackHand)
	s.Nil(res.twoWeapon)
}

// TestOffHandStrike_ThroughResolveAttack_DoesNotHardFail drives off_hand_strike
// through the full public ResolveAttack (prepareHeldAttack installs the
// TwoWeaponContext into the resolve ctx via combat.WithTwoWeaponContext). It
// proves end-to-end that the wiring prevents the "two-weapon context not
// available" hard-fail #712 flagged, and still yields a valid outcome.
func (s *ResolveWeaponTestSuite) TestOffHandStrike_ThroughResolveAttack_DoesNotHardFail() {
	attacker := s.dualWielder("char-twf")
	defender := s.weaponHoldingMonk("char-monk")

	outcome, err := s.resolver.ResolveAttack(tkenc.AttackInput{
		AttackerID: "char-twf",
		TargetID:   "char-monk",
		ActionRef:  *refs.Actions.OffHandStrike(),
		AttackHand: "off",
		TargetAC:   13,
		Attacker:   attacker,
		Defender:   defender,
	})

	s.Require().NoError(err, "off_hand_strike must not hard-fail once the TwoWeaponContext is wired into the resolve ctx")
	s.Require().NotNil(outcome)
	s.GreaterOrEqual(outcome.AttackRoll, 1)
	s.LessOrEqual(outcome.AttackRoll, 20)
}
