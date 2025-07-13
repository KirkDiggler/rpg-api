package character

//go:generate mockgen -destination=mock/mock_service.go -package=charactermock github.com/KirkDiggler/rpg-api/internal/services/character Service

import (
	"context"

	"github.com/KirkDiggler/rpg-api/internal/entities"
)

// Service defines the interface for character operations
type Service interface {
	// Draft lifecycle
	CreateDraft(ctx context.Context, input *CreateDraftInput) (*CreateDraftOutput, error)
	GetDraft(ctx context.Context, input *GetDraftInput) (*GetDraftOutput, error)
	ListDrafts(ctx context.Context, input *ListDraftsInput) (*ListDraftsOutput, error)
	DeleteDraft(ctx context.Context, input *DeleteDraftInput) (*DeleteDraftOutput, error)

	// Section-based updates
	UpdateName(ctx context.Context, input *UpdateNameInput) (*UpdateNameOutput, error)
	UpdateRace(ctx context.Context, input *UpdateRaceInput) (*UpdateRaceOutput, error)
	UpdateClass(ctx context.Context, input *UpdateClassInput) (*UpdateClassOutput, error)
	UpdateBackground(ctx context.Context, input *UpdateBackgroundInput) (*UpdateBackgroundOutput, error)
	UpdateAbilityScores(ctx context.Context, input *UpdateAbilityScoresInput) (*UpdateAbilityScoresOutput, error)
	UpdateSkills(ctx context.Context, input *UpdateSkillsInput) (*UpdateSkillsOutput, error)

	// Validation
	ValidateDraft(ctx context.Context, input *ValidateDraftInput) (*ValidateDraftOutput, error)

	// Character finalization
	FinalizeDraft(ctx context.Context, input *FinalizeDraftInput) (*FinalizeDraftOutput, error)

	// Completed character operations
	GetCharacter(ctx context.Context, input *GetCharacterInput) (*GetCharacterOutput, error)
	ListCharacters(ctx context.Context, input *ListCharactersInput) (*ListCharactersOutput, error)
	DeleteCharacter(ctx context.Context, input *DeleteCharacterInput) (*DeleteCharacterOutput, error)
}

// Draft lifecycle types

type CreateDraftInput struct {
	PlayerID    string
	SessionID   string // Optional
	InitialData *entities.CharacterDraft
}

type CreateDraftOutput struct {
	Draft *entities.CharacterDraft
}

type GetDraftInput struct {
	DraftID string
}

type GetDraftOutput struct {
	Draft *entities.CharacterDraft
}

type ListDraftsInput struct {
	PlayerID  string
	SessionID string // Optional filter
	PageSize  int32
	PageToken string
}

type ListDraftsOutput struct {
	Drafts        []*entities.CharacterDraft
	NextPageToken string
}

type DeleteDraftInput struct {
	DraftID string
}

type DeleteDraftOutput struct {
	Message string
}

// Section update types

type UpdateNameInput struct {
	DraftID string
	Name    string
}

type UpdateNameOutput struct {
	Draft    *entities.CharacterDraft
	Warnings []ValidationWarning
}

type UpdateRaceInput struct {
	DraftID   string
	RaceID    string
	SubraceID string // Optional
}

type UpdateRaceOutput struct {
	Draft    *entities.CharacterDraft
	Warnings []ValidationWarning
}

type UpdateClassInput struct {
	DraftID string
	ClassID string
}

type UpdateClassOutput struct {
	Draft    *entities.CharacterDraft
	Warnings []ValidationWarning
}

type UpdateBackgroundInput struct {
	DraftID      string
	BackgroundID string
}

type UpdateBackgroundOutput struct {
	Draft    *entities.CharacterDraft
	Warnings []ValidationWarning
}

type UpdateAbilityScoresInput struct {
	DraftID       string
	AbilityScores entities.AbilityScores
}

type UpdateAbilityScoresOutput struct {
	Draft    *entities.CharacterDraft
	Warnings []ValidationWarning
}

type UpdateSkillsInput struct {
	DraftID  string
	SkillIDs []string
}

type UpdateSkillsOutput struct {
	Draft    *entities.CharacterDraft
	Warnings []ValidationWarning
}

// Validation types

type ValidateDraftInput struct {
	DraftID string
}

type ValidateDraftOutput struct {
	IsComplete   bool
	IsValid      bool
	Errors       []ValidationError
	Warnings     []ValidationWarning
	MissingSteps []string
}

type ValidationError struct {
	Field   string
	Message string
	Type    string
}

type ValidationWarning struct {
	Field   string
	Message string
	Type    string
}

// Finalization types

type FinalizeDraftInput struct {
	DraftID string
}

type FinalizeDraftOutput struct {
	Character    *entities.Character
	DraftDeleted bool
}

// Character operation types

type GetCharacterInput struct {
	CharacterID string
}

type GetCharacterOutput struct {
	Character *entities.Character
}

type ListCharactersInput struct {
	PageSize  int32
	PageToken string
	SessionID string // Optional filter
	PlayerID  string // Optional filter
}

type ListCharactersOutput struct {
	Characters    []*entities.Character
	NextPageToken string
	TotalSize     int32
}

type DeleteCharacterInput struct {
	CharacterID string
}

type DeleteCharacterOutput struct {
	Message string
}
