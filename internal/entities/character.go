package entities

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

// CreationProgress tracks completion of character creation steps
type CreationProgress struct {
	HasName              bool
	HasRace              bool
	HasClass             bool
	HasBackground        bool
	HasAbilityScores     bool
	HasSkills            bool
	HasLanguages         bool
	CompletionPercentage int32
	CurrentStep          string
}
