package character

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/KirkDiggler/rpg-api/internal/clients/external"
	"github.com/KirkDiggler/rpg-api/internal/errors"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/dice"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	"github.com/KirkDiggler/rpg-api/internal/repositories/character"
	draftrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/effects"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/races"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/skills"
)

// Config holds dependencies for the orchestrator
type Config struct {
	CharacterRepo      character.Repository
	CharacterDraftRepo draftrepo.Repository
	ExternalClient     external.Client
	DiceService        dice.Service
	IDGenerator        idgen.Generator
	DraftIDGenerator   idgen.Generator
}

// Validate ensures all required dependencies are present
func (c *Config) Validate() error {
	if c.CharacterRepo == nil {
		return errors.InvalidArgument("character repository is required")
	}
	if c.CharacterDraftRepo == nil {
		return errors.InvalidArgument("character draft repository is required")
	}
	if c.ExternalClient == nil {
		return errors.InvalidArgument("external client is required")
	}
	if c.DiceService == nil {
		return errors.InvalidArgument("dice service is required")
	}
	if c.IDGenerator == nil {
		return errors.InvalidArgument("ID generator is required")
	}
	if c.DraftIDGenerator == nil {
		return errors.InvalidArgument("draft ID generator is required")
	}
	return nil
}

// Orchestrator implements the character service
type Orchestrator struct {
	charRepo       character.Repository
	draftRepo      draftrepo.Repository
	externalClient external.Client
	diceService    dice.Service
	idGen          idgen.Generator
	draftIDGen     idgen.Generator
}

// New creates a new character orchestrator
func New(cfg *Config) (*Orchestrator, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return &Orchestrator{
		charRepo:       cfg.CharacterRepo,
		draftRepo:      cfg.CharacterDraftRepo,
		externalClient: cfg.ExternalClient,
		diceService:    cfg.DiceService,
		idGen:          cfg.IDGenerator,
		draftIDGen:     cfg.DraftIDGenerator,
	}, nil
}

// skillNameToConstant maps skill names from external API to skill constants
var skillNameToConstant = map[string]skills.Skill{
	"acrobatics":      skills.Acrobatics,
	"animal-handling": skills.AnimalHandling,
	"arcana":          skills.Arcana,
	"athletics":       skills.Athletics,
	"deception":       skills.Deception,
	"history":         skills.History,
	"insight":         skills.Insight,
	"intimidation":    skills.Intimidation,
	"investigation":   skills.Investigation,
	"medicine":        skills.Medicine,
	"nature":          skills.Nature,
	"perception":      skills.Perception,
	"performance":     skills.Performance,
	"persuasion":      skills.Persuasion,
	"religion":        skills.Religion,
	"sleight-of-hand": skills.SleightOfHand,
	"stealth":         skills.Stealth,
	"survival":        skills.Survival,
}

// mapSkillNameToConstant converts a skill name to a skill constant
// Returns the constant and true if found, empty constant and false otherwise
func mapSkillNameToConstant(skillName string) (skills.Skill, bool) {
	// Normalize skill name: lowercase and replace spaces with hyphens
	normalizedName := strings.ToLower(strings.ReplaceAll(skillName, " ", "-"))
	skillConst, exists := skillNameToConstant[normalizedName]
	return skillConst, exists
}

