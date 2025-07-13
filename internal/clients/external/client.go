package external

//go:generate mockgen -destination=mock/mock_client.go -package=externalmock github.com/KirkDiggler/rpg-api/internal/clients/external Client

import (
	"context"
)

// Client defines the interface for external API interactions
type Client interface {
	// GetRaceData fetches race information from external source
	GetRaceData(ctx context.Context, raceID string) (*RaceData, error)

	// GetClassData fetches class information from external source
	GetClassData(ctx context.Context, classID string) (*ClassData, error)

	// GetBackgroundData fetches background information from external source
	GetBackgroundData(ctx context.Context, backgroundID string) (*BackgroundData, error)

	// GetSpellData fetches spell information from external source
	GetSpellData(ctx context.Context, spellID string) (*SpellData, error)

	// ListAvailableRaces returns all available races
	ListAvailableRaces(ctx context.Context) ([]*RaceData, error)

	// ListAvailableClasses returns all available classes
	ListAvailableClasses(ctx context.Context) ([]*ClassData, error)

	// ListAvailableBackgrounds returns all available backgrounds
	ListAvailableBackgrounds(ctx context.Context) ([]*BackgroundData, error)
}

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
