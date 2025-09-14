package character

import (
	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/armor"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/backgrounds"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character/choices"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	// "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/equipment" // TODO: Uncomment when using typed equipment
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/fightingstyles"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/languages"
	// "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/packs" // TODO: Uncomment when Pack enum is available
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/proficiencies"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/races"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/skills"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/tools"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/weapons"
)

// convertDraftDataToProto converts toolkit DraftData to proto CharacterDraft
func convertDraftDataToProto(draft *toolkitchar.DraftData) *dnd5ev1alpha1.CharacterDraft {
	if draft == nil {
		return nil
	}

	return &dnd5ev1alpha1.CharacterDraft{
		Id:       draft.ID,
		Name:     draft.Name,
		PlayerId: draft.PlayerID,
		// SessionId is not in DraftData
		Race:              convertRaceToProtoEnum(draft.Race),
		Subrace:           convertSubraceToProtoEnum(draft.Subrace),
		Class:             convertClassToProtoEnum(draft.Class),
		Subclass:          convertSubclassToProtoEnum(draft.Subclass),
		Background:        convertBackgroundToProtoEnum(draft.Background),
		BaseAbilityScores: convertAbilityScoresToProto(draft.BaseAbilityScores),
		Choices:           convertChoicesToProto(draft.Choices),
		Progress:          convertProgressToProto(draft.Progress),
		// Validation will be populated by the orchestrator if needed
		// Info fields (race_info, class_info, etc.) can be populated later if needed for UI
	}
}

// convertProgressToProto converts toolkit Progress to proto CreationProgress
func convertProgressToProto(progress toolkitchar.Progress) *dnd5ev1alpha1.CreationProgress {
	return &dnd5ev1alpha1.CreationProgress{
		HasName:              progress.Has(toolkitchar.ProgressName),
		HasRace:              progress.Has(toolkitchar.ProgressRace),
		HasClass:             progress.Has(toolkitchar.ProgressClass),
		HasBackground:        progress.Has(toolkitchar.ProgressBackground),
		HasAbilityScores:     progress.Has(toolkitchar.ProgressAbilityScores),
		CompletionPercentage: int32(progress.PercentComplete()),
		// Deprecated fields are not set:
		// HasSkills, HasLanguages, CurrentStep
	}
}

// convertAbilityScoresToProto converts toolkit AbilityScores to proto
func convertAbilityScoresToProto(scores shared.AbilityScores) *dnd5ev1alpha1.AbilityScores {
	if scores == nil {
		return nil
	}

	// The toolkit uses abilities constants as keys
	return &dnd5ev1alpha1.AbilityScores{
		Strength:     int32(scores[abilities.STR]),
		Dexterity:    int32(scores[abilities.DEX]),
		Constitution: int32(scores[abilities.CON]),
		Intelligence: int32(scores[abilities.INT]),
		Wisdom:       int32(scores[abilities.WIS]),
		Charisma:     int32(scores[abilities.CHA]),
	}
}

// convertChoicesToProto converts toolkit choices to proto
func convertChoicesToProto(choices []choices.ChoiceData) []*dnd5ev1alpha1.ChoiceData {
	if len(choices) == 0 {
		return nil
	}

	result := make([]*dnd5ev1alpha1.ChoiceData, 0, len(choices))
	for _, choice := range choices {
		protoChoice := convertChoiceToProto(choice)
		if protoChoice != nil {
			result = append(result, protoChoice)
		}
	}
	return result
}

func convertSkillToProtoEnum(skill skills.Skill) dnd5ev1alpha1.Skill {
	switch skill {
	case skills.Acrobatics:
		return dnd5ev1alpha1.Skill_SKILL_ACROBATICS
	case skills.AnimalHandling:
		return dnd5ev1alpha1.Skill_SKILL_ANIMAL_HANDLING
	case skills.Arcana:
		return dnd5ev1alpha1.Skill_SKILL_ARCANA
	case skills.Athletics:
		return dnd5ev1alpha1.Skill_SKILL_ATHLETICS
	case skills.Deception:
		return dnd5ev1alpha1.Skill_SKILL_DECEPTION
	case skills.History:
		return dnd5ev1alpha1.Skill_SKILL_HISTORY
	case skills.Insight:
		return dnd5ev1alpha1.Skill_SKILL_INSIGHT
	case skills.Intimidation:
		return dnd5ev1alpha1.Skill_SKILL_INTIMIDATION
	case skills.Investigation:
		return dnd5ev1alpha1.Skill_SKILL_INVESTIGATION
	case skills.Medicine:
		return dnd5ev1alpha1.Skill_SKILL_MEDICINE
	case skills.Nature:
		return dnd5ev1alpha1.Skill_SKILL_NATURE
	case skills.Perception:
		return dnd5ev1alpha1.Skill_SKILL_PERCEPTION
	case skills.Performance:
		return dnd5ev1alpha1.Skill_SKILL_PERFORMANCE
	case skills.Persuasion:
		return dnd5ev1alpha1.Skill_SKILL_PERSUASION
	case skills.Religion:
		return dnd5ev1alpha1.Skill_SKILL_RELIGION
	case skills.SleightOfHand:
		return dnd5ev1alpha1.Skill_SKILL_SLEIGHT_OF_HAND
	case skills.Stealth:
		return dnd5ev1alpha1.Skill_SKILL_STEALTH
	case skills.Survival:
		return dnd5ev1alpha1.Skill_SKILL_SURVIVAL
	default:
		return dnd5ev1alpha1.Skill_SKILL_UNSPECIFIED
	}
}

func convertLanguageToProtoEnum(lang languages.Language) dnd5ev1alpha1.Language {
	switch lang {
	case languages.Common:
		return dnd5ev1alpha1.Language_LANGUAGE_COMMON
	case languages.Dwarvish:
		return dnd5ev1alpha1.Language_LANGUAGE_DWARVISH
	case languages.Elvish:
		return dnd5ev1alpha1.Language_LANGUAGE_ELVISH
	case languages.Giant:
		return dnd5ev1alpha1.Language_LANGUAGE_GIANT
	case languages.Gnomish:
		return dnd5ev1alpha1.Language_LANGUAGE_GNOMISH
	case languages.Goblin:
		return dnd5ev1alpha1.Language_LANGUAGE_GOBLIN
	case languages.Halfling:
		return dnd5ev1alpha1.Language_LANGUAGE_HALFLING
	case languages.Orc:
		return dnd5ev1alpha1.Language_LANGUAGE_ORC
	case languages.Abyssal:
		return dnd5ev1alpha1.Language_LANGUAGE_ABYSSAL
	case languages.Celestial:
		return dnd5ev1alpha1.Language_LANGUAGE_CELESTIAL
	case languages.Draconic:
		return dnd5ev1alpha1.Language_LANGUAGE_DRACONIC
	case languages.DeepSpeech:
		return dnd5ev1alpha1.Language_LANGUAGE_DEEP_SPEECH
	case languages.Infernal:
		return dnd5ev1alpha1.Language_LANGUAGE_INFERNAL
	case languages.Primordial:
		return dnd5ev1alpha1.Language_LANGUAGE_PRIMORDIAL
	case languages.Sylvan:
		return dnd5ev1alpha1.Language_LANGUAGE_SYLVAN
	case languages.Undercommon:
		return dnd5ev1alpha1.Language_LANGUAGE_UNDERCOMMON
	default:
		return dnd5ev1alpha1.Language_LANGUAGE_UNSPECIFIED
	}
}

