package character

import (
	"context"
	
	pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/backgrounds"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character/choices"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/languages"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/skills"
)

// validateDraft validates a character draft using the toolkit and returns proto validation
func (o *Orchestrator) validateDraft(ctx context.Context, draft *character.DraftData) *pb.ValidationResult {
	if draft == nil {
		return &pb.ValidationResult{
			IsValid: true,
			Issues:  []*pb.ValidationResult_Issue{},
		}
	}

	// Create typed submissions from draft choices
	submissions := choices.NewTypedSubmissions()
	
	// Add choices from draft
	for _, choice := range draft.Choices {
		// Map source and field from choice metadata
		source := mapChoiceSourceToValidationSource(choice.Source)
		field := mapChoiceCategoryToValidationField(choice.Category)
		
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

	// Convert to proto format
	return convertToolkitValidationToProto(result)
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

// mapProtoChoiceCategoryToShared maps proto ChoiceCategory to shared.ChoiceCategory
func mapProtoChoiceCategoryToShared(category pb.ChoiceCategory) shared.ChoiceCategory {
	switch category {
	case pb.ChoiceCategory_CHOICE_CATEGORY_SKILLS:
		return shared.ChoiceSkills
	case pb.ChoiceCategory_CHOICE_CATEGORY_LANGUAGES:
		return shared.ChoiceLanguages
	case pb.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT:
		return shared.ChoiceEquipment
	case pb.ChoiceCategory_CHOICE_CATEGORY_FIGHTING_STYLE:
		return shared.ChoiceFightingStyle
	case pb.ChoiceCategory_CHOICE_CATEGORY_CANTRIPS:
		return shared.ChoiceCantrips
	case pb.ChoiceCategory_CHOICE_CATEGORY_SPELLS:
		return shared.ChoiceSpells
	default:
		return shared.ChoiceCategory("")
	}
}

// mapProtoChoiceSourceToShared maps proto ChoiceSource to shared.ChoiceSource
func mapProtoChoiceSourceToShared(source pb.ChoiceSource) shared.ChoiceSource {
	switch source {
	case pb.ChoiceSource_CHOICE_SOURCE_CLASS:
		return shared.SourceClass
	case pb.ChoiceSource_CHOICE_SOURCE_RACE:
		return shared.SourceRace
	case pb.ChoiceSource_CHOICE_SOURCE_BACKGROUND:
		return shared.SourceBackground
	default:
		// TODO: Add shared.ChoiceSourceInvalid to toolkit
		return shared.ChoiceSource("invalid")
	}
}

// attachValidationToDraft adds validation to a draft for responses
func (o *Orchestrator) attachValidationToDraft(ctx context.Context, draft *pb.CharacterDraft) {
	if draft == nil {
		return
	}

	// Convert proto draft to toolkit DraftData for validation
	draftData := &character.DraftData{
		ID:       draft.Id,
		PlayerID: draft.PlayerId,
		Name:     draft.Name,
	}

	// Map proto enums to toolkit types
	if draft.RaceId != pb.Race_RACE_UNSPECIFIED {
		draftData.RaceChoice = character.RaceChoice{
			RaceID: convertProtoRaceToToolkit(draft.RaceId),
		}
	}

	if draft.ClassId != pb.Class_CLASS_UNSPECIFIED {
		draftData.ClassChoice = character.ClassChoice{
			ClassID: convertProtoClassToToolkit(draft.ClassId),
		}
	}

	if draft.BackgroundId != pb.Background_BACKGROUND_UNSPECIFIED {
		draftData.BackgroundChoice = backgrounds.Background(convertProtoBackgroundToString(draft.BackgroundId))
	}

	// Map proto choices to toolkit choices
	for _, choice := range draft.Choices {
		// Map to toolkit ChoiceData
		choiceData := character.ChoiceData{
			Category: mapProtoChoiceCategoryToShared(choice.Category),
			Source:   mapProtoChoiceSourceToShared(choice.Source),
			ChoiceID: choice.ChoiceId,
		}
		
		// Convert proto selections to toolkit types using proper converters
		switch s := choice.Selection.(type) {
		case *pb.ChoiceData_Skills:
			if s.Skills != nil && s.Skills.Skills != nil {
				choiceData.SkillSelection = make([]skills.Skill, len(s.Skills.Skills))
				for i, skill := range s.Skills.Skills {
					choiceData.SkillSelection[i] = convertProtoSkillToToolkit(skill)
				}
			}
		case *pb.ChoiceData_Languages:
			if s.Languages != nil && s.Languages.Languages != nil {
				choiceData.LanguageSelection = make([]languages.Language, len(s.Languages.Languages))
				for i, lang := range s.Languages.Languages {
					choiceData.LanguageSelection[i] = convertProtoLanguageToToolkit(lang)
				}
			}
		case *pb.ChoiceData_Equipment:
			if s.Equipment != nil && s.Equipment.Items != nil {
				choiceData.EquipmentSelection = s.Equipment.Items
			}
		case *pb.ChoiceData_FightingStyle:
			if s.FightingStyle != "" {
				choiceData.FightingStyleSelection = &s.FightingStyle
			}
		case *pb.ChoiceData_Spells:
			if s.Spells != nil && s.Spells.Spells != nil {
				choiceData.SpellSelection = s.Spells.Spells
			}
		case *pb.ChoiceData_Cantrips:
			if s.Cantrips != nil && s.Cantrips.Cantrips != nil {
				choiceData.CantripSelection = s.Cantrips.Cantrips
			}
		}
		
		draftData.Choices = append(draftData.Choices, choiceData)
	}

	// Validate and attach
	draft.Validation = o.validateDraft(ctx, draftData)
}