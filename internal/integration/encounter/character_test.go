// Copyright (C) 2024 Kirk Diggler
// SPDX-License-Identifier: GPL-3.0-or-later

// Package encounter_integration provides character creation integration tests.
package encounter_integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc/metadata"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/integration/harness"
)

// CharacterCreationSuite tests the full character creation flow.
type CharacterCreationSuite struct {
	suite.Suite
	ctx    context.Context
	cancel context.CancelFunc
	server *harness.TestServer
}

func TestCharacterCreationSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	suite.Run(t, new(CharacterCreationSuite))
}

func (s *CharacterCreationSuite) SetupSuite() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 2*time.Minute)

	var err error
	s.server, err = harness.New(s.ctx, nil)
	s.Require().NoError(err, "failed to create test server")

	s.T().Log("╔══════════════════════════════════════════════════════════════════╗")
	s.T().Log("║  Character Creation Integration Tests                            ║")
	s.T().Log("╚══════════════════════════════════════════════════════════════════╝")
}

func (s *CharacterCreationSuite) TearDownSuite() {
	if s.server != nil {
		s.server.Close()
	}
	if s.cancel != nil {
		s.cancel()
	}
}

func (s *CharacterCreationSuite) SetupTest() {
	err := s.server.FlushRedis(s.ctx)
	s.Require().NoError(err, "failed to flush redis")
}

func (s *CharacterCreationSuite) authCtx(playerID string) context.Context {
	return metadata.AppendToOutgoingContext(s.ctx, "authorization", "Dev "+playerID)
}

// =============================================================================
// DRAFT INSPECTION TESTS
// =============================================================================

// TestInspectDraftChoices creates a draft and inspects what choices are generated.
// This is a diagnostic test to understand the choice structure.
func (s *CharacterCreationSuite) TestInspectDraftChoices() {
	s.T().Log("Inspecting draft choices for Fighter...")

	ctx := s.authCtx("test-player-inspect")

	// Create draft
	createResp, err := s.server.CharacterClient.CreateDraft(ctx, &dnd5ev1alpha1.CreateDraftRequest{})
	s.Require().NoError(err)
	draftID := createResp.GetDraft().GetId()

	// Set race (Human - no subrace choices)
	_, err = s.server.CharacterClient.UpdateRace(ctx, &dnd5ev1alpha1.UpdateRaceRequest{
		DraftId: draftID,
		Race:    dnd5ev1alpha1.Race_RACE_HUMAN,
	})
	s.Require().NoError(err)

	// Set class (Fighter) with skills
	_, err = s.server.CharacterClient.UpdateClass(ctx, &dnd5ev1alpha1.UpdateClassRequest{
		DraftId: draftID,
		Class:   dnd5ev1alpha1.Class_CLASS_FIGHTER,
		ClassChoices: []*dnd5ev1alpha1.ChoiceData{
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
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_FIGHTING_STYLE,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				Selection: &dnd5ev1alpha1.ChoiceData_FightingStyle{
					FightingStyle: &dnd5ev1alpha1.FightingStyleSelection{
						Style: dnd5ev1alpha1.FightingStyle_FIGHTING_STYLE_DEFENSE,
					},
				},
			},
		},
	})
	s.Require().NoError(err)

	// Get the full draft to see what choices remain
	getDraftResp, err := s.server.CharacterClient.GetDraft(ctx, &dnd5ev1alpha1.GetDraftRequest{
		DraftId: draftID,
	})
	s.Require().NoError(err)

	draft := getDraftResp.GetDraft()
	s.T().Logf("Draft ID: %s", draft.GetId())
	s.T().Logf("Race: %s", draft.GetRace())
	s.T().Logf("Class: %s", draft.GetClass())

	// Log all choices
	s.T().Logf("\n📋 Choices (%d):", len(draft.GetChoices()))
	for _, choice := range draft.GetChoices() {
		s.T().Logf("  • [%s] %s from %s", choice.GetCategory(), choice.GetChoiceId(), choice.GetSource())
	}

	// Log validation issues
	if draft.GetValidation() != nil && len(draft.GetValidation().GetIssues()) > 0 {
		s.T().Logf("\n⚠️  Validation Issues (%d):", len(draft.GetValidation().GetIssues()))
		for _, issue := range draft.GetValidation().GetIssues() {
			s.T().Logf("  • [%s:%s] %s - %s", issue.GetSeverity(), issue.GetField(), issue.GetMessage(), issue.GetSource())
		}
	} else {
		s.T().Log("\n✅ No validation issues")
	}
}