func convertProficiencyToolToProtoEnum(tool proficiencies.Tool) dnd5ev1alpha1.Tool {
	switch tool {
	// Artisan's tools
	case proficiencies.ToolAlchemist:
		return dnd5ev1alpha1.Tool_TOOL_ALCHEMIST_SUPPLIES
	case proficiencies.ToolBrewer:
		return dnd5ev1alpha1.Tool_TOOL_BREWER_SUPPLIES
	case proficiencies.ToolCalligrapher:
		return dnd5ev1alpha1.Tool_TOOL_CALLIGRAPHER_SUPPLIES
	case proficiencies.ToolCarpenter:
		return dnd5ev1alpha1.Tool_TOOL_CARPENTER_TOOLS
	case proficiencies.ToolCartographer:
		return dnd5ev1alpha1.Tool_TOOL_CARTOGRAPHER_TOOLS
	case proficiencies.ToolCobbler:
		return dnd5ev1alpha1.Tool_TOOL_COBBLER_TOOLS
	case proficiencies.ToolCook:
		return dnd5ev1alpha1.Tool_TOOL_COOK_UTENSILS
	case proficiencies.ToolGlassblower:
		return dnd5ev1alpha1.Tool_TOOL_GLASSBLOWER_TOOLS
	case proficiencies.ToolJeweler:
		return dnd5ev1alpha1.Tool_TOOL_JEWELER_TOOLS
	case proficiencies.ToolLeatherworker:
		return dnd5ev1alpha1.Tool_TOOL_LEATHERWORKER_TOOLS
	case proficiencies.ToolMason:
		return dnd5ev1alpha1.Tool_TOOL_MASON_TOOLS
	case proficiencies.ToolPainter:
		return dnd5ev1alpha1.Tool_TOOL_PAINTER_SUPPLIES
	case proficiencies.ToolPotter:
		return dnd5ev1alpha1.Tool_TOOL_POTTER_TOOLS
	case proficiencies.ToolSmith:
		return dnd5ev1alpha1.Tool_TOOL_SMITH_TOOLS
	case proficiencies.ToolTinker:
		return dnd5ev1alpha1.Tool_TOOL_TINKER_TOOLS
	case proficiencies.ToolWeaver:
		return dnd5ev1alpha1.Tool_TOOL_WEAVER_TOOLS
	case proficiencies.ToolWoodcarver:
		return dnd5ev1alpha1.Tool_TOOL_WOODCARVER_TOOLS
	// Gaming sets
	case proficiencies.ToolDiceSet:
		return dnd5ev1alpha1.Tool_TOOL_DICE_SET
	case proficiencies.ToolPlayingCardSet:
		return dnd5ev1alpha1.Tool_TOOL_PLAYING_CARD_SET
	case proficiencies.ToolDragonchessSet:
		return dnd5ev1alpha1.Tool_TOOL_DRAGONCHESS_SET
	case proficiencies.ToolThreeDragonAnte:
		return dnd5ev1alpha1.Tool_TOOL_THREE_DRAGON_ANTE
	// Musical instruments
	case proficiencies.ToolBagpipes:
		return dnd5ev1alpha1.Tool_TOOL_BAGPIPES
	case proficiencies.ToolDrum:
		return dnd5ev1alpha1.Tool_TOOL_DRUM
	case proficiencies.ToolDulcimer:
		return dnd5ev1alpha1.Tool_TOOL_DULCIMER
	case proficiencies.ToolFlute:
		return dnd5ev1alpha1.Tool_TOOL_FLUTE
	case proficiencies.ToolLute:
		return dnd5ev1alpha1.Tool_TOOL_LUTE
	case proficiencies.ToolLyre:
		return dnd5ev1alpha1.Tool_TOOL_LYRE
	case proficiencies.ToolHorn:
		return dnd5ev1alpha1.Tool_TOOL_HORN
	case proficiencies.ToolPanFlute:
		return dnd5ev1alpha1.Tool_TOOL_PAN_FLUTE
	case proficiencies.ToolShawm:
		return dnd5ev1alpha1.Tool_TOOL_SHAWM
	case proficiencies.ToolViol:
		return dnd5ev1alpha1.Tool_TOOL_VIOL
	// Other tools
	case proficiencies.ToolDisguiseKit:
		return dnd5ev1alpha1.Tool_TOOL_DISGUISE_KIT
	case proficiencies.ToolForgeryKit:
		return dnd5ev1alpha1.Tool_TOOL_FORGERY_KIT
	case proficiencies.ToolHerbalism:
		return dnd5ev1alpha1.Tool_TOOL_HERBALISM_KIT
	case proficiencies.ToolNavigator:
		return dnd5ev1alpha1.Tool_TOOL_NAVIGATOR_TOOLS
	case proficiencies.ToolPoisoner:
		return dnd5ev1alpha1.Tool_TOOL_POISONER_KIT
	case proficiencies.ToolThieves:
		return dnd5ev1alpha1.Tool_TOOL_THIEVES_TOOLS
	case proficiencies.ToolVehicleLand:
		return dnd5ev1alpha1.Tool_TOOL_VEHICLES_LAND
	case proficiencies.ToolVehicleWater:
		return dnd5ev1alpha1.Tool_TOOL_VEHICLES_WATER
	default:
		return dnd5ev1alpha1.Tool_TOOL_UNSPECIFIED
	}
}

// convertChoiceToProto converts a single choice
func convertChoiceToProto(choice choices.ChoiceData) *dnd5ev1alpha1.ChoiceData {
	protoChoice := &dnd5ev1alpha1.ChoiceData{
		Category: convertChoiceCategoryToProto(choice.Category),
		Source:   convertChoiceSourceToProto(choice.Source),
		ChoiceId: string(choice.ChoiceID),
	}

	// Map selections based on category
	switch choice.Category {
	case shared.ChoiceSkills:
		if len(choice.SkillSelection) > 0 {
			skills := make([]dnd5ev1alpha1.Skill, 0, len(choice.SkillSelection))
			for _, skill := range choice.SkillSelection {
				skills = append(skills, convertSkillToProtoEnum(skill))
			}
			protoChoice.Selection = &dnd5ev1alpha1.ChoiceData_Skills{
				Skills: &dnd5ev1alpha1.SkillSelection{Skills: skills},
			}
		}
	case shared.ChoiceLanguages:
		if len(choice.LanguageSelection) > 0 {
			langs := make([]dnd5ev1alpha1.Language, 0, len(choice.LanguageSelection))
			for _, lang := range choice.LanguageSelection {
				langs = append(langs, convertLanguageToProtoEnum(lang))
			}
			protoChoice.Selection = &dnd5ev1alpha1.ChoiceData_Languages{
				Languages: &dnd5ev1alpha1.LanguageSelection{Languages: langs},
			}
		}
	case shared.ChoiceEquipment:
		if len(choice.EquipmentSelection) > 0 {
			protoChoice.Selection = &dnd5ev1alpha1.ChoiceData_Equipment{
				Equipment: convertEquipmentSelectionToProto(choice.EquipmentSelection),
			}
		}
	case shared.ChoiceFightingStyle:
		if choice.FightingStyleSelection != nil {
			protoChoice.Selection = &dnd5ev1alpha1.ChoiceData_FightingStyle{
				FightingStyle: &dnd5ev1alpha1.FightingStyleSelection{
					Style: convertFightingStyleToProto(*choice.FightingStyleSelection),
				},
			}
		}
	case shared.ChoiceBackground:
		if choice.BackgroundSelection != nil {
			protoChoice.Selection = &dnd5ev1alpha1.ChoiceData_Background{
				Background: convertBackgroundToProtoEnum(*choice.BackgroundSelection),
			}
		}
	}

	// Handle spell selection
	if len(choice.SpellSelection) > 0 {
		spells := make([]dnd5ev1alpha1.Spell, 0, len(choice.SpellSelection))
		for range choice.SpellSelection {
			// TODO: Convert to Spell enum when available, for now use SPELL_UNSPECIFIED
			spells = append(spells, dnd5ev1alpha1.Spell_SPELL_UNSPECIFIED)
		}
		protoChoice.Selection = &dnd5ev1alpha1.ChoiceData_Spells{
			Spells: &dnd5ev1alpha1.SpellSelection{Spells: spells},
		}
	}

	// Handle tool selection
	if len(choice.ToolSelection) > 0 {
		tools := make([]dnd5ev1alpha1.Tool, 0, len(choice.ToolSelection))
		for _, tool := range choice.ToolSelection {
			tools = append(tools, convertProficiencyToolToProtoEnum(tool))
		}
		protoChoice.Selection = &dnd5ev1alpha1.ChoiceData_Tools{
			Tools: &dnd5ev1alpha1.ToolSelection{Tools: tools},
		}
	}

	// Handle expertise selection
	if len(choice.ExpertiseSelection) > 0 {
		expertise := make([]dnd5ev1alpha1.Skill, 0, len(choice.ExpertiseSelection))
		for _, skill := range choice.ExpertiseSelection {
			expertise = append(expertise, convertSkillToProtoEnum(skill))
		}
		protoChoice.Selection = &dnd5ev1alpha1.ChoiceData_Expertise{
			Expertise: &dnd5ev1alpha1.ExpertiseSelection{Skills: expertise},
		}
	}

	// Handle trait selection
	if len(choice.TraitSelection) > 0 {
		traits := make([]dnd5ev1alpha1.Trait, 0, len(choice.TraitSelection))
		for range choice.TraitSelection {
			// TODO: Convert to Trait enum when available, for now use TRAIT_UNSPECIFIED
			traits = append(traits, dnd5ev1alpha1.Trait_TRAIT_UNSPECIFIED)
		}
		// TODO: Handle trait selection when proto supports it
		// For now, skip traits as the proto structure has changed
		_ = traits
	}

	// Handle ability scores if present (for ability score improvement choices)
	if choice.AbilityScoreSelection != nil {
		protoChoice.Selection = &dnd5ev1alpha1.ChoiceData_AbilityScores{
			AbilityScores: convertAbilityScoresToProto(choice.AbilityScoreSelection),
		}
	}

	// Handle name selection
	if choice.NameSelection != nil {
		protoChoice.Selection = &dnd5ev1alpha1.ChoiceData_Name{
			Name: *choice.NameSelection,
		}
	}

	return protoChoice
}

// Enum converters for race, class, background, etc.

func convertRaceToProtoEnum(race races.Race) dnd5ev1alpha1.Race {
	// Map toolkit race string to proto enum
	switch race {
	case races.Human:
		return dnd5ev1alpha1.Race_RACE_HUMAN
	case races.Elf:
		return dnd5ev1alpha1.Race_RACE_ELF
	case races.Dwarf:
		return dnd5ev1alpha1.Race_RACE_DWARF
	case races.Halfling:
		return dnd5ev1alpha1.Race_RACE_HALFLING
	case races.Dragonborn:
		return dnd5ev1alpha1.Race_RACE_DRAGONBORN
	case races.Gnome:
		return dnd5ev1alpha1.Race_RACE_GNOME
	case races.HalfElf:
		return dnd5ev1alpha1.Race_RACE_HALF_ELF
	case races.HalfOrc:
		return dnd5ev1alpha1.Race_RACE_HALF_ORC
	case races.Tiefling:
		return dnd5ev1alpha1.Race_RACE_TIEFLING
	default:
		return dnd5ev1alpha1.Race_RACE_UNSPECIFIED
	}
}

