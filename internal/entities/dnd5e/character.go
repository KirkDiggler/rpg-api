// Package dnd5e implements the D&D 5e entities
package dnd5e

// Character represents a finalized D&D 5e character
// NOTE: This is a data-only struct. All calculations (AC, proficiency bonus, etc.)
// are done by the engine (rpg-toolkit), not here. See internal/entities/README.md
type Character struct {
	ID               string
	Name             string
	Level            int32
	ExperiencePoints int32
	RaceID           string
	SubraceID        string
	ClassID          string
	BackgroundID     string
	Alignment        string
	AbilityScores    AbilityScores
	CurrentHP        int32
	TempHP           int32
	SessionID        string
	PlayerID         string
	CreatedAt        int64
	UpdatedAt        int64
}

// CharacterDraft represents a character in creation
type CharacterDraft struct {
	ID                  string
	PlayerID            string
	SessionID           string
	Name                string
	RaceID              string
	SubraceID           string
	ClassID             string
	BackgroundID        string
	AbilityScores       *AbilityScores
	Alignment           string
	StartingSkillIDs    []string
	AdditionalLanguages []string
	Choices             *CharacterChoices // Class-specific choices (fighting styles, cantrips, spells)
	Progress            CreationProgress
	ExpiresAt           int64
	CreatedAt           int64
	UpdatedAt           int64
	DiscordChannelID    string
	DiscordMessageID    string
}

// AbilityScores holds the six core ability scores
type AbilityScores struct {
	Strength     int32
	Dexterity    int32
	Constitution int32
	Intelligence int32
	Wisdom       int32
	Charisma     int32
}

// CreationProgress tracks completion of character creation steps using bitflags
type CreationProgress struct {
	StepsCompleted       uint8 // Bitflags for completed steps
	CompletionPercentage int32
	CurrentStep          string
}

// Progress step bitflags
const (
	ProgressStepName          uint8 = 1 << iota // 1
	ProgressStepRace                            // 2
	ProgressStepClass                           // 4
	ProgressStepBackground                      // 8
	ProgressStepAbilityScores                   // 16
	ProgressStepSkills                          // 32
	ProgressStepLanguages                       // 64
	ProgressStepChoices                         // 128 (fighting styles, cantrips, spells)
)

// HasStep checks if a specific step is completed
func (p CreationProgress) HasStep(step uint8) bool {
	return p.StepsCompleted&step != 0
}

// SetStep marks a step as completed
func (p *CreationProgress) SetStep(step uint8, completed bool) {
	if completed {
		p.StepsCompleted |= step
	} else {
		p.StepsCompleted &^= step
	}
}

// Convenience methods for backward compatibility

// HasName checks if the name step is completed
func (p CreationProgress) HasName() bool { return p.HasStep(ProgressStepName) }

// HasRace checks if the race step is completed
func (p CreationProgress) HasRace() bool { return p.HasStep(ProgressStepRace) }

// HasClass checks if the class step is completed
func (p CreationProgress) HasClass() bool { return p.HasStep(ProgressStepClass) }

// HasBackground checks if the background step is completed
func (p CreationProgress) HasBackground() bool { return p.HasStep(ProgressStepBackground) }

// HasAbilityScores checks if the ability scores step is completed
func (p CreationProgress) HasAbilityScores() bool { return p.HasStep(ProgressStepAbilityScores) }

// HasSkills checks if the skills step is completed
func (p CreationProgress) HasSkills() bool { return p.HasStep(ProgressStepSkills) }

// HasChoices checks if the choices step is completed
func (p CreationProgress) HasChoices() bool { return p.HasStep(ProgressStepChoices) }

// HasLanguages checks if the languages step is completed
func (p CreationProgress) HasLanguages() bool { return p.HasStep(ProgressStepLanguages) }

// Data loading entities for character creation UI

