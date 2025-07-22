// Package external is the location for the dnd5e-api client
package external

//go:generate mockgen -destination=mock/mock_client.go -package=externalmock github.com/KirkDiggler/rpg-api/internal/clients/external Client

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/fadedpez/dnd5e-api/clients/dnd5e"
	"github.com/fadedpez/dnd5e-api/entities"

	internalDnd5e "github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
	"github.com/KirkDiggler/rpg-api/internal/errors"
)

// slugPattern matches characters that should be replaced in slugs
var slugPattern = regexp.MustCompile(`[^a-z0-9-]+`)

// generateSlug creates a URL-safe slug from a string
func generateSlug(s string) string {
	// Convert to lowercase
	slug := strings.ToLower(s)

	// Replace spaces with hyphens
	slug = strings.ReplaceAll(slug, " ", "-")

	// Replace any non-alphanumeric characters (except hyphens) with hyphens
	slug = slugPattern.ReplaceAllString(slug, "-")

	// Remove leading/trailing hyphens
	slug = strings.Trim(slug, "-")

	// Collapse multiple hyphens into one
	slug = regexp.MustCompile(`-+`).ReplaceAllString(slug, "-")

	return slug
}

// D&D 5e class name mappings for spell filtering
var dnd5eClassNames = map[string]string{
	"bard":     "bard",
	"cleric":   "cleric",
	"druid":    "druid",
	"paladin":  "paladin",
	"ranger":   "ranger",
	"sorcerer": "sorcerer",
	"warlock":  "warlock",
	"wizard":   "wizard",
}

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

	// ListAvailableSpells returns all available spells with full details
	// Implementation should handle reference->details conversion internally
	ListAvailableSpells(ctx context.Context, input *ListSpellsInput) ([]*SpellData, error)

	// ListAvailableEquipment returns all available equipment with full details
	// Implementation should handle reference->details conversion internally
	ListAvailableEquipment(ctx context.Context) ([]*EquipmentData, error)

	// ListEquipmentByCategory returns equipment filtered by category
	// Categories include: "simple-weapons", "martial-weapons", "light-armor", etc.
	ListEquipmentByCategory(ctx context.Context, category string) ([]*EquipmentData, error)

	// GetEquipmentData fetches equipment information from external source
	GetEquipmentData(ctx context.Context, equipmentID string) (*EquipmentData, error)

	// GetFeatureData fetches feature information from external source
	GetFeatureData(ctx context.Context, featureID string) (*FeatureData, error)
}

type client struct {
	dnd5eClient dnd5e.Interface
}

// toAPIFormat converts our internal constant format to API format
// e.g., "RACE_DRAGONBORN" -> "dragonborn"
func toAPIFormat(id string) string {
	// Remove prefix (RACE_, CLASS_, etc.)
	parts := strings.SplitN(id, "_", 2)
	if len(parts) == 2 {
		// Convert to lowercase and replace underscores with hyphens
		return strings.ToLower(strings.ReplaceAll(parts[1], "_", "-"))
	}
	// If no prefix, just lowercase and replace underscores
	return strings.ToLower(strings.ReplaceAll(id, "_", "-"))
}

// fromAPIFormat converts API format to our internal constant format
// e.g., "dragonborn" -> "RACE_DRAGONBORN"
func fromAPIFormat(apiID string, prefix string) string {
	// Convert to uppercase and replace hyphens with underscores
	upperID := strings.ToUpper(strings.ReplaceAll(apiID, "-", "_"))
	if prefix != "" {
		return prefix + "_" + upperID
	}
	return upperID
}