func convertSubraceToProtoEnum(subrace races.Subrace) dnd5ev1alpha1.Subrace {
	switch subrace {
	case races.WoodElf:
		return dnd5ev1alpha1.Subrace_SUBRACE_WOOD_ELF
	case races.DarkElf:
		return dnd5ev1alpha1.Subrace_SUBRACE_DARK_ELF
	case races.HillDwarf:
		return dnd5ev1alpha1.Subrace_SUBRACE_HILL_DWARF
	case races.MountainDwarf:
		return dnd5ev1alpha1.Subrace_SUBRACE_MOUNTAIN_DWARF
	case races.LightfootHalfling:
		return dnd5ev1alpha1.Subrace_SUBRACE_LIGHTFOOT_HALFLING
	case races.StoutHalfling:
		return dnd5ev1alpha1.Subrace_SUBRACE_STOUT_HALFLING
	case races.ForestGnome:
		return dnd5ev1alpha1.Subrace_SUBRACE_FOREST_GNOME
	case races.RockGnome:
		return dnd5ev1alpha1.Subrace_SUBRACE_ROCK_GNOME
	default:
		return dnd5ev1alpha1.Subrace_SUBRACE_UNSPECIFIED
	}
}

func convertClassToProtoEnum(class classes.Class) dnd5ev1alpha1.Class {
	switch class {
	case classes.Barbarian:
		return dnd5ev1alpha1.Class_CLASS_BARBARIAN
	case classes.Bard:
		return dnd5ev1alpha1.Class_CLASS_BARD
	case classes.Cleric:
		return dnd5ev1alpha1.Class_CLASS_CLERIC
	case classes.Druid:
		return dnd5ev1alpha1.Class_CLASS_DRUID
	case classes.Fighter:
		return dnd5ev1alpha1.Class_CLASS_FIGHTER
	case classes.Monk:
		return dnd5ev1alpha1.Class_CLASS_MONK
	case classes.Paladin:
		return dnd5ev1alpha1.Class_CLASS_PALADIN
	case classes.Ranger:
		return dnd5ev1alpha1.Class_CLASS_RANGER
	case classes.Rogue:
		return dnd5ev1alpha1.Class_CLASS_ROGUE
	case classes.Sorcerer:
		return dnd5ev1alpha1.Class_CLASS_SORCERER
	case classes.Warlock:
		return dnd5ev1alpha1.Class_CLASS_WARLOCK
	case classes.Wizard:
		return dnd5ev1alpha1.Class_CLASS_WIZARD
	default:
		return dnd5ev1alpha1.Class_CLASS_UNSPECIFIED
	}
}

func convertSubclassToProtoEnum(subclass classes.Subclass) dnd5ev1alpha1.Subclass {
	// TODO: fill in remaining
	switch subclass {
	case classes.Champion:
		return dnd5ev1alpha1.Subclass_SUBCLASS_CHAMPION
	case classes.BattleMaster:
		return dnd5ev1alpha1.Subclass_SUBCLASS_BATTLE_MASTER
	case classes.EldritchKnight:
		return dnd5ev1alpha1.Subclass_SUBCLASS_ELDRITCH_KNIGHT
	// Add more as needed
	default:
		return dnd5ev1alpha1.Subclass_SUBCLASS_UNSPECIFIED
	}
}

func convertBackgroundToProtoEnum(background backgrounds.Background) dnd5ev1alpha1.Background {
	switch background {
	case backgrounds.Acolyte:
		return dnd5ev1alpha1.Background_BACKGROUND_ACOLYTE
	case backgrounds.Charlatan:
		return dnd5ev1alpha1.Background_BACKGROUND_CHARLATAN
	case backgrounds.Criminal:
		return dnd5ev1alpha1.Background_BACKGROUND_CRIMINAL
	case backgrounds.Entertainer:
		return dnd5ev1alpha1.Background_BACKGROUND_ENTERTAINER
	case backgrounds.FolkHero:
		return dnd5ev1alpha1.Background_BACKGROUND_FOLK_HERO
	case backgrounds.GuildArtisan:
		return dnd5ev1alpha1.Background_BACKGROUND_GUILD_ARTISAN
	case backgrounds.Hermit:
		return dnd5ev1alpha1.Background_BACKGROUND_HERMIT
	case backgrounds.Noble:
		return dnd5ev1alpha1.Background_BACKGROUND_NOBLE
	case backgrounds.Outlander:
		return dnd5ev1alpha1.Background_BACKGROUND_OUTLANDER
	case backgrounds.Sage:
		return dnd5ev1alpha1.Background_BACKGROUND_SAGE
	case backgrounds.Sailor:
		return dnd5ev1alpha1.Background_BACKGROUND_SAILOR
	case backgrounds.Soldier:
		return dnd5ev1alpha1.Background_BACKGROUND_SOLDIER
	case backgrounds.Urchin:
		return dnd5ev1alpha1.Background_BACKGROUND_URCHIN
	default:
		return dnd5ev1alpha1.Background_BACKGROUND_UNSPECIFIED
	}
}

func convertChoiceCategoryToProto(category shared.ChoiceCategory) dnd5ev1alpha1.ChoiceCategory {
	switch category {
	case shared.ChoiceSkills:
		return dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SKILLS
	case shared.ChoiceLanguages:
		return dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_LANGUAGES
	case shared.ChoiceEquipment:
		return dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT
	case shared.ChoiceFightingStyle:
		return dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_FIGHTING_STYLE
	case shared.ChoiceRace:
		return dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_RACE
	case shared.ChoiceClass:
		return dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_CLASS
	case shared.ChoiceBackground:
		return dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_BACKGROUND
	// Add more as needed
	default:
		return dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_UNSPECIFIED
	}
}

func convertChoiceSourceToProto(source shared.ChoiceSource) dnd5ev1alpha1.ChoiceSource {
	switch source {
	case shared.SourcePlayer:
		return dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_PLAYER
	case shared.SourceRace:
		return dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_RACE
	case shared.SourceSubrace:
		return dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_SUBRACE
	case shared.SourceClass:
		return dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS
	case shared.SourceBackground:
		return dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_BACKGROUND
	// Add more as needed
	default:
		return dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_UNSPECIFIED
	}
}

// convertRaceDataToProto converts toolkit races.Data to proto RaceInfo
func convertRaceDataToProto(data *races.Data) *dnd5ev1alpha1.RaceInfo {
	if data == nil {
		return nil
	}

	// Convert ability bonuses map
	abilityBonuses := make(map[string]int32)
	for ability, bonus := range data.AbilityIncreases {
		abilityBonuses[string(ability)] = int32(bonus)
	}

	// Convert languages
	langs := make([]dnd5ev1alpha1.Language, 0, len(data.Languages))
	for _, lang := range data.Languages {
		langs = append(langs, convertLanguageToProtoEnum(lang))
	}

	// TODO: Convert skills when proto supports them
	// Skills are available in data.Skills but proto doesn't have the field yet

	// TODO: Convert Size string to proto enum when available
	// TODO: Convert Traits when proto supports them
	// TODO: Convert subraces when proto supports them

	return &dnd5ev1alpha1.RaceInfo{
		RaceId:         convertRaceToProtoEnum(data.ID),
		Name:           data.Name(),
		Description:    data.Description(),
		Speed:          int32(data.Speed),
		AbilityBonuses: abilityBonuses,
		Languages:      langs,
	}
}

