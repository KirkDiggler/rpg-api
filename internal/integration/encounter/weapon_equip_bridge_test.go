// Copyright (C) 2024 Kirk Diggler
// SPDX-License-Identifier: GPL-3.0-or-later

// Package encounter_integration provides integration tests verifying the
// weapon-equipping bridge for the Wave 2.11b combat resolver.
//
// Scope (issue #524 — Wave 2.11b, slice 3 of 3):
//
// Confirms that the path:
//
//	character creation → EquipItem (sets EquipmentSlots) → character.Data persistence
//	→ character.LoadFromData rehydration → GetEquippedSlot(SlotMainHand).AsWeapon()
//
// returns a non-nil *weapons.Weapon, giving the Dnd5eCombatResolver a real
// weapon to pass to combat.AttackInput.Weapon.
//
// The concern: rpg-api's EquipItem (orchestrators/character/orchestrator.go)
// only sets charData.EquipmentSlots — it does NOT add the weapon to
// charData.Inventory. GetEquippedSlot resolves the slot by scanning the live
// inventory for an item whose ID matches the slot's value; if the weapon is
// absent from inventory, it returns nil.
//
// This test verifies the full round-trip through realistic plumbing:
//  1. Fighter character created via the full draft→finalize pipeline
//     (compileInventory populates Data.Inventory with the longsword chosen
//     at draft time).
//  2. EquipItem sets Data.EquipmentSlots[SlotMainHand] = "longsword".
//  3. Character data fetched from Redis repo.
//  4. character.LoadFromData rebuilds inventory from Data.Inventory
//     (equipment.GetByID("longsword") → *weapons.Weapon) and slots from
//     Data.EquipmentSlots.
//  5. GetEquippedSlot(SlotMainHand) returns the EquippedItem.
//  6. AsWeapon() returns the *weapons.Weapon (non-nil, correct DamageType).
package encounter_integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc/metadata"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/integration/harness"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-toolkit/events"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/damage"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/weapons"
)

// WeaponEquipBridgeSuite verifies that the weapon-equipping bridge survives
// encounter rehydration (Wave 2.11b — slice 3 of 3, issue #524).
type WeaponEquipBridgeSuite struct {
	suite.Suite
	ctx    context.Context
	cancel context.CancelFunc
	server *harness.TestServer
}

func TestWeaponEquipBridgeSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	suite.Run(t, new(WeaponEquipBridgeSuite))
}

func (s *WeaponEquipBridgeSuite) SetupSuite() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 3*time.Minute)

	var err error
	s.server, err = harness.New(s.ctx, nil)
	s.Require().NoError(err, "failed to create test server")
}

func (s *WeaponEquipBridgeSuite) TearDownSuite() {
	if s.server != nil {
		s.server.Close()
	}
	if s.cancel != nil {
		s.cancel()
	}
}

func (s *WeaponEquipBridgeSuite) SetupTest() {
	err := s.server.FlushRedis(s.ctx)
	s.Require().NoError(err, "failed to flush redis")
}

func (s *WeaponEquipBridgeSuite) authCtx(playerID string) context.Context {
	return metadata.AppendToOutgoingContext(s.ctx, "authorization", "Dev "+playerID)
}

