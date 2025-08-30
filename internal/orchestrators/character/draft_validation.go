package character

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character/choices"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
)

// validateDraft validates a character draft using the toolkit and returns validation result
func (o *Orchestrator) validateDraft(ctx context.Context, draft *character.DraftData) *choices.ValidationResult {
	if draft == nil {
		return choices.NewValidationResult()
	}

	// Create typed submissions from draft choices
	submissions := choices.NewTypedSubmissions()

	// Add choices from draft
	for _, choice := range draft.Choices {
		// Map source and field from choice metadata
		source := mapChoiceSourceToValidationSource(choice.Source)
		field := mapChoiceCategoryToValidationField(choice.Category)

		// For equipment choices, map to the field format the validator expects
		// e.g., "rogue_equipment_1" -> "equipment_choice_0"
		if choice.Category == shared.ChoiceEquipment && strings.Contains(choice.ChoiceID, "_equipment_") {
			// Extract the number from "{class}_equipment_{n}"
			parts := strings.Split(choice.ChoiceID, "_equipment_")
			if len(parts) == 2 {
				if num, err := strconv.Atoi(parts[1]); err == nil {
					// Convert 1-based to 0-based and use expected field format
					field = choices.Field(fmt.Sprintf("equipment_choice_%d", num-1))
				}
			}
		}

		// Extract values based on choice type
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

	if draft.BackgroundChoice != "" {
		// Background skills are automatic, not choices
		// But we could add tool/language choices if they exist
	}

	// Create validation context
	context := choices.NewValidationContext()
	// TODO: Populate context with automatic grants from race/class/background

	// Validate based on draft selections
	var result *choices.ValidationResult
	if draft.ClassChoice.ClassID != "" && draft.RaceChoice.RaceID != "" {
		// Full validation with class, race and background
		result = choices.Validate(
			draft.ClassChoice.ClassID,
			draft.RaceChoice.RaceID,
			draft.BackgroundChoice,
			1, // Level 1 for new characters
			submissions,
			context,
		)
	} else if draft.ClassChoice.ClassID != "" {
		// Class-only validation
		result = choices.ValidateClassChoices(draft.ClassChoice.ClassID, 1, submissions, context)
	} else if draft.RaceChoice.RaceID != "" {
		// Race-only validation
		result = choices.ValidateRaceChoices(draft.RaceChoice.RaceID, submissions, context)
	} else {
		// No selections yet, return incomplete status
		result = choices.NewValidationResult()
		result.AddIssue(choices.ValidationIssue{
			Severity: choices.SeverityIncomplete,
			Field:    choices.FieldClass,
			Message:  "Class selection required",
		})
		result.AddIssue(choices.ValidationIssue{
			Severity: choices.SeverityIncomplete,
			Field:    choices.FieldRace,
			Message:  "Race selection required",
		})
	}

	// Return the toolkit validation result directly
	return result
}
