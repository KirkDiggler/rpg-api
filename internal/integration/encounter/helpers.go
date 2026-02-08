// Copyright (C) 2024 Kirk Diggler
// SPDX-License-Identifier: GPL-3.0-or-later

// Package encounter_integration provides test helpers for integration tests.
package encounter_integration

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/integration/harness"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	coreResources "github.com/KirkDiggler/rpg-toolkit/core/resources"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	dnd5eResources "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/resources"
)

// CharacterBuilder provides a fluent interface for creating test characters.
type CharacterBuilder struct {
	server  *harness.TestServer
	ctx     context.Context
	draftID string

	// Configuration
	name       string
	race       dnd5ev1alpha1.Race
	subrace    dnd5ev1alpha1.Subrace
	class      dnd5ev1alpha1.Class
	background dnd5ev1alpha1.Background

	// Ability scores (standard array by default)
	str, dex, con, int_, wis, cha int32

	// Class-specific choices
	skills        []dnd5ev1alpha1.Skill
	fightingStyle dnd5ev1alpha1.FightingStyle
}

// NewCharacterBuilder creates a new character builder with sensible defaults.
func NewCharacterBuilder(server *harness.TestServer, ctx context.Context) *CharacterBuilder {
	return &CharacterBuilder{
		server:     server,
		ctx:        ctx,
		name:       "Test Character",
		race:       dnd5ev1alpha1.Race_RACE_HUMAN,
		class:      dnd5ev1alpha1.Class_CLASS_FIGHTER,
		background: dnd5ev1alpha1.Background_BACKGROUND_SOLDIER,
		// Standard array defaults
		str: 15, dex: 14, con: 13, int_: 12, wis: 10, cha: 8,
	}
}

// WithName sets the character name.
func (b *CharacterBuilder) WithName(name string) *CharacterBuilder {
	b.name = name
	return b
}

// WithRace sets the character race.
func (b *CharacterBuilder) WithRace(race dnd5ev1alpha1.Race) *CharacterBuilder {
	b.race = race
	return b
}

// WithSubrace sets the character subrace.
func (b *CharacterBuilder) WithSubrace(subrace dnd5ev1alpha1.Subrace) *CharacterBuilder {
	b.subrace = subrace
	return b
}

// WithClass sets the character class.
func (b *CharacterBuilder) WithClass(class dnd5ev1alpha1.Class) *CharacterBuilder {
	b.class = class
	return b
}

// WithBackground sets the character background.
func (b *CharacterBuilder) WithBackground(bg dnd5ev1alpha1.Background) *CharacterBuilder {
	b.background = bg
	return b
}

// WithAbilityScores sets custom ability scores.
func (b *CharacterBuilder) WithAbilityScores(str, dex, con, int_, wis, cha int32) *CharacterBuilder {
	b.str, b.dex, b.con, b.int_, b.wis, b.cha = str, dex, con, int_, wis, cha
	return b
}

// WithSkills sets the skill proficiency choices.
func (b *CharacterBuilder) WithSkills(skills ...dnd5ev1alpha1.Skill) *CharacterBuilder {
	b.skills = skills
	return b
}

// WithFightingStyle sets the fighting style choice (for Fighter, Paladin, Ranger).
func (b *CharacterBuilder) WithFightingStyle(style dnd5ev1alpha1.FightingStyle) *CharacterBuilder {
	b.fightingStyle = style
	return b
}

// AsBarbarian configures the builder for a typical barbarian.
func (b *CharacterBuilder) AsBarbarian() *CharacterBuilder {
	return b.
		WithClass(dnd5ev1alpha1.Class_CLASS_BARBARIAN).
		WithAbilityScores(15, 13, 14, 8, 12, 10). // STR, DEX, CON, INT, WIS, CHA
		WithSkills(dnd5ev1alpha1.Skill_SKILL_ATHLETICS, dnd5ev1alpha1.Skill_SKILL_INTIMIDATION)
}

// AsFighter configures the builder for a typical fighter.
func (b *CharacterBuilder) AsFighter() *CharacterBuilder {
	return b.
		WithClass(dnd5ev1alpha1.Class_CLASS_FIGHTER).
		WithAbilityScores(15, 14, 13, 8, 12, 10).
		WithSkills(dnd5ev1alpha1.Skill_SKILL_ATHLETICS, dnd5ev1alpha1.Skill_SKILL_PERCEPTION).
		WithFightingStyle(dnd5ev1alpha1.FightingStyle_FIGHTING_STYLE_DEFENSE)
}