// convertClassDataToProto converts toolkit classes.Data to proto ClassInfo
func convertClassDataToProto(data *classes.Data) *dnd5ev1alpha1.ClassInfo {
	if data == nil {
		return nil
	}

	// Convert primary ability
	var primaryAbility dnd5ev1alpha1.Ability
	switch data.PrimaryAbility {
	case abilities.STR:
		primaryAbility = dnd5ev1alpha1.Ability_ABILITY_STRENGTH
	case abilities.DEX:
		primaryAbility = dnd5ev1alpha1.Ability_ABILITY_DEXTERITY
	case abilities.CON:
		primaryAbility = dnd5ev1alpha1.Ability_ABILITY_CONSTITUTION
	case abilities.INT:
		primaryAbility = dnd5ev1alpha1.Ability_ABILITY_INTELLIGENCE
	case abilities.WIS:
		primaryAbility = dnd5ev1alpha1.Ability_ABILITY_WISDOM
	case abilities.CHA:
		primaryAbility = dnd5ev1alpha1.Ability_ABILITY_CHARISMA
	}

	// GRANTS - Things automatically given (no choice)
	// Convert armor proficiency categories
	armorProfCategories := make([]dnd5ev1alpha1.ArmorProficiencyCategory, 0, len(data.Armor))
	for _, armor := range data.Armor {
		if cat := convertArmorProficiencyCategoryToProto(armor); cat != dnd5ev1alpha1.ArmorProficiencyCategory_ARMOR_PROFICIENCY_CATEGORY_UNSPECIFIED {
			armorProfCategories = append(armorProfCategories, cat)
		}
	}

	// Convert weapon proficiency categories and specific weapons
	weaponProfCategories := make([]dnd5ev1alpha1.WeaponProficiencyCategory, 0)
	specificWeapons := make([]dnd5ev1alpha1.Weapon, 0)
	for _, weapon := range data.Weapons {
		cat, specific := convertWeaponProficiencyToProto(weapon)
		if cat != dnd5ev1alpha1.WeaponProficiencyCategory_WEAPON_PROFICIENCY_CATEGORY_UNSPECIFIED {
			weaponProfCategories = append(weaponProfCategories, cat)
		}
		if specific != dnd5ev1alpha1.Weapon_WEAPON_UNSPECIFIED {
			specificWeapons = append(specificWeapons, specific)
		}
	}

	// Convert tool proficiencies (if automatic grants)
	toolProfs := make([]dnd5ev1alpha1.Tool, 0, len(data.Tools))
	for _, tool := range data.Tools {
		toolProfs = append(toolProfs, convertToolProficiencyToProto(tool))
	}

	// Convert saving throw proficiencies (automatic grants)
	savingThrows := make([]dnd5ev1alpha1.Ability, 0, len(data.SavingThrows))
	for _, save := range data.SavingThrows {
		switch save {
		case abilities.STR:
			savingThrows = append(savingThrows, dnd5ev1alpha1.Ability_ABILITY_STRENGTH)
		case abilities.DEX:
			savingThrows = append(savingThrows, dnd5ev1alpha1.Ability_ABILITY_DEXTERITY)
		case abilities.CON:
			savingThrows = append(savingThrows, dnd5ev1alpha1.Ability_ABILITY_CONSTITUTION)
		case abilities.INT:
			savingThrows = append(savingThrows, dnd5ev1alpha1.Ability_ABILITY_INTELLIGENCE)
		case abilities.WIS:
			savingThrows = append(savingThrows, dnd5ev1alpha1.Ability_ABILITY_WISDOM)
		case abilities.CHA:
			savingThrows = append(savingThrows, dnd5ev1alpha1.Ability_ABILITY_CHARISMA)
		}
	}

	// REQUIREMENTS - Load ALL choices from toolkit
	allChoices := loadAllClassChoices(data.ID)

	return &dnd5ev1alpha1.ClassInfo{
		ClassId:        convertClassToProtoEnum(data.ID),
		Name:           data.Name(),
		Description:    data.Description(),
		HitDie:         int32(data.HitDice),
		PrimaryAbility: primaryAbility,
		// Grants (automatic)
		ArmorProficiencyCategories:  armorProfCategories,
		WeaponProficiencyCategories: weaponProfCategories,
		SpecificWeaponProficiencies: specificWeapons,
		ToolProficiencies:           toolProfs,
		SavingThrowProficiencies:    savingThrows,
		// Requirements (choices) - ALL in one place
		Choices: allChoices,
	}
}

// convertBackgroundDataToProto converts toolkit backgrounds.Data to proto BackgroundInfo
func convertBackgroundDataToProto(data *backgrounds.Data) *dnd5ev1alpha1.BackgroundInfo {
	if data == nil {
		return nil
	}

	// Convert skills
	skillList := make([]dnd5ev1alpha1.Skill, 0, len(data.Skills))
	for _, skill := range data.Skills {
		skillList = append(skillList, convertSkillToProtoEnum(skill))
	}

	// TODO: Convert tool proficiencies when proto enums are available
	// TODO: Convert languages when background data includes them
	// TODO: Convert starting equipment when available

	return &dnd5ev1alpha1.BackgroundInfo{
		BackgroundId:        convertBackgroundToProtoEnum(data.ID),
		Name:                data.Name(),
		Description:         data.Description(),
		SkillProficiencies:  skillList,
		AdditionalLanguages: int32(data.LanguageCount),
	}
}

// convertCharacterDataToProto converts toolkit character.Data to proto Character
func convertCharacterDataToProto(data *toolkitchar.Data) *dnd5ev1alpha1.Character {
	if data == nil {
		return nil
	}

	char := &dnd5ev1alpha1.Character{
		Id:    data.ID,
		Name:  data.Name,
		Level: int32(data.Level),
	}

	// Convert race and subrace
	if data.RaceID != "" {
		char.Race = convertRaceToProtoEnum(data.RaceID)
	}
	if data.SubraceID != "" {
		char.Subrace = convertSubraceToProtoEnum(data.SubraceID)
	}

	// Convert class (subclass is not in the Character proto yet)
	if data.ClassID != "" {
		char.Class = convertClassToProtoEnum(data.ClassID)
	}

	// Convert background
	if data.BackgroundID != "" {
		char.Background = convertBackgroundToProtoEnum(data.BackgroundID)
	}

	// Convert ability scores
	char.AbilityScores = convertAbilityScoresToProto(data.AbilityScores)

	// Convert combat stats using nested structure
	char.CombatStats = &dnd5ev1alpha1.CombatStats{
		HitPointMaximum:  int32(data.MaxHitPoints),
		ArmorClass:       int32(data.ArmorClass),
		ProficiencyBonus: int32(data.ProficiencyBonus),
		// TODO: Add initiative and speed when available in toolkit
	}

	// Set current hit points
	char.CurrentHitPoints = int32(data.HitPoints)

	// Convert proficiencies using nested structure
	skillList := make([]dnd5ev1alpha1.Skill, 0)
	for skill, profLevel := range data.Skills {
		if profLevel != shared.NotProficient {
			skillList = append(skillList, convertSkillToProtoEnum(skill))
		}
	}

	// Convert saving throws for proficiencies
	savingThrows := make([]dnd5ev1alpha1.Ability, 0)
	for ability, profLevel := range data.SavingThrows {
		if profLevel != shared.NotProficient {
			var protoAbility dnd5ev1alpha1.Ability
			switch ability {
			case abilities.STR:
				protoAbility = dnd5ev1alpha1.Ability_ABILITY_STRENGTH
			case abilities.DEX:
				protoAbility = dnd5ev1alpha1.Ability_ABILITY_DEXTERITY
			case abilities.CON:
				protoAbility = dnd5ev1alpha1.Ability_ABILITY_CONSTITUTION
			case abilities.INT:
				protoAbility = dnd5ev1alpha1.Ability_ABILITY_INTELLIGENCE
			case abilities.WIS:
				protoAbility = dnd5ev1alpha1.Ability_ABILITY_WISDOM
			case abilities.CHA:
				protoAbility = dnd5ev1alpha1.Ability_ABILITY_CHARISMA
			}
			savingThrows = append(savingThrows, protoAbility)
		}
	}

	// Set proficiencies structure
	char.Proficiencies = &dnd5ev1alpha1.Proficiencies{
		Skills:       skillList,
		SavingThrows: savingThrows,
		// TODO: Add armor, weapons, and tools when we have that data
	}

	// Convert languages to enum
	char.Languages = make([]dnd5ev1alpha1.Language, 0, len(data.Languages))
	// TODO: Convert string to Language enum when we have the mapping
	// For now, skip languages

	// Convert inventory
	char.Inventory = make([]*dnd5ev1alpha1.InventoryItem, 0, len(data.Inventory))
	for _, item := range data.Inventory {
		char.Inventory = append(char.Inventory, &dnd5ev1alpha1.InventoryItem{
			ItemId:   item.ID,
			Quantity: int32(item.Quantity),
			// TODO: Add equipment data when available
		})
	}

	// TODO: Convert spell slots when the proto supports them

	// Convert class resources
	for _, resource := range data.ClassResources {
		// TODO: Convert to proto ClassResource when available
		_ = resource // Avoid unused variable warning
	}

	// Set metadata
	if data.CreatedAt.Unix() > 0 || data.UpdatedAt.Unix() > 0 {
		char.Metadata = &dnd5ev1alpha1.CharacterMetadata{
			PlayerId:  data.PlayerID,
			CreatedAt: data.CreatedAt.Unix(),
			UpdatedAt: data.UpdatedAt.Unix(),
		}
	}

	return char
}

// convertFightingStyleToProto converts toolkit FightingStyle to proto enum
func convertFightingStyleToProto(style fightingstyles.FightingStyle) dnd5ev1alpha1.FightingStyle {
	switch style {
	case fightingstyles.Archery:
		return dnd5ev1alpha1.FightingStyle_FIGHTING_STYLE_ARCHERY
	case fightingstyles.Defense:
		return dnd5ev1alpha1.FightingStyle_FIGHTING_STYLE_DEFENSE
	case fightingstyles.Dueling:
		return dnd5ev1alpha1.FightingStyle_FIGHTING_STYLE_DUELING
	case fightingstyles.GreatWeaponFighting:
		return dnd5ev1alpha1.FightingStyle_FIGHTING_STYLE_GREAT_WEAPON_FIGHTING
	case fightingstyles.Protection:
		return dnd5ev1alpha1.FightingStyle_FIGHTING_STYLE_PROTECTION
	case fightingstyles.TwoWeaponFighting:
		return dnd5ev1alpha1.FightingStyle_FIGHTING_STYLE_TWO_WEAPON_FIGHTING
	default:
		return dnd5ev1alpha1.FightingStyle_FIGHTING_STYLE_UNSPECIFIED
	}
}

