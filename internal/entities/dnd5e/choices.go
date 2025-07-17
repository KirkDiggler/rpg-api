package dnd5e

// ChoiceType represents the type of choice to be made
type ChoiceType string

const (
	// ChoiceTypeFightingStyle represents fighting style choices for fighters
	ChoiceTypeFightingStyle ChoiceType = "fighting_style"
	// ChoiceTypeCantrips represents cantrip spell choices
	ChoiceTypeCantrips ChoiceType = "cantrips"
	// ChoiceTypeSpells represents spell choices
	ChoiceTypeSpells ChoiceType = "spells"
	// ChoiceTypeSkills represents skill proficiency choices
	ChoiceTypeSkills ChoiceType = "skills"
	// ChoiceTypeLanguages represents language choices
	ChoiceTypeLanguages ChoiceType = "languages"
	// ChoiceTypeTools represents tool proficiency choices
	ChoiceTypeTools ChoiceType = "tools"
	// ChoiceTypeEquipment represents equipment choices
	ChoiceTypeEquipment ChoiceType = "equipment"
)

// ChoiceCategory represents a grouping of related choices
type ChoiceCategory struct {
	ID          string
	Type        ChoiceType
	Name        string
	Description string
	Required    bool
	MinChoices  int32
	MaxChoices  int32
	Options     []*ChoiceOption
}

// ChoiceOption represents a single option within a choice category
type ChoiceOption struct {
	ID            string
	Name          string
	Description   string
	Prerequisites []string // IDs of other choices that must be selected first
	Conflicts     []string // IDs of other choices that cannot be selected together
	Level         int32    // For spells/cantrips, the level requirement
	School        string   // For spells, the school of magic
	Source        string   // Where this choice comes from (class, race, background, etc.)
}

// CharacterChoices represents all choices made by a character
type CharacterChoices struct {
	FightingStyles []string // IDs of selected fighting styles
	Cantrips       []string // IDs of selected cantrips
	Spells         []string // IDs of selected spells (level 1)
	Skills         []string // IDs of selected skills (already exists in draft)
	Languages      []string // IDs of selected additional languages
	Tools          []string // IDs of selected tool proficiencies
	Equipment      []string // IDs of selected equipment options
}

// ChoiceSelection represents a selection made by the player
type ChoiceSelection struct {
	CategoryID string   // Which choice category this selection is for
	OptionIDs  []string // Which options were selected
}

// ChoiceValidationResult represents the result of validating choices
type ChoiceValidationResult struct {
	IsValid  bool
	Errors   []ChoiceValidationError
	Warnings []ChoiceValidationWarning
}

// ChoiceValidationError represents an error in choice validation
type ChoiceValidationError struct {
	CategoryID string
	OptionID   string
	Message    string
	Code       string
}

// ChoiceValidationWarning represents a warning in choice validation
type ChoiceValidationWarning struct {
	CategoryID string
	OptionID   string
	Message    string
	Code       string
}