// createFighterWithLongsword creates a fully finalized Fighter character whose
// equipment choice includes a longsword. The finalization pipeline's
// compileInventory step populates Data.Inventory with the longsword, which
// is the prerequisite for GetEquippedSlot to resolve it after EquipItem sets
// the SlotMainHand.
//
// Returns the character's ID.
func (s *WeaponEquipBridgeSuite) createFighterWithLongsword(playerID string) string {
	ctx := s.authCtx(playerID)

	// Create draft
	createResp, err := s.server.CharacterClient.CreateDraft(ctx, &dnd5ev1alpha1.CreateDraftRequest{})
	s.Require().NoError(err)
	draftID := createResp.GetDraft().GetId()

	// Name
	_, err = s.server.CharacterClient.UpdateName(ctx, &dnd5ev1alpha1.UpdateNameRequest{
		DraftId: draftID,
		Name:    "Sir Roland",
	})
	s.Require().NoError(err)

	// Human (with required language choice)
	_, err = s.server.CharacterClient.UpdateRace(ctx, &dnd5ev1alpha1.UpdateRaceRequest{
		DraftId: draftID,
		Race:    dnd5ev1alpha1.Race_RACE_HUMAN,
		RaceChoices: []*dnd5ev1alpha1.ChoiceData{
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_LANGUAGES,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_RACE,
				Selection: &dnd5ev1alpha1.ChoiceData_Languages{
					Languages: &dnd5ev1alpha1.LanguageSelection{
						Languages: []dnd5ev1alpha1.Language{dnd5ev1alpha1.Language_LANGUAGE_DWARVISH},
					},
				},
			},
		},
	})
	s.Require().NoError(err)

	// Fighter with longsword equipment choice. The "fighter-weapon-a" choice
	// with WEAPON_LONGSWORD explicitly selects the longsword into the
	// equipment pool — compileInventory will include it in Data.Inventory.
	_, err = s.server.CharacterClient.UpdateClass(ctx, &dnd5ev1alpha1.UpdateClassRequest{
		DraftId: draftID,
		Class:   dnd5ev1alpha1.Class_CLASS_FIGHTER,
		ClassChoices: []*dnd5ev1alpha1.ChoiceData{
			// Skills (required)
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SKILLS,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				Selection: &dnd5ev1alpha1.ChoiceData_Skills{
					Skills: &dnd5ev1alpha1.SkillSelection{
						Skills: []dnd5ev1alpha1.Skill{
							dnd5ev1alpha1.Skill_SKILL_ATHLETICS,
							dnd5ev1alpha1.Skill_SKILL_PERCEPTION,
						},
					},
				},
			},
			// Fighting style
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_FIGHTING_STYLE,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				Selection: &dnd5ev1alpha1.ChoiceData_FightingStyle{
					FightingStyle: &dnd5ev1alpha1.FightingStyleSelection{
						Style: dnd5ev1alpha1.FightingStyle_FIGHTING_STYLE_DEFENSE,
					},
				},
			},
			// Armor: Chain mail
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "fighter-armor",
				OptionId: "fighter-armor-a",
				Selection: &dnd5ev1alpha1.ChoiceData_Equipment{
					Equipment: &dnd5ev1alpha1.EquipmentSelection{},
				},
			},
			// Primary weapon: Longsword (explicit selection)
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "fighter-weapons-primary",
				OptionId: "fighter-weapon-a",
				Selection: &dnd5ev1alpha1.ChoiceData_Equipment{
					Equipment: &dnd5ev1alpha1.EquipmentSelection{
						Items: []*dnd5ev1alpha1.EquipmentSelectionItem{
							{
								Equipment: &dnd5ev1alpha1.EquipmentSelectionItem_Weapon{
									Weapon: dnd5ev1alpha1.Weapon_WEAPON_LONGSWORD,
								},
							},
						},
					},
				},
			},
			// Secondary: Light crossbow
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "fighter-weapons-secondary",
				OptionId: "fighter-ranged-a",
				Selection: &dnd5ev1alpha1.ChoiceData_Equipment{
					Equipment: &dnd5ev1alpha1.EquipmentSelection{},
				},
			},
			// Pack: Dungeoneer's
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "fighter-pack",
				OptionId: "fighter-pack-a",
				Selection: &dnd5ev1alpha1.ChoiceData_Equipment{
					Equipment: &dnd5ev1alpha1.EquipmentSelection{},
				},
			},
		},
	})
	s.Require().NoError(err)

	// Background
	_, err = s.server.CharacterClient.UpdateBackground(ctx, &dnd5ev1alpha1.UpdateBackgroundRequest{
		DraftId:    draftID,
		Background: dnd5ev1alpha1.Background_BACKGROUND_SOLDIER,
	})
	s.Require().NoError(err)

	// Ability scores (STR-focused fighter)
	_, err = s.server.CharacterClient.UpdateAbilityScores(ctx, &dnd5ev1alpha1.UpdateAbilityScoresRequest{
		DraftId: draftID,
		ScoresInput: &dnd5ev1alpha1.UpdateAbilityScoresRequest_AbilityScores{
			AbilityScores: &dnd5ev1alpha1.AbilityScores{
				Strength:     15,
				Dexterity:    13,
				Constitution: 14,
				Intelligence: 10,
				Wisdom:       12,
				Charisma:     8,
			},
		},
	})
	s.Require().NoError(err)

	// Finalize
	finalizeResp, err := s.server.CharacterClient.FinalizeDraft(ctx, &dnd5ev1alpha1.FinalizeDraftRequest{
		DraftId: draftID,
	})
	s.Require().NoError(err)
	s.Require().NotNil(finalizeResp.GetCharacter())

	return finalizeResp.GetCharacter().GetId()
}