// AsRogue configures the builder for a typical rogue.
func (b *CharacterBuilder) AsRogue() *CharacterBuilder {
	return b.
		WithClass(dnd5ev1alpha1.Class_CLASS_ROGUE).
		WithAbilityScores(10, 15, 13, 12, 14, 8).
		WithSkills(
			dnd5ev1alpha1.Skill_SKILL_ACROBATICS,
			dnd5ev1alpha1.Skill_SKILL_STEALTH,
			dnd5ev1alpha1.Skill_SKILL_PERCEPTION,
			dnd5ev1alpha1.Skill_SKILL_SLEIGHT_OF_HAND,
		)
}

// Build creates the character and returns its ID.
func (b *CharacterBuilder) Build() (string, error) {
	// Step 1: Create draft
	createResp, err := b.server.CharacterClient.CreateDraft(b.ctx, &dnd5ev1alpha1.CreateDraftRequest{})
	if err != nil {
		return "", fmt.Errorf("create draft: %w", err)
	}
	b.draftID = createResp.GetDraft().GetId()

	// Step 2: Set name
	_, err = b.server.CharacterClient.UpdateName(b.ctx, &dnd5ev1alpha1.UpdateNameRequest{
		DraftId: b.draftID,
		Name:    b.name,
	})
	if err != nil {
		return "", fmt.Errorf("set name: %w", err)
	}

	// Step 3: Set race
	_, err = b.server.CharacterClient.UpdateRace(b.ctx, &dnd5ev1alpha1.UpdateRaceRequest{
		DraftId: b.draftID,
		Race:    b.race,
		Subrace: b.subrace,
	})
	if err != nil {
		return "", fmt.Errorf("set race: %w", err)
	}

	// Step 4: Set class with choices
	classChoices := b.buildClassChoices()
	_, err = b.server.CharacterClient.UpdateClass(b.ctx, &dnd5ev1alpha1.UpdateClassRequest{
		DraftId:      b.draftID,
		Class:        b.class,
		ClassChoices: classChoices,
	})
	if err != nil {
		return "", fmt.Errorf("set class: %w", err)
	}

	// Step 5: Set background
	_, err = b.server.CharacterClient.UpdateBackground(b.ctx, &dnd5ev1alpha1.UpdateBackgroundRequest{
		DraftId:    b.draftID,
		Background: b.background,
	})
	if err != nil {
		return "", fmt.Errorf("set background: %w", err)
	}

	// Step 6: Set ability scores
	_, err = b.server.CharacterClient.UpdateAbilityScores(b.ctx, &dnd5ev1alpha1.UpdateAbilityScoresRequest{
		DraftId: b.draftID,
		ScoresInput: &dnd5ev1alpha1.UpdateAbilityScoresRequest_AbilityScores{
			AbilityScores: &dnd5ev1alpha1.AbilityScores{
				Strength:     b.str,
				Dexterity:    b.dex,
				Constitution: b.con,
				Intelligence: b.int_,
				Wisdom:       b.wis,
				Charisma:     b.cha,
			},
		},
	})
	if err != nil {
		return "", fmt.Errorf("set ability scores: %w", err)
	}

	// Step 7: Check what choices still need to be made
	getDraftResp, err := b.server.CharacterClient.GetDraft(b.ctx, &dnd5ev1alpha1.GetDraftRequest{
		DraftId: b.draftID,
	})
	if err != nil {
		return "", fmt.Errorf("get draft: %w", err)
	}

	// Handle any remaining required choices (equipment, etc.)
	if err := b.handleRemainingChoices(getDraftResp.GetDraft()); err != nil {
		return "", fmt.Errorf("handle choices: %w", err)
	}

	// Step 8: Finalize
	finalizeResp, err := b.server.CharacterClient.FinalizeDraft(b.ctx, &dnd5ev1alpha1.FinalizeDraftRequest{
		DraftId: b.draftID,
	})
	if err != nil {
		return "", fmt.Errorf("finalize: %w", err)
	}

	return finalizeResp.GetCharacter().GetId(), nil
}