// Config contains configuration options for the external client.
type Config struct {
	// BaseURL for the D&D 5e API (optional, defaults to https://www.dnd5eapi.co/api/2014/)
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
		cfg.BaseURL = "https://www.dnd5eapi.co/api/2014/"
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

func (c *client) GetRaceData(_ context.Context, raceID string) (*RaceData, error) {
	// Convert our internal ID format to API format
	apiID := toAPIFormat(raceID)

	// Get full race details
	race, err := c.dnd5eClient.GetRace(apiID)
	if err != nil {
		return nil, fmt.Errorf("failed to get race %s (api: %s): %w", raceID, apiID, err)
	}

	// Convert to our internal format
	raceData := convertRaceToRaceData(race)
	// Ensure the ID matches our internal format
	raceData.ID = raceID
	return raceData, nil
}

func (c *client) GetClassData(_ context.Context, classID string) (*ClassData, error) {
	// Convert our internal ID format to API format
	apiID := toAPIFormat(classID)

	// Get full class details
	class, err := c.dnd5eClient.GetClass(apiID)
	if err != nil {
		return nil, fmt.Errorf("failed to get class %s (api: %s): %w", classID, apiID, err)
	}

	// Get level 1 data for features
	level1, err := c.dnd5eClient.GetClassLevel(apiID, 1)
	if err != nil {
		return nil, fmt.Errorf("failed to get class level 1 for %s (api: %s): %w", classID, apiID, err)
	}

	// Convert to our internal format with level 1 features
	classData, err := c.convertClassWithFeatures(class, level1)
	if err != nil {
		return nil, fmt.Errorf("failed to convert class %s: %w", classID, err)
	}

	// Ensure the ID matches our internal format
	classData.ID = classID
	return classData, nil
}

func (c *client) GetBackgroundData(_ context.Context, _ string) (*BackgroundData, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (c *client) GetSpellData(_ context.Context, spellID string) (*SpellData, error) {
	// Convert our internal ID format to API format
	apiID := toAPIFormat(spellID)

	spell, err := c.dnd5eClient.GetSpell(apiID)
	if err != nil {
		return nil, fmt.Errorf("failed to get spell %s (api: %s): %w", spellID, apiID, err)
	}

	spellData, err := c.convertSpellToSpellData(spell)
	if err != nil {
		return nil, err
	}

	// Ensure the ID matches our internal format
	spellData.ID = spellID
	return spellData, nil
}

// convertSpellToSpellData converts a dnd5e-api spell entity to our internal SpellData format
func (c *client) convertSpellToSpellData(spell *entities.Spell) (*SpellData, error) {
	if spell == nil {
		return nil, fmt.Errorf("spell is nil")
	}

	// Build components array - currently the dnd5e-api library doesn't expose
	// the actual V/S/M components, so we'll build a descriptive components array
	components := []string{}

	// Add spell properties to the components array
	if spell.Ritual {
		components = append(components, "Ritual")
	}
	if spell.Concentration {
		components = append(components, "Concentration")
	}

	// If no special components, provide a more accurate placeholder
	if len(components) == 0 {
		components = append(components, "See official sources for components")
	}

	// Build a comprehensive description using available data
	description := buildSpellDescription(spell)

	// Convert to our internal format
	return &SpellData{
		ID:          spell.Key,
		Name:        spell.Name,
		Level:       int32(spell.SpellLevel), // nolint:gosec // D&D spell levels are always 0-9
		School:      spell.SpellSchool.Name,
		CastingTime: spell.CastingTime,
		Range:       spell.Range,
		Components:  components,
		Duration:    spell.Duration,
		Description: description,
	}, nil
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
			raceData := convertRaceToRaceData(race)
			// Ensure ID is in our internal format
			raceData.ID = fromAPIFormat(key, "RACE")
			races[idx] = raceData
			slog.Debug("Loaded race details", "race", name, "id", raceData.ID)
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
			// Ensure ID is in our internal format
			classData.ID = fromAPIFormat(key, "CLASS")
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

func (c *client) ListAvailableSpells(_ context.Context, input *ListSpellsInput) ([]*SpellData, error) {
	// Convert our input to the dnd5e-api client input format
	var dnd5eInput *dnd5e.ListSpellsInput
	if input != nil {
		dnd5eInput = &dnd5e.ListSpellsInput{}

		// Convert level filter
		if input.Level != nil {
			level := int(*input.Level)
			dnd5eInput.Level = &level
		}

		// Convert class filter using the package-level mapping
		if input.ClassID != "" {
			if className, exists := dnd5eClassNames[input.ClassID]; exists {
				dnd5eInput.Class = className
			}
		}
	}

	// Step 1: Get spell references from D&D 5e API
	slog.Info("Calling D&D 5e API to list spells")
	refs, err := c.dnd5eClient.ListSpells(dnd5eInput)
	if err != nil {
		return nil, fmt.Errorf("failed to list spells from D&D 5e API: %w", err)
	}
	slog.Info("Got spell references", "count", len(refs))

	// Step 2: Concurrently load full details for each spell
	slog.Info("Loading full details for each spell concurrently")
	spells := make([]*SpellData, len(refs))
	errChan := make(chan error, len(refs))
	var wg sync.WaitGroup

	for i, ref := range refs {
		wg.Add(1)
		go func(idx int, key string, name string) {
			defer wg.Done()

			// Get full spell details (cached after first call)
			spell, err := c.dnd5eClient.GetSpell(key)
			if err != nil {
				slog.Error("Failed to get spell details", "spell", key, "error", err)
				errChan <- fmt.Errorf("failed to get spell %s: %w", key, err)
				return
			}

			// Convert to our internal format using existing GetSpellData logic
			spellData, err := c.convertSpellToSpellData(spell)
			if err != nil {
				slog.Error("Failed to convert spell data", "spell", key, "error", err)
				errChan <- fmt.Errorf("failed to convert spell %s: %w", key, err)
				return
			}

			// Ensure ID is in our internal format
			spellData.ID = fromAPIFormat(key, "SPELL")
			spells[idx] = spellData
			slog.Debug("Loaded spell details", "spell", name, "id", spellData.ID)
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

	return spells, nil
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
		// Determine the specific proficiency type from the description
		profType := "proficiency"
		desc := strings.ToLower(race.StartingProficiencyOptions.Description)
		switch {
		case strings.Contains(desc, "tool") || strings.Contains(desc, "supplies"):
			profType = "tool"
		case strings.Contains(desc, "skill"):
			profType = "skill"
		case strings.Contains(desc, "weapon"):
			profType = "weapon"
		case strings.Contains(desc, "armor"):
			profType = "armor"
		}

		proficiencyOptions = []*ChoiceData{
			convertChoiceOption(race.StartingProficiencyOptions, profType),
		}
	}

	// Parse proficiency choices to rich format
	parsedChoices := parseProficiencyChoices(proficiencyOptions, race.Key)
	if languageOptions != nil {
		parsedChoices = append(parsedChoices, parseProficiencyChoices([]*ChoiceData{languageOptions}, race.Key)...)
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
		Choices:            parsedChoices,
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

	// Keep equipment options for backward compatibility
	// (deprecated fields still populated)
	var equipmentOptions []*EquipmentChoiceData
	for _, option := range class.StartingEquipmentOptions {
		if option != nil {
			equipmentOptions = append(equipmentOptions, convertEquipmentChoice(option))
		}
	}

	// Parse choices directly from rich entity structure
	parsedChoices := parseEquipmentChoicesFromEntities(class.StartingEquipmentOptions, class.Key)
	parsedChoices = append(parsedChoices, parseProficiencyChoices(proficiencyChoices, class.Key)...)

	// Add skill choice if applicable
	if skillsCount > 0 && len(availableSkills) > 0 {
		skillOptions := make([]internalDnd5e.ChoiceOption, len(availableSkills))
		for i, skill := range availableSkills {
			skillOptions[i] = &internalDnd5e.ItemReference{
				ItemID: generateSlug(skill),
				Name:   skill,
			}
		}
		parsedChoices = append(parsedChoices, internalDnd5e.Choice{
			ID:          fmt.Sprintf("%s_skills", class.Key),
			Description: fmt.Sprintf("Choose %d skills", skillsCount),
			Type:        internalDnd5e.ChoiceTypeSkill,
			ChooseCount: skillsCount,
			OptionSet: &internalDnd5e.ExplicitOptions{
				Options: skillOptions,
			},
		})
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
		Choices:                  parsedChoices,
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

		// Fetch full feature details for each feature
		for _, featureRef := range level1.Features {
			if featureRef != nil {
				// Fetch the full feature data including choices
				feature, err := c.dnd5eClient.GetFeature(featureRef.Key)
				if err != nil {
					// Log error but continue with partial data
					slog.Error("Failed to fetch feature details", "feature", featureRef.Key, "error", err)

					// Create a minimal Feature object as fallback
					tempFeature := &entities.Feature{
						Key:   featureRef.Key,
						Name:  featureRef.Name,
						Level: 1,
						Class: &entities.ReferenceItem{
							Name: class.Name,
							Key:  class.Key,
						},
					}

					featureData := &FeatureData{
						ID:          featureRef.Key,
						Name:        featureRef.Name,
						Description: buildFeatureDescription(tempFeature),
						Level:       1,
						HasChoices:  false,
						Choices:     nil,
					}

					// Add spell selection data if applicable
					if spellSelection := buildSpellSelectionData(tempFeature); spellSelection != nil {
						featureData.SpellSelection = spellSelection
					}

					features = append(features, featureData)
				} else {
					// Convert the full feature data
					featureData := convertFeatureToFeatureData(feature)
					if featureData != nil {
						features = append(features, featureData)
					}
				}
			}
		}

		classData.LevelOneFeatures = features

		// Extract feature choices and add to class choices
		for _, feature := range features {
			if feature != nil && len(feature.Choices) > 0 {
				slog.Debug("Found feature with choices", "feature", feature.Name, "choiceCount", len(feature.Choices))
				// Convert feature choices to dnd5e.Choice format
				for _, choiceData := range feature.Choices {
					if choiceData != nil {
						// Parse the choice data to create a proper dnd5e.Choice
						// Use :: as delimiter to avoid collisions with underscores in IDs
						featureChoice := internalDnd5e.Choice{
							ID:          fmt.Sprintf("%s::%s", feature.ID, choiceData.Type),
							Description: fmt.Sprintf("%s: Choose %d %s", feature.Name, choiceData.Choose, choiceData.Type),
							Type:        mapExternalChoiceType(choiceData.Type),
							ChooseCount: int32(choiceData.Choose),
						}

						// Convert options
						if len(choiceData.Options) > 0 {
							options := make([]internalDnd5e.ChoiceOption, 0, len(choiceData.Options))
							for _, opt := range choiceData.Options {
								options = append(options, &internalDnd5e.ItemReference{
									ItemID: generateSlug(opt),
									Name:   opt,
								})
							}
							featureChoice.OptionSet = &internalDnd5e.ExplicitOptions{
								Options: options,
							}
						}

						classData.Choices = append(classData.Choices, featureChoice)
					}
				}
			}
		}
	}

	// Add spellcasting information if available (from level data)
	if level1 != nil && level1.SpellCasting != nil {
		classData.Spellcasting = convertSpellcastingData(nil, level1)
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
			case *entities.CountedReferenceOption:
				if opt.Reference != nil {
					// Include count in the description
					if opt.Count > 1 {
						options = append(options, fmt.Sprintf("%dx %s", opt.Count, opt.Reference.Name))
					} else {
						options = append(options, opt.Reference.Name)
					}
				}
			case *entities.MultipleOption:
				// Handle multiple equipment options
				var multiDesc []string
				for _, item := range opt.Items {
					switch itemOpt := item.(type) {
					case *entities.ReferenceOption:
						if itemOpt.Reference != nil {
							multiDesc = append(multiDesc, itemOpt.Reference.Name)
						}
					case *entities.CountedReferenceOption:
						if itemOpt.Reference != nil {
							if itemOpt.Count > 1 {
								multiDesc = append(multiDesc, fmt.Sprintf("%dx %s", itemOpt.Count, itemOpt.Reference.Name))
							} else {
								multiDesc = append(multiDesc, itemOpt.Reference.Name)
							}
						}
					case *entities.ChoiceOption:
						// Handle nested choice options (like "a martial weapon")
						if itemOpt.Description != "" {
							multiDesc = append(multiDesc, itemOpt.Description)
						}
					}
				}
				if len(multiDesc) > 0 {
					options = append(options, strings.Join(multiDesc, " and "))
				}
			case *entities.ChoiceOption:
				// Handle top-level choice options (like "two martial weapons")
				if opt.Description != "" {
					options = append(options, opt.Description)
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

// buildSpellDescription creates a comprehensive description using available spell data
func buildSpellDescription(spell *entities.Spell) string {
	if spell == nil {
		return "Spell details not available"
	}

	var parts []string

	// Add basic spell info
	parts = append(parts, buildSpellHeader(spell))

	// Add casting info
	if spell.CastingTime != "" {
		parts = append(parts, fmt.Sprintf("Casting Time: %s", spell.CastingTime))
	}
	if spell.Range != "" {
		parts = append(parts, fmt.Sprintf("Range: %s", spell.Range))
	}
	if spell.Duration != "" {
		parts = append(parts, fmt.Sprintf("Duration: %s", spell.Duration))
	}

	// Add special properties
	var properties []string
	if spell.Ritual {
		properties = append(properties, "Ritual")
	}
	if spell.Concentration {
		properties = append(properties, "Concentration")
	}
	if len(properties) > 0 {
		parts = append(parts, fmt.Sprintf("Properties: %s", strings.Join(properties, ", ")))
	}

	// Add damage information if available
	if spell.SpellDamage != nil {
		if spell.SpellDamage.SpellDamageType != nil {
			parts = append(parts, fmt.Sprintf("Damage Type: %s", spell.SpellDamage.SpellDamageType.Name))
		}
		if spell.SpellDamage.SpellDamageAtSlotLevel != nil {
			// Add damage information for the spell's base level
			baseDamage := getBaseDamageForSpellLevel(spell.SpellLevel, spell.SpellDamage.SpellDamageAtSlotLevel)
			if baseDamage != "" {
				parts = append(parts, fmt.Sprintf("Base Damage: %s", baseDamage))
			}
		}
	}

	// Add DC information if available
	if spell.DC != nil {
		dcInfo := "Saving Throw"
		if spell.DC.DCType != nil {
			dcInfo = fmt.Sprintf("%s Save", spell.DC.DCType.Name)
		}
		if spell.DC.DCSuccess != "" {
			dcInfo += fmt.Sprintf(" (%s)", spell.DC.DCSuccess)
		}
		parts = append(parts, dcInfo)
	}

	// Add area of effect if available
	if spell.AreaOfEffect != nil {
		// Default to feet as the unit since D&D 5e API typically uses feet
		unit := "ft"
		parts = append(parts, fmt.Sprintf("Area: %s (%d %s)", spell.AreaOfEffect.Type, spell.AreaOfEffect.Size, unit))
	}

	// Add available classes
	if len(spell.SpellClasses) > 0 {
		var classNames []string
		for _, class := range spell.SpellClasses {
			if class != nil {
				classNames = append(classNames, class.Name)
			}
		}
		if len(classNames) > 0 {
			parts = append(parts, fmt.Sprintf("Classes: %s", strings.Join(classNames, ", ")))
		}
	}

	// Note about full description
	parts = append(parts, "Note: Full spell description available in official D&D 5e sources")

	return strings.Join(parts, ". ")
}

// getBaseDamageForSpellLevel returns the base damage for a spell at its minimum casting level
func getBaseDamageForSpellLevel(level int, damageAtSlotLevel *entities.SpellDamageAtSlotLevel) string {
	switch level {
	case 0, 1:
		return damageAtSlotLevel.FirstLevel
	case 2:
		return damageAtSlotLevel.SecondLevel
	case 3:
		return damageAtSlotLevel.ThirdLevel
	case 4:
		return damageAtSlotLevel.FourthLevel
	case 5:
		return damageAtSlotLevel.FifthLevel
	case 6:
		return damageAtSlotLevel.SixthLevel
	case 7:
		return damageAtSlotLevel.SeventhLevel
	case 8:
		return damageAtSlotLevel.EighthLevel
	case 9:
		return damageAtSlotLevel.NinthLevel
	default:
		return ""
	}
}

// buildSpellHeader creates the basic spell information header
func buildSpellHeader(spell *entities.Spell) string {
	levelStr := "Cantrip"
	if spell.SpellLevel > 0 {
		levelStr = fmt.Sprintf("Level %d", spell.SpellLevel)
	}

	schoolName := "Unknown School"
	if spell.SpellSchool != nil {
		schoolName = spell.SpellSchool.Name
	}

	return fmt.Sprintf("%s %s spell", levelStr, schoolName)
}

func (c *client) ListAvailableEquipment(_ context.Context) ([]*EquipmentData, error) {
	// Get reference items from D&D 5e API
	refs, err := c.dnd5eClient.ListEquipment()
	if err != nil {
		return nil, fmt.Errorf("failed to list equipment from D&D 5e API: %w", err)
	}

	// Load full details for all equipment items
	return c.loadEquipmentDetails(refs)
}

func (c *client) ListEquipmentByCategory(_ context.Context, category string) ([]*EquipmentData, error) {
	// Get equipment category from D&D 5e API
	equipmentCategory, err := c.dnd5eClient.GetEquipmentCategory(category)
	if err != nil {
		return nil, fmt.Errorf("failed to get equipment category %s from D&D 5e API: %w", category, err)
	}

	// Load full details for all equipment items in category
	return c.loadEquipmentDetails(equipmentCategory.Equipment)
}

func (c *client) GetEquipmentData(_ context.Context, equipmentID string) (*EquipmentData, error) {
	// Convert our internal ID format to API format
	apiID := toAPIFormat(equipmentID)

	// Get equipment details from D&D 5e API
	slog.Info("Calling D&D 5e API to get equipment", "equipment", equipmentID, "api", apiID)
	equipmentItem, err := c.dnd5eClient.GetEquipment(apiID)
	if err != nil {
		return nil, fmt.Errorf("failed to get equipment %s (api: %s): %w", equipmentID, apiID, err)
	}

	// Convert to our internal format
	equipmentData := convertEquipmentToEquipmentData(equipmentItem)
	// Ensure the ID matches our internal format
	equipmentData.ID = equipmentID
	return equipmentData, nil
}

// convertEquipmentToEquipmentData converts dnd5e-api equipment to our internal format
func convertEquipmentToEquipmentData(equipment dnd5e.EquipmentInterface) *EquipmentData {
	if equipment == nil {
		return nil
	}

	equipmentData := &EquipmentData{
		EquipmentType: equipment.GetType(),
	}

	switch eq := equipment.(type) {
	case *entities.Weapon:
		equipmentData.ID = eq.Key
		equipmentData.Name = eq.Name
		equipmentData.WeaponCategory = eq.WeaponCategory
		equipmentData.WeaponRange = eq.WeaponRange
		equipmentData.Weight = eq.Weight
		if eq.EquipmentCategory != nil {
			equipmentData.Category = eq.EquipmentCategory.Key
		}
		if eq.Cost != nil {
			equipmentData.Cost = &CostData{
				Quantity: eq.Cost.Quantity,
				Unit:     eq.Cost.Unit,
			}
		}
		if eq.Damage != nil {
			equipmentData.Damage = &DamageData{
				DamageDice: eq.Damage.DamageDice,
				DamageType: "",
			}
			if eq.Damage.DamageType != nil {
				equipmentData.Damage.DamageType = eq.Damage.DamageType.Name
			}
		}
		// Convert properties
		if eq.Properties != nil {
			equipmentData.Properties = make([]string, len(eq.Properties))
			for i, prop := range eq.Properties {
				equipmentData.Properties[i] = prop.Name
			}
		}

	case *entities.Armor:
		equipmentData.ID = eq.Key
		equipmentData.Name = eq.Name
		equipmentData.ArmorCategory = eq.ArmorCategory
		equipmentData.Weight = eq.Weight
		equipmentData.StrengthMinimum = eq.StrMinimum
		equipmentData.StealthDisadvantage = eq.StealthDisadvantage
		if eq.EquipmentCategory != nil {
			equipmentData.Category = eq.EquipmentCategory.Key
		}
		if eq.Cost != nil {
			equipmentData.Cost = &CostData{
				Quantity: eq.Cost.Quantity,
				Unit:     eq.Cost.Unit,
			}
		}
		if eq.ArmorClass != nil {
			equipmentData.ArmorClass = &ArmorClassData{
				Base:     eq.ArmorClass.Base,
				DexBonus: eq.ArmorClass.DexBonus,
			}
		}

	case *entities.Equipment:
		equipmentData.ID = eq.Key
		equipmentData.Name = eq.Name
		equipmentData.Weight = eq.Weight
		if eq.EquipmentCategory != nil {
			equipmentData.Category = eq.EquipmentCategory.Key
		}
		if eq.Cost != nil {
			equipmentData.Cost = &CostData{
				Quantity: eq.Cost.Quantity,
				Unit:     eq.Cost.Unit,
			}
		}
	}

	return equipmentData
}

// loadEquipmentDetails loads full equipment details for a list of reference items concurrently
func (c *client) loadEquipmentDetails(refs []*entities.ReferenceItem) ([]*EquipmentData, error) {
	slog.Info("Loading full details for equipment items concurrently", "count", len(refs))
	equipment := make([]*EquipmentData, len(refs))
	errChan := make(chan error, len(refs))
	var wg sync.WaitGroup

	for i, ref := range refs {
		wg.Add(1)
		go func(idx int, key string, name string) {
			defer wg.Done()

			// Get full equipment details (cached after first call)
			equipmentItem, err := c.dnd5eClient.GetEquipment(key)
			if err != nil {
				slog.Error("Failed to get equipment details", "equipment", key, "error", err)
				errChan <- fmt.Errorf("failed to get equipment %s: %w", key, err)
				return
			}

			// Convert to our internal format
			equipmentData := convertEquipmentToEquipmentData(equipmentItem)
			// Ensure ID is in our internal format
			equipmentData.ID = fromAPIFormat(key, "EQUIPMENT")
			equipment[idx] = equipmentData
			slog.Debug("Loaded equipment details", "equipment", name, "id", equipmentData.ID)
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

	return equipment, nil
}

// convertSpellcastingData converts level spellcasting data to our internal format
func convertSpellcastingData(_ interface{}, level1 *entities.Level) *SpellcastingData {
	// If we have no level-based spellcasting data, return nil
	if level1 == nil || level1.SpellCasting == nil {
		return nil
	}

	spellcastingData := &SpellcastingData{}

	// SpellCasting entity only has numeric data - no ability or info
	// For now, leave spellcasting ability and focus empty
	spellcastingData.SpellcastingAbility = ""
	spellcastingData.RitualCasting = false
	spellcastingData.SpellcastingFocus = ""

	// Get level 1 spell slot info from available data
	// nolint:gosec // Cantrips known can exceed 9 at higher character levels
	spellcastingData.CantripsKnown = int32(level1.SpellCasting.CantripsKnown)
	// nolint:gosec // D&D values are always 0-20
	spellcastingData.SpellsKnown = int32(level1.SpellCasting.SpellsKnown)
	// nolint:gosec // D&D values are always 0-9
	spellcastingData.SpellSlotsLevel1 = int32(level1.SpellCasting.SpellSlotsLevel1)

	return spellcastingData
}

func (c *client) GetFeatureData(_ context.Context, featureID string) (*FeatureData, error) {
	// Convert our internal ID format to API format
	apiID := toAPIFormat(featureID)

	// Get feature details from D&D 5e API
	slog.Info("Calling D&D 5e API to get feature", "feature", featureID, "api", apiID)
	feature, err := c.dnd5eClient.GetFeature(apiID)
	if err != nil {
		return nil, fmt.Errorf("failed to get feature %s (api: %s): %w", featureID, apiID, err)
	}

	// Convert to our internal format with enhanced descriptions
	featureData := convertFeatureToFeatureData(feature)
	// Ensure the ID matches our internal format
	featureData.ID = featureID
	return featureData, nil
}

// convertFeatureToFeatureData converts dnd5e-api feature to our internal format with enhanced descriptions
func convertFeatureToFeatureData(feature *entities.Feature) *FeatureData {
	if feature == nil {
		return nil
	}

	featureData := &FeatureData{
		ID:         feature.Key,
		Name:       feature.Name,
		Level:      int32(feature.Level), // nolint:gosec // D&D levels are always 1-20
		HasChoices: feature.FeatureSpecific != nil && feature.FeatureSpecific.SubFeatureOptions != nil,
	}

	if feature.Class != nil {
		featureData.ClassName = feature.Class.Name
	}

	// Add enhanced descriptions based on known D&D 5e rules
	featureData.Description = buildFeatureDescription(feature)

	// Add programmatic spell selection data
	featureData.SpellSelection = buildSpellSelectionData(feature)

	// Convert choices if available
	if feature.FeatureSpecific != nil && feature.FeatureSpecific.SubFeatureOptions != nil {
		featureData.Choices = []*ChoiceData{
			convertChoiceOption(feature.FeatureSpecific.SubFeatureOptions, "feature"),
		}
	}

	return featureData
}

// buildFeatureDescription creates enhanced descriptions for important features
func buildFeatureDescription(feature *entities.Feature) string {
	switch feature.Key {
	case "spellcasting-wizard":
		return buildWizardSpellcastingDescription()
	case "spellcasting-bard":
		return buildBardSpellcastingDescription()
	case "spellcasting-sorcerer":
		return buildSorcererSpellcastingDescription()
	case "spellcasting-warlock":
		return buildWarlockSpellcastingDescription()
	case "spellcasting-cleric":
		return buildClericSpellcastingDescription()
	case "spellcasting-druid":
		return buildDruidSpellcastingDescription()
	case "spellcasting-paladin":
		return buildPaladinSpellcastingDescription()
	case "spellcasting-ranger":
		return buildRangerSpellcastingDescription()
	default:
		return fmt.Sprintf("A %s class feature gained at level %d.", feature.Class.Name, feature.Level)
	}
}

// buildWizardSpellcastingDescription provides the crucial wizard spellbook information
func buildWizardSpellcastingDescription() string {
	return `As a 1st-level wizard, you have a spellbook containing six 1st-level wizard spells ` +
		`of your choice. Your spellbook is the repository of the wizard spells you know, ` +
		`except your cantrips.

**Spellbook Selection:**
- Choose 6 first-level wizard spells for your spellbook
- All spells must be from the wizard spell list
- All spells must be 1st level (you cannot choose cantrips for your spellbook)

**Learning New Spells:**
Each time you gain a wizard level, you can add two wizard spells of your choice to your ` +
		`spellbook. Each of these spells must be of a level for which you have spell slots.

**Preparing Spells:**
You prepare a number of wizard spells equal to your Intelligence modifier + your wizard ` +
		`level (minimum of one spell). The spells must be of a level for which you have spell slots.`
}

// buildBardSpellcastingDescription provides bard spellcasting information
func buildBardSpellcastingDescription() string {
	return `You have learned to untangle and reshape the fabric of reality in harmony ` +
		`with your wishes and music. Your spells are part of your vast repertoire.

**Spells Known:**
You know two cantrips of your choice from the bard spell list. You learn ` +
		`additional bard cantrips of your choice at higher levels.

You know four 1st-level spells of your choice from the bard spell list. You learn ` +
		`additional bard spells of your choice at higher levels.

**Spellcasting Ability:**
Charisma is your spellcasting ability for your bard spells. You use your Charisma ` +
		`whenever a spell refers to your spellcasting ability.`
}

// buildSorcererSpellcastingDescription provides sorcerer spellcasting information
func buildSorcererSpellcastingDescription() string {
	return `An event in your past, or in the life of a parent or ancestor, left an ` +
		`indelible mark on you, infusing you with arcane magic.

**Cantrips:**
You know four cantrips of your choice from the sorcerer spell list. You learn ` +
		`additional sorcerer cantrips of your choice at higher levels.

**Spells Known:**
You know two 1st-level spells of your choice from the sorcerer spell list. You learn ` +
		`additional sorcerer spells of your choice at higher levels.

**Spellcasting Ability:**
Charisma is your spellcasting ability for your sorcerer spells.`
}

// buildWarlockSpellcastingDescription provides warlock spellcasting information
func buildWarlockSpellcastingDescription() string {
	return `Your arcane research and the magic bestowed on you by your patron have ` +
		`given you facility with spells.

**Cantrips:**
You know two cantrips of your choice from the warlock spell list. You learn ` +
		`additional warlock cantrips of your choice at higher levels.

**Spell Slots:**
You have two 1st-level spell slots. You regain all expended spell slots when you ` +
		`finish a short or long rest.

**Spells Known:**
You know two 1st-level spells of your choice from the warlock spell list. You learn ` +
		`additional warlock spells of your choice at higher levels.

**Spellcasting Ability:**
Charisma is your spellcasting ability for your warlock spells.`
}

// buildClericSpellcastingDescription provides cleric spellcasting information
func buildClericSpellcastingDescription() string {
	return `As a conduit for divine power, you can cast cleric spells.

**Cantrips:**
You know three cantrips of your choice from the cleric spell list. You learn ` +
		`additional cleric cantrips of your choice at higher levels.

**Preparing Spells:**
You prepare a number of cleric spells equal to your Wisdom modifier + your cleric ` +
		`level (minimum of one spell). The spells must be of a level for which you have ` +
		`spell slots.

**Spellcasting Ability:**
Wisdom is your spellcasting ability for your cleric spells. You use your Wisdom ` +
		`whenever a spell refers to your spellcasting ability.

**Ritual Casting:**
You can cast a cleric spell as a ritual if that spell has the ritual tag and you ` +
		`have the spell prepared.`
}

// buildDruidSpellcastingDescription provides druid spellcasting information
func buildDruidSpellcastingDescription() string {
	return `Drawing on the divine essence of nature itself, you can cast spells to ` +
		`shape that essence to your will.

**Cantrips:**
You know two cantrips of your choice from the druid spell list. You learn ` +
		`additional druid cantrips of your choice at higher levels.

**Preparing Spells:**
You prepare a number of druid spells equal to your Wisdom modifier + your druid ` +
		`level (minimum of one spell). The spells must be of a level for which you have ` +
		`spell slots.

**Spellcasting Ability:**
Wisdom is your spellcasting ability for your druid spells. You use your Wisdom ` +
		`whenever a spell refers to your spellcasting ability.

**Ritual Casting:**
You can cast a druid spell as a ritual if that spell has the ritual tag and you ` +
		`have the spell prepared.`
}

// buildPaladinSpellcastingDescription provides paladin spellcasting information
func buildPaladinSpellcastingDescription() string {
	return `By 2nd level, you have learned to draw on divine magic through meditation ` +
		`and prayer to cast spells as a cleric does.

**Preparing Spells:**
You prepare a number of paladin spells equal to your Charisma modifier + half your ` +
		`paladin level, rounded down (minimum of one spell). The spells must be of a level ` +
		`for which you have spell slots.

**Spellcasting Ability:**
Charisma is your spellcasting ability for your paladin spells. You use your Charisma ` +
		`whenever a spell refers to your spellcasting ability.

**Spellcasting Focus:**
You can use a holy symbol as a spellcasting focus for your paladin spells.`
}

// buildRangerSpellcastingDescription provides ranger spellcasting information
func buildRangerSpellcastingDescription() string {
	return `By the time you reach 2nd level, you have learned to use the magical essence ` +
		`of nature to cast spells, much as a druid does.

**Spells Known:**
You know two 1st-level spells of your choice from the ranger spell list. You learn ` +
		`additional ranger spells of your choice at higher levels.

**Spellcasting Ability:**
Wisdom is your spellcasting ability for your ranger spells. You use your Wisdom ` +
		`whenever a spell refers to your spellcasting ability.`
}

// buildSpellSelectionData creates programmatic spell selection requirements
func buildSpellSelectionData(feature *entities.Feature) *SpellSelectionData {
	switch feature.Key {
	case "spellcasting-wizard":
		return &SpellSelectionData{
			SpellsToSelect:  6,
			SpellLevels:     []int32{1},
			SpellLists:      []string{"wizard"},
			SelectionType:   "spellbook",
			RequiresReplace: false,
		}
	case "spellcasting-bard":
		return &SpellSelectionData{
			SpellsToSelect:  4,
			SpellLevels:     []int32{1},
			SpellLists:      []string{"bard"},
			SelectionType:   "known",
			RequiresReplace: true,
		}
	case "spellcasting-sorcerer":
		return &SpellSelectionData{
			SpellsToSelect:  2,
			SpellLevels:     []int32{1},
			SpellLists:      []string{"sorcerer"},
			SelectionType:   "known",
			RequiresReplace: true,
		}
	case "spellcasting-warlock":
		return &SpellSelectionData{
			SpellsToSelect:  2,
			SpellLevels:     []int32{1},
			SpellLists:      []string{"warlock"},
			SelectionType:   "known",
			RequiresReplace: true,
		}
	case "spellcasting-cleric":
		return &SpellSelectionData{
			SpellsToSelect:  -1, // Special case: WIS modifier + level
			SpellLevels:     []int32{1},
			SpellLists:      []string{"cleric"},
			SelectionType:   "prepared",
			RequiresReplace: true,
		}
	case "spellcasting-druid":
		return &SpellSelectionData{
			SpellsToSelect:  -1, // Special case: WIS modifier + level
			SpellLevels:     []int32{1},
			SpellLists:      []string{"druid"},
			SelectionType:   "prepared",
			RequiresReplace: true,
		}
	case "spellcasting-paladin":
		return &SpellSelectionData{
			SpellsToSelect:  -1, // Special case: CHA modifier + half level
			SpellLevels:     []int32{1},
			SpellLists:      []string{"paladin"},
			SelectionType:   "prepared",
			RequiresReplace: true,
		}
	case "spellcasting-ranger":
		return &SpellSelectionData{
			SpellsToSelect:  2,
			SpellLevels:     []int32{1},
			SpellLists:      []string{"ranger"},
			SelectionType:   "known",
			RequiresReplace: true,
		}
	default:
		return nil
	}
}