// TestWeaponEquipBridge_LongswordSurvivesLoadFromData is the primary
// verification gate for Wave 2.11b slice 3. It proves the full bridge:
//
//	EquipItem(SlotMainHand, "longsword")
//	→ character.Data persistence (Redis)
//	→ character.LoadFromData rehydration
//	→ GetEquippedSlot(SlotMainHand).AsWeapon() ≠ nil
//
// If this test fails, the Dnd5eCombatResolver (issue #523) cannot populate
// combat.AttackInput.Weapon and is blocked from shipping.
func (s *WeaponEquipBridgeSuite) TestWeaponEquipBridge_LongswordSurvivesLoadFromData() {
	playerID := "test-player-bridge"
	ctx := s.authCtx(playerID)

	// Step 1: Create a fighter character. The draft finalization pipeline's
	// compileInventory populates Data.Inventory with the longsword.
	charID := s.createFighterWithLongsword(playerID)
	s.T().Logf("Created fighter character: %s", charID)

	// Step 2: Equip the longsword to the main hand via the character service.
	// This sets Data.EquipmentSlots[SlotMainHand] = "longsword".
	// NOTE: EquipItem does NOT add the weapon to Data.Inventory — the weapon
	// must already be there (from step 1's draft finalization).
	equipResp, err := s.server.CharacterClient.EquipItem(ctx, &dnd5ev1alpha1.EquipItemRequest{
		CharacterId: charID,
		ItemId:      weapons.Longsword, // "longsword"
		Slot:        dnd5ev1alpha1.EquipmentSlot_EQUIPMENT_SLOT_MAIN_HAND,
	})
	s.Require().NoError(err, "EquipItem must succeed — longsword should already be in inventory from draft")
	s.Require().NotNil(equipResp)
	s.T().Logf("EquipItem succeeded; previous slot item: %q", equipResp.GetPreviouslyEquippedItem().GetItemId())

	// Step 3: Fetch the persisted character.Data directly from the repo.
	// This mirrors what the encounter orchestrator does at its LoadFromData
	// call sites (orchestrators/encounter/orchestrator.go:246, 324, 807, 2431).
	repoOutput, err := s.server.CharacterRepo.Get(s.ctx, characterrepo.GetInput{ID: charID})
	s.Require().NoError(err, "character must be retrievable from repo after EquipItem")
	s.Require().NotNil(repoOutput.Character, "repo must return a non-nil character")
	s.Require().NotNil(repoOutput.Character.Data, "character.Data must not be nil")

	charData := repoOutput.Character.Data

	// Verify the slot was written to the persisted data before rehydration.
	s.Require().NotEmpty(charData.EquipmentSlots, "EquipmentSlots must be non-empty after EquipItem")
	mainHandID := charData.EquipmentSlots.Get(character.SlotMainHand)
	s.Require().Equal(weapons.Longsword, mainHandID,
		"EquipmentSlots[SlotMainHand] must equal %q", weapons.Longsword)
	s.T().Logf("Persisted EquipmentSlots[SlotMainHand] = %q", mainHandID)

	// Verify the inventory contains the longsword (prerequisite for GetEquippedSlot
	// to resolve the slot — it scans inventory for an item matching the slot ID).
	var longswordInInventory bool
	for _, item := range charData.Inventory {
		if item.ID == weapons.Longsword {
			longswordInInventory = true
			s.T().Logf("Longsword found in Data.Inventory with quantity %d", item.Quantity)
			break
		}
	}
	s.Require().True(longswordInInventory,
		"Data.Inventory must contain %q for GetEquippedSlot to resolve — "+
			"if this fails, the draft finalization's compileInventory did not "+
			"include the longsword; the bridge is broken at equipment-choice serialization",
		weapons.Longsword)

	// Step 4: Rehydrate via character.LoadFromData — the same call the encounter
	// orchestrator makes at its LoadFromData sites. Uses a fresh event bus
	// (mirrors the orchestrator's per-call bus construction).
	bus := events.NewEventBus()
	rehydrated, err := character.LoadFromData(s.ctx, charData, bus)
	s.Require().NoError(err, "character.LoadFromData must succeed on persisted data")
	s.Require().NotNil(rehydrated, "rehydrated character must not be nil")
	defer func() { _ = rehydrated.Cleanup(s.ctx) }()

	// Step 5: Assert the equipped slot resolves to an EquippedItem.
	// GetEquippedSlot scans the in-memory inventory (rebuilt by LoadFromData
	// from Data.Inventory via equipment.GetByID) for the item ID stored in
	// Data.EquipmentSlots[SlotMainHand].
	equipped := rehydrated.GetEquippedSlot(character.SlotMainHand)
	s.Require().NotNil(equipped,
		"GetEquippedSlot(SlotMainHand) must return non-nil after LoadFromData — "+
			"nil means the inventory does not contain an item matching EquipmentSlots[SlotMainHand]=%q; "+
			"the weapon-equipping bridge is broken",
		mainHandID)
	s.T().Logf("GetEquippedSlot(SlotMainHand) returned non-nil EquippedItem")

	// Step 6: Assert AsWeapon returns the *weapons.Weapon.
	// This is the exact call the Dnd5eCombatResolver (issue #523) will make
	// to populate combat.AttackInput.Weapon.
	w := equipped.AsWeapon()
	s.Require().NotNil(w,
		"EquippedItem.AsWeapon() must return non-nil *weapons.Weapon — "+
			"nil means the equipped item is not a weapon type; check equipment type registration")

	// Step 7: Verify weapon identity and damage type.
	// The resolver uses DamageType to translate *combat.AttackResult back to
	// *tkenc.AttackOutcome.DamageType — verify this is "slashing" for a longsword.
	s.Equal(weapons.Longsword, w.ID,
		"equipped weapon ID must be %q", weapons.Longsword)
	s.Equal(damage.Slashing, w.DamageType,
		"longsword DamageType must be slashing for correct damage-type translation in AttackOutcome")
	s.NotEmpty(w.Damage, "longsword Damage notation must be non-empty (e.g. '1d8')")

	s.T().Logf("Bridge verified: longsword round-trips through LoadFromData; "+
		"ID=%q DamageType=%q Damage=%q — combat.AttackInput.Weapon is resolvable",
		w.ID, w.DamageType, w.Damage)
}

