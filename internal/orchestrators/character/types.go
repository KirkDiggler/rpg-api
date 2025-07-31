package character

import (
	"context"

	"github.com/KirkDiggler/rpg-api/internal/clients/external"
	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/class"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/race"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
)

//go:generate mockgen -destination=mock/mock_service.go -package=charactermock github.com/KirkDiggler/rpg-api/internal/orchestrators/character Service

// Service defines the character orchestrator interface
type Service interface {
	// Draft lifecycle
	CreateDraft(ctx context.Context, input *CreateDraftInput) (*CreateDraftOutput, error)
	GetDraft(ctx context.Context, input *GetDraftInput) (*GetDraftOutput, error)
	ListDrafts(ctx context.Context, input *ListDraftsInput) (*ListDraftsOutput, error)
	DeleteDraft(ctx context.Context, input *DeleteDraftInput) (*DeleteDraftOutput, error)

	// Draft updates
	UpdateName(ctx context.Context, input *UpdateNameInput) (*UpdateNameOutput, error)
	UpdateRace(ctx context.Context, input *UpdateRaceInput) (*UpdateRaceOutput, error)
	UpdateClass(ctx context.Context, input *UpdateClassInput) (*UpdateClassOutput, error)
	UpdateBackground(ctx context.Context, input *UpdateBackgroundInput) (*UpdateBackgroundOutput, error)
	UpdateAbilityScores(ctx context.Context, input *UpdateAbilityScoresInput) (*UpdateAbilityScoresOutput, error)
	UpdateSkills(ctx context.Context, input *UpdateSkillsInput) (*UpdateSkillsOutput, error)
	UpdateChoices(ctx context.Context, input *UpdateChoicesInput) (*UpdateChoicesOutput, error)

	// Validation and finalization
	ValidateDraft(ctx context.Context, input *ValidateDraftInput) (*ValidateDraftOutput, error)
	FinalizeDraft(ctx context.Context, input *FinalizeDraftInput) (*FinalizeDraftOutput, error)

	// Character operations
	GetCharacter(ctx context.Context, input *GetCharacterInput) (*GetCharacterOutput, error)
	ListCharacters(ctx context.Context, input *ListCharactersInput) (*ListCharactersOutput, error)
	DeleteCharacter(ctx context.Context, input *DeleteCharacterInput) (*DeleteCharacterOutput, error)

	// Data loading for UI
	ListRaces(ctx context.Context, input *ListRacesInput) (*ListRacesOutput, error)
	ListClasses(ctx context.Context, input *ListClassesInput) (*ListClassesOutput, error)
	ListBackgrounds(ctx context.Context, input *ListBackgroundsInput) (*ListBackgroundsOutput, error)
	GetRaceDetails(ctx context.Context, input *GetRaceDetailsInput) (*GetRaceDetailsOutput, error)
	GetClassDetails(ctx context.Context, input *GetClassDetailsInput) (*GetClassDetailsOutput, error)
	GetBackgroundDetails(ctx context.Context, input *GetBackgroundDetailsInput) (*GetBackgroundDetailsOutput, error)
	ListChoiceOptions(ctx context.Context, input *ListChoiceOptionsInput) (*ListChoiceOptionsOutput, error)

	// Additional operations
	RollAbilityScores(ctx context.Context, input *RollAbilityScoresInput) (*RollAbilityScoresOutput, error)
	
	// Additional operations
	GetDraftPreview(ctx context.Context, input *GetDraftPreviewInput) (*GetDraftPreviewOutput, error)
	GetFeature(ctx context.Context, input *GetFeatureInput) (*GetFeatureOutput, error)
	ListSpellsByLevel(ctx context.Context, input *ListSpellsByLevelInput) (*ListSpellsByLevelOutput, error)
	ListEquipmentByType(ctx context.Context, input *ListEquipmentByTypeInput) (*ListEquipmentByTypeOutput, error)
	
	// Inventory management
	GetCharacterInventory(ctx context.Context, input *GetCharacterInventoryInput) (*GetCharacterInventoryOutput, error)
	EquipItem(ctx context.Context, input *EquipItemInput) (*EquipItemOutput, error)
	UnequipItem(ctx context.Context, input *UnequipItemInput) (*UnequipItemOutput, error)
	AddToInventory(ctx context.Context, input *AddToInventoryInput) (*AddToInventoryOutput, error)
	RemoveFromInventory(ctx context.Context, input *RemoveFromInventoryInput) (*RemoveFromInventoryOutput, error)
}

// Draft lifecycle types

// CreateDraftInput defines the request for creating a draft
type CreateDraftInput struct {
	PlayerID    string
	SessionID   string // Optional
	InitialData *character.DraftData
}

// CreateDraftOutput defines the response for creating a draft
type CreateDraftOutput struct {
	Draft *character.DraftData
}

// GetDraftInput defines the request for getting a draft
type GetDraftInput struct {
	DraftID string
}

