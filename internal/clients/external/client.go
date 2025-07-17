// Package external is the location for the dnd5e-api client
package external

//go:generate mockgen -destination=mock/mock_client.go -package=externalmock github.com/KirkDiggler/rpg-api/internal/clients/external Client

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
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

// Config contains configuration options for the external client.
type Config struct {
	// BaseURL for the D&D 5e API (optional, defaults to https://www.dnd5eapi.co)
	BaseURL string
	// HTTPTimeout for API requests (optional, defaults to 30 seconds)
	HTTPTimeout time.Duration
	// CacheTTL for the cached client (optional, defaults to 24 hours)
	CacheTTL time.Duration
}

// Validate validates the Config and sets defaults if not provided.
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

// New creates a new external client with the given configuration.
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

func (c *client) GetRaceData(_ context.Context, _ string) (*RaceData, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (c *client) GetClassData(_ context.Context, _ string) (*ClassData, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (c *client) GetBackgroundData(_ context.Context, _ string) (*BackgroundData, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (c *client) GetSpellData(_ context.Context, _ string) (*SpellData, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (c *client) ListAvailableRaces(_ context.Context) ([]*RaceData, error) {
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

func (c *client) ListAvailableClasses(_ context.Context) ([]*ClassData, error) {
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

			// Get level 1 data for features (cached after first call)
			level1, err := c.dnd5eClient.GetClassLevel(key, 1)
			if err != nil {
				errChan <- fmt.Errorf("failed to get class level 1 for %s: %w", key, err)
				return
			}

			// Convert to our internal format with level 1 features
			classData, err := c.convertClassWithFeatures(class, level1)
			if err != nil {
				errChan <- fmt.Errorf("failed to convert class %s: %w", key, err)
				return
			}
			classes[idx] = classData
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

func (c *client) ListAvailableBackgrounds(_ context.Context) ([]*BackgroundData, error) {
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
			// nolint:gosec // D&D ability bonuses are always small values
			abilityBonuses[bonus.AbilityScore.Key] = int32(bonus.Bonus)
		}
	}

	// Convert traits to TraitData
	traits := make([]TraitData, len(race.Traits))
	for i, trait := range race.Traits {
		traits[i] = TraitData{
			Name: trait.Name,
			// TODO(#45): Fetch full trait details for description
		}
	}

	// Convert languages
	languages := make([]string, len(race.Languages))
	for i, lang := range race.Languages {
		languages[i] = lang.Name
	}

	// Convert starting proficiencies
	proficiencies := make([]string, len(race.StartingProficiencies))
	for i, prof := range race.StartingProficiencies {
		proficiencies[i] = prof.Name
	}

	// Convert language options
	var languageOptions *ChoiceData
	if race.LanguageOptions != nil {
		languageOptions = convertChoiceOption(race.LanguageOptions, "language")
	}

	// Convert proficiency options
	var proficiencyOptions []*ChoiceData
	if race.StartingProficiencyOptions != nil {
		proficiencyOptions = []*ChoiceData{
			convertChoiceOption(race.StartingProficiencyOptions, "proficiency"),
		}
	}

	// Convert subraces
	subraces := make([]SubraceData, len(race.SubRaces))
	for i, subrace := range race.SubRaces {
		subraces[i] = SubraceData{
			ID:   subrace.Key,
			Name: subrace.Name,
			// TODO(#45): Fetch full subrace details for complete data
		}
	}

	return &RaceData{
		ID:                 race.Key,
		Name:               race.Name,
		Size:               race.Size,
		SizeDescription:    race.SizeDescription,
		Speed:              int32(race.Speed), // nolint:gosec // safe conversion
		AbilityBonuses:     abilityBonuses,
		Traits:             traits,
		Subraces:           subraces,
		Languages:          languages,
		LanguageOptions:    languageOptions,
		Proficiencies:      proficiencies,
		ProficiencyOptions: proficiencyOptions,
		// TODO(#45): These fields need additional API calls or data sources
		// Description, AgeDescription, AlignmentDescription
	}
}

func convertClassToClassData(class *entities.Class) *ClassData {
	if class == nil {
		return nil
	}

	// Convert primary abilities
	primaryAbilities := make([]string, len(class.PrimaryAbilities))
	for i, ability := range class.PrimaryAbilities {
		primaryAbilities[i] = ability.Name
	}

	// Convert saving throws to string slice
	savingThrows := make([]string, len(class.SavingThrows))
	for i, st := range class.SavingThrows {
		savingThrows[i] = st.Name
	}

	// Extract available skills from proficiency choices
	var availableSkills []string
	var skillsCount int32
	var proficiencyChoices []*ChoiceData

	for _, choice := range class.ProficiencyChoices {
		if choice != nil {
			if choice.ChoiceType == "skills" {
				// nolint:gosec // D&D skill counts are always small values
				skillsCount = int32(choice.ChoiceCount)
				if choice.OptionList != nil {
					for _, option := range choice.OptionList.Options {
						if refOpt, ok := option.(*entities.ReferenceOption); ok && refOpt.Reference != nil {
							availableSkills = append(availableSkills, refOpt.Reference.Name)
						}
					}
				}
			}
			// Convert all proficiency choices
			proficiencyChoices = append(proficiencyChoices, convertChoiceOption(choice, choice.ChoiceType))
		}
	}

	// Convert armor proficiencies
	armorProficiencies := make([]string, len(class.ArmorProficiencies))
	for i, armor := range class.ArmorProficiencies {
		armorProficiencies[i] = armor.Name
	}

	// Convert weapon proficiencies
	weaponProficiencies := make([]string, len(class.WeaponProficiencies))
	for i, weapon := range class.WeaponProficiencies {
		weaponProficiencies[i] = weapon.Name
	}

	// Convert tool proficiencies
	toolProficiencies := make([]string, len(class.ToolProficiencies))
	for i, tool := range class.ToolProficiencies {
		toolProficiencies[i] = tool.Name
	}

	// Convert starting equipment to string slice
	startingEquipment := make([]string, len(class.StartingEquipment))
	for i, eq := range class.StartingEquipment {
		if eq.Equipment != nil {
			startingEquipment[i] = fmt.Sprintf("%dx %s", eq.Quantity, eq.Equipment.Name)
		}
	}

	// Convert equipment options
	var equipmentOptions []*EquipmentChoiceData
	for _, option := range class.StartingEquipmentOptions {
		if option != nil {
			equipmentOptions = append(equipmentOptions, convertEquipmentChoice(option))
		}
	}

	return &ClassData{
		ID:                       class.Key,
		Name:                     class.Name,
		Description:              class.Description,
		HitDice:                  fmt.Sprintf("1d%d", class.HitDie),
		PrimaryAbilities:         primaryAbilities,
		SavingThrows:             savingThrows,
		SkillsCount:              skillsCount,
		AvailableSkills:          availableSkills,
		StartingEquipment:        startingEquipment,
		StartingEquipmentOptions: equipmentOptions,
		ArmorProficiencies:       armorProficiencies,
		WeaponProficiencies:      weaponProficiencies,
		ToolProficiencies:        toolProficiencies,
		ProficiencyChoices:       proficiencyChoices,
		// TODO(#45): LevelOneFeatures and Spellcasting require additional API calls
	}
}

func (c *client) convertClassWithFeatures(class *entities.Class, level1 *entities.Level) (*ClassData, error) {
	if class == nil {
		return nil, errors.InvalidArgument("class is required")
	}

	// Start with the basic class data
	classData := convertClassToClassData(class)

	// Add level 1 features if level1 data is available
	if level1 != nil && len(level1.Features) > 0 {
		features := make([]*FeatureData, 0, len(level1.Features))

		// We could fetch full feature details here, but for now just use the reference data
		for _, featureRef := range level1.Features {
			if featureRef != nil {
				features = append(features, &FeatureData{
					Name:        featureRef.Name,
					Description: "", // TODO(#45): Fetch full feature details with GetFeature
					Level:       1,
					HasChoices:  false,
					Choices:     nil,
				})
			}
		}

		classData.LevelOneFeatures = features
	}

	return classData, nil
}

// Helper function to convert ChoiceOption to ChoiceData
func convertChoiceOption(choice *entities.ChoiceOption, choiceType string) *ChoiceData {
	if choice == nil {
		return nil
	}

	var options []string
	if choice.OptionList != nil {
		for _, option := range choice.OptionList.Options {
			if refOpt, ok := option.(*entities.ReferenceOption); ok && refOpt.Reference != nil {
				options = append(options, refOpt.Reference.Name)
			}
		}
	}

	return &ChoiceData{
		Type:    choiceType,
		Choose:  choice.ChoiceCount,
		Options: options,
		From:    choice.Description,
	}
}

// Helper function to convert equipment choice
func convertEquipmentChoice(choice *entities.ChoiceOption) *EquipmentChoiceData {
	if choice == nil {
		return nil
	}

	var options []string
	if choice.OptionList != nil {
		for _, option := range choice.OptionList.Options {
			// Equipment choices might have different option types
			switch opt := option.(type) {
			case *entities.ReferenceOption:
				if opt.Reference != nil {
					options = append(options, opt.Reference.Name)
				}
			case *entities.MultipleOption:
				// Handle multiple equipment options
				var multiDesc []string
				for _, item := range opt.Items {
					if refOpt, ok := item.(*entities.ReferenceOption); ok && refOpt.Reference != nil {
						multiDesc = append(multiDesc, refOpt.Reference.Name)
					}
				}
				if len(multiDesc) > 0 {
					options = append(options, strings.Join(multiDesc, " and "))
				}
			}
		}
	}

	return &EquipmentChoiceData{
		Description: choice.Description,
		Options:     options,
		ChooseCount: choice.ChoiceCount,
	}
}