// convertWeaponToProto converts toolkit WeaponID to proto enum
func convertWeaponToProto(id weapons.WeaponID) dnd5ev1alpha1.Weapon {
	switch id {
	// Simple Melee
	case weapons.Club:
		return dnd5ev1alpha1.Weapon_WEAPON_CLUB
	case weapons.Dagger:
		return dnd5ev1alpha1.Weapon_WEAPON_DAGGER
	case weapons.Greatclub:
		return dnd5ev1alpha1.Weapon_WEAPON_GREATCLUB
	case weapons.Handaxe:
		return dnd5ev1alpha1.Weapon_WEAPON_HANDAXE
	case weapons.Javelin:
		return dnd5ev1alpha1.Weapon_WEAPON_JAVELIN
	case weapons.LightHammer:
		return dnd5ev1alpha1.Weapon_WEAPON_LIGHT_HAMMER
	case weapons.Mace:
		return dnd5ev1alpha1.Weapon_WEAPON_MACE
	case weapons.Quarterstaff:
		return dnd5ev1alpha1.Weapon_WEAPON_QUARTERSTAFF
	case weapons.Sickle:
		return dnd5ev1alpha1.Weapon_WEAPON_SICKLE
	case weapons.Spear:
		return dnd5ev1alpha1.Weapon_WEAPON_SPEAR
	// Simple Ranged
	case weapons.LightCrossbow:
		return dnd5ev1alpha1.Weapon_WEAPON_LIGHT_CROSSBOW
	case weapons.Dart:
		return dnd5ev1alpha1.Weapon_WEAPON_DART
	case weapons.Shortbow:
		return dnd5ev1alpha1.Weapon_WEAPON_SHORTBOW
	case weapons.Sling:
		return dnd5ev1alpha1.Weapon_WEAPON_SLING
	// Martial Melee
	case weapons.Battleaxe:
		return dnd5ev1alpha1.Weapon_WEAPON_BATTLEAXE
	case weapons.Flail:
		return dnd5ev1alpha1.Weapon_WEAPON_FLAIL
	case weapons.Glaive:
		return dnd5ev1alpha1.Weapon_WEAPON_GLAIVE
	case weapons.Greataxe:
		return dnd5ev1alpha1.Weapon_WEAPON_GREATAXE
	case weapons.Greatsword:
		return dnd5ev1alpha1.Weapon_WEAPON_GREATSWORD
	case weapons.Halberd:
		return dnd5ev1alpha1.Weapon_WEAPON_HALBERD
	case weapons.Lance:
		return dnd5ev1alpha1.Weapon_WEAPON_LANCE
	case weapons.Longsword:
		return dnd5ev1alpha1.Weapon_WEAPON_LONGSWORD
	case weapons.Maul:
		return dnd5ev1alpha1.Weapon_WEAPON_MAUL
	case weapons.Morningstar:
		return dnd5ev1alpha1.Weapon_WEAPON_MORNINGSTAR
	case weapons.Pike:
		return dnd5ev1alpha1.Weapon_WEAPON_PIKE
	case weapons.Rapier:
		return dnd5ev1alpha1.Weapon_WEAPON_RAPIER
	case weapons.Scimitar:
		return dnd5ev1alpha1.Weapon_WEAPON_SCIMITAR
	case weapons.Shortsword:
		return dnd5ev1alpha1.Weapon_WEAPON_SHORTSWORD
	case weapons.Trident:
		return dnd5ev1alpha1.Weapon_WEAPON_TRIDENT
	case weapons.WarPick:
		return dnd5ev1alpha1.Weapon_WEAPON_WAR_PICK
	case weapons.Warhammer:
		return dnd5ev1alpha1.Weapon_WEAPON_WARHAMMER
	case weapons.Whip:
		return dnd5ev1alpha1.Weapon_WEAPON_WHIP
	// Martial Ranged
	case weapons.Blowgun:
		return dnd5ev1alpha1.Weapon_WEAPON_BLOWGUN
	case weapons.HandCrossbow:
		return dnd5ev1alpha1.Weapon_WEAPON_HAND_CROSSBOW
	case weapons.HeavyCrossbow:
		return dnd5ev1alpha1.Weapon_WEAPON_HEAVY_CROSSBOW
	case weapons.Longbow:
		return dnd5ev1alpha1.Weapon_WEAPON_LONGBOW
	case weapons.Net:
		return dnd5ev1alpha1.Weapon_WEAPON_NET
	// Ammunition
	case weapons.Arrows20:
		return dnd5ev1alpha1.Weapon_WEAPON_ARROWS_20
	case weapons.Bolts20:
		return dnd5ev1alpha1.Weapon_WEAPON_BOLTS_20
	// Category placeholders
	case weapons.AnySimpleWeapon:
		return dnd5ev1alpha1.Weapon_WEAPON_ANY_SIMPLE
	case weapons.AnyMartialWeapon:
		return dnd5ev1alpha1.Weapon_WEAPON_ANY_MARTIAL
	case weapons.AnyWeapon:
		return dnd5ev1alpha1.Weapon_WEAPON_ANY
	default:
		return dnd5ev1alpha1.Weapon_WEAPON_UNSPECIFIED
	}
}

// convertArmorToProto converts toolkit ArmorID to proto enum
func convertArmorToProto(id armor.ArmorID) dnd5ev1alpha1.Armor {
	switch id {
	// Light Armor
	case "padded":
		return dnd5ev1alpha1.Armor_ARMOR_PADDED
	case "leather":
		return dnd5ev1alpha1.Armor_ARMOR_LEATHER
	case "studded-leather":
		return dnd5ev1alpha1.Armor_ARMOR_STUDDED_LEATHER
	// Medium Armor
	case "hide":
		return dnd5ev1alpha1.Armor_ARMOR_HIDE
	case "chain-shirt":
		return dnd5ev1alpha1.Armor_ARMOR_CHAIN_SHIRT
	case "scale-mail":
		return dnd5ev1alpha1.Armor_ARMOR_SCALE_MAIL
	case "breastplate":
		return dnd5ev1alpha1.Armor_ARMOR_BREASTPLATE
	case "half-plate":
		return dnd5ev1alpha1.Armor_ARMOR_HALF_PLATE
	// Heavy Armor
	case "ring-mail":
		return dnd5ev1alpha1.Armor_ARMOR_RING_MAIL
	case "chain-mail":
		return dnd5ev1alpha1.Armor_ARMOR_CHAIN_MAIL
	case "splint":
		return dnd5ev1alpha1.Armor_ARMOR_SPLINT
	case "plate":
		return dnd5ev1alpha1.Armor_ARMOR_PLATE
	// Shield
	case "shield":
		return dnd5ev1alpha1.Armor_ARMOR_SHIELD
	default:
		return dnd5ev1alpha1.Armor_ARMOR_UNSPECIFIED
	}
}

