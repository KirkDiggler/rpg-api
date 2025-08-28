package character

// TODO: Uncomment and implement once protos are regenerated with ValidationResult types
// This file provides conversion between toolkit validation and proto validation formats

/*
import (
	pb "github.com/KirkDiggler/rpg-api/gen/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character/choices"
)

// convertToolkitValidationToProto converts toolkit validation results to proto format
func convertToolkitValidationToProto(result *choices.ValidationResult) *pb.ValidationResult {
	if result == nil {
		return &pb.ValidationResult{
			IsValid: true,
			Issues:  []*pb.ValidationResult_Issue{},
		}
	}

	// Convert issues
	issues := make([]*pb.ValidationResult_Issue, 0, len(result.Errors)+len(result.Warnings)+len(result.Incomplete))
	
	// Add errors
	for _, err := range result.Errors {
		issues = append(issues, &pb.ValidationResult_Issue{
			Severity:     pb.ValidationResult_SEVERITY_ERROR,
			Source:       convertSourceToProto(err.Source),
			Field:        convertFieldToProto(err.Field),
			Message:      err.Message,
			Details:      err.Details,
			SourceDetail: string(err.Source), // Keep string representation for detail
		})
	}

	// Add incomplete issues
	for _, inc := range result.Incomplete {
		issues = append(issues, &pb.ValidationResult_Issue{
			Severity:     pb.ValidationResult_SEVERITY_INCOMPLETE,
			Source:       convertSourceToProto(inc.Source),
			Field:        convertFieldToProto(inc.Field),
			Message:      inc.Message,
			Details:      inc.Details,
			SourceDetail: string(inc.Source),
		})
	}

	// Add warnings
	for _, warn := range result.Warnings {
		issues = append(issues, &pb.ValidationResult_Issue{
			Severity:     pb.ValidationResult_SEVERITY_WARNING,
			Source:       convertSourceToProto(warn.Source),
			Field:        convertFieldToProto(warn.Field),
			Message:      warn.Message,
			Details:      warn.Details,
			SourceDetail: string(warn.Source),
		})
	}

	return &pb.ValidationResult{
		IsValid:         result.IsValid(),
		Issues:          issues,
		ErrorCount:      int32(len(result.Errors)),
		IncompleteCount: int32(len(result.Incomplete)),
		WarningCount:    int32(len(result.Warnings)),
	}
}

// convertSourceToProto converts toolkit source to proto enum
func convertSourceToProto(source choices.Source) pb.ValidationSource {
	switch source {
	case choices.SourceRace:
		return pb.ValidationSource_VALIDATION_SOURCE_RACE
	case choices.SourceClass:
		return pb.ValidationSource_VALIDATION_SOURCE_CLASS
	case choices.SourceBackground:
		return pb.ValidationSource_VALIDATION_SOURCE_BACKGROUND
	case choices.SourceAbilityScores:
		return pb.ValidationSource_VALIDATION_SOURCE_ABILITY_SCORES
	case choices.SourceName:
		return pb.ValidationSource_VALIDATION_SOURCE_NAME
	case choices.SourceAlignment:
		return pb.ValidationSource_VALIDATION_SOURCE_ALIGNMENT
	case choices.SourceLevel:
		return pb.ValidationSource_VALIDATION_SOURCE_LEVEL
	default:
		return pb.ValidationSource_VALIDATION_SOURCE_UNSPECIFIED
	}
}

// convertFieldToProto converts toolkit field to proto enum
func convertFieldToProto(field choices.Field) pb.ValidationField {
	switch field {
	case choices.FieldSkills:
		return pb.ValidationField_VALIDATION_FIELD_SKILLS
	case choices.FieldLanguages:
		return pb.ValidationField_VALIDATION_FIELD_LANGUAGES
	case choices.FieldEquipment:
		return pb.ValidationField_VALIDATION_FIELD_EQUIPMENT
	case choices.FieldSpells:
		return pb.ValidationField_VALIDATION_FIELD_SPELLS
	case choices.FieldCantrips:
		return pb.ValidationField_VALIDATION_FIELD_CANTRIPS
	case choices.FieldTools:
		return pb.ValidationField_VALIDATION_FIELD_TOOLS
	case choices.FieldExpertise:
		return pb.ValidationField_VALIDATION_FIELD_EXPERTISE
	case choices.FieldFightingStyle:
		return pb.ValidationField_VALIDATION_FIELD_FIGHTING_STYLE
	case choices.FieldAbilityScores:
		return pb.ValidationField_VALIDATION_FIELD_ABILITY_SCORES
	case choices.FieldName:
		return pb.ValidationField_VALIDATION_FIELD_NAME
	case choices.FieldRace:
		return pb.ValidationField_VALIDATION_FIELD_RACE
	case choices.FieldClass:
		return pb.ValidationField_VALIDATION_FIELD_CLASS
	case choices.FieldBackground:
		return pb.ValidationField_VALIDATION_FIELD_BACKGROUND
	case choices.FieldDraconicAncestry:
		return pb.ValidationField_VALIDATION_FIELD_DRACONIC_ANCESTRY
	default:
		return pb.ValidationField_VALIDATION_FIELD_UNSPECIFIED
	}
}
*/