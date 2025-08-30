package character

import (
	"context"
	"fmt"

	"github.com/KirkDiggler/rpg-api/internal/errors"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character/choices"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
)

func (o *Orchestrator) UpdateChoices(ctx context.Context, input *UpdateChoicesInput) (*UpdateChoicesOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) ListChoiceOptions(ctx context.Context, input *ListChoiceOptionsInput) (*ListChoiceOptionsOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}

// validateClassRequirements validates that all required choices for a class are present
// This delegates to the new toolkit choices system which knows the D&D 5e rules
func (o *Orchestrator) validateClassRequirements(ctx context.Context, draft *toolkitchar.DraftData) ([]ValidationWarning, error) {
	// Convert our ChoiceData to the submission format expected by the new system
	// Create typed submissions from draft choices
	submissions := choices.NewTypedSubmissions()
	for _, choice := range draft.Choices {
		// Map to validation submission
		source := mapChoiceSourceToValidationSource(choice.Source)
		field := mapChoiceCategoryToValidationField(choice.Category)

		var values []string
		if choice.SkillSelection != nil {
			values = make([]string, len(choice.SkillSelection))
			for i, skill := range choice.SkillSelection {
				values[i] = string(skill)
			}
		} else if choice.LanguageSelection != nil {
			values = make([]string, len(choice.LanguageSelection))
			for i, lang := range choice.LanguageSelection {
				values[i] = string(lang)
			}
		} else if choice.EquipmentSelection != nil {
			values = choice.EquipmentSelection
		} else if choice.FightingStyleSelection != nil {
			values = []string{*choice.FightingStyleSelection}
		}

		if len(values) > 0 {
			submissions.AddChoice(choices.ChoiceSubmission{
				Source:   source,
				Field:    field,
				ChoiceID: choice.ChoiceID,
				Values:   values,
			})
		}
	}

	// Get the requirements based on whether a subclass is selected
	var reqs *choices.Requirements
	if draft.ClassChoice.SubclassID != "" {
		// If a subclass is selected, get the combined requirements
		reqs = choices.GetSubclassRequirements(draft.ClassChoice.SubclassID)
	} else {
		// Otherwise just get the base class requirements
		reqs = choices.GetClassRequirements(draft.ClassChoice.ClassID)
	}

	// Manually validate using the correct requirements
	result := &choices.ValidationResult{
		CanSave:     true,
		CanFinalize: true,
		IsOptimal:   true,
		Errors:      []choices.ValidationIssue{},
		Incomplete:  []choices.ValidationIssue{},
		Warnings:    []choices.ValidationIssue{},
		AllIssues:   []choices.ValidationIssue{},
	}

	// If we have requirements, validate them
	if reqs != nil {
		// Validate skills
		if reqs.Skills != nil {
			classSkills := submissions.GetValues(choices.SourceClass, choices.FieldSkills)
			if len(classSkills) != reqs.Skills.Count {
				issue := choices.ValidationIssue{
					Field:   choices.FieldSkills,
					Message: fmt.Sprintf("Must choose exactly %d skills, but selected %d", reqs.Skills.Count, len(classSkills)),
				}
				result.Incomplete = append(result.Incomplete, issue)
				result.AllIssues = append(result.AllIssues, issue)
				result.CanFinalize = false
			}
		}

		// We could validate other requirements here too if needed
		// For now, just validate the skill count which is the immediate issue
	}

	// Convert validation result to our warnings
	var warnings []ValidationWarning

	// Add all issues from the ValidationResult
	for _, issue := range result.AllIssues {
		warnings = append(warnings, ValidationWarning{
			Field:   string(issue.Field),
			Message: issue.Message,
		})
	}

	return warnings, nil
}

