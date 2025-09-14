package character

import (
	"context"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/backgrounds"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character/choices"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/races"
)

//go:generate mockgen -destination=mock/mock_service.go -package=charactermock github.com/KirkDiggler/rpg-api/internal/orchestrators/character Service

// Service defines the character orchestrator interface
// Starting minimal - we'll add more operations as needed
type Service interface {
	// Draft lifecycle
	CreateDraft(ctx context.Context, input *CreateDraftInput) (*CreateDraftOutput, error)
	GetDraft(ctx context.Context, input *GetDraftInput) (*GetDraftOutput, error)
	ListDrafts(ctx context.Context, input *ListDraftsInput) (*ListDraftsOutput, error)
	DeleteDraft(ctx context.Context, input *DeleteDraftInput) (*DeleteDraftOutput, error)

	// Requirements - what choices need to be made
	GetRequirements(ctx context.Context, input *GetRequirementsInput) (*GetRequirementsOutput, error)

	// Draft updates with validation
	SetName(ctx context.Context, input *SetNameInput) (*SetNameOutput, error)
	SetRace(ctx context.Context, input *SetRaceInput) (*SetRaceOutput, error)
	SetClass(ctx context.Context, input *SetClassInput) (*SetClassOutput, error)
	SetBackground(ctx context.Context, input *SetBackgroundInput) (*SetBackgroundOutput, error)
	SetAbilityScores(ctx context.Context, input *SetAbilityScoresInput) (*SetAbilityScoresOutput, error)

	// Validation and finalization
	ValidateDraft(ctx context.Context, input *ValidateDraftInput) (*ValidateDraftOutput, error)
	FinalizeDraft(ctx context.Context, input *FinalizeDraftInput) (*FinalizeDraftOutput, error)

	// Character operations
	ListCharacters(ctx context.Context, input *ListCharactersInput) (*ListCharactersOutput, error)

	// Data loading for UI
	ListRaces(ctx context.Context, input *ListRacesInput) (*ListRacesOutput, error)
	ListClasses(ctx context.Context, input *ListClassesInput) (*ListClassesOutput, error)
	ListBackgrounds(ctx context.Context, input *ListBackgroundsInput) (*ListBackgroundsOutput, error)

	// Dice rolling
	RollAbilityScores(ctx context.Context, input *RollAbilityScoresInput) (*RollAbilityScoresOutput, error)
}

// CreateDraftInput creates a new character draft
type CreateDraftInput struct {
	PlayerID  string
	SessionID string // Optional
}

// CreateDraftOutput returns the created draft
type CreateDraftOutput struct {
	Draft *character.DraftData
}

// GetDraftInput gets a draft by ID
type GetDraftInput struct {
	DraftID string
}

// GetDraftOutput returns the draft and its progress
type GetDraftOutput struct {
	Draft    *character.DraftData
	Progress character.Progress
}

// DeleteDraftInput deletes a draft
type DeleteDraftInput struct {
	DraftID string
}

// DeleteDraftOutput confirms deletion
type DeleteDraftOutput struct {
	Success bool
}

// GetRequirementsInput gets requirements for character creation choices
type GetRequirementsInput struct {
	Class classes.Class
	Race  races.Race
	Level int // Default to 1 if not specified
}

// GetRequirementsOutput returns what choices need to be made
type GetRequirementsOutput struct {
	Requirements *choices.Requirements
}

// SetNameInput sets the character name
type SetNameInput struct {
	DraftID string
	Name    string
}

// SetNameOutput returns updated draft
type SetNameOutput struct {
	Draft    *character.DraftData
	Progress character.Progress
}

// SetRaceInput sets the race with choices
type SetRaceInput struct {
	DraftID string
	Input   *character.SetRaceInput
}

// SetRaceOutput returns updated draft
type SetRaceOutput struct {
	Draft      *character.DraftData
	Progress   character.Progress
	Validation *choices.ValidationResult
}

// SetClassInput sets the class with choices
type SetClassInput struct {
	DraftID string
	Input   *character.SetClassInput
}

// SetClassOutput returns updated draft
type SetClassOutput struct {
	Draft      *character.DraftData
	Progress   character.Progress
	Validation *choices.ValidationResult
}

// SetBackgroundInput sets the background with choices
type SetBackgroundInput struct {
	DraftID string
	Input   *character.SetBackgroundInput
}

// SetBackgroundOutput returns updated draft
type SetBackgroundOutput struct {
	Draft      *character.DraftData
	Progress   character.Progress
	Validation *choices.ValidationResult
}

// SetAbilityScoresInput sets ability scores
type SetAbilityScoresInput struct {
	DraftID string
	Input   *character.SetAbilityScoresInput
}

// SetAbilityScoresOutput returns updated draft
type SetAbilityScoresOutput struct {
	Draft    *character.DraftData
	Progress character.Progress
}

// ValidateDraftInput validates a draft
type ValidateDraftInput struct {
	DraftID string
}

// ValidateDraftOutput returns validation results
type ValidateDraftOutput struct {
	Valid      bool
	Progress   character.Progress
	Validation *choices.ValidationResult
}

// FinalizeDraftInput finalizes a draft into a character
type FinalizeDraftInput struct {
	DraftID string
}

// FinalizeDraftOutput returns the created character
type FinalizeDraftOutput struct {
	Character *character.Character
}

// ListRacesInput lists available races
type ListRacesInput struct {
	// Future: pagination
}

// ListRacesOutput returns available races
type ListRacesOutput struct {
	Races []*races.Data // Toolkit Data is self-contained with ID, Name(), Description()
}

// ListClassesInput lists available classes
type ListClassesInput struct {
	// Future: pagination
}

// ListClassesOutput returns available classes
type ListClassesOutput struct {
	Classes []*classes.Data // Toolkit Data is self-contained with ID, Name(), Description()
}

// ListBackgroundsInput lists available backgrounds
type ListBackgroundsInput struct {
	// Future: pagination
}

// ListBackgroundsOutput returns available backgrounds
type ListBackgroundsOutput struct {
	Backgrounds []*backgrounds.Data // Toolkit Data is self-contained with ID, Name(), Description()
}

// RollAbilityScoresInput requests ability score rolls
type RollAbilityScoresInput struct {
	DraftID string
	Method  string // "standard" (4d6 drop lowest), "classic" (3d6), etc.
}

// AbilityScoreRoll represents a single ability score roll
type AbilityScoreRoll struct {
	RollID      string
	Total       int
	Dice        []int
	Dropped     []int
	Description string
}

// RollAbilityScoresOutput returns the rolled scores
type RollAbilityScoresOutput struct {
	Rolls     []AbilityScoreRoll
	SessionID string // For audit trail
}

// ListDraftsInput lists drafts with optional filters
type ListDraftsInput struct {
	PlayerID  string
	SessionID string // Optional filter
	PageSize  int
	PageToken string
}

// ListDraftsOutput returns the draft list
type ListDraftsOutput struct {
	Drafts        []*character.DraftData
	NextPageToken string
}

// ListCharactersInput lists characters with optional filters
type ListCharactersInput struct {
	PlayerID  string
	SessionID string // Optional filter
	PageSize  int
	PageToken string
}

// ListCharactersOutput returns the character list
type ListCharactersOutput struct {
	Characters    []*character.Data
	NextPageToken string
	TotalSize     int
}