// GetDraftOutput defines the response for getting a draft
type GetDraftOutput struct {
	Draft *character.DraftData
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
	Drafts        []*character.DraftData
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
	Draft    *character.DraftData
	Warnings []ValidationWarning
}

// UpdateRaceInput defines the request for updating a draft's race
type UpdateRaceInput struct {
	DraftID   string
	RaceID    string
	SubraceID string                  // Optional
	Choices   []character.ChoiceData // Race-specific choices
}

// UpdateRaceOutput defines the response for updating a draft's race
type UpdateRaceOutput struct {
	Draft    *character.DraftData
	Warnings []ValidationWarning
}

// UpdateClassInput defines the request for updating a draft's class
type UpdateClassInput struct {
	DraftID string
	ClassID string
	Choices []character.ChoiceData // Class-specific choices
}

// UpdateClassOutput defines the response for updating a draft's class
type UpdateClassOutput struct {
	Draft    *character.DraftData
	Warnings []ValidationWarning
}

// UpdateBackgroundInput defines the request for updating a draft's background
type UpdateBackgroundInput struct {
	DraftID      string
	BackgroundID string
	Choices      []character.ChoiceData // Background-specific choices
}

// UpdateBackgroundOutput defines the response for updating a draft's background
type UpdateBackgroundOutput struct {
	Draft    *character.DraftData
	Warnings []ValidationWarning
}

// UpdateAbilityScoresInput defines the request for updating a draft's ability scores
type UpdateAbilityScoresInput struct {
	DraftID       string
	AbilityScores shared.AbilityScores
}

// UpdateAbilityScoresOutput defines the response for updating a draft's ability scores
type UpdateAbilityScoresOutput struct {
	Draft    *character.DraftData
	Warnings []ValidationWarning
}

// UpdateSkillsInput defines the request for updating a draft's skills
type UpdateSkillsInput struct {
	DraftID  string
	SkillIDs []string
}

// UpdateSkillsOutput defines the response for updating a draft's skills
type UpdateSkillsOutput struct {
	Draft    *character.DraftData
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
	Character    *character.Data
	DraftDeleted bool
}

// Character operation types

// GetCharacterInput defines the request for getting a character
type GetCharacterInput struct {
	CharacterID string
}

// GetCharacterOutput defines the response for getting a character
type GetCharacterOutput struct {
	Character *character.Data
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
	Characters    []*character.Data
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
	Races         []*RaceSummary
	NextPageToken string
	TotalSize     int32
}

// RaceSummary contains basic race info for listing
type RaceSummary struct {
	ID          string
	Name        string
	Description string
	Size        string
	Speed       int
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
	Classes       []*ClassSummary
	NextPageToken string
	TotalSize     int32
}

// ClassSummary contains basic class info for listing
type ClassSummary struct {
	ID          string
	Name        string
	Description string
	HitDice     int
}

// ListBackgroundsInput defines the request for listing backgrounds
type ListBackgroundsInput struct {
	PageSize  int32
	PageToken string
}

// ListBackgroundsOutput defines the response for listing backgrounds
type ListBackgroundsOutput struct {
	Backgrounds   []*dnd5e.BackgroundData
	NextPageToken string
	TotalSize     int32
}

// ListSpellsInput defines the request for listing spells
type ListSpellsInput struct {
	PageSize   int32
	PageToken  string
	Level      *int32 // Optional filter by spell level (0-9)
	School     string // Optional filter by school
	ClassID    string // Optional filter by class
	SearchTerm string // Optional search term for name/description
}

// ListSpellsOutput defines the response for listing spells
type ListSpellsOutput struct {
	Spells        []*dnd5e.SpellData
	NextPageToken string
	TotalSize     int32
}

// ListEquipmentInput defines the request for listing equipment
type ListEquipmentInput struct {
	PageSize      int32
	PageToken     string
	EquipmentType string // Optional filter by type ("simple-weapon", "martial-weapon", etc.)
	Category      string // Optional filter by category
	SearchTerm    string // Optional search term for name/description
}

// ListEquipmentOutput defines the response for listing equipment
type ListEquipmentOutput struct {
	Equipment     []*dnd5e.EquipmentAPIData
	NextPageToken string
	TotalSize     int32
}

// ListSpellsByLevelInput defines the request for listing spells by level
type ListSpellsByLevelInput struct {
	Level     int32  // Required: spell level (0-9, 0 = cantrips)
	ClassID   string // Optional: filter by class
	PageSize  int32
	PageToken string
}

// ListSpellsByLevelOutput defines the response for listing spells by level
type ListSpellsByLevelOutput struct {
	Spells        []*dnd5e.SpellData
	NextPageToken string
	TotalSize     int32
}

// ListEquipmentByTypeInput defines the request for listing equipment by type
type ListEquipmentByTypeInput struct {
	EquipmentType string // Required: equipment type ("simple-weapon", "martial-weapon", etc.)
	PageSize      int32
	PageToken     string
}

// ListEquipmentByTypeOutput defines the response for listing equipment by type
type ListEquipmentByTypeOutput struct {
	Equipment     []*dnd5e.EquipmentAPIData
	NextPageToken string
	TotalSize     int32
}