// convertRequirementsToChoiceData converts the new Requirements format to our internal ChoiceData format
// This creates empty choice templates that the player will fill out
func convertRequirementsToChoiceData(classID string, reqs *choices.Requirements) []toolkitchar.ChoiceData {
	if reqs == nil {
		return nil
	}

	var choiceData []toolkitchar.ChoiceData

	// Skills choice
	if reqs.Skills != nil {
		choiceData = append(choiceData, toolkitchar.ChoiceData{
			Category: shared.ChoiceSkills,
			Source:   shared.SourceClass,
			ChoiceID: "class_skills",
		})
	}

	// Fighting style choice
	if reqs.FightingStyle != nil {
		choiceData = append(choiceData, toolkitchar.ChoiceData{
			Category: shared.ChoiceFightingStyle,
			Source:   shared.SourceClass,
			ChoiceID: "fighting_style",
		})
	}

	// Cantrips choice
	if reqs.Cantrips != nil {
		choiceData = append(choiceData, toolkitchar.ChoiceData{
			Category: shared.ChoiceCantrips,
			Source:   shared.SourceClass,
			ChoiceID: "class_cantrips",
		})
	}

	// Spells choice
	if reqs.Spells != nil {
		choiceData = append(choiceData, toolkitchar.ChoiceData{
			Category: shared.ChoiceSpells,
			Source:   shared.SourceClass,
			ChoiceID: "class_spells",
		})
	}

	// Expertise choice
	if reqs.Expertise != nil {
		choiceData = append(choiceData, toolkitchar.ChoiceData{
			Category: shared.ChoiceExpertise,
			Source:   shared.SourceClass,
			ChoiceID: "expertise",
		})
	}

	// Equipment choices - use format that matches ListClasses: {class}_equipment_{n}
	for i := range reqs.Equipment {
		choiceData = append(choiceData, toolkitchar.ChoiceData{
			Category: shared.ChoiceEquipment,
			Source:   shared.SourceClass,
			ChoiceID: fmt.Sprintf("%s_equipment_%d", classID, i+1),
		})
	}

	// Language choices
	if reqs.Languages != nil {
		choiceData = append(choiceData, toolkitchar.ChoiceData{
			Category: shared.ChoiceLanguages,
			Source:   shared.SourceClass,
			ChoiceID: "class_languages",
		})
	}

	// Tool choices
	if reqs.Tools != nil {
		choiceData = append(choiceData, toolkitchar.ChoiceData{
			Category: shared.ChoiceToolProficiency,
			Source:   shared.SourceClass,
			ChoiceID: "class_tools",
		})
	}

	// Instrument choices
	if reqs.Instruments != nil {
		choiceData = append(choiceData, toolkitchar.ChoiceData{
			Category: shared.ChoiceToolProficiency, // Instruments are a type of tool proficiency
			Source:   shared.SourceClass,
			ChoiceID: "class_instruments",
		})
	}

	return choiceData
}

// mapChoiceSourceToValidationSource maps shared.ChoiceSource to choices.Source
func mapChoiceSourceToValidationSource(source shared.ChoiceSource) choices.Source {
	switch source {
	case shared.SourceClass:
		return choices.SourceClass
	case shared.SourceRace:
		return choices.SourceRace
	case shared.SourceBackground:
		return choices.SourceBackground
	default:
		return choices.SourceInvalid
	}
}

// mapChoiceCategoryToValidationField maps shared.ChoiceCategory to choices.Field
func mapChoiceCategoryToValidationField(category shared.ChoiceCategory) choices.Field {
	switch category {
	case shared.ChoiceSkills:
		return choices.FieldSkills
	case shared.ChoiceLanguages:
		return choices.FieldLanguages
	case shared.ChoiceEquipment:
		return choices.FieldEquipment
	case shared.ChoiceFightingStyle:
		return choices.FieldFightingStyle
	case shared.ChoiceCantrips:
		return choices.FieldCantrips
	case shared.ChoiceSpells:
		return choices.FieldSpells
	case shared.ChoiceExpertise:
		return choices.FieldExpertise
	case shared.ChoiceToolProficiency:
		return choices.FieldTools
	default:
		return choices.FieldInvalid
	}
}

// Proto conversion functions have been moved to the handler layer
// The orchestrator now works exclusively with toolkit types