// TestInspectBarbarianChoices shows what choices a barbarian needs.
func (s *CharacterCreationSuite) TestInspectBarbarianChoices() {
	s.T().Log("Inspecting draft choices for Barbarian...")

	ctx := s.authCtx("test-player-barbarian")

	// Create draft
	createResp, err := s.server.CharacterClient.CreateDraft(ctx, &dnd5ev1alpha1.CreateDraftRequest{})
	s.Require().NoError(err)
	draftID := createResp.GetDraft().GetId()

	// Set Human with language choice (humans get 1 extra language)
	_, err = s.server.CharacterClient.UpdateRace(ctx, &dnd5ev1alpha1.UpdateRaceRequest{
		DraftId: draftID,
		Race:    dnd5ev1alpha1.Race_RACE_HUMAN,
		RaceChoices: []*dnd5ev1alpha1.ChoiceData{
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_LANGUAGES,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_RACE,
				Selection: &dnd5ev1alpha1.ChoiceData_Languages{
					Languages: &dnd5ev1alpha1.LanguageSelection{
						Languages: []dnd5ev1alpha1.Language{
							dnd5ev1alpha1.Language_LANGUAGE_ORC, // Useful for a barbarian!
						},
					},
				},
			},
		},
	})
	s.Require().NoError(err)

	// Set Barbarian with skills AND equipment choices
	// Barbarian equipment choices:
	// - barbarian-weapons-primary: greataxe (barbarian-weapon-a) or martial (barbarian-weapon-b)
	// - barbarian-weapons-secondary: 2 handaxes (barbarian-secondary-a) or simple weapon (barbarian-secondary-b)
	_, err = s.server.CharacterClient.UpdateClass(ctx, &dnd5ev1alpha1.UpdateClassRequest{
		DraftId: draftID,
		Class:   dnd5ev1alpha1.Class_CLASS_BARBARIAN,
		ClassChoices: []*dnd5ev1alpha1.ChoiceData{
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SKILLS,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				Selection: &dnd5ev1alpha1.ChoiceData_Skills{
					Skills: &dnd5ev1alpha1.SkillSelection{
						Skills: []dnd5ev1alpha1.Skill{
							dnd5ev1alpha1.Skill_SKILL_ATHLETICS,
							dnd5ev1alpha1.Skill_SKILL_INTIMIDATION,
						},
					},
				},
			},
			// Primary weapon: Greataxe (bundle choice - equipment field required but empty)
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "barbarian-weapons-primary",
				OptionId: "barbarian-weapon-a", // Greataxe
				Selection: &dnd5ev1alpha1.ChoiceData_Equipment{
					Equipment: &dnd5ev1alpha1.EquipmentSelection{}, // Empty - bundle provides items
				},
			},
			// Secondary weapon: 2 handaxes
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "barbarian-weapons-secondary",
				OptionId: "barbarian-secondary-a", // 2 handaxes
				Selection: &dnd5ev1alpha1.ChoiceData_Equipment{
					Equipment: &dnd5ev1alpha1.EquipmentSelection{},
				},
			},
			// Pack: Explorer's pack
			{
				Category: dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
				Source:   dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS,
				ChoiceId: "barbarian-pack",
				OptionId: "barbarian-pack-a", // Explorer's pack
				Selection: &dnd5ev1alpha1.ChoiceData_Equipment{
					Equipment: &dnd5ev1alpha1.EquipmentSelection{},
				},
			},
		},
	})
	s.Require().NoError(err)

	// Set background
	_, err = s.server.CharacterClient.UpdateBackground(ctx, &dnd5ev1alpha1.UpdateBackgroundRequest{
		DraftId:    draftID,
		Background: dnd5ev1alpha1.Background_BACKGROUND_OUTLANDER,
	})
	s.Require().NoError(err)

	// Set ability scores
	_, err = s.server.CharacterClient.UpdateAbilityScores(ctx, &dnd5ev1alpha1.UpdateAbilityScoresRequest{
		DraftId: draftID,
		ScoresInput: &dnd5ev1alpha1.UpdateAbilityScoresRequest_AbilityScores{
			AbilityScores: &dnd5ev1alpha1.AbilityScores{
				Strength:     15,
				Dexterity:    13,
				Constitution: 14,
				Intelligence: 8,
				Wisdom:       12,
				Charisma:     10,
			},
		},
	})
	s.Require().NoError(err)

	// Set name
	_, err = s.server.CharacterClient.UpdateName(ctx, &dnd5ev1alpha1.UpdateNameRequest{
		DraftId: draftID,
		Name:    "Grog",
	})
	s.Require().NoError(err)

	// Get the full draft
	getDraftResp, err := s.server.CharacterClient.GetDraft(ctx, &dnd5ev1alpha1.GetDraftRequest{
		DraftId: draftID,
	})
	s.Require().NoError(err)

	draft := getDraftResp.GetDraft()

	// Log all choices to understand what's needed
	s.T().Logf("\n📋 Draft Choices (%d):", len(draft.GetChoices()))
	for _, choice := range draft.GetChoices() {
		s.T().Logf("  • [%s] ID: %s, Source: %s, OptionID: %s",
			choice.GetCategory(),
			choice.GetChoiceId(),
			choice.GetSource(),
			choice.GetOptionId())
	}

	// Log validation issues
	if draft.GetValidation() != nil && len(draft.GetValidation().GetIssues()) > 0 {
		s.T().Logf("\n⚠️  Validation Issues (%d):", len(draft.GetValidation().GetIssues()))
		for _, issue := range draft.GetValidation().GetIssues() {
			s.T().Logf("  • [%s:%s] %s - %s", issue.GetSeverity(), issue.GetField(), issue.GetMessage(), issue.GetSource())
		}
	} else {
		s.T().Log("\n✅ No validation issues - attempting finalize...")

		// Finalize should succeed when there are no validation issues
		finalizeResp, err := s.server.CharacterClient.FinalizeDraft(ctx, &dnd5ev1alpha1.FinalizeDraftRequest{
			DraftId: draftID,
		})
		s.Require().NoError(err, "finalize should succeed with no validation issues")
		s.Require().NotEmpty(finalizeResp.GetCharacter().GetId(), "finalized character should have an ID")
		s.T().Logf("✅ Character created: %s", finalizeResp.GetCharacter().GetId())
	}
}
