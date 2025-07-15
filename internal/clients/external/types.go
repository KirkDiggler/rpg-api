package external

// RaceData represents race information from external source
type RaceData struct {
	ID             string
	Name           string
	Description    string
	Size           string
	Speed          int32
	AbilityBonuses map[string]int32
	Traits         []string
	Subraces       []SubraceData
}

// SubraceData represents subrace information
type SubraceData struct {
	ID             string
	Name           string
	Description    string
	AbilityBonuses map[string]int32
	Traits         []string
}

// ClassData represents class information from external source
type ClassData struct {
	ID                string
	Name              string
	Description       string
	HitDice           string
	PrimaryAbility    string
	SavingThrows      []string
	SkillsCount       int32
	AvailableSkills   []string
	StartingEquipment []string
}

// BackgroundData represents background information from external source
type BackgroundData struct {
	ID                 string
	Name               string
	Description        string
	SkillProficiencies []string
	Languages          int32
	Equipment          []string
	Feature            string
}

// SpellData represents spell information from external source
type SpellData struct {
	ID          string
	Name        string
	Level       int32
	School      string
	CastingTime string
	Range       string
	Components  []string
	Duration    string
	Description string
}
