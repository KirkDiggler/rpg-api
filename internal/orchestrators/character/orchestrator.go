package character

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/KirkDiggler/rpg-api/internal/clients/external"
	"github.com/KirkDiggler/rpg-api/internal/errors"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/dice"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	"github.com/KirkDiggler/rpg-api/internal/repositories/character"
	draftrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/conditions"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/constants"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/effects"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
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
var skillNameToConstant = map[string]constants.Skill{
	"acrobatics":      constants.SkillAcrobatics,
	"animal-handling": constants.SkillAnimalHandling,
	"arcana":          constants.SkillArcana,
	"athletics":       constants.SkillAthletics,
	"deception":       constants.SkillDeception,
	"history":         constants.SkillHistory,
	"insight":         constants.SkillInsight,
	"intimidation":    constants.SkillIntimidation,
	"investigation":   constants.SkillInvestigation,
	"medicine":        constants.SkillMedicine,
	"nature":          constants.SkillNature,
	"perception":      constants.SkillPerception,
	"performance":     constants.SkillPerformance,
	"persuasion":      constants.SkillPersuasion,
	"religion":        constants.SkillReligion,
	"sleight-of-hand": constants.SkillSleightOfHand,
	"stealth":         constants.SkillStealth,
	"survival":        constants.SkillSurvival,
}