// TestWeaponEquipBridge_NoEquipItem_SlotIsEmpty verifies the negative case:
// a newly created fighter character (inventory populated by draft) whose
// SlotMainHand has NOT been set via EquipItem returns nil from GetEquippedSlot.
// This confirms the test above is actually testing something meaningful — the
// slot must be explicitly set for the weapon to be accessible.
func (s *WeaponEquipBridgeSuite) TestWeaponEquipBridge_NoEquipItem_SlotIsEmpty() {
	playerID := "test-player-no-equip"

	// Create fighter without calling EquipItem.
	charID := s.createFighterWithLongsword(playerID)

	// Fetch raw data without equipping.
	repoOutput, err := s.server.CharacterRepo.Get(s.ctx, characterrepo.GetInput{ID: charID})
	s.Require().NoError(err)
	charData := repoOutput.Character.Data

	// Rehydrate.
	bus := events.NewEventBus()
	rehydrated, err := character.LoadFromData(s.ctx, charData, bus)
	s.Require().NoError(err)
	s.Require().NotNil(rehydrated)
	defer func() { _ = rehydrated.Cleanup(s.ctx) }()

	// Without EquipItem, SlotMainHand should be empty.
	equipped := rehydrated.GetEquippedSlot(character.SlotMainHand)
	s.Nil(equipped,
		"GetEquippedSlot(SlotMainHand) must be nil before EquipItem is called — "+
			"draft finalization does NOT auto-equip weapons to slots")
	s.T().Logf("Confirmed: SlotMainHand is nil before EquipItem — test above is meaningful")
}

// TestWeaponEquipBridge_InventoryContainsLongswordAfterFinalization verifies
// that the fighter draft finalization compiles the longsword into Data.Inventory.
// This is the prerequisite for the bridge to work; if this test fails, the root
// cause is in compileInventory or the equipment-choice serialization pipeline,
// not in EquipItem or LoadFromData.
func (s *WeaponEquipBridgeSuite) TestWeaponEquipBridge_InventoryContainsLongswordAfterFinalization() {
	playerID := "test-player-inv-check"
	charID := s.createFighterWithLongsword(playerID)

	repoOutput, err := s.server.CharacterRepo.Get(s.ctx, characterrepo.GetInput{ID: charID})
	s.Require().NoError(err)
	charData := repoOutput.Character.Data

	var found bool
	for _, item := range charData.Inventory {
		if item.ID == weapons.Longsword {
			found = true
			break
		}
	}
	s.Require().True(found,
		"Data.Inventory must contain %q after draft finalization with longsword equipment choice — "+
			"check draft.compileInventory() and the fighter equipment choice 'fighter-weapon-a'",
		weapons.Longsword)
	s.T().Logf("Inventory check passed: Data.Inventory contains %q", weapons.Longsword)
}