// convertToolToProto converts toolkit ToolID to proto enum
func convertToolToProto(id tools.ToolID) dnd5ev1alpha1.Tool {
	switch id {
	// Artisan's tools
	case tools.AlchemistSupplies:
		return dnd5ev1alpha1.Tool_TOOL_ALCHEMIST_SUPPLIES
	case tools.BrewerSupplies:
		return dnd5ev1alpha1.Tool_TOOL_BREWER_SUPPLIES
	case tools.CalligrapherSupplies:
		return dnd5ev1alpha1.Tool_TOOL_CALLIGRAPHER_SUPPLIES
	case tools.CarpenterTools:
		return dnd5ev1alpha1.Tool_TOOL_CARPENTER_TOOLS
	case tools.CartographerTools:
		return dnd5ev1alpha1.Tool_TOOL_CARTOGRAPHER_TOOLS
	case tools.CobblerTools:
		return dnd5ev1alpha1.Tool_TOOL_COBBLER_TOOLS
	case tools.CookUtensils:
		return dnd5ev1alpha1.Tool_TOOL_COOK_UTENSILS
	case tools.GlassblowerTools:
		return dnd5ev1alpha1.Tool_TOOL_GLASSBLOWER_TOOLS
	case tools.JewelerTools:
		return dnd5ev1alpha1.Tool_TOOL_JEWELER_TOOLS
	case tools.LeatherworkerTools:
		return dnd5ev1alpha1.Tool_TOOL_LEATHERWORKER_TOOLS
	case tools.MasonTools:
		return dnd5ev1alpha1.Tool_TOOL_MASON_TOOLS
	case tools.PainterSupplies:
		return dnd5ev1alpha1.Tool_TOOL_PAINTER_SUPPLIES
	case tools.PotterTools:
		return dnd5ev1alpha1.Tool_TOOL_POTTER_TOOLS
	case tools.SmithTools:
		return dnd5ev1alpha1.Tool_TOOL_SMITH_TOOLS
	case tools.TinkerTools:
		return dnd5ev1alpha1.Tool_TOOL_TINKER_TOOLS
	case tools.WeaverTools:
		return dnd5ev1alpha1.Tool_TOOL_WEAVER_TOOLS
	case tools.WoodcarverTools:
		return dnd5ev1alpha1.Tool_TOOL_WOODCARVER_TOOLS
	// Gaming sets
	case tools.DiceSet:
		return dnd5ev1alpha1.Tool_TOOL_DICE_SET
	case tools.DragonchessSet:
		return dnd5ev1alpha1.Tool_TOOL_DRAGONCHESS_SET
	case tools.PlayingCardSet:
		return dnd5ev1alpha1.Tool_TOOL_PLAYING_CARD_SET
	case tools.ThreeDragonAnte:
		return dnd5ev1alpha1.Tool_TOOL_THREE_DRAGON_ANTE
	// Musical instruments
	case tools.Bagpipes:
		return dnd5ev1alpha1.Tool_TOOL_BAGPIPES
	case tools.Drum:
		return dnd5ev1alpha1.Tool_TOOL_DRUM
	case tools.Dulcimer:
		return dnd5ev1alpha1.Tool_TOOL_DULCIMER
	case tools.Flute:
		return dnd5ev1alpha1.Tool_TOOL_FLUTE
	case tools.Lute:
		return dnd5ev1alpha1.Tool_TOOL_LUTE
	case tools.Lyre:
		return dnd5ev1alpha1.Tool_TOOL_LYRE
	case tools.Horn:
		return dnd5ev1alpha1.Tool_TOOL_HORN
	case tools.PanFlute:
		return dnd5ev1alpha1.Tool_TOOL_PAN_FLUTE
	case tools.Shawm:
		return dnd5ev1alpha1.Tool_TOOL_SHAWM
	case tools.Viol:
		return dnd5ev1alpha1.Tool_TOOL_VIOL
	// Other tools
	case tools.DisguiseKit:
		return dnd5ev1alpha1.Tool_TOOL_DISGUISE_KIT
	case tools.ForgeryKit:
		return dnd5ev1alpha1.Tool_TOOL_FORGERY_KIT
	case tools.HerbalismKit:
		return dnd5ev1alpha1.Tool_TOOL_HERBALISM_KIT
	case tools.NavigatorTools:
		return dnd5ev1alpha1.Tool_TOOL_NAVIGATOR_TOOLS
	case tools.PoisonerKit:
		return dnd5ev1alpha1.Tool_TOOL_POISONER_KIT
	case tools.ThievesTools:
		return dnd5ev1alpha1.Tool_TOOL_THIEVES_TOOLS
	case tools.VehiclesLand:
		return dnd5ev1alpha1.Tool_TOOL_VEHICLES_LAND
	case tools.VehiclesWater:
		return dnd5ev1alpha1.Tool_TOOL_VEHICLES_WATER
	default:
		return dnd5ev1alpha1.Tool_TOOL_UNSPECIFIED
	}
}

// convertPackToProto converts toolkit PackID to proto enum
// TODO: Uncomment when Pack enum is available in generated protos
/*
func convertPackToProto(id packs.PackID) dnd5ev1alpha1.Pack {
	switch id {
	case "burglars-pack":
		return dnd5ev1alpha1.Pack_PACK_BURGLARS
	case "diplomats-pack":
		return dnd5ev1alpha1.Pack_PACK_DIPLOMATS
	case "dungeoneers-pack":
		return dnd5ev1alpha1.Pack_PACK_DUNGEONEERS
	case "entertainers-pack":
		return dnd5ev1alpha1.Pack_PACK_ENTERTAINERS
	case "explorers-pack":
		return dnd5ev1alpha1.Pack_PACK_EXPLORERS
	case "priests-pack":
		return dnd5ev1alpha1.Pack_PACK_PRIESTS
	case "scholars-pack":
		return dnd5ev1alpha1.Pack_PACK_SCHOLARS
	default:
		return dnd5ev1alpha1.Pack_PACK_UNSPECIFIED
	}
}
*/

// convertEquipmentSelectionToProto converts toolkit equipment IDs to proto EquipmentSelection
func convertEquipmentSelectionToProto(items []shared.SelectionID) *dnd5ev1alpha1.EquipmentSelection {
	equipmentItems := make([]*dnd5ev1alpha1.EquipmentSelectionItem, 0, len(items))
	for _, item := range items {
		equipItem := &dnd5ev1alpha1.EquipmentSelectionItem{
			Quantity: 1, // Default quantity
		}

		// Try to convert the SelectionID to the appropriate proto enum type
		// The SelectionID in toolkit is a string alias that could be a weapon, armor, tool, pack, etc.
		itemStr := string(item)

		// Try weapon conversion first
		if weapon := convertWeaponToProto(weapons.WeaponID(item)); weapon != dnd5ev1alpha1.Weapon_WEAPON_UNSPECIFIED {
			equipItem.Equipment = &dnd5ev1alpha1.EquipmentSelectionItem_Weapon{
				Weapon: weapon,
			}
		} else if armorEnum := convertArmorToProto(armor.ArmorID(item)); armorEnum != dnd5ev1alpha1.Armor_ARMOR_UNSPECIFIED {
			// Try armor conversion
			equipItem.Equipment = &dnd5ev1alpha1.EquipmentSelectionItem_Armor{
				Armor: armorEnum,
			}
		} else if tool := convertToolToProto(tools.ToolID(item)); tool != dnd5ev1alpha1.Tool_TOOL_UNSPECIFIED {
			// Try tool conversion
			equipItem.Equipment = &dnd5ev1alpha1.EquipmentSelectionItem_Tool{
				Tool: tool,
			}
		} else {
			// Fall back to OtherEquipmentId for anything we can't identify
			// This includes packs and other items not yet enumerated
			equipItem.Equipment = &dnd5ev1alpha1.EquipmentSelectionItem_OtherEquipmentId{
				OtherEquipmentId: itemStr,
			}
		}

		equipmentItems = append(equipmentItems, equipItem)
	}

	return &dnd5ev1alpha1.EquipmentSelection{
		Items: equipmentItems,
	}
}

// convertProtoLanguageToToolkit converts proto Language enum to toolkit Language
func convertProtoLanguageToToolkit(lang dnd5ev1alpha1.Language) languages.Language {
	switch lang {
	case dnd5ev1alpha1.Language_LANGUAGE_COMMON:
		return languages.Common
	case dnd5ev1alpha1.Language_LANGUAGE_DWARVISH:
		return languages.Dwarvish
	case dnd5ev1alpha1.Language_LANGUAGE_ELVISH:
		return languages.Elvish
	case dnd5ev1alpha1.Language_LANGUAGE_GIANT:
		return languages.Giant
	case dnd5ev1alpha1.Language_LANGUAGE_GNOMISH:
		return languages.Gnomish
	case dnd5ev1alpha1.Language_LANGUAGE_GOBLIN:
		return languages.Goblin
	case dnd5ev1alpha1.Language_LANGUAGE_HALFLING:
		return languages.Halfling
	case dnd5ev1alpha1.Language_LANGUAGE_ORC:
		return languages.Orc
	case dnd5ev1alpha1.Language_LANGUAGE_ABYSSAL:
		return languages.Abyssal
	case dnd5ev1alpha1.Language_LANGUAGE_CELESTIAL:
		return languages.Celestial
	case dnd5ev1alpha1.Language_LANGUAGE_DRACONIC:
		return languages.Draconic
	case dnd5ev1alpha1.Language_LANGUAGE_DEEP_SPEECH:
		return languages.DeepSpeech
	case dnd5ev1alpha1.Language_LANGUAGE_INFERNAL:
		return languages.Infernal
	case dnd5ev1alpha1.Language_LANGUAGE_PRIMORDIAL:
		return languages.Primordial
	case dnd5ev1alpha1.Language_LANGUAGE_SYLVAN:
		return languages.Sylvan
	case dnd5ev1alpha1.Language_LANGUAGE_UNDERCOMMON:
		return languages.Undercommon
	default:
		return languages.Common // Default fallback
	}
}

// convertProtoSkillToToolkit converts proto Skill enum to toolkit Skill
func convertProtoSkillToToolkit(skill dnd5ev1alpha1.Skill) skills.Skill {
	switch skill {
	case dnd5ev1alpha1.Skill_SKILL_ACROBATICS:
		return skills.Acrobatics
	case dnd5ev1alpha1.Skill_SKILL_ANIMAL_HANDLING:
		return skills.AnimalHandling
	case dnd5ev1alpha1.Skill_SKILL_ARCANA:
		return skills.Arcana
	case dnd5ev1alpha1.Skill_SKILL_ATHLETICS:
		return skills.Athletics
	case dnd5ev1alpha1.Skill_SKILL_DECEPTION:
		return skills.Deception
	case dnd5ev1alpha1.Skill_SKILL_HISTORY:
		return skills.History
	case dnd5ev1alpha1.Skill_SKILL_INSIGHT:
		return skills.Insight
	case dnd5ev1alpha1.Skill_SKILL_INTIMIDATION:
		return skills.Intimidation
	case dnd5ev1alpha1.Skill_SKILL_INVESTIGATION:
		return skills.Investigation
	case dnd5ev1alpha1.Skill_SKILL_MEDICINE:
		return skills.Medicine
	case dnd5ev1alpha1.Skill_SKILL_NATURE:
		return skills.Nature
	case dnd5ev1alpha1.Skill_SKILL_PERCEPTION:
		return skills.Perception
	case dnd5ev1alpha1.Skill_SKILL_PERFORMANCE:
		return skills.Performance
	case dnd5ev1alpha1.Skill_SKILL_PERSUASION:
		return skills.Persuasion
	case dnd5ev1alpha1.Skill_SKILL_RELIGION:
		return skills.Religion
	case dnd5ev1alpha1.Skill_SKILL_SLEIGHT_OF_HAND:
		return skills.SleightOfHand
	case dnd5ev1alpha1.Skill_SKILL_STEALTH:
		return skills.Stealth
	case dnd5ev1alpha1.Skill_SKILL_SURVIVAL:
		return skills.Survival
	default:
		return skills.Acrobatics // Default fallback
	}
}