// mapSkillNameToConstant converts a skill name to a skill constant
// Returns the constant and true if found, empty constant and false otherwise
func mapSkillNameToConstant(skillName string) (constants.Skill, bool) {
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

// All methods return unimplemented for now

func (o *Orchestrator) CreateDraft(ctx context.Context, input *CreateDraftInput) (*CreateDraftOutput, error) {
	// Validate input
	if input.PlayerID == "" {
		return nil, errors.InvalidArgument("player ID is required")
	}

	// Create new draft with minimal data
	draft := &toolkitchar.DraftData{
		ID:       o.draftIDGen.Generate(),
		PlayerID: input.PlayerID,
	}

	// If initial data provided, merge it
	if input.InitialData != nil {
		if input.InitialData.Name != "" {
			draft.Name = input.InitialData.Name
		}
		// Add other fields as we implement them
	}

	// Save to repository
	createOutput, err := o.draftRepo.Create(ctx, draftrepo.CreateInput{
		Draft: draft,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create draft: %w", err)
	}

	return &CreateDraftOutput{
		Draft: createOutput.Draft,
	}, nil
}

func (o *Orchestrator) GetDraft(ctx context.Context, input *GetDraftInput) (*GetDraftOutput, error) {
	// Validate input
	if input.DraftID == "" {
		return nil, errors.InvalidArgument("draft ID is required")
	}

	// Get draft from repository
	getDraftOutput, err := o.draftRepo.Get(ctx, draftrepo.GetInput{
		ID: input.DraftID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get draft: %w", err)
	}

	// Return the draft data directly
	// The repository returns toolkit DraftData which is what we want
	return &GetDraftOutput{
		Draft: getDraftOutput.Draft,
	}, nil
}

func (o *Orchestrator) ListDrafts(ctx context.Context, input *ListDraftsInput) (*ListDraftsOutput, error) {
	// Validate input
	if input.PlayerID == "" {
		return nil, errors.InvalidArgument("player ID is required")
	}

	// Get the player's single draft
	getDraftOutput, err := o.draftRepo.GetByPlayerID(ctx, draftrepo.GetByPlayerIDInput{
		PlayerID: input.PlayerID,
	})
	if err != nil {
		if errors.IsNotFound(err) {
			// No draft found - return empty list
			return &ListDraftsOutput{
				Drafts:        []*toolkitchar.DraftData{},
				NextPageToken: "",
			}, nil
		}
		return nil, errors.Wrapf(err, "failed to get draft for player %s", input.PlayerID)
	}

	// Return the single draft as a list
	// Note: We ignore SessionID filter since we only have one draft per player
	return &ListDraftsOutput{
		Drafts:        []*toolkitchar.DraftData{getDraftOutput.Draft},
		NextPageToken: "", // No pagination needed for single draft
	}, nil
}

func (o *Orchestrator) DeleteDraft(ctx context.Context, input *DeleteDraftInput) (*DeleteDraftOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) UpdateName(ctx context.Context, input *UpdateNameInput) (*UpdateNameOutput, error) {
	// Validate input
	if input.DraftID == "" {
		return nil, errors.InvalidArgument("draft ID is required")
	}
	if strings.TrimSpace(input.Name) == "" {
		return nil, errors.InvalidArgument("name is required")
	}

	// Get the existing draft
	getDraftOutput, err := o.draftRepo.Get(ctx, draftrepo.GetInput{
		ID: input.DraftID,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get draft %s", input.DraftID)
	}

	// Update the name
	draft := getDraftOutput.Draft
	draft.Name = strings.TrimSpace(input.Name)

	// Save the updated draft
	updateOutput, err := o.draftRepo.Update(ctx, draftrepo.UpdateInput{
		Draft: draft,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to update draft %s", input.DraftID)
	}

	// Return updated draft with any warnings
	return &UpdateNameOutput{
		Draft:    updateOutput.Draft,
		Warnings: []ValidationWarning{}, // No warnings for name update
	}, nil
}

func (o *Orchestrator) UpdateRace(ctx context.Context, input *UpdateRaceInput) (*UpdateRaceOutput, error) {
	// Validate input
	if input.DraftID == "" {
		return nil, errors.InvalidArgument("draft ID is required")
	}
	if input.RaceID == "" {
		return nil, errors.InvalidArgument("race ID is required")
	}

	// Get the existing draft
	getDraftOutput, err := o.draftRepo.Get(ctx, draftrepo.GetInput{
		ID: input.DraftID,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get draft %s", input.DraftID)
	}

	// Update the race choice
	draft := getDraftOutput.Draft
	draft.RaceChoice = toolkitchar.RaceChoice{
		RaceID:    input.RaceID,
		SubraceID: input.SubraceID,
	}

	// Always clear existing race choices when updating race
	var nonRaceChoices []toolkitchar.ChoiceData
	for _, choice := range draft.Choices {
		if choice.Source != shared.SourceRace {
			nonRaceChoices = append(nonRaceChoices, choice)
		}
	}

	// Add new race choices if provided
	if len(input.Choices) > 0 {
		// Ensure all new choices have the race source set
		for i := range input.Choices {
			if input.Choices[i].Source == "" {
				input.Choices[i].Source = shared.SourceRace
			}
		}
		draft.Choices = append(nonRaceChoices, input.Choices...)
	} else {
		// No choices provided, just keep non-race choices
		draft.Choices = nonRaceChoices
	}

	// Save the updated draft
	updateOutput, err := o.draftRepo.Update(ctx, draftrepo.UpdateInput{
		Draft: draft,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to update draft %s", input.DraftID)
	}

	// Return updated draft with any warnings
	return &UpdateRaceOutput{
		Draft:    updateOutput.Draft,
		Warnings: []ValidationWarning{}, // TODO: Add validation for race/subrace compatibility
	}, nil
}

func (o *Orchestrator) UpdateClass(ctx context.Context, input *UpdateClassInput) (*UpdateClassOutput, error) {
	// Validate input
	if input.DraftID == "" {
		return nil, errors.InvalidArgument("draft ID is required")
	}
	if input.ClassID == "" {
		return nil, errors.InvalidArgument("class ID is required")
	}

	// Get the existing draft
	getDraftOutput, err := o.draftRepo.Get(ctx, draftrepo.GetInput{
		ID: input.DraftID,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get draft %s", input.DraftID)
	}

	// Update the class choice
	draft := getDraftOutput.Draft
	draft.ClassChoice = toolkitchar.ClassChoice{
		ClassID: input.ClassID,
	}

	// Always clear existing class choices when updating class
	var nonClassChoices []toolkitchar.ChoiceData
	for _, choice := range draft.Choices {
		if choice.Source != shared.SourceClass {
			nonClassChoices = append(nonClassChoices, choice)
		}
	}

	// Check if this is a spellcasting class and add spell/cantrip choices
	switch input.ClassID {
	case constants.ClassWizard:
		// Wizards get 3 cantrips and 6 first-level spells at level 1
		cantripChoice := toolkitchar.ChoiceData{
			Category: shared.ChoiceCantrips,
			Source:   shared.SourceClass,
			ChoiceID: "wizard_cantrips",
		}
		spellChoice := toolkitchar.ChoiceData{
			Category: shared.ChoiceSpells,
			Source:   shared.SourceClass,
			ChoiceID: "wizard_spells",
		}
		nonClassChoices = append(nonClassChoices, cantripChoice, spellChoice)

	case constants.ClassSorcerer:
		// Sorcerers get 4 cantrips and 2 first-level spells at level 1
		cantripChoice := toolkitchar.ChoiceData{
			Category: shared.ChoiceCantrips,
			Source:   shared.SourceClass,
			ChoiceID: "sorcerer_cantrips",
		}
		spellChoice := toolkitchar.ChoiceData{
			Category: shared.ChoiceSpells,
			Source:   shared.SourceClass,
			ChoiceID: "sorcerer_spells",
		}
		nonClassChoices = append(nonClassChoices, cantripChoice, spellChoice)

	case constants.ClassBard:
		// Bards get 2 cantrips and 4 first-level spells at level 1
		cantripChoice := toolkitchar.ChoiceData{
			Category: shared.ChoiceCantrips,
			Source:   shared.SourceClass,
			ChoiceID: "bard_cantrips",
		}
		spellChoice := toolkitchar.ChoiceData{
			Category: shared.ChoiceSpells,
			Source:   shared.SourceClass,
			ChoiceID: "bard_spells",
		}
		nonClassChoices = append(nonClassChoices, cantripChoice, spellChoice)

	case constants.ClassCleric, constants.ClassDruid:
		// Clerics and Druids get cantrips but prepare spells (no spell choice needed at level 1)
		cantripChoice := toolkitchar.ChoiceData{
			Category: shared.ChoiceCantrips,
			Source:   shared.SourceClass,
			ChoiceID: string(input.ClassID) + "_cantrips",
		}
		nonClassChoices = append(nonClassChoices, cantripChoice)

	case constants.ClassWarlock:
		// Warlocks get 2 cantrips and 2 first-level spells at level 1
		cantripChoice := toolkitchar.ChoiceData{
			Category: shared.ChoiceCantrips,
			Source:   shared.SourceClass,
			ChoiceID: "warlock_cantrips",
		}
		spellChoice := toolkitchar.ChoiceData{
			Category: shared.ChoiceSpells,
			Source:   shared.SourceClass,
			ChoiceID: "warlock_spells",
		}
		nonClassChoices = append(nonClassChoices, cantripChoice, spellChoice)
	}

	// Add new class choices if provided
	if len(input.Choices) > 0 {
		// Ensure all new choices have the class source set
		for i := range input.Choices {
			if input.Choices[i].Source == "" {
				input.Choices[i].Source = shared.SourceClass
			}
		}
		draft.Choices = append(nonClassChoices, input.Choices...)
	} else {
		// No choices provided, just keep non-class choices
		draft.Choices = nonClassChoices
	}

	// Save the updated draft
	updateOutput, err := o.draftRepo.Update(ctx, draftrepo.UpdateInput{
		Draft: draft,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to update draft %s", input.DraftID)
	}

	// Return updated draft with any warnings
	return &UpdateClassOutput{
		Draft:    updateOutput.Draft,
		Warnings: []ValidationWarning{}, // TODO: Add validation for class requirements
	}, nil
}

func (o *Orchestrator) UpdateBackground(ctx context.Context, input *UpdateBackgroundInput) (*UpdateBackgroundOutput, error) {
	// Validate input
	if input.DraftID == "" {
		return nil, errors.InvalidArgument("draft ID is required")
	}
	if input.BackgroundID == "" {
		return nil, errors.InvalidArgument("background ID is required")
	}

	// Get the existing draft
	getDraftOutput, err := o.draftRepo.Get(ctx, draftrepo.GetInput{
		ID: input.DraftID,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get draft %s", input.DraftID)
	}

	// Update the background choice
	draft := getDraftOutput.Draft
	draft.BackgroundChoice = constants.Background(input.BackgroundID)

	// Always clear existing background choices when updating background
	var nonBackgroundChoices []toolkitchar.ChoiceData
	for _, choice := range draft.Choices {
		if choice.Source != shared.SourceBackground {
			nonBackgroundChoices = append(nonBackgroundChoices, choice)
		}
	}

	// Add new background choices if provided
	if len(input.Choices) > 0 {
		// Ensure all new choices have the background source set
		for i := range input.Choices {
			if input.Choices[i].Source == "" {
				input.Choices[i].Source = shared.SourceBackground
			}
		}
		draft.Choices = append(nonBackgroundChoices, input.Choices...)
	} else {
		// No choices provided, just keep non-background choices
		draft.Choices = nonBackgroundChoices
	}

	// Save the updated draft
	updateOutput, err := o.draftRepo.Update(ctx, draftrepo.UpdateInput{
		Draft: draft,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to update draft %s", input.DraftID)
	}

	// Return updated draft with any warnings
	return &UpdateBackgroundOutput{
		Draft:    updateOutput.Draft,
		Warnings: []ValidationWarning{}, // TODO: Add validation for background requirements
	}, nil
}

func (o *Orchestrator) UpdateAbilityScores(ctx context.Context, input *UpdateAbilityScoresInput) (*UpdateAbilityScoresOutput, error) {
	// Validate input
	if input.DraftID == "" {
		return nil, errors.InvalidArgument("draft ID is required")
	}

	// Must have either manual scores or roll assignments
	if input.AbilityScores == nil && input.RollAssignments == nil {
		return nil, errors.InvalidArgument("either ability scores or roll assignments must be provided")
	}

	// Get the existing draft
	getDraftOutput, err := o.draftRepo.Get(ctx, draftrepo.GetInput{
		ID: input.DraftID,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get draft %s", input.DraftID)
	}

	draft := getDraftOutput.Draft

	// Handle roll-based assignment
	if input.RollAssignments != nil {
		// Get the player ID from the draft
		playerID := draft.PlayerID

		slog.Info("Looking for dice session for ability score assignment",
			"draft_id", input.DraftID,
			"player_id", playerID,
			"context", "ability_scores")

		// Get the dice session for this player
		// The dice service uses "ability_scores" as the context for ability score rolls
		sessionOutput, err := o.diceService.GetRollSession(ctx, &dice.GetRollSessionInput{
			EntityID: playerID,
			Context:  "ability_scores",
		})
		if err != nil {
			slog.Error("Failed to get dice session",
				"draft_id", input.DraftID,
				"player_id", playerID,
				"context", "ability_scores",
				"error", err)
			return nil, errors.Wrapf(err, "failed to get dice session for player %s", playerID)
		}

		slog.Info("Found dice session",
			"draft_id", input.DraftID,
			"player_id", playerID,
			"rolls_count", len(sessionOutput.Session.Rolls))

		// Create a map of roll IDs to totals
		rollTotals := make(map[string]int32)
		for _, roll := range sessionOutput.Session.Rolls {
			rollTotals[roll.RollID] = roll.Total
		}

		// Validate all roll IDs exist and belong to this session
		rollIDs := []struct {
			ability string
			rollID  string
		}{
			{"strength", input.RollAssignments.StrengthRollID},
			{"dexterity", input.RollAssignments.DexterityRollID},
			{"constitution", input.RollAssignments.ConstitutionRollID},
			{"intelligence", input.RollAssignments.IntelligenceRollID},
			{"wisdom", input.RollAssignments.WisdomRollID},
			{"charisma", input.RollAssignments.CharismaRollID},
		}

		// Check all rolls exist
		for _, r := range rollIDs {
			if _, exists := rollTotals[r.rollID]; !exists {
				return nil, errors.InvalidArgumentf("roll ID %s for %s not found in session", r.rollID, r.ability)
			}
		}

		// Create ability scores from rolls
		abilityScores := shared.AbilityScores{
			constants.STR: int(rollTotals[input.RollAssignments.StrengthRollID]),
			constants.DEX: int(rollTotals[input.RollAssignments.DexterityRollID]),
			constants.CON: int(rollTotals[input.RollAssignments.ConstitutionRollID]),
			constants.INT: int(rollTotals[input.RollAssignments.IntelligenceRollID]),
			constants.WIS: int(rollTotals[input.RollAssignments.WisdomRollID]),
			constants.CHA: int(rollTotals[input.RollAssignments.CharismaRollID]),
		}

		// Update the draft with the ability scores
		draft.AbilityScoreChoice = abilityScores

		// Clear the dice session after using the rolls
		_, err = o.diceService.ClearRollSession(ctx, &dice.ClearRollSessionInput{
			EntityID: playerID,
			Context:  "ability_scores",
		})
		if err != nil {
			// Log warning but don't fail the operation
			slog.Warn("Failed to clear dice session after ability score assignment",
				"player_id", playerID,
				"context", "ability_scores",
				"error", err)
		}
	} else if input.AbilityScores != nil {
		// Manual assignment
		draft.AbilityScoreChoice = *input.AbilityScores
	}

	// Save the updated draft
	updateOutput, err := o.draftRepo.Update(ctx, draftrepo.UpdateInput{
		Draft: draft,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to update draft %s", input.DraftID)
	}

	// Return updated draft with any warnings
	return &UpdateAbilityScoresOutput{
		Draft:    updateOutput.Draft,
		Warnings: []ValidationWarning{}, // TODO: Add validation for ability score ranges
	}, nil
}

func (o *Orchestrator) UpdateSkills(ctx context.Context, input *UpdateSkillsInput) (*UpdateSkillsOutput, error) {
	return nil, errors.Unimplemented("not implemented")
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
	conMod := (draft.AbilityScoreChoice[constants.CON] - 10) / 2
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
		Skills:         make(map[constants.Skill]shared.ProficiencyLevel),
		SavingThrows:   make(map[constants.Ability]shared.ProficiencyLevel),
		SpellSlots:     make(map[int]toolkitchar.SlotInfo),
		ClassResources: make(map[string]toolkitchar.ResourceData),

		// Initialize empty slices
		Languages:     []string{},
		Equipment:     []string{},
		Conditions:    []conditions.Condition{}, // New character has no conditions
		Effects:       []effects.Effect{},       // New character has no effects
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

	// Process skills from choices
	for _, choice := range draft.Choices {
		if choice.Category == shared.ChoiceSkills {
			for _, skill := range choice.SkillSelection {
				characterData.Skills[skill] = shared.Proficient
			}
		}
	}

	// Process languages from race and choices
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

	// Add skill proficiencies from background
	if backgroundDataOutput != nil {
		for _, skill := range backgroundDataOutput.SkillProficiencies {
			if skillConst, ok := mapSkillNameToConstant(skill); ok {
				characterData.Skills[skillConst] = shared.Proficient
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
		if characterData.Skills[skill] == 0 {
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
	if draft.RaceChoice.SubraceID == constants.SubraceHillDwarf {
		// Hill Dwarf gets +1 HP per level
		characterData.MaxHitPoints += characterData.Level
		characterData.HitPoints += characterData.Level
	}

	// Initialize class resources based on class (level 1 only)
	// Note: Monk gets Ki at level 2, not level 1
	// Note: Ranger has no resources at level 1
	switch classDataOutput.ClassData.ID {
	case constants.ClassFighter:
		characterData.ClassResources["second_wind"] = toolkitchar.ResourceData{
			Name:    "Second Wind",
			Max:     1,
			Current: 1,
			Resets:  "short_rest",
		}
	case constants.ClassBarbarian:
		characterData.ClassResources["rage"] = toolkitchar.ResourceData{
			Name:    "Rage",
			Max:     2, // 2 rages at level 1
			Current: 2,
			Resets:  "long_rest",
		}
	case constants.ClassPaladin:
		characterData.ClassResources["lay_on_hands"] = toolkitchar.ResourceData{
			Name:    "Lay on Hands",
			Max:     5, // 5 HP pool at level 1
			Current: 5,
			Resets:  "long_rest",
		}
	case constants.ClassBard:
		// Bardic Inspiration uses = CHA modifier (minimum 1)
		uses := (draft.AbilityScoreChoice[constants.CHA] - 10) / 2
		if uses < 1 {
			uses = 1
		}
		characterData.ClassResources["bardic_inspiration"] = toolkitchar.ResourceData{
			Name:    "Bardic Inspiration",
			Max:     uses,
			Current: uses,
			Resets:  "long_rest", // Changes to short_rest at level 5
		}
	}

	// Initialize spell slots for spellcasters (level 1 only)
	// Note: Rangers and Paladins don't get spell slots until level 2
	switch classDataOutput.ClassData.ID {
	case constants.ClassWizard, constants.ClassSorcerer, constants.ClassCleric,
		constants.ClassDruid, constants.ClassBard:
		// Full casters get 2 first-level slots at level 1
		characterData.SpellSlots[1] = toolkitchar.SlotInfo{
			Max:  2,
			Used: 0,
		}
	case constants.ClassWarlock:
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

func (o *Orchestrator) ListRaces(ctx context.Context, input *ListRacesInput) (*ListRacesOutput, error) {
	// For now, we'll return all races from a hardcoded list
	// In a real implementation, this might come from a database or be cached

	allRaces := []constants.Race{
		constants.RaceDragonborn,
		constants.RaceDwarf,
		constants.RaceElf,
		constants.RaceGnome,
		constants.RaceHalfElf,
		constants.RaceHalfling,
		constants.RaceHalfOrc,
		constants.RaceHuman,
		constants.RaceTiefling,
	}

	// Get race data for each race
	races := make([]RaceListItem, 0, len(allRaces))
	for _, raceID := range allRaces {
		raceDataOutput, err := o.externalClient.GetRaceData(ctx, string(raceID))
		if err != nil {
			// Skip races that fail to load
			continue
		}

		races = append(races, RaceListItem{
			RaceData: raceDataOutput.RaceData,
			UIData:   raceDataOutput.UIData,
		})
	}

	// Simple pagination - for now just return all races
	// TODO: Implement proper pagination when needed
	return &ListRacesOutput{
		Races:         races,
		NextPageToken: "",
		TotalSize:     len(races),
	}, nil
}

func (o *Orchestrator) ListClasses(ctx context.Context, input *ListClassesInput) (*ListClassesOutput, error) {
	// For now, we'll return all classes from a hardcoded list
	// In a real implementation, this might come from a database or be cached

	allClasses := []constants.Class{
		constants.ClassBarbarian,
		constants.ClassBard,
		constants.ClassCleric,
		constants.ClassDruid,
		constants.ClassFighter,
		constants.ClassMonk,
		constants.ClassPaladin,
		constants.ClassRanger,
		constants.ClassRogue,
		constants.ClassSorcerer,
		constants.ClassWarlock,
		constants.ClassWizard,
	}

	// Get class data for each class
	classes := make([]ClassListItem, 0, len(allClasses))
	for _, classID := range allClasses {
		classDataOutput, err := o.externalClient.GetClassData(ctx, string(classID))
		if err != nil {
			// Skip classes that fail to load
			continue
		}

		classes = append(classes, ClassListItem{
			ClassData: classDataOutput.ClassData,
			UIData:    classDataOutput.UIData,
		})
	}

	// Simple pagination - for now just return all classes
	// TODO: Implement proper pagination when needed
	return &ListClassesOutput{
		Classes:       classes,
		NextPageToken: "",
		TotalSize:     len(classes),
	}, nil
}

func (o *Orchestrator) ListBackgrounds(ctx context.Context, input *ListBackgroundsInput) (*ListBackgroundsOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) UpdateChoices(ctx context.Context, input *UpdateChoicesInput) (*UpdateChoicesOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) ListChoiceOptions(ctx context.Context, input *ListChoiceOptionsInput) (*ListChoiceOptionsOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) GetRaceDetails(ctx context.Context, input *GetRaceDetailsInput) (*GetRaceDetailsOutput, error) {
	if input.RaceID == "" {
		return nil, errors.InvalidArgument("race ID is required")
	}

	// Get race data from external client
	raceDataOutput, err := o.externalClient.GetRaceData(ctx, input.RaceID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get race data for %s", input.RaceID)
	}

	return &GetRaceDetailsOutput{
		RaceData: raceDataOutput.RaceData,
		UIData:   raceDataOutput.UIData,
	}, nil
}

func (o *Orchestrator) GetClassDetails(ctx context.Context, input *GetClassDetailsInput) (*GetClassDetailsOutput, error) {
	if input.ClassID == "" {
		return nil, errors.InvalidArgument("class ID is required")
	}

	// Get class data from external client
	classDataOutput, err := o.externalClient.GetClassData(ctx, input.ClassID)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get class data for %s", input.ClassID)
	}

	return &GetClassDetailsOutput{
		ClassData: classDataOutput.ClassData,
		UIData:    classDataOutput.UIData,
	}, nil
}

func (o *Orchestrator) GetBackgroundDetails(ctx context.Context, input *GetBackgroundDetailsInput) (*GetBackgroundDetailsOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) RollAbilityScores(ctx context.Context, input *RollAbilityScoresInput) (*RollAbilityScoresOutput, error) {
	// Validate input
	if input.DraftID == "" {
		return nil, errors.InvalidArgument("draft ID is required")
	}

	// Default to standard method if not specified
	method := input.Method
	if method == "" {
		method = dice.MethodStandard
	}

	// Get the draft to ensure it exists and get player ID
	getDraftOutput, err := o.draftRepo.Get(ctx, draftrepo.GetInput{
		ID: input.DraftID,
	})
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get draft %s", input.DraftID)
	}

	// Use player ID as entity ID (this must match what UpdateAbilityScores expects)
	playerID := getDraftOutput.Draft.PlayerID

	slog.Info("Rolling ability scores",
		"draft_id", input.DraftID,
		"player_id", playerID,
		"method", method)

	// Roll ability scores using dice service
	rollOutput, err := o.diceService.RollAbilityScores(ctx, &dice.RollAbilityScoresInput{
		EntityID: playerID,
		Method:   method,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to roll ability scores")
	}

	slog.Info("Ability scores rolled successfully",
		"draft_id", input.DraftID,
		"player_id", playerID,
		"session_entity_id", rollOutput.Session.EntityID,
		"session_context", rollOutput.Session.Context,
		"rolls_count", len(rollOutput.Rolls))

	// Convert dice rolls to our format
	rolls := make([]*AbilityScoreRoll, 0, len(rollOutput.Rolls))
	for _, roll := range rollOutput.Rolls {
		rolls = append(rolls, &AbilityScoreRoll{
			RollID:      roll.RollID,
			Total:       roll.Total,
			Description: roll.Description,
			Dice:        roll.Dice,
			Dropped:     roll.Dropped,
		})
	}

	// For now, we just return the rolls
	// The user will need to assign them to abilities later
	// This could be done with an UpdateAbilityScores call
	// We're not updating the draft here because the user needs to decide
	// which roll goes to which ability score

	return &RollAbilityScoresOutput{
		Rolls:     rolls,
		SessionID: playerID, // The session is identified by playerID + context
		ExpiresAt: rollOutput.Session.ExpiresAt,
	}, nil
}

func (o *Orchestrator) GetDraftPreview(ctx context.Context, input *GetDraftPreviewInput) (*GetDraftPreviewOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) GetFeature(ctx context.Context, input *GetFeatureInput) (*GetFeatureOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) ListSpellsByLevel(ctx context.Context, input *ListSpellsByLevelInput) (*ListSpellsByLevelOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) ListEquipmentByType(ctx context.Context, input *ListEquipmentByTypeInput) (*ListEquipmentByTypeOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) GetCharacterInventory(ctx context.Context, input *GetCharacterInventoryInput) (*GetCharacterInventoryOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) EquipItem(ctx context.Context, input *EquipItemInput) (*EquipItemOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) UnequipItem(ctx context.Context, input *UnequipItemInput) (*UnequipItemOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) AddToInventory(ctx context.Context, input *AddToInventoryInput) (*AddToInventoryOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}

func (o *Orchestrator) RemoveFromInventory(ctx context.Context, input *RemoveFromInventoryInput) (*RemoveFromInventoryOutput, error) {
	return nil, errors.Unimplemented("not implemented")
}

// unpackBundleItem extracts the actual item ID from a bundle reference
// Bundle references come from the Discord bot in format "bundle_X:Y:item_id"
// where X is the bundle number, Y is the index, and item_id is the actual item
// For example: "bundle_1:0:greatclub" -> "greatclub"
func unpackBundleItem(bundleRef string) string {
	// Check if this is a bundle reference
	if strings.HasPrefix(bundleRef, "bundle_") {
		// Split by colon to get parts
		parts := strings.Split(bundleRef, ":")
		if len(parts) == 3 {
			// Return the actual item ID (last part)
			return parts[2]
		}
	}
	// Not a bundle reference, return as-is
	return bundleRef
}