// contains checks if a string slice contains a specific string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func (o *Orchestrator) ValidateDraft(ctx context.Context, input *ValidateDraftInput) (*ValidateDraftOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) FinalizeDraft(ctx context.Context, input *FinalizeDraftInput) (*FinalizeDraftOutput, error) {
	// Validate input
	if input.DraftID == "" {
		return nil, errors.InvalidArgument("draft ID is required")
	}

	// Get the draft
	getDraftOutput, err := o.draftRepo.Get(ctx, draftrepo.GetInput{
		ID: input.DraftID,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get draft %s", input.DraftID)
	}

	draft := getDraftOutput.Draft

	// Validate draft is complete
	// TODO(#166): This should call ValidateDraft when implemented
	if draft.Name == "" {
		return nil, errors.InvalidArgument("draft is incomplete: name is required")
	}
	if draft.RaceChoice.RaceID == "" {
		return nil, errors.InvalidArgument("draft is incomplete: race is required")
	}
	if draft.ClassChoice.ClassID == "" {
		return nil, errors.InvalidArgument("draft is incomplete: class is required")
	}
	// Background is optional for now (UI not ready)
	// if draft.BackgroundChoice == "" {
	// 	return nil, errors.InvalidArgument("draft is incomplete: background is required")
	// }
	if len(draft.AbilityScoreChoice) == 0 {
		return nil, errors.InvalidArgument("draft is incomplete: ability scores are required")
	}

	// Get race data
	raceDataOutput, err := o.externalClient.GetRaceData(ctx, string(draft.RaceChoice.RaceID))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get race data for %s", draft.RaceChoice.RaceID)
	}

	// Get class data
	classDataOutput, err := o.externalClient.GetClassData(ctx, string(draft.ClassChoice.ClassID))
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get class data for %s", draft.ClassChoice.ClassID)
	}

	// Get background data (optional for now)
	var backgroundDataOutput *external.BackgroundData
	if draft.BackgroundChoice != "" {
		backgroundDataOutput, err = o.externalClient.GetBackgroundData(ctx, string(draft.BackgroundChoice))
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get background data for %s", draft.BackgroundChoice)
		}
	}

	// Calculate hit points
	conMod := (draft.AbilityScoreChoice[abilities.CON] - 10) / 2
	maxHP := classDataOutput.ClassData.HitDice + conMod
	if maxHP < 1 {
		maxHP = 1 // TODO(#169): Extract minimum HP constant
	}

	// Convert draft to character data
	characterData := &toolkitchar.Data{
		ID:       o.idGen.Generate(),
		PlayerID: draft.PlayerID,
		Name:     draft.Name,
		Level:    1, // Starting level

		// Race and class info
		RaceID:       draft.RaceChoice.RaceID,
		SubraceID:    draft.RaceChoice.SubraceID,
		ClassID:      draft.ClassChoice.ClassID,
		BackgroundID: draft.BackgroundChoice,

		// Ability scores
		AbilityScores: draft.AbilityScoreChoice,

		// Hit points
		HitPoints:    maxHP,
		MaxHitPoints: maxHP,

		// Speed from race
		Speed: raceDataOutput.RaceData.Speed,
		Size:  raceDataOutput.RaceData.Size,

		// Initialize empty maps
		Skills:         make(map[skills.Skill]shared.ProficiencyLevel),
		SavingThrows:   make(map[abilities.Ability]shared.ProficiencyLevel),
		SpellSlots:     make(map[int]toolkitchar.SlotInfo),
		ClassResources: make(map[shared.ClassResourceType]toolkitchar.ResourceData),

		// Initialize empty slices
		Languages:     []string{},
		Equipment:     []string{},
		Conditions:    []json.RawMessage{}, // New character has no conditions
		Effects:       []effects.Effect{},  // New character has no effects
		Proficiencies: shared.Proficiencies{},

		// Transfer choices from draft
		Choices: draft.Choices,

		// Timestamps
		CreatedAt: draft.CreatedAt,
		UpdatedAt: draft.UpdatedAt,
	}

	// Process saving throw proficiencies from class
	for _, ability := range classDataOutput.ClassData.SavingThrows {
		characterData.SavingThrows[ability] = shared.Proficient
	}

	// Process skills from choices (both class and background)
	for _, choice := range draft.Choices {
		if choice.Category == shared.ChoiceSkills {
			for _, skill := range choice.SkillSelection {
				characterData.Skills[skill] = shared.Proficient
			}
		}
	}

	// Process languages from race and choices (from all sources)
	for _, lang := range raceDataOutput.RaceData.Languages {
		characterData.Languages = append(characterData.Languages, string(lang))
	}

	for _, choice := range draft.Choices {
		if choice.Category == shared.ChoiceLanguages {
			for _, lang := range choice.LanguageSelection {
				characterData.Languages = append(characterData.Languages, string(lang))
			}
		}
	}

	// Process proficiencies
	// Weapon proficiencies from class
	characterData.Proficiencies.Weapons = classDataOutput.ClassData.WeaponProficiencies

	// Armor proficiencies from class
	characterData.Proficiencies.Armor = classDataOutput.ClassData.ArmorProficiencies

	// Tool proficiencies from background
	// TODO: Add tool proficiencies when they are available in BackgroundData
	// Current BackgroundData structure doesn't include tool proficiencies

	// Add skill proficiencies from background (these are the default skills if no choices were made)
	if backgroundDataOutput != nil {
		for _, skill := range backgroundDataOutput.SkillProficiencies {
			if skillConst, ok := mapSkillNameToConstant(skill); ok {
				// Only add if not already proficient (choices take precedence)
				if characterData.Skills[skillConst] == shared.NotProficient {
					characterData.Skills[skillConst] = shared.Proficient
				}
			} else {
				slog.Warn("Unknown skill in background skill proficiencies", "skill", skill)
			}
		}
	}

	// Add equipment from background
	if backgroundDataOutput != nil {
		characterData.Equipment = append(characterData.Equipment, backgroundDataOutput.Equipment...)
	}

	// Process equipment from choices, unpacking any bundle references
	for _, choice := range draft.Choices {
		if choice.Category == shared.ChoiceEquipment {
			for _, item := range choice.EquipmentSelection {
				// Unpack bundle references (e.g., "bundle_1:0:greatclub" -> "greatclub")
				actualItem := unpackBundleItem(item)
				characterData.Equipment = append(characterData.Equipment, actualItem)
			}
		}
	}

	// Process racial skill proficiencies
	for _, skill := range raceDataOutput.RaceData.SkillProficiencies {
		// Check if not already proficient (from class or background)
		if characterData.Skills[skill] == shared.NotProficient {
			characterData.Skills[skill] = shared.Proficient
		}
	}

	// Store racial traits for display/reference
	for _, trait := range raceDataOutput.RaceData.Traits {
		slog.Debug("Character has racial trait", "trait", trait.Name)
		// TODO: Add Traits []string to character.Data to store these
		// This is tracked in a GitHub issue for adding racial traits field to character data
		// When Traits field is added: characterData.Traits = append(characterData.Traits, trait.Name)
	}

	// Add racial weapon proficiencies
	for _, weapon := range raceDataOutput.RaceData.WeaponProficiencies {
		if !contains(characterData.Proficiencies.Weapons, weapon) {
			characterData.Proficiencies.Weapons = append(characterData.Proficiencies.Weapons, weapon)
		}
	}

	// Add racial tool proficiencies
	for _, tool := range raceDataOutput.RaceData.ToolProficiencies {
		if !contains(characterData.Proficiencies.Tools, tool) {
			characterData.Proficiencies.Tools = append(characterData.Proficiencies.Tools, tool)
		}
	}

	// Handle subrace bonuses
	if draft.RaceChoice.SubraceID == races.HillDwarf {
		// Hill Dwarf gets +1 HP per level
		characterData.MaxHitPoints += characterData.Level
		characterData.HitPoints += characterData.Level
	}

	// Initialize class resources based on class (level 1 only)
	// Note: Monk gets Ki at level 2, not level 1
	// Note: Ranger has no resources at level 1
	switch classDataOutput.ClassData.ID {
	case "fighter": // Using string literal temporarily
		characterData.ClassResources[shared.ClassResourceSecondWind] = toolkitchar.ResourceData{
			Name:    "Second Wind",
			Max:     1,
			Current: 1,
			Resets:  "short_rest",
		}
	case "barbarian": // Using string literal temporarily
		characterData.ClassResources[shared.ClassResourceRage] = toolkitchar.ResourceData{
			Name:    "Rage",
			Max:     2, // 2 rages at level 1
			Current: 2,
			Resets:  "long_rest",
		}
	case "paladin": // Using string literal temporarily
		characterData.ClassResources[shared.ClassResourceLayOnHands] = toolkitchar.ResourceData{
			Name:    "Lay on Hands",
			Max:     5, // 5 HP pool at level 1
			Current: 5,
			Resets:  "long_rest",
		}
	case "bard": // Using string literal temporarily
		// Bardic Inspiration uses = CHA modifier (minimum 1)
		uses := (draft.AbilityScoreChoice[abilities.CHA] - 10) / 2
		if uses < 1 {
			uses = 1
		}
		characterData.ClassResources[shared.ClassResourceBardicInspiration] = toolkitchar.ResourceData{
			Name:    "Bardic Inspiration",
			Max:     uses,
			Current: uses,
			Resets:  "long_rest", // Changes to short_rest at level 5
		}
	}

	// Initialize spell slots for spellcasters (level 1 only)
	// Note: Rangers and Paladins don't get spell slots until level 2
	switch classDataOutput.ClassData.ID {
	case "wizard", "sorcerer", "cleric", // Using string literals temporarily
		"druid", "bard": // Using string literals temporarily
		// Full casters get 2 first-level slots at level 1
		characterData.SpellSlots[1] = toolkitchar.SlotInfo{
			Max:  2,
			Used: 0,
		}
	case "warlock": // Using string literal temporarily
		// Warlock gets 1 first-level slot (Pact Magic)
		characterData.SpellSlots[1] = toolkitchar.SlotInfo{
			Max:  1,
			Used: 0,
		}
	}
	// Save the character
	createCharOutput, err := o.charRepo.Create(ctx, character.CreateInput{
		CharacterData: characterData,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create character from draft %s", input.DraftID)
	}

	// Delete the draft
	_, err = o.draftRepo.Delete(ctx, draftrepo.DeleteInput{
		ID: input.DraftID,
	})
	if err != nil {
		// Log the error but don't fail the operation
		slog.Warn("Failed to delete draft after finalizing",
			"draft_id", input.DraftID,
			"character_id", createCharOutput.CharacterData.ID,
			"error", err)
		return &FinalizeDraftOutput{
			Character:    createCharOutput.CharacterData,
			DraftDeleted: false,
		}, nil
	}

	return &FinalizeDraftOutput{
		Character:    createCharOutput.CharacterData,
		DraftDeleted: true,
	}, nil
}