func (b *CharacterBuilder) buildClassChoices() []*dnd5ev1alpha1.ChoiceData {
	var choices []*dnd5ev1alpha1.ChoiceData

	// Add skills if specified
	if len(b.skills) > 0 {
		choices = append(choices, &dnd5ev1alpha1.ChoiceData{
			Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SKILLS,
			Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
			Selection: &dnd5ev1alpha1.ChoiceData_Skills{
				Skills: &dnd5ev1alpha1.SkillSelection{
					Skills: b.skills,
				},
			},
		})
	}

	// Add fighting style if specified
	if b.fightingStyle != dnd5ev1alpha1.FightingStyle_FIGHTING_STYLE_UNSPECIFIED {
		choices = append(choices, &dnd5ev1alpha1.ChoiceData{
			Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_FIGHTING_STYLE,
			Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
			Selection: &dnd5ev1alpha1.ChoiceData_FightingStyle{
				FightingStyle: &dnd5ev1alpha1.FightingStyleSelection{
					Style: b.fightingStyle,
				},
			},
		})
	}

	return choices
}

func (b *CharacterBuilder) handleRemainingChoices(draft *dnd5ev1alpha1.CharacterDraft) error {
	if draft == nil || draft.Validation == nil {
		return nil
	}

	// Check validation issues to see what's missing.
	// TODO: Auto-handle common equipment choices based on draft.Validation.Issues.
	// For now, we'll need to be explicit about equipment in the builder.
	_ = draft.Validation.Issues

	return nil
}

// =============================================================================
// SHARED ENCOUNTER HELPERS
// These are suite-agnostic: they take a testing.T + TestServer instead of a
// specific suite receiver. Any class integration suite can use them.
// =============================================================================

// CreateEncounterAndStartCombat creates an encounter with the given character,
// sets the player ready, and starts combat. Returns the encounter ID.
func CreateEncounterAndStartCombat(t *testing.T, server *harness.TestServer, ctx context.Context, playerID, characterID string) string {
	t.Helper()

	createResp, err := server.EncounterClient.CreateEncounter(ctx, &dnd5ev1alpha1.CreateEncounterRequest{
		CharacterIds: []string{characterID},
	})
	if err != nil {
		t.Fatalf("failed to create encounter: %v", err)
	}
	encounterID := createResp.GetEncounterId()
	if encounterID == "" {
		t.Fatal("encounter ID should not be empty")
	}

	_, err = server.EncounterClient.SetReady(ctx, &dnd5ev1alpha1.SetReadyRequest{
		EncounterId: encounterID,
		PlayerId:    playerID,
		IsReady:     true,
	})
	if err != nil {
		t.Fatalf("failed to set ready: %v", err)
	}

	_, err = server.EncounterClient.StartCombat(ctx, &dnd5ev1alpha1.StartCombatRequest{
		EncounterId: encounterID,
		Theme:       dnd5ev1alpha1.DungeonTheme_DUNGEON_THEME_CRYPT,
		Difficulty:  dnd5ev1alpha1.DungeonDifficulty_DUNGEON_DIFFICULTY_MEDIUM,
		Length:      dnd5ev1alpha1.DungeonLength_DUNGEON_LENGTH_SHORT,
	})
	if err != nil {
		t.Fatalf("failed to start combat: %v", err)
	}

	return encounterID
}

// FindMonsterTarget finds the first monster in the encounter's turn order.
func FindMonsterTarget(t *testing.T, server *harness.TestServer, ctx context.Context, playerID, encounterID string) string {
	t.Helper()

	stateResp, err := server.EncounterClient.GetEncounterState(ctx, &dnd5ev1alpha1.GetEncounterStateRequest{
		EncounterId: encounterID,
		PlayerId:    playerID,
	})
	if err != nil {
		t.Fatalf("failed to get encounter state: %v", err)
	}

	for _, entry := range stateResp.GetCombatState().GetTurnOrder() {
		if entry.GetEntityType() == "monster" {
			return entry.GetEntityId()
		}
	}

	t.Fatal("no monster found in encounter")
	return ""
}