// UpdateChoicesInput defines the request for updating character choices
type UpdateChoicesInput struct {
	DraftID    string
	Selections []character.ChoiceData
}

// UpdateChoicesOutput defines the response for updating character choices
type UpdateChoicesOutput struct {
	Draft *character.DraftData
}

// ListChoiceOptionsInput defines the request for listing available choice options
type ListChoiceOptionsInput struct {
	DraftID    string            // Required: Which draft to get choices for
	ChoiceType *shared.ChoiceCategory // Optional: Filter by choice type
	PageSize   int32             // Optional: Page size for pagination
	PageToken  string            // Optional: Page token for pagination
}

// ListChoiceOptionsOutput defines the response for listing choice options
type ListChoiceOptionsOutput struct {
	Categories    []*ChoiceCategory
	NextPageToken string
	TotalSize     int32
}

// GetRaceDetailsInput defines the request for getting race details
type GetRaceDetailsInput struct {
	RaceID string
}

// GetRaceDetailsOutput defines the response for getting race details
type GetRaceDetailsOutput struct {
	// Core mechanics data from toolkit
	RaceData *race.Data
	// UI/presentation data
	UIData *external.RaceUIData
}


// GetClassDetailsInput defines the request for getting class details
type GetClassDetailsInput struct {
	ClassID string
}

// GetClassDetailsOutput defines the response for getting class details
type GetClassDetailsOutput struct {
	// Core mechanics data from toolkit
	ClassData *class.Data
	// UI/presentation data
	UIData *external.ClassUIData
}


// GetBackgroundDetailsInput defines the request for getting background details
type GetBackgroundDetailsInput struct {
	BackgroundID string
}

// GetBackgroundDetailsOutput defines the response for getting background details
type GetBackgroundDetailsOutput struct {
	Background *dnd5e.BackgroundData
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

// Equipment management types

// InventoryAddition represents an item to add to inventory
type InventoryAddition struct {
	Item   *dnd5e.InventoryItem
	Source string // Where the item came from (quest, purchase, etc.)
}

// GetDraftPreviewInput defines the request for getting draft preview
type GetDraftPreviewInput struct {
	DraftID string
}

// GetDraftPreviewOutput defines the response for getting draft preview
type GetDraftPreviewOutput struct {
	Character *character.Data
}

// GetFeatureInput defines the request for getting feature details
type GetFeatureInput struct {
	FeatureID string
	ClassID   string
	Level     int32
}

// GetFeatureOutput defines the response for getting feature details
type GetFeatureOutput struct {
	Feature *dnd5e.FeatureData
}

// GetCharacterInventoryInput defines the request for getting character inventory
type GetCharacterInventoryInput struct {
	CharacterID string
}

// GetCharacterInventoryOutput defines the response for getting character inventory
type GetCharacterInventoryOutput struct {
	EquipmentSlots      *dnd5e.EquipmentSlots
	Inventory           []dnd5e.InventoryItem
	Encumbrance         *dnd5e.EncumbranceInfo
	AttunementSlotsUsed int32
	AttunementSlotsMax  int32
}

// EquipItemInput defines the request for equipping an item
type EquipItemInput struct {
	CharacterID string
	ItemID      string
	Slot        string
}

// EquipItemOutput defines the response for equipping an item
type EquipItemOutput struct {
	Success                bool
	Character              *character.Data
	PreviouslyEquippedItem *dnd5e.InventoryItem
}

// UnequipItemInput defines the request for unequipping an item
type UnequipItemInput struct {
	CharacterID string
	Slot        string
}

// UnequipItemOutput defines the response for unequipping an item
type UnequipItemOutput struct {
	Success   bool
	Character *character.Data
}

// AddToInventoryInput defines the request for adding item to inventory
type AddToInventoryInput struct {
	CharacterID string
	Items       []InventoryAddition
}

// AddToInventoryOutput defines the response for adding item to inventory
type AddToInventoryOutput struct {
	Success   bool
	Character *character.Data
	Errors    []string
}

// RemoveFromInventoryInput defines the request for removing item from inventory
type RemoveFromInventoryInput struct {
	CharacterID string
	ItemID      string
	Quantity    int32
	RemoveAll   bool
}

// RemoveFromInventoryOutput defines the response for removing item from inventory
type RemoveFromInventoryOutput struct {
	Success         bool
	Character       *character.Data
	QuantityRemoved int32
}

// ChoiceCategory represents a category of choices for character creation
type ChoiceCategory struct {
	ID          string
	Name        string
	Description string
	Choices     []Choice
}

// Choice represents a single choice within a category
type Choice struct {
	ID          string
	Label       string
	Description string
	Type        string
	Options     []ChoiceOption
	MinChoices  int32
	MaxChoices  int32
}

// ChoiceOption represents an option within a choice
type ChoiceOption struct {
	ID          string
	Label       string
	Description string
	Value       interface{}
}
