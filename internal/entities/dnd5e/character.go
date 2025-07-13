package dnd5e

// Character represents a finalized D&D 5e character
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
func (p CreationProgress) HasName() bool          { return p.HasStep(ProgressStepName) }
func (p CreationProgress) HasRace() bool          { return p.HasStep(ProgressStepRace) }
func (p CreationProgress) HasClass() bool         { return p.HasStep(ProgressStepClass) }
func (p CreationProgress) HasBackground() bool    { return p.HasStep(ProgressStepBackground) }
func (p CreationProgress) HasAbilityScores() bool { return p.HasStep(ProgressStepAbilityScores) }
func (p CreationProgress) HasSkills() bool        { return p.HasStep(ProgressStepSkills) }
func (p CreationProgress) HasLanguages() bool     { return p.HasStep(ProgressStepLanguages) }
