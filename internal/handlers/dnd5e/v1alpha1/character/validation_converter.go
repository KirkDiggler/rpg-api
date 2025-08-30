package character

import (
	"fmt"

	pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
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

	// Convert issues - the toolkit ValidationResult has AllIssues array
	issues := make([]*pb.ValidationResult_Issue, 0, len(result.AllIssues))

	for _, issue := range result.AllIssues {
		// Map severity
		var severity pb.ValidationResult_Severity
		switch issue.Severity {
		case choices.SeverityError:
			severity = pb.ValidationResult_SEVERITY_ERROR
		case choices.SeverityIncomplete:
			severity = pb.ValidationResult_SEVERITY_INCOMPLETE
		case choices.SeverityWarning:
			severity = pb.ValidationResult_SEVERITY_WARNING
		default:
			severity = pb.ValidationResult_SEVERITY_UNSPECIFIED
		}

		// Extract details as string array
		var details []string
		if issue.Details != nil {
			for key, value := range issue.Details {
				details = append(details, string(key)+": "+fmt.Sprintf("%v", value))
			}
		}

		issues = append(issues, &pb.ValidationResult_Issue{
			Severity:     severity,
			Source:       mapSourceToProto(issue.Source),
			Field:        mapFieldToProto(issue.Field),
			Message:      issue.Message,
			Details:      details,
			SourceDetail: string(issue.Source),
		})
	}

	return &pb.ValidationResult{
		IsValid:         result.CanFinalize,
		Issues:          issues,
		ErrorCount:      int32(len(result.Errors)),
		IncompleteCount: int32(len(result.Incomplete)),
		WarningCount:    int32(len(result.Warnings)),
	}
}

// mapSourceToProto maps toolkit Source to proto enum
func mapSourceToProto(source choices.Source) pb.ValidationSource {
	switch source {
	case choices.SourceClass:
		return pb.ValidationSource_VALIDATION_SOURCE_CLASS
	case choices.SourceRace:
		return pb.ValidationSource_VALIDATION_SOURCE_RACE
	case choices.SourceBackground:
		return pb.ValidationSource_VALIDATION_SOURCE_BACKGROUND
	default:
		return pb.ValidationSource_VALIDATION_SOURCE_UNSPECIFIED
	}
}

// mapFieldToProto maps toolkit Field to proto enum
func mapFieldToProto(field choices.Field) pb.ValidationField {
	switch field {
	case choices.FieldSkills, choices.FieldRaceSkills, choices.FieldBackgroundSkills:
		return pb.ValidationField_VALIDATION_FIELD_SKILLS
	case choices.FieldLanguages, choices.FieldRaceLanguages:
		return pb.ValidationField_VALIDATION_FIELD_LANGUAGES
	case choices.FieldEquipment:
		return pb.ValidationField_VALIDATION_FIELD_EQUIPMENT
	case choices.FieldSpells:
		return pb.ValidationField_VALIDATION_FIELD_SPELLS
	case choices.FieldCantrips:
		return pb.ValidationField_VALIDATION_FIELD_CANTRIPS
	case choices.FieldTools, choices.FieldInstruments:
		return pb.ValidationField_VALIDATION_FIELD_TOOLS
	case choices.FieldExpertise:
		return pb.ValidationField_VALIDATION_FIELD_EXPERTISE
	case choices.FieldFightingStyle:
		return pb.ValidationField_VALIDATION_FIELD_FIGHTING_STYLE
	case choices.FieldAbilityScores:
		return pb.ValidationField_VALIDATION_FIELD_ABILITY_SCORES
	case choices.FieldDraconicAncestry:
		return pb.ValidationField_VALIDATION_FIELD_DRACONIC_ANCESTRY
	case choices.FieldClass:
		return pb.ValidationField_VALIDATION_FIELD_CLASS
	case choices.FieldRace:
		return pb.ValidationField_VALIDATION_FIELD_RACE
	case choices.FieldBackground:
		return pb.ValidationField_VALIDATION_FIELD_BACKGROUND
	default:
		return pb.ValidationField_VALIDATION_FIELD_UNSPECIFIED
	}
}