func (o *Orchestrator) GetCharacter(ctx context.Context, input *GetCharacterInput) (*GetCharacterOutput, error) {
	// Validate input
	if input.CharacterID == "" {
		return nil, errors.InvalidArgument("character ID is required")
	}

	// Get character from repository
	getOutput, err := o.charRepo.Get(ctx, character.GetInput{
		ID: input.CharacterID,
	})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, errors.NotFoundf("character %s not found", input.CharacterID)
		}
		return nil, errors.Wrapf(err, "failed to get character %s", input.CharacterID)
	}

	return &GetCharacterOutput{
		Character: getOutput.CharacterData,
	}, nil
}

func (o *Orchestrator) ListCharacters(ctx context.Context, input *ListCharactersInput) (*ListCharactersOutput, error) {
	// List characters from repository by player ID
	listOutput, err := o.charRepo.ListByPlayerID(ctx, character.ListByPlayerIDInput{
		PlayerID: input.PlayerID,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to list characters")
	}

	return &ListCharactersOutput{
		Characters: listOutput.Characters,
	}, nil
}

func (o *Orchestrator) DeleteCharacter(ctx context.Context, input *DeleteCharacterInput) (*DeleteCharacterOutput, error) {
	// Validate input
	if input.CharacterID == "" {
		return nil, errors.InvalidArgument("character ID is required")
	}

	// Delete character from repository
	_, err := o.charRepo.Delete(ctx, character.DeleteInput{
		ID: input.CharacterID,
	})
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, errors.NotFoundf("character %s not found", input.CharacterID)
		}
		return nil, errors.Wrapf(err, "failed to delete character %s", input.CharacterID)
	}

	return &DeleteCharacterOutput{
		Message: fmt.Sprintf("Character %s deleted successfully", input.CharacterID),
	}, nil
}

func (o *Orchestrator) GetFeature(ctx context.Context, input *GetFeatureInput) (*GetFeatureOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) ListSpellsByLevel(ctx context.Context, input *ListSpellsByLevelInput) (*ListSpellsByLevelOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}
