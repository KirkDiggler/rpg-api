package character

import "github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"

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

// Data loading types for character creation UI

// ListRacesInput defines the request for listing races
type ListRacesInput struct {
	PageSize        int32
	PageToken       string
	IncludeSubraces bool
}

// ListRacesOutput defines the response for listing races
type ListRacesOutput struct {
	Races         []*dnd5e.RaceInfo
	NextPageToken string
	TotalSize     int32
}

// ListClassesInput defines the request for listing classes
type ListClassesInput struct {
	PageSize                int32
	PageToken               string
	IncludeSpellcastersOnly bool
	IncludeFeatures         bool
}

// ListClassesOutput defines the response for listing classes
type ListClassesOutput struct {
	Classes       []*dnd5e.ClassInfo
	NextPageToken string
	TotalSize     int32
}

// ListBackgroundsInput defines the request for listing backgrounds
type ListBackgroundsInput struct {
	PageSize  int32
	PageToken string
}

// ListBackgroundsOutput defines the response for listing backgrounds
type ListBackgroundsOutput struct {
	Backgrounds   []*dnd5e.BackgroundInfo
	NextPageToken string
	TotalSize     int32
}

// ListSpellsInput defines the request for listing spells
type ListSpellsInput struct {
	PageSize   int32
	PageToken  string
	Level      *int32   // Optional filter by spell level (0-9)
	School     string   // Optional filter by school
	ClassID    string   // Optional filter by class
	SearchTerm string   // Optional search term for name/description
}

// ListSpellsOutput defines the response for listing spells
type ListSpellsOutput struct {
	Spells        []*dnd5e.SpellInfo
	NextPageToken string
	TotalSize     int32
}

// GetRaceDetailsInput defines the request for getting race details
type GetRaceDetailsInput struct {
	RaceID string
}

// GetRaceDetailsOutput defines the response for getting race details
type GetRaceDetailsOutput struct {
	Race *dnd5e.RaceInfo
}

// GetClassDetailsInput defines the request for getting class details
type GetClassDetailsInput struct {
	ClassID string
}

// GetClassDetailsOutput defines the response for getting class details
type GetClassDetailsOutput struct {
	Class *dnd5e.ClassInfo
}

// GetBackgroundDetailsInput defines the request for getting background details
type GetBackgroundDetailsInput struct {
	BackgroundID string
}

// GetBackgroundDetailsOutput defines the response for getting background details
type GetBackgroundDetailsOutput struct {
	Background *dnd5e.BackgroundInfo
}

// Dice rolling types

// RollAbilityScoresInput defines the request for rolling ability scores for character creation
type RollAbilityScoresInput struct {
	DraftID string
	Method  string // "4d6_drop_lowest", "3d6", "point_buy", etc.
}

// RollAbilityScoresOutput defines the response for rolling ability scores
type RollAbilityScoresOutput struct {
	Rolls     []*AbilityScoreRoll
	SessionID string
	ExpiresAt int64
}

// AbilityScoreRoll represents a single ability score roll with ID and value
type AbilityScoreRoll struct {
	ID          string
	Value       int32
	Description string
	RolledAt    int64
	Dice        []int32
	Dropped     []int32
	Notation    string
}
