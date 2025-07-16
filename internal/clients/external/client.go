// Package external is the location for the dnd5e-api client
package external

//go:generate mockgen -destination=mock/mock_client.go -package=externalmock github.com/KirkDiggler/rpg-api/internal/clients/external Client

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/fadedpez/dnd5e-api/clients/dnd5e"
	"github.com/fadedpez/dnd5e-api/entities"

	"github.com/KirkDiggler/rpg-api/internal/errors"
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

	// ListAvailableRaces returns all available races with full details
	// Implementation should handle reference->details conversion internally
	ListAvailableRaces(ctx context.Context) ([]*RaceData, error)

	// ListAvailableClasses returns all available classes with full details
	// Implementation should handle reference->details conversion internally
	ListAvailableClasses(ctx context.Context) ([]*ClassData, error)

	// ListAvailableBackgrounds returns all available backgrounds with full details
	// Implementation should handle reference->details conversion internally
	ListAvailableBackgrounds(ctx context.Context) ([]*BackgroundData, error)
}

type client struct {
	dnd5eClient dnd5e.Interface
}

type Config struct {
	// BaseURL for the D&D 5e API (optional, defaults to https://www.dnd5eapi.co)
	BaseURL string
	// HTTPTimeout for API requests (optional, defaults to 30 seconds)
	HTTPTimeout time.Duration
	// CacheTTL for the cached client (optional, defaults to 24 hours)
	CacheTTL time.Duration
}

func (cfg *Config) Validate() error {
	// Set defaults if not provided
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://www.dnd5eapi.co"
	}
	if cfg.HTTPTimeout == 0 {
		cfg.HTTPTimeout = 30 * time.Second
	}
	if cfg.CacheTTL == 0 {
		cfg.CacheTTL = 24 * time.Hour
	}
	return nil
}

