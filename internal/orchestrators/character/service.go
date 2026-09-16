package character

import (
	"context"

	"github.com/KirkDiggler/rpg-api/internal/entities"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/backgrounds"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character/choices"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/customization"
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

	// Appearance customization is part of toolkit character draft data.
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

	// Advancement. One read that describes the next level and one write that
	// takes it -- there is no draft between them, because a level is one
	// atomic call (design R4.16).
	GetNextLevel(ctx context.Context, input *GetNextLevelInput) (*GetNextLevelOutput, error)
	LevelUp(ctx context.Context, input *LevelUpInput) (*LevelUpOutput, error)

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

// FinalizeDraftOutput returns the created character.
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

// GetNextLevelInput names the character whose next level to describe.
type GetNextLevelInput struct {
	CharacterID string
}

// GetNextLevelOutput is everything the level-up screen needs to be either a
// form or a confirmation, with no class in it (design R4.13, R4.14).
type GetNextLevelOutput struct {
	// CharacterLevel is the level the sheet would read afterwards.
	CharacterLevel int

	// ClassID is the class the level is taken in: the character's own, until
	// multiclassing exists.
	ClassID classes.Class

	// ClassLevel is the level in that class, which is what indexes the tables
	// below. It equals CharacterLevel while nobody multiclasses, and saying
	// both out loud is what keeps the day they diverge from being a silent
	// wrong answer.
	ClassLevel int

	// Requirements are the choices this level asks for and did not ask for
	// before it. Empty for a level that asks nothing, which is a confirmation
	// rather than a form.
	Requirements *choices.Requirements

	// FeatureRefs are the canonical refs of the features the level grants,
	// e.g. "dnd5e:features:action_surge".
	FeatureRefs []string

	// HitDice is the class's hit die, the number the hit point method is
	// applied to.
	HitDice int
}

// LevelUpInput is the level-up itself: one call, atomic, no draft.
type LevelUpInput struct {
	CharacterID string

	// HitPointMethod is rolled or average. The level-1-only "max" is not on
	// the wire and Advance refuses it.
	HitPointMethod character.HitPointMethod

	// Choices are the selections GetNextLevel asked for, and nothing else. The
	// toolkit validates them; nothing here does (R4.15).
	Choices []choices.ChoiceData
}

// LevelUpOutput is the persisted post-level sheet and what the level brought.
type LevelUpOutput struct {
	// Character is the actual persisted post-level entity.
	Character *entities.Character

	// Entry is the record entry that was appended -- the level's INPUTS, which
	// are what is stored (rung-1 design §7.1).
	Entry character.LevelEntry

	// Gained is what the level added, derived for display and never read back.
	Gained character.GainedAtLevel
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

// SetAppearanceInput sets the appearance for a draft.
type SetAppearanceInput struct {
	DraftID    string
	PlayerID   string
	Appearance *customization.Appearance
}

// SetAppearanceOutput returns the complete updated draft data.
type SetAppearanceOutput struct {
	Draft *character.DraftData
}
