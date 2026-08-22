package character

import (
	"context"

	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	characterdraft "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character/choices"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/skills"
)

// finalizeAndCapture drives FinalizeDraft for the given (already-complete)
// draft through the orchestrator's mocked repos and returns the character
// actually handed to characterRepo.Create -- the same object GetCharacter
// would later read back.
func (s *OrchestratorTestSuite) finalizeAndCapture(draft *character.Draft) *character.Data {
	draftID := draft.ID()

	s.mockDraftRepo.EXPECT().Get(s.ctx, gomock.Any()).DoAndReturn(
		func(_ context.Context, input characterdraft.GetInput) (*characterdraft.GetOutput, error) {
			s.Assert().Equal(draftID, input.ID)
			return &characterdraft.GetOutput{Draft: &entities.CharacterDraft{Data: draft.ToData()}}, nil
		})
	s.mockIDGen.EXPECT().Generate().Return(s.testCharacterID)

	var saved *entities.Character
	s.mockCharacterRepo.EXPECT().Create(s.ctx, gomock.Any()).DoAndReturn(
		func(_ context.Context, input characterrepo.CreateInput) (*characterrepo.CreateOutput, error) {
			saved = input.Character
			return &characterrepo.CreateOutput{Character: input.Character}, nil
		})
	s.mockDraftRepo.EXPECT().Delete(s.ctx, gomock.Any()).Return(&characterdraft.DeleteOutput{}, nil)

	output, err := s.orchestrator.FinalizeDraft(s.ctx, &FinalizeDraftInput{DraftID: draftID})
	s.Require().NoError(err)
	s.Require().NotNil(output)
	s.Require().NotNil(saved)
	return saved.Data
}

// TestFinalizeDraft_EquipsChosenPrimaryWeaponArmorAndShield pins rpg-api#746's
// central claim: a Fighter who chose "a martial weapon and a shield" (the
// longsword) plus chain mail finalizes with the longsword in the main hand,
// the shield in the off hand, and the chain mail in the armor slot -- no
// follow-up EquipItem call required, which is exactly what the real
// (non-devseed) creation flow never makes today. The shield rides in the
// SAME EquipmentSelection as the category-resolved weapon (buildEquipmentList
// puts option.Items before the category pick), and equipChosenWeapons equips
// it into the off hand precisely because the longsword is one-handed and
// nothing else claimed that hand first.
func (s *OrchestratorTestSuite) TestFinalizeDraft_EquipsChosenPrimaryWeaponArmorAndShield() {
	draft := s.createTestDraft("draft-equip-fighter", s.testPlayerID)
	s.Require().NoError(draft.SetName(&character.SetNameInput{Name: "Sir Roland"}))
	s.Require().NoError(draft.SetRace(s.validHuman))
	s.Require().NoError(draft.SetClass(s.validFighter))
	s.Require().NoError(draft.SetBackground(s.validSoldier))
	s.Require().NoError(draft.SetAbilityScores(s.validFighterScores))

	data := s.finalizeAndCapture(draft)

	s.Equal("longsword", data.EquipmentSlots.Get(character.SlotMainHand))
	s.Equal("shield", data.EquipmentSlots.Get(character.SlotOffHand))
	s.Equal("chain-mail", data.EquipmentSlots.Get(character.SlotArmor))
}