// convertProtoRaceToToolkit converts proto Race enum to toolkit Race
func convertProtoRaceToToolkit(race dnd5ev1alpha1.Race) races.Race {
	switch race {
	case dnd5ev1alpha1.Race_RACE_HUMAN:
		return races.Human
	case dnd5ev1alpha1.Race_RACE_ELF:
		return races.Elf
	case dnd5ev1alpha1.Race_RACE_DWARF:
		return races.Dwarf
	case dnd5ev1alpha1.Race_RACE_HALFLING:
		return races.Halfling
	case dnd5ev1alpha1.Race_RACE_DRAGONBORN:
		return races.Dragonborn
	case dnd5ev1alpha1.Race_RACE_GNOME:
		return races.Gnome
	case dnd5ev1alpha1.Race_RACE_HALF_ELF:
		return races.HalfElf
	case dnd5ev1alpha1.Race_RACE_HALF_ORC:
		return races.HalfOrc
	case dnd5ev1alpha1.Race_RACE_TIEFLING:
		return races.Tiefling
	default:
		return races.Human // Default fallback
	}
}

// convertProtoSubraceToToolkit converts proto Subrace enum to toolkit Subrace
func convertProtoSubraceToToolkit(subrace dnd5ev1alpha1.Subrace) races.Subrace {
	switch subrace {
	case dnd5ev1alpha1.Subrace_SUBRACE_WOOD_ELF:
		return races.WoodElf
	case dnd5ev1alpha1.Subrace_SUBRACE_DARK_ELF:
		return races.DarkElf
	case dnd5ev1alpha1.Subrace_SUBRACE_HILL_DWARF:
		return races.HillDwarf
	case dnd5ev1alpha1.Subrace_SUBRACE_MOUNTAIN_DWARF:
		return races.MountainDwarf
	case dnd5ev1alpha1.Subrace_SUBRACE_LIGHTFOOT_HALFLING:
		return races.LightfootHalfling
	case dnd5ev1alpha1.Subrace_SUBRACE_STOUT_HALFLING:
		return races.StoutHalfling
	case dnd5ev1alpha1.Subrace_SUBRACE_FOREST_GNOME:
		return races.ForestGnome
	case dnd5ev1alpha1.Subrace_SUBRACE_ROCK_GNOME:
		return races.RockGnome
	default:
		return "" // No default subrace
	}
}

// convertProtoFightingStyleToToolkit converts proto FightingStyle enum to toolkit FightingStyle
func convertProtoFightingStyleToToolkit(style dnd5ev1alpha1.FightingStyle) fightingstyles.FightingStyle {
	switch style {
	case dnd5ev1alpha1.FightingStyle_FIGHTING_STYLE_ARCHERY:
		return fightingstyles.Archery
	case dnd5ev1alpha1.FightingStyle_FIGHTING_STYLE_DEFENSE:
		return fightingstyles.Defense
	case dnd5ev1alpha1.FightingStyle_FIGHTING_STYLE_DUELING:
		return fightingstyles.Dueling
	case dnd5ev1alpha1.FightingStyle_FIGHTING_STYLE_GREAT_WEAPON_FIGHTING:
		return fightingstyles.GreatWeaponFighting
	case dnd5ev1alpha1.FightingStyle_FIGHTING_STYLE_PROTECTION:
		return fightingstyles.Protection
	case dnd5ev1alpha1.FightingStyle_FIGHTING_STYLE_TWO_WEAPON_FIGHTING:
		return fightingstyles.TwoWeaponFighting
	default:
		return fightingstyles.Defense // Default fallback
	}
}

// convertProtoClassToToolkit converts proto Class enum to toolkit Class
func convertProtoClassToToolkit(class dnd5ev1alpha1.Class) classes.Class {
	switch class {
	case dnd5ev1alpha1.Class_CLASS_BARBARIAN:
		return classes.Barbarian
	case dnd5ev1alpha1.Class_CLASS_BARD:
		return classes.Bard
	case dnd5ev1alpha1.Class_CLASS_CLERIC:
		return classes.Cleric
	case dnd5ev1alpha1.Class_CLASS_DRUID:
		return classes.Druid
	case dnd5ev1alpha1.Class_CLASS_FIGHTER:
		return classes.Fighter
	case dnd5ev1alpha1.Class_CLASS_MONK:
		return classes.Monk
	case dnd5ev1alpha1.Class_CLASS_PALADIN:
		return classes.Paladin
	case dnd5ev1alpha1.Class_CLASS_RANGER:
		return classes.Ranger
	case dnd5ev1alpha1.Class_CLASS_ROGUE:
		return classes.Rogue
	case dnd5ev1alpha1.Class_CLASS_SORCERER:
		return classes.Sorcerer
	case dnd5ev1alpha1.Class_CLASS_WARLOCK:
		return classes.Warlock
	case dnd5ev1alpha1.Class_CLASS_WIZARD:
		return classes.Wizard
	default:
		return classes.Fighter // Default fallback
	}
}

// convertProtoSubclassToToolkit converts proto Subclass enum to toolkit Subclass
func convertProtoSubclassToToolkit(subclass dnd5ev1alpha1.Subclass) classes.Subclass {
	switch subclass {
	case dnd5ev1alpha1.Subclass_SUBCLASS_CHAMPION:
		return classes.Champion
	case dnd5ev1alpha1.Subclass_SUBCLASS_BATTLE_MASTER:
		return classes.BattleMaster
	case dnd5ev1alpha1.Subclass_SUBCLASS_ELDRITCH_KNIGHT:
		return classes.EldritchKnight
	default:
		return "" // No default subclass
	}
}

// convertProtoBackgroundToToolkit converts proto Background enum to toolkit Background
func convertProtoBackgroundToToolkit(background dnd5ev1alpha1.Background) backgrounds.Background {
	switch background {
	case dnd5ev1alpha1.Background_BACKGROUND_ACOLYTE:
		return backgrounds.Acolyte
	case dnd5ev1alpha1.Background_BACKGROUND_CHARLATAN:
		return backgrounds.Charlatan
	case dnd5ev1alpha1.Background_BACKGROUND_CRIMINAL:
		return backgrounds.Criminal
	case dnd5ev1alpha1.Background_BACKGROUND_ENTERTAINER:
		return backgrounds.Entertainer
	case dnd5ev1alpha1.Background_BACKGROUND_FOLK_HERO:
		return backgrounds.FolkHero
	case dnd5ev1alpha1.Background_BACKGROUND_GUILD_ARTISAN:
		return backgrounds.GuildArtisan
	case dnd5ev1alpha1.Background_BACKGROUND_HERMIT:
		return backgrounds.Hermit
	case dnd5ev1alpha1.Background_BACKGROUND_NOBLE:
		return backgrounds.Noble
	case dnd5ev1alpha1.Background_BACKGROUND_OUTLANDER:
		return backgrounds.Outlander
	case dnd5ev1alpha1.Background_BACKGROUND_SAGE:
		return backgrounds.Sage
	case dnd5ev1alpha1.Background_BACKGROUND_SAILOR:
		return backgrounds.Sailor
	case dnd5ev1alpha1.Background_BACKGROUND_SOLDIER:
		return backgrounds.Soldier
	case dnd5ev1alpha1.Background_BACKGROUND_URCHIN:
		return backgrounds.Urchin
	default:
		return backgrounds.Acolyte // Default fallback
	}
}

// convertProtoAbilityScoresToToolkit converts proto AbilityScores to toolkit format
func convertProtoAbilityScoresToToolkit(scores *dnd5ev1alpha1.AbilityScores) shared.AbilityScores {
	if scores == nil {
		return nil
	}

	return shared.AbilityScores{
		abilities.STR: int(scores.Strength),
		abilities.DEX: int(scores.Dexterity),
		abilities.CON: int(scores.Constitution),
		abilities.INT: int(scores.Intelligence),
		abilities.WIS: int(scores.Wisdom),
		abilities.CHA: int(scores.Charisma),
	}
}

// convertArmorProficiencyCategoryToProto converts toolkit armor proficiency to proto category
func convertArmorProficiencyCategoryToProto(armor proficiencies.Armor) dnd5ev1alpha1.ArmorProficiencyCategory {
	switch armor {
	case proficiencies.ArmorLight:
		return dnd5ev1alpha1.ArmorProficiencyCategory_ARMOR_PROFICIENCY_CATEGORY_LIGHT
	case proficiencies.ArmorMedium:
		return dnd5ev1alpha1.ArmorProficiencyCategory_ARMOR_PROFICIENCY_CATEGORY_MEDIUM
	case proficiencies.ArmorHeavy:
		return dnd5ev1alpha1.ArmorProficiencyCategory_ARMOR_PROFICIENCY_CATEGORY_HEAVY
	case proficiencies.ArmorShields:
		return dnd5ev1alpha1.ArmorProficiencyCategory_ARMOR_PROFICIENCY_CATEGORY_SHIELDS
	default:
		return dnd5ev1alpha1.ArmorProficiencyCategory_ARMOR_PROFICIENCY_CATEGORY_UNSPECIFIED
	}
}

