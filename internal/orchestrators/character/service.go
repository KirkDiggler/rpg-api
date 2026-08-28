package character

import (
	"context"

	"github.com/KirkDiggler/rpg-api/internal/entities"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
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
	SetAbilityScoresFromRolls(ctx context.Context, input *SetAbilityScoresFromRollsInput) (*SetAbilityScoresFromRollsOutput, error)

	// Appearance (cosmetic, stored separately from game data)
	SetAppearance(ctx context.Context, input *SetAppearanceInput) (*SetAppearanceOutput, error)

	// Validation and finalization
	ValidateDraft(ctx context.Context, input *ValidateDraftInput) (*ValidateDraftOutput, error)
	FinalizeDraft(ctx context.Context, input *FinalizeDraftInput) (*FinalizeDraftOutput, error)

	// Character operations
	GetCharacter(ctx context.Context, input *GetCharacterInput) (*GetCharacterOutput, error)
	ListCharacters(ctx context.Context, input *ListCharactersInput) (*ListCharactersOutput, error)
	DeleteCharacter(ctx context.Context, input *DeleteCharacterInput) (*DeleteCharacterOutput, error)

	// Equipment management (equipment slots are part of character.Data)
	EquipItem(ctx context.Context, input *EquipItemInput) (*EquipItemOutput, error)
	UnequipItem(ctx context.Context, input *UnequipItemInput) (*UnequipItemOutput, error)

	// Data loading for UI
	ListRaces(ctx context.Context, input *ListRacesInput) (*ListRacesOutput, error)
	ListClasses(ctx context.Context, input *ListClassesInput) (*ListClassesOutput, error)
	ListBackgrounds(ctx context.Context, input *ListBackgroundsInput) (*ListBackgroundsOutput, error)
	ListEquipmentByType(ctx context.Context, input *ListEquipmentByTypeInput) (*ListEquipmentByTypeOutput, error)

	// Dice rolling
	RollAbilityScores(ctx context.Context, input *RollAbilityScoresInput) (*RollAbilityScoresOutput, error)

	// Spell information
	ListSpellsByLevel(ctx context.Context, input *ListSpellsByLevelInput) (*ListSpellsByLevelOutput, error)
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
	Draft    *entities.CharacterDraft // includes appearance
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
	Class    classes.Class
	Subclass classes.Subclass // Optional: for getting subclass-modified requirements
	Race     races.Race
	Level    int // Default to 1 if not specified
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

// SetAbilityScoresFromRollsInput provides roll assignments for ability scores
type SetAbilityScoresFromRollsInput struct {
	DraftID         string
	RollAssignments map[abilities.Ability]string // Maps ability to roll ID
}

// SetAbilityScoresFromRollsOutput returns updated draft
type SetAbilityScoresFromRollsOutput struct {
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

// GetCharacterInput gets a character by ID
type GetCharacterInput struct {
	CharacterID string
}

// GetCharacterOutput returns the character
type GetCharacterOutput struct {
	Character *entities.Character // includes appearance
}

// EquipItemInput equips an item to a slot
type EquipItemInput struct {
	CharacterID string
	ItemID      string
	Slot        character.InventorySlot
}

// EquipItemOutput returns the result of equipping
type EquipItemOutput struct {
	PreviousItemID string              // Item that was previously in the slot, if any
	Character      *entities.Character // Actual persisted post-equip entity for legacy conversion
	View           *View               // Complete detached post-equip projection
}

// UnequipItemInput unequips an item from a slot
type UnequipItemInput struct {
	CharacterID string
	Slot        character.InventorySlot
}

// UnequipItemOutput returns the unequipped item
type UnequipItemOutput struct {
	UnequippedItemID string              // Item that was removed from the slot
	Character        *entities.Character // Actual persisted post-unequip entity for legacy conversion
	View             *View               // Complete detached post-unequip projection
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
	Characters    []*entities.Character
	NextPageToken string
	TotalSize     int
}

// DeleteCharacterInput deletes a character
type DeleteCharacterInput struct {
	CharacterID string
}

// DeleteCharacterOutput confirms deletion
type DeleteCharacterOutput struct {
	// Empty for now - can add deleted character data if needed
}

// ListEquipmentByTypeInput requests equipment by type
type ListEquipmentByTypeInput struct {
	EquipmentType interface{} // Will use the proto enum value directly
}

// ListEquipmentByTypeOutput returns equipment list
type ListEquipmentByTypeOutput struct {
	Equipment []interface{} // Will hold toolkit Equipment interface values
}

// ListSpellsByLevelInput specifies the spell level to query
type ListSpellsByLevelInput struct {
	Level    int           // 0 for cantrips, 1-9 for leveled spells
	ClassID  classes.Class // Optional: filter by class (not implemented yet)
	PageSize int
}

// ListSpellsByLevelOutput returns spell information
type ListSpellsByLevelOutput struct {
	Spells []SpellInfo
	Total  int
}

// SpellInfo contains spell details
type SpellInfo struct {
	ID          string
	Name        string
	Description string
	Level       int
}

// SetAppearanceInput sets the appearance for a draft
type SetAppearanceInput struct {
	DraftID    string
	Appearance *entities.Appearance
}

// SetAppearanceOutput returns the updated appearance
type SetAppearanceOutput struct {
	Appearance *entities.Appearance
}
