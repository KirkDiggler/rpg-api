package character

// TODO: Uncomment and implement once protos are regenerated with ValidationResult types
// This file provides draft validation using the toolkit's 3-tier validation system

/*
import (
	"context"
	
	pb "github.com/KirkDiggler/rpg-api/gen/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character/choices"
)

// validateDraft validates a character draft using the toolkit and returns proto validation
func (o *Orchestrator) validateDraft(ctx context.Context, draft *dnd5e.CharacterDraftData) *pb.ValidationResult {
	if draft == nil {
		return &pb.ValidationResult{
			IsValid: true,
			Issues:  []*pb.ValidationResult_Issue{},
		}
	}

	// Create validation context
	validationCtx := choices.NewValidationContext()
	
	// Set draft vs finalized mode
	validationCtx.SetDraftMode(true) // Allow incomplete choices for drafts
	
	// Create typed submissions from draft choices
	submissions := choices.NewTypedSubmissions()
	
	// Add choices from draft
	if draft.ClassChoice != nil {
		// Add class skills if present
		if draft.ClassChoice.SkillChoices != nil {
			submissions.AddSkills(choices.SourceClass, draft.ClassChoice.SkillChoices)
		}
		// Add equipment choices
		for _, equip := range draft.ClassChoice.EquipmentChoices {
			submissions.AddEquipment(choices.SourceClass, equip)
		}
		// Add fighting style if present
		if draft.ClassChoice.FightingStyle != "" {
			submissions.AddFightingStyle(draft.ClassChoice.FightingStyle)
		}
	}

	if draft.RaceChoice != nil {
		// Add race language choices
		if draft.RaceChoice.LanguageChoices != nil {
			submissions.AddLanguages(choices.SourceRace, draft.RaceChoice.LanguageChoices)
		}
		// Add race skill choices
		if draft.RaceChoice.SkillChoices != nil {
			submissions.AddSkills(choices.SourceRace, draft.RaceChoice.SkillChoices)
		}
	}

	if draft.BackgroundChoice != "" {
		// Background skills are automatic, not choices
		// But we could add tool/language choices if they exist
	}

	// Get requirements based on draft selections
	var requirements *choices.Requirements
	if draft.ClassChoice != nil && draft.RaceChoice != nil {
		requirements = choices.GetRequirements(
			draft.ClassChoice.ClassID,
			draft.RaceChoice.RaceID,
			1, // Level 1 for new characters
		)
	} else if draft.ClassChoice != nil {
		requirements = choices.GetClassRequirements(draft.ClassChoice.ClassID, 1)
	} else if draft.RaceChoice != nil {
		requirements = choices.GetRaceRequirements(draft.RaceChoice.RaceID)
	}

	// Validate if we have requirements
	var result *choices.ValidationResult
	if requirements != nil {
		validator := choices.NewValidator(validationCtx)
		result = validator.Validate(requirements, submissions)
	} else {
		// No requirements yet, return valid but incomplete
		result = choices.NewValidationResult()
		result.AddIssue(choices.ValidationIssue{
			Severity: choices.SeverityIncomplete,
			Source:   choices.SourceClass,
			Field:    choices.FieldClass,
			Message:  "Class selection required",
		})
		result.AddIssue(choices.ValidationIssue{
			Severity: choices.SeverityIncomplete,
			Source:   choices.SourceRace,
			Field:    choices.FieldRace,
			Message:  "Race selection required",
		})
	}

	// Convert to proto format
	return convertToolkitValidationToProto(result)
}

// attachValidationToDraft adds validation to a draft for responses
func (o *Orchestrator) attachValidationToDraft(ctx context.Context, draft *pb.CharacterDraft) {
	if draft == nil {
		return
	}

	// Convert proto draft to entity for validation
	draftData := &dnd5e.CharacterDraftData{
		ID:       draft.Id,
		PlayerID: draft.PlayerId,
	}

	// Map proto enums to entity types
	if draft.RaceId != pb.Race_RACE_UNSPECIFIED {
		draftData.RaceChoice = &dnd5e.RaceChoice{
			RaceID: convertProtoRaceToToolkit(draft.RaceId),
		}
	}

	if draft.ClassId != pb.Class_CLASS_UNSPECIFIED {
		draftData.ClassChoice = &dnd5e.ClassChoice{
			ClassID: convertProtoClassToToolkit(draft.ClassId),
		}
	}

	if draft.BackgroundId != pb.Background_BACKGROUND_UNSPECIFIED {
		draftData.BackgroundChoice = convertProtoBackgroundToString(draft.BackgroundId)
	}

	// Map choices
	for _, choice := range draft.Choices {
		// Convert proto choices to entity choices
		// This would need more detailed mapping based on choice types
	}

	// Validate and attach
	draft.Validation = o.validateDraft(ctx, draftData)
}
*/