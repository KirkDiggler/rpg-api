// Package character defines the interface for character operations
package character

//go:generate mockgen -destination=mock/mock_service.go -package=charactermock github.com/KirkDiggler/rpg-api/internal/services/character Service

import (
	"context"

	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
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

// CreateDraftInput defines the request for creating a draft
type CreateDraftInput struct {
	PlayerID    string
	SessionID   string // Optional
	InitialData *dnd5e.CharacterDraft
}

// CreateDraftOutput defines the response for creating a draft
type CreateDraftOutput struct {
	Draft *dnd5e.CharacterDraft
}

// GetDraftInput defines the request for getting a draft
type GetDraftInput struct {
	DraftID string
}

// GetDraftOutput defines the response for getting a draft
type GetDraftOutput struct {
	Draft *dnd5e.CharacterDraft
}

// ListDraftsInput defines the request for listing drafts
type ListDraftsInput struct {
	PlayerID  string
	SessionID string // Optional filter
	PageSize  int32
	PageToken string
}

// ListDraftsOutput defines the response for listing drafts
type ListDraftsOutput struct {
	Drafts        []*dnd5e.CharacterDraft
	NextPageToken string
}

// DeleteDraftInput defines the request for deleting a draft
type DeleteDraftInput struct {
	DraftID string
}

// DeleteDraftOutput defines the response for deleting a draft
type DeleteDraftOutput struct {
	Message string
}

// Section update types

// UpdateNameInput defines the request for updating a draft's name
type UpdateNameInput struct {
	DraftID string
	Name    string
}

// UpdateNameOutput defines the response for updating a draft's name
type UpdateNameOutput struct {
	Draft    *dnd5e.CharacterDraft
	Warnings []ValidationWarning
}

// UpdateRaceInput defines the request for updating a draft's race
type UpdateRaceInput struct {
	DraftID   string
	RaceID    string
	SubraceID string // Optional
}

// UpdateRaceOutput defines the response for updating a draft's race
type UpdateRaceOutput struct {
	Draft    *dnd5e.CharacterDraft
	Warnings []ValidationWarning
}

// UpdateClassInput defines the request for updating a draft's class
type UpdateClassInput struct {
	DraftID string
	ClassID string
}

// UpdateClassOutput defines the response for updating a draft's class
type UpdateClassOutput struct {
	Draft    *dnd5e.CharacterDraft
	Warnings []ValidationWarning
}

// UpdateBackgroundInput defines the request for updating a draft's background
type UpdateBackgroundInput struct {
	DraftID      string
	BackgroundID string
}

// UpdateBackgroundOutput defines the response for updating a draft's background
type UpdateBackgroundOutput struct {
	Draft    *dnd5e.CharacterDraft
	Warnings []ValidationWarning
}

// UpdateAbilityScoresInput defines the request for updating a draft's ability scores
type UpdateAbilityScoresInput struct {
	DraftID       string
	AbilityScores dnd5e.AbilityScores
}

// UpdateAbilityScoresOutput defines the response for updating a draft's ability scores
type UpdateAbilityScoresOutput struct {
	Draft    *dnd5e.CharacterDraft
	Warnings []ValidationWarning
}

// UpdateSkillsInput defines the request for updating a draft's skills
type UpdateSkillsInput struct {
	DraftID  string
	SkillIDs []string
}

// UpdateSkillsOutput defines the response for updating a draft's skills
type UpdateSkillsOutput struct {
	Draft    *dnd5e.CharacterDraft
	Warnings []ValidationWarning
}

// Validation types

// ValidateDraftInput defines the request for validating a draft
type ValidateDraftInput struct {
	DraftID string
}

// ValidateDraftOutput defines the response for validating a draft
type ValidateDraftOutput struct {
	IsComplete   bool
	IsValid      bool
	Errors       []ValidationError
	Warnings     []ValidationWarning
	MissingSteps []string
}

// ValidationError defines a validation error
type ValidationError struct {
	Field   string
	Message string
	Type    string
}

// ValidationWarning defines a validation warning
type ValidationWarning struct {
	Field   string
	Message string
	Type    string
}

// Finalization types

// FinalizeDraftInput defines the request for finalizing a draft
type FinalizeDraftInput struct {
	DraftID string
}

// FinalizeDraftOutput defines the response for finalizing a draft
type FinalizeDraftOutput struct {
	Character    *dnd5e.Character
	DraftDeleted bool
}

// Character operation types

// GetCharacterInput defines the request for getting a character
type GetCharacterInput struct {
	CharacterID string
}

// GetCharacterOutput defines the response for getting a character
type GetCharacterOutput struct {
	Character *dnd5e.Character
}

// ListCharactersInput defines the request for listing characters
type ListCharactersInput struct {
	PageSize  int32
	PageToken string
	SessionID string // Optional filter
	PlayerID  string // Optional filter
}

// ListCharactersOutput defines the response for listing characters
type ListCharactersOutput struct {
	Characters    []*dnd5e.Character
	NextPageToken string
	TotalSize     int32
}

// DeleteCharacterInput defines the request for deleting a character
type DeleteCharacterInput struct {
	CharacterID string
}

// DeleteCharacterOutput defines the response for deleting a character
type DeleteCharacterOutput struct {
	Message string
}