// FindCharacterInState finds a character by ID in the encounter state response.
// Returns nil if the character is not found.
func FindCharacterInState(resp *dnd5ev1alpha1.GetEncounterStateResponse, characterID string) *dnd5ev1alpha1.Character {
	for _, member := range resp.GetParty() {
		if member.GetCharacter() != nil && member.GetCharacter().GetId() == characterID {
			return member.GetCharacter()
		}
	}
	return nil
}

// =============================================================================
// LEVEL-UP HELPERS
// These patch a level 1 character to a higher level by adding the
// appropriate features, conditions, and resources via the character repo.
// The normal creation pipeline builds level 1; these extend it for testing.
// =============================================================================

// LevelUpMonkToLevel2 patches a level 1 Monk to level 2, adding:
//   - Ki resource (2 points, short rest recovery)
//   - Flurry of Blows, Patient Defense, Step of the Wind features
//   - Unarmored Movement condition (+10 ft at level 2)
func LevelUpMonkToLevel2(t *testing.T, server *harness.TestServer, ctx context.Context, characterID string) {
	t.Helper()

	// 1. Get the character from the repo
	getOutput, err := server.CharacterRepo.Get(ctx, characterrepo.GetInput{ID: characterID})
	if err != nil {
		t.Fatalf("failed to get character for level-up: %v", err)
	}
	char := getOutput.Character
	if char == nil || char.Data == nil {
		t.Fatal("character or character data is nil")
	}

	data := char.Data

	// 2. Bump level
	data.Level = 2

	// 3. Add Ki resource (Monk gets Ki points = Monk level at level 2+)
	if data.Resources == nil {
		data.Resources = make(map[coreResources.ResourceKey]toolkitchar.RecoverableResourceData)
	}
	data.Resources[dnd5eResources.Ki] = toolkitchar.RecoverableResourceData{
		Current:   2,
		Maximum:   2,
		ResetType: coreResources.ResetShortRest,
	}

	// 4. Add Ki features (Flurry of Blows, Patient Defense, Step of the Wind)
	kiFeatures := []json.RawMessage{
		json.RawMessage(fmt.Sprintf(`{
			"ref": {"module": "dnd5e", "type": "features", "id": "flurry_of_blows"},
			"id": "flurry_of_blows",
			"name": "Flurry of Blows",
			"character_id": %q
		}`, characterID)),
		json.RawMessage(fmt.Sprintf(`{
			"ref": {"module": "dnd5e", "type": "features", "id": "patient_defense"},
			"id": "patient_defense",
			"name": "Patient Defense",
			"character_id": %q
		}`, characterID)),
		json.RawMessage(fmt.Sprintf(`{
			"ref": {"module": "dnd5e", "type": "features", "id": "step_of_the_wind"},
			"id": "step_of_the_wind",
			"name": "Step of the Wind",
			"character_id": %q
		}`, characterID)),
	}
	data.Features = append(data.Features, kiFeatures...)

	// 5. Add Martial Arts + Unarmored Movement conditions (Monk level 2)
	// Martial Arts governs DEX-for-attacks and bonus unarmed strike — required for Flurry of Blows.
	// Matches toolkit's canonical createLevel2Monk() test setup.
	martialArts := json.RawMessage(fmt.Sprintf(`{
		"ref": {"module": "dnd5e", "type": "conditions", "id": "martial_arts"},
		"character_id": %q,
		"monk_level": 2
	}`, characterID))
	unarmoredMovement := json.RawMessage(fmt.Sprintf(`{
		"ref": {"module": "dnd5e", "type": "conditions", "id": "unarmored_movement"},
		"character_id": %q,
		"monk_level": 2
	}`, characterID))
	data.Conditions = append(data.Conditions, martialArts, unarmoredMovement)

	// 6. Bump HP for level 2 (1d8 + CON mod; use average: 5 + CON mod)
	// We don't know exact CON mod here, so just add 5 (average d8) + 2 (typical CON mod)
	data.HitPoints += 7
	data.MaxHitPoints += 7

	// 7. Write back
	_, err = server.CharacterRepo.Update(ctx, characterrepo.UpdateInput{Character: char})
	if err != nil {
		t.Fatalf("failed to update character for level-up: %v", err)
	}

	t.Logf("  ✓ Leveled up to 2: Ki 2/2, +3 features, +Unarmored Movement")
}