// TestFinalizeDraft_TwoHandedWeapon_EquipsMainHandOnly pins the two-handed
// rule from the issue's own algorithm: a Barbarian who chose the Greataxe
// finalizes with it in the main hand and NOTHING in the off hand -- the
// toolkit's own EquipItem occupancy rule (a two-handed weapon claims the
// main hand and forces the off hand empty), never re-decided here.
func (s *OrchestratorTestSuite) TestFinalizeDraft_TwoHandedWeapon_EquipsMainHandOnly() {
	draft := s.createTestDraft("draft-equip-barbarian", s.testPlayerID)
	s.Require().NoError(draft.SetName(&character.SetNameInput{Name: "Grosh"}))
	s.Require().NoError(draft.SetRace(s.validHuman))
	s.Require().NoError(draft.SetClass(&character.SetClassInput{
		ClassID: classes.Barbarian,
		Choices: character.ClassChoices{
			Skills: []skills.Skill{skills.Athletics, skills.Intimidation},
			Equipment: []character.EquipmentChoiceSelection{
				{ChoiceID: choices.BarbarianWeaponsPrimary, OptionID: "barbarian-weapon-a"}, // Greataxe
				{ChoiceID: choices.BarbarianWeaponsSecondary, OptionID: "barbarian-secondary-a"},
				{ChoiceID: choices.BarbarianPack, OptionID: "barbarian-pack-a"},
			},
		},
	}))
	s.Require().NoError(draft.SetBackground(s.validSoldier))
	s.Require().NoError(draft.SetAbilityScores(&character.SetAbilityScoresInput{
		Scores: shared.AbilityScores{
			abilities.STR: 15, abilities.DEX: 13, abilities.CON: 14,
			abilities.INT: 10, abilities.WIS: 12, abilities.CHA: 8,
		},
		Method: "standard",
	}))

	data := s.finalizeAndCapture(draft)

	s.Equal("greataxe", data.EquipmentSlots.Get(character.SlotMainHand))
	s.Empty(data.EquipmentSlots.Get(character.SlotOffHand))
}

// TestFinalizeDraft_ReEquippingTheAlreadyEquippedShieldIsHarmless proves the
// idempotency Kirk's gate review asked for: cmd/sandboxseed calls EquipItem
// for the shield unconditionally, on every seed run, regardless of whether
// finalize already put it there. A second EquipItem for the SAME item
// already in that slot must be a harmless no-op (not an error, not a swap
// that clears the main hand) -- and it already is, by EquipItem's own
// existing logic: with a single copy owned, it clears every slot currently
// holding that item id before resetting it, which nets out to the same
// state. No new toolkit rule needed; this test exists to pin that fact
// rather than assume it.
func (s *OrchestratorTestSuite) TestFinalizeDraft_ReEquippingTheAlreadyEquippedShieldIsHarmless() {
	draft := s.createTestDraft("draft-equip-shield-reequip", s.testPlayerID)
	s.Require().NoError(draft.SetName(&character.SetNameInput{Name: "Dame Elara"}))
	s.Require().NoError(draft.SetRace(s.validHuman))
	s.Require().NoError(draft.SetClass(s.validFighter))
	s.Require().NoError(draft.SetBackground(s.validSoldier))
	s.Require().NoError(draft.SetAbilityScores(s.validFighterScores))

	data := s.finalizeAndCapture(draft)
	s.Equal("shield", data.EquipmentSlots.Get(character.SlotOffHand), "finalize already equipped it")

	s.mockCharacterRepo.EXPECT().Get(s.ctx, gomock.Any()).Return(
		&characterrepo.GetOutput{Character: &entities.Character{Data: data}}, nil)
	var updated *entities.Character
	s.mockCharacterRepo.EXPECT().Update(s.ctx, gomock.Any()).DoAndReturn(
		func(_ context.Context, input characterrepo.UpdateInput) (*characterrepo.UpdateOutput, error) {
			updated = input.Character
			return &characterrepo.UpdateOutput{Character: input.Character}, nil
		})

	_, err := s.orchestrator.EquipItem(s.ctx, &EquipItemInput{
		CharacterID: s.testCharacterID,
		ItemID:      "shield",
		Slot:        character.SlotOffHand,
	})
	s.Require().NoError(err)
	s.Require().NotNil(updated)
	s.Equal("shield", updated.Data.EquipmentSlots.Get(character.SlotOffHand))
	// The weapon auto-equipped at finalize is untouched by this later call.
	s.Equal("longsword", updated.Data.EquipmentSlots.Get(character.SlotMainHand))
}