func New(cfg *Config) (Client, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	// Create HTTP client with timeout
	httpClient := &http.Client{
		Timeout: cfg.HTTPTimeout,
	}

	// Create the base D&D 5e API client
	baseClient, err := dnd5e.NewDND5eAPI(&dnd5e.DND5eAPIConfig{
		Client:  httpClient,
		BaseURL: cfg.BaseURL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create D&D 5e API client: %w", err)
	}

	// Wrap with caching for better performance
	cachedClient := dnd5e.NewCachedClient(baseClient, cfg.CacheTTL)

	return &client{
		dnd5eClient: cachedClient,
	}, nil
}

func (c *client) GetRaceData(ctx context.Context, raceID string) (*RaceData, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (c *client) GetClassData(ctx context.Context, classID string) (*ClassData, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (c *client) GetBackgroundData(ctx context.Context, backgroundID string) (*BackgroundData, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (c *client) GetSpellData(ctx context.Context, spellID string) (*SpellData, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (c *client) ListAvailableRaces(ctx context.Context) ([]*RaceData, error) {
	// Step 1: Get reference items (just key/name)
	slog.Info("Calling D&D 5e API to list races")
	refs, err := c.dnd5eClient.ListRaces()
	if err != nil {
		return nil, fmt.Errorf("failed to list races from D&D 5e API: %w", err)
	}
	slog.Info("Got race references", "count", len(refs))

	// Step 2: Concurrently load full details for each race
	slog.Info("Loading full details for each race concurrently")
	races := make([]*RaceData, len(refs))
	errChan := make(chan error, len(refs))
	var wg sync.WaitGroup

	for i, ref := range refs {
		wg.Add(1)
		go func(idx int, key string, name string) {
			defer wg.Done()

			// Get full race details (cached after first call)
			race, err := c.dnd5eClient.GetRace(key)
			if err != nil {
				slog.Error("Failed to get race details", "race", key, "error", err)
				errChan <- fmt.Errorf("failed to get race %s: %w", key, err)
				return
			}

			// Convert to our internal format
			races[idx] = convertRaceToRaceData(race)
			slog.Debug("Loaded race details", "race", name)
		}(i, ref.Key, ref.Name)
	}

	wg.Wait()
	close(errChan)

	// Check for any errors
	for err := range errChan {
		if err != nil {
			return nil, err
		}
	}

	return races, nil
}

func (c *client) ListAvailableClasses(ctx context.Context) ([]*ClassData, error) {
	// Step 1: Get reference items (just key/name)
	refs, err := c.dnd5eClient.ListClasses()
	if err != nil {
		return nil, fmt.Errorf("failed to list classes: %w", err)
	}

	// Step 2: Concurrently load full details for each class
	classes := make([]*ClassData, len(refs))
	errChan := make(chan error, len(refs))
	var wg sync.WaitGroup

	for i, ref := range refs {
		wg.Add(1)
		go func(idx int, key string) {
			defer wg.Done()

			// Get full class details (cached after first call)
			class, err := c.dnd5eClient.GetClass(key)
			if err != nil {
				errChan <- fmt.Errorf("failed to get class %s: %w", key, err)
				return
			}

			// Convert to our internal format
			classes[idx] = convertClassToClassData(class)
		}(i, ref.Key)
	}

	wg.Wait()
	close(errChan)

	// Check for any errors
	for err := range errChan {
		if err != nil {
			return nil, err
		}
	}

	return classes, nil
}

func (c *client) ListAvailableBackgrounds(ctx context.Context) ([]*BackgroundData, error) {
	return nil, errors.Unimplemented("not implemented")
}

// Conversion functions

func convertRaceToRaceData(race *entities.Race) *RaceData {
	if race == nil {
		return nil
	}

	// Convert ability bonuses to map
	abilityBonuses := make(map[string]int32)
	for _, bonus := range race.AbilityBonuses {
		if bonus.AbilityScore != nil {
			abilityBonuses[bonus.AbilityScore.Key] = int32(bonus.Bonus)
		}
	}

	// Convert traits to string slice
	traits := make([]string, len(race.Traits))
	for i, trait := range race.Traits {
		traits[i] = trait.Name
	}

	// Convert subraces
	subraces := make([]SubraceData, len(race.SubRaces))
	for i, subrace := range race.SubRaces {
		subraces[i] = SubraceData{
			ID:   subrace.Key,
			Name: subrace.Name,
			// Note: Would need to fetch full subrace details for complete data
		}
	}

	return &RaceData{
		ID:             race.Key,
		Name:           race.Name,
		Speed:          int32(race.Speed),
		AbilityBonuses: abilityBonuses,
		Traits:         traits,
		Subraces:       subraces,
		// Size and Description would need to come from additional API calls or be hardcoded
		Size: "Medium", // Default, actual size would need to be fetched
	}
}

func convertClassToClassData(class *entities.Class) *ClassData {
	if class == nil {
		return nil
	}

	// Convert saving throws to string slice
	savingThrows := make([]string, len(class.SavingThrows))
	for i, st := range class.SavingThrows {
		savingThrows[i] = st.Name
	}

	// Extract available skills from proficiency choices
	var availableSkills []string
	var skillsCount int32
	for _, choice := range class.ProficiencyChoices {
		if choice != nil && choice.ChoiceType == "skills" {
			skillsCount = int32(choice.ChoiceCount)
			if choice.OptionList != nil {
				for _, option := range choice.OptionList.Options {
					if refOpt, ok := option.(*entities.ReferenceOption); ok && refOpt.Reference != nil {
						availableSkills = append(availableSkills, refOpt.Reference.Name)
					}
				}
			}
		}
	}

	// Convert starting equipment to string slice
	startingEquipment := make([]string, len(class.StartingEquipment))
	for i, eq := range class.StartingEquipment {
		if eq.Equipment != nil {
			startingEquipment[i] = fmt.Sprintf("%dx %s", eq.Quantity, eq.Equipment.Name)
		}
	}

	return &ClassData{
		ID:                class.Key,
		Name:              class.Name,
		HitDice:           fmt.Sprintf("1d%d", class.HitDie),
		SavingThrows:      savingThrows,
		SkillsCount:       skillsCount,
		AvailableSkills:   availableSkills,
		StartingEquipment: startingEquipment,
		// PrimaryAbility and Description would need additional data
	}
}