// convertWeaponProficiencyToProto converts toolkit weapon proficiency to proto category and specific weapon
func convertWeaponProficiencyToProto(weapon proficiencies.Weapon) (dnd5ev1alpha1.WeaponProficiencyCategory, dnd5ev1alpha1.Weapon) {
	// Check if it's a category
	switch weapon {
	case proficiencies.WeaponSimple:
		return dnd5ev1alpha1.WeaponProficiencyCategory_WEAPON_PROFICIENCY_CATEGORY_SIMPLE, dnd5ev1alpha1.Weapon_WEAPON_UNSPECIFIED
	case proficiencies.WeaponMartial:
		return dnd5ev1alpha1.WeaponProficiencyCategory_WEAPON_PROFICIENCY_CATEGORY_MARTIAL, dnd5ev1alpha1.Weapon_WEAPON_UNSPECIFIED
	}

	// Otherwise it's a specific weapon
	var specificWeapon dnd5ev1alpha1.Weapon
	switch weapon {
	case proficiencies.WeaponClub:
		specificWeapon = dnd5ev1alpha1.Weapon_WEAPON_CLUB
	case proficiencies.WeaponDagger:
		specificWeapon = dnd5ev1alpha1.Weapon_WEAPON_DAGGER
	case proficiencies.WeaponDart:
		specificWeapon = dnd5ev1alpha1.Weapon_WEAPON_DART
	case proficiencies.WeaponJavelin:
		specificWeapon = dnd5ev1alpha1.Weapon_WEAPON_JAVELIN
	case proficiencies.WeaponLightCrossbow:
		specificWeapon = dnd5ev1alpha1.Weapon_WEAPON_LIGHT_CROSSBOW
	case proficiencies.WeaponMace:
		specificWeapon = dnd5ev1alpha1.Weapon_WEAPON_MACE
	case proficiencies.WeaponQuarterstaff:
		specificWeapon = dnd5ev1alpha1.Weapon_WEAPON_QUARTERSTAFF
	case proficiencies.WeaponShortbow:
		specificWeapon = dnd5ev1alpha1.Weapon_WEAPON_SHORTBOW
	case proficiencies.WeaponSickle:
		specificWeapon = dnd5ev1alpha1.Weapon_WEAPON_SICKLE
	case proficiencies.WeaponSling:
		specificWeapon = dnd5ev1alpha1.Weapon_WEAPON_SLING
	case proficiencies.WeaponSpear:
		specificWeapon = dnd5ev1alpha1.Weapon_WEAPON_SPEAR
	case proficiencies.WeaponHandCrossbow:
		specificWeapon = dnd5ev1alpha1.Weapon_WEAPON_HAND_CROSSBOW
	case proficiencies.WeaponLongbow:
		specificWeapon = dnd5ev1alpha1.Weapon_WEAPON_LONGBOW
	case proficiencies.WeaponLongsword:
		specificWeapon = dnd5ev1alpha1.Weapon_WEAPON_LONGSWORD
	case proficiencies.WeaponRapier:
		specificWeapon = dnd5ev1alpha1.Weapon_WEAPON_RAPIER
	case proficiencies.WeaponScimitar:
		specificWeapon = dnd5ev1alpha1.Weapon_WEAPON_SCIMITAR
	case proficiencies.WeaponShortsword:
		specificWeapon = dnd5ev1alpha1.Weapon_WEAPON_SHORTSWORD
	default:
		specificWeapon = dnd5ev1alpha1.Weapon_WEAPON_UNSPECIFIED
	}

	if specificWeapon != dnd5ev1alpha1.Weapon_WEAPON_UNSPECIFIED {
		return dnd5ev1alpha1.WeaponProficiencyCategory_WEAPON_PROFICIENCY_CATEGORY_SPECIFIC, specificWeapon
	}

	return dnd5ev1alpha1.WeaponProficiencyCategory_WEAPON_PROFICIENCY_CATEGORY_UNSPECIFIED, dnd5ev1alpha1.Weapon_WEAPON_UNSPECIFIED
}

// convertToolProficiencyToProto converts toolkit tool proficiency to proto
func convertToolProficiencyToProto(tool proficiencies.Tool) dnd5ev1alpha1.Tool {
	return convertProficiencyToolToProtoEnum(tool)
}

// loadAllClassChoices loads ALL choices (skills, equipment, etc.) from toolkit requirements
func loadAllClassChoices(classID classes.Class) []*dnd5ev1alpha1.Choice {
	// Get all requirements from the toolkit
	requirements := choices.GetClassRequirements(classID)
	if requirements == nil {
		return nil
	}

	result := make([]*dnd5ev1alpha1.Choice, 0)

	// Add skill choice if present
	if requirements.Skills != nil && requirements.Skills.Count > 0 {
		skillChoice := createSkillChoice(requirements.Skills)
		if skillChoice != nil {
			result = append(result, skillChoice)
		}
	}

	// Add equipment choices
	for _, req := range requirements.Equipment {
		equipChoice := createEquipmentChoice(req)
		if equipChoice != nil {
			result = append(result, equipChoice)
		}
	}

	// Add fighting style choice if present
	if requirements.FightingStyle != nil && len(requirements.FightingStyle.Options) > 0 {
		fightingChoice := createFightingStyleChoice(requirements.FightingStyle)
		if fightingChoice != nil {
			result = append(result, fightingChoice)
		}
	}

	// TODO: Add other choice types as needed:
	// - Language choices (requirements.Languages)
	// - Tool choices (requirements.Tools)
	// - Expertise (requirements.Expertise)

	return result
}

// createSkillChoice converts a skill requirement to a proto Choice
func createSkillChoice(req *choices.SkillRequirement) *dnd5ev1alpha1.Choice {
	if req == nil || len(req.Options) == 0 {
		return nil
	}

	// Convert skill options to proto skills
	skillOptions := make([]dnd5ev1alpha1.Skill, 0, len(req.Options))
	for _, skill := range req.Options {
		skillOptions = append(skillOptions, convertSkillToProtoEnum(skill))
	}

	return &dnd5ev1alpha1.Choice{
		Id:          string(req.ID),
		Description: req.Label,
		ChooseCount: int32(req.Count),
		ChoiceType:  dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SKILLS,
		Options: &dnd5ev1alpha1.Choice_SkillOptions{
			SkillOptions: &dnd5ev1alpha1.SkillOptions{
				Available: skillOptions,
			},
		},
	}
}

// createEquipmentChoice converts an equipment requirement to a proto Choice
func createEquipmentChoice(req *choices.EquipmentRequirement) *dnd5ev1alpha1.Choice {
	if req == nil {
		return nil
	}

	// Convert equipment options to proto bundles
	equipmentBundles := make([]*dnd5ev1alpha1.EquipmentBundle, 0, len(req.Options))
	for _, opt := range req.Options {
		// Create items for this bundle
		equipmentItems := make([]*dnd5ev1alpha1.EquipmentItem, 0, len(opt.Items))
		for _, item := range opt.Items {
			// Create equipment item with selection ID and quantity
			equipItem := &dnd5ev1alpha1.EquipmentItem{
				SelectionId: item.ID,
				Quantity:    int32(item.Quantity),
				// TODO: Add type_hint based on item ID when we have better type detection
			}
			equipmentItems = append(equipmentItems, equipItem)
		}

		equipmentBundles = append(equipmentBundles, &dnd5ev1alpha1.EquipmentBundle{
			Id:    opt.ID,
			Label: opt.Label,
			Items: equipmentItems,
		})
	}

	return &dnd5ev1alpha1.Choice{
		Id:          string(req.ID),
		Description: req.Label,
		ChooseCount: int32(req.Choose),
		ChoiceType:  dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT,
		Options: &dnd5ev1alpha1.Choice_EquipmentOptions{
			EquipmentOptions: &dnd5ev1alpha1.EquipmentOptions{
				Bundles: equipmentBundles,
			},
		},
	}
}

// createFightingStyleChoice converts a fighting style requirement to a proto Choice
func createFightingStyleChoice(req *choices.FightingStyleRequirement) *dnd5ev1alpha1.Choice {
	if req == nil || len(req.Options) == 0 {
		return nil
	}

	// Convert fighting style options to proto enums
	fightingStyles := make([]dnd5ev1alpha1.FightingStyle, 0, len(req.Options))
	for _, style := range req.Options {
		fightingStyles = append(fightingStyles, convertFightingStyleToProto(style))
	}

	return &dnd5ev1alpha1.Choice{
		Id:          string(req.ID),
		Description: req.Label,
		ChooseCount: 1, // Fighting styles are typically choose 1
		ChoiceType:  dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_FIGHTING_STYLE,
		Options: &dnd5ev1alpha1.Choice_FightingStyleOptions{
			FightingStyleOptions: &dnd5ev1alpha1.FightingStyleOptions{
				Available: fightingStyles,
			},
		},
	}
}