// RaceInfo contains detailed information about a D&D 5e race
type RaceInfo struct {
	ID                   string
	Name                 string
	Description          string
	Speed                int32
	Size                 string
	SizeDescription      string
	AbilityBonuses       map[string]int32
	Traits               []RacialTrait
	Subraces             []SubraceInfo
	Proficiencies        []string
	Languages            []string
	AgeDescription       string
	AlignmentDescription string
	LanguageOptions      *Choice
	ProficiencyOptions   []Choice
}

// SubraceInfo contains information about a D&D 5e subrace
type SubraceInfo struct {
	ID             string
	Name           string
	Description    string
	AbilityBonuses map[string]int32
	Traits         []RacialTrait
	Languages      []string
	Proficiencies  []string
}

// RacialTrait contains information about a racial trait
type RacialTrait struct {
	Name        string
	Description string
	IsChoice    bool
	Options     []string
}

// Choice represents a generic choice for proficiencies, languages, etc
type Choice struct {
	Type    string   // e.g., "language", "skill", "tool_proficiency"
	Choose  int32    // How many to choose
	Options []string // Available options
	From    string   // Optional filter/category
}

// ClassInfo contains detailed information about a D&D 5e class
type ClassInfo struct {
	ID                       string
	Name                     string
	Description              string
	HitDie                   string
	PrimaryAbilities         []string
	ArmorProficiencies       []string
	WeaponProficiencies      []string
	ToolProficiencies        []string
	SavingThrowProficiencies []string
	SkillChoicesCount        int32
	AvailableSkills          []string
	StartingEquipment        []string
	EquipmentChoices         []EquipmentChoice
	Level1Features           []FeatureInfo
	Spellcasting             *SpellcastingInfo
	ProficiencyChoices       []Choice
}

// EquipmentChoice represents a choice in starting equipment
type EquipmentChoice struct {
	Description string
	Options     []string
	ChooseCount int32
}

// ClassFeature represents a class feature (deprecated - use FeatureInfo instead)
type ClassFeature struct {
	Name        string
	Description string
	Level       int32
	HasChoices  bool
	Choices     []string
}

// FeatureInfo represents detailed information about a class feature, racial trait, or other feature
type FeatureInfo struct {
	ID             string
	Name           string
	Description    string
	Level          int32
	ClassName      string
	HasChoices     bool
	Choices        []Choice
	SpellSelection *SpellSelectionInfo
}

// SpellSelectionInfo contains programmatic spell selection requirements
type SpellSelectionInfo struct {
	SpellsToSelect   int32
	SpellLevels      []int32
	SpellLists       []string
	SelectionType    string
	RequiresReplace  bool
}

// SpellcastingInfo contains spellcasting information for a class
type SpellcastingInfo struct {
	SpellcastingAbility string
	RitualCasting       bool
	SpellcastingFocus   string
	CantripsKnown       int32
	SpellsKnown         int32
	SpellSlotsLevel1    int32
}

// BackgroundInfo contains detailed information about a D&D 5e background
type BackgroundInfo struct {
	ID                  string
	Name                string
	Description         string
	SkillProficiencies  []string
	ToolProficiencies   []string
	Languages           []string
	AdditionalLanguages int32
	StartingEquipment   []string
	StartingGold        int32
	FeatureName         string
	FeatureDescription  string
	PersonalityTraits   []string
	Ideals              []string
	Bonds               []string
	Flaws               []string
}

// SpellInfo contains information about a D&D 5e spell
type SpellInfo struct {
	ID          string
	Name        string
	Level       int32
	School      string
	CastingTime string
	Range       string
	Components  []string
	Duration    string
	Description string
	Classes     []string
}

// EquipmentInfo contains information about D&D 5e equipment
type EquipmentInfo struct {
	ID          string
	Name        string
	Type        string // "weapon", "armor", "gear", etc.
	Category    string // "simple-weapon", "martial-weapon", "light-armor", etc.
	Cost        string // "2 gp", "50 gp", etc.
	Weight      string // "1 lb", "2 lbs", etc.
	Description string
	Properties  []string // For weapons: "light", "finesse", etc.
}
