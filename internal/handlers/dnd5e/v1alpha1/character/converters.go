package character

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	"github.com/KirkDiggler/rpg-toolkit/core/combat"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/ammunition"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/armor"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/backgrounds"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character/choices"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/equipment"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/features"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/fightingstyles"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/languages"

	// "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/packs" // TODO: Uncomment when Pack enum is available
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/proficiencies"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/races"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/skills"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/spells"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/tools"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/weapons"
)

// Feature source constants
const (
	featureSourceClass      = "class"
	featureSourceRace       = "race"
	featureSourceBackground = "background"
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

//nolint:dupl // Bidirectional enum conversion (skill->proto vs proto->skill) results in similar switch statements
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

//nolint:dupl // Bidirectional enum conversion (language->proto vs proto->language) results in similar switch statements
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

//nolint:dupl // Bidirectional enum conversion (tool->proto vs proto->tool) results in similar switch statements
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
		OptionId: choice.OptionID,
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
	case shared.ChoiceToolProficiency:
		return dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_TOOLS
	case shared.ChoiceExpertise:
		return dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EXPERTISE
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

// convertRaceTraitsToProto converts toolkit races.Trait entries to proto RacialTrait.
//
// races.Trait today only carries {ID, Name, Description} — there is no choice
// information on the toolkit side, so is_choice/options are deliberately left at
// their zero values (false/empty) rather than invented. If the toolkit ever grows
// choice-bearing traits, extend this mapping then.
func convertRaceTraitsToProto(traits []races.Trait) []*dnd5ev1alpha1.RacialTrait {
	if len(traits) == 0 {
		return nil
	}

	result := make([]*dnd5ev1alpha1.RacialTrait, 0, len(traits))
	for _, trait := range traits {
		result = append(result, &dnd5ev1alpha1.RacialTrait{
			Name:        trait.Name,
			Description: trait.Description,
		})
	}
	return result
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

	// Convert proficiency grants (automatic proficiencies from race)
	profGrants := &dnd5ev1alpha1.ProficiencyGrants{}

	// Convert skill proficiencies
	if len(data.Skills) > 0 {
		profGrants.Skills = make([]dnd5ev1alpha1.Skill, 0, len(data.Skills))
		for _, skill := range data.Skills {
			profGrants.Skills = append(profGrants.Skills, convertSkillToProtoEnum(skill))
		}
	}

	// Convert weapon proficiencies
	if len(data.Weapons) > 0 {
		profGrants.Weapons = make([]dnd5ev1alpha1.Weapon, 0, len(data.Weapons))
		for _, weapon := range data.Weapons {
			_, specific := convertWeaponProficiencyToProto(weapon)
			if specific != dnd5ev1alpha1.Weapon_WEAPON_UNSPECIFIED {
				profGrants.Weapons = append(profGrants.Weapons, specific)
			}
		}
	}

	// Convert tool proficiencies
	if len(data.Tools) > 0 {
		profGrants.Tools = make([]dnd5ev1alpha1.Tool, 0, len(data.Tools))
		for _, tool := range data.Tools {
			profGrants.Tools = append(profGrants.Tools, convertToolProficiencyToProto(tool))
		}
	}

	// Convert race choices
	var choices []*dnd5ev1alpha1.Choice

	// Add language choice if present
	if data.LanguageChoice != nil && data.LanguageChoice.Count > 0 {
		var availableLanguages []dnd5ev1alpha1.Language

		if len(data.LanguageChoice.Options) == 0 {
			// Empty options means "any language" - populate all standard languages
			// Note: In D&D 5e, players typically choose from standard languages
			// unless they have special permission for exotic languages
			availableLanguages = []dnd5ev1alpha1.Language{
				dnd5ev1alpha1.Language_LANGUAGE_COMMON,
				dnd5ev1alpha1.Language_LANGUAGE_DWARVISH,
				dnd5ev1alpha1.Language_LANGUAGE_ELVISH,
				dnd5ev1alpha1.Language_LANGUAGE_GIANT,
				dnd5ev1alpha1.Language_LANGUAGE_GNOMISH,
				dnd5ev1alpha1.Language_LANGUAGE_GOBLIN,
				dnd5ev1alpha1.Language_LANGUAGE_HALFLING,
				dnd5ev1alpha1.Language_LANGUAGE_ORC,
				// Exotic languages (could be gated by DM permission)
				dnd5ev1alpha1.Language_LANGUAGE_ABYSSAL,
				dnd5ev1alpha1.Language_LANGUAGE_CELESTIAL,
				dnd5ev1alpha1.Language_LANGUAGE_DRACONIC,
				dnd5ev1alpha1.Language_LANGUAGE_DEEP_SPEECH,
				dnd5ev1alpha1.Language_LANGUAGE_INFERNAL,
				dnd5ev1alpha1.Language_LANGUAGE_PRIMORDIAL,
				dnd5ev1alpha1.Language_LANGUAGE_SYLVAN,
				dnd5ev1alpha1.Language_LANGUAGE_UNDERCOMMON,
			}
		} else {
			// Convert specified language options
			availableLanguages = make([]dnd5ev1alpha1.Language, 0, len(data.LanguageChoice.Options))
			// TODO: Parse language strings when options are specified
		}

		langChoice := &dnd5ev1alpha1.Choice{
			Id:          fmt.Sprintf("%s-languages", strings.ToLower(string(data.ID))),
			Description: data.LanguageChoice.Description,
			ChooseCount: int32(data.LanguageChoice.Count),
			ChoiceType:  dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_LANGUAGES,
			Options: &dnd5ev1alpha1.Choice_LanguageOptions{
				LanguageOptions: &dnd5ev1alpha1.LanguageOptions{
					Available: availableLanguages,
				},
			},
		}
		choices = append(choices, langChoice)
	}

	// Add skill choice if present
	if data.SkillChoice != nil && data.SkillChoice.Count > 0 {
		var availableSkills []dnd5ev1alpha1.Skill

		if len(data.SkillChoice.Options) == 0 {
			// Empty options means "any skill" - populate all skills
			availableSkills = []dnd5ev1alpha1.Skill{
				dnd5ev1alpha1.Skill_SKILL_ACROBATICS,
				dnd5ev1alpha1.Skill_SKILL_ANIMAL_HANDLING,
				dnd5ev1alpha1.Skill_SKILL_ARCANA,
				dnd5ev1alpha1.Skill_SKILL_ATHLETICS,
				dnd5ev1alpha1.Skill_SKILL_DECEPTION,
				dnd5ev1alpha1.Skill_SKILL_HISTORY,
				dnd5ev1alpha1.Skill_SKILL_INSIGHT,
				dnd5ev1alpha1.Skill_SKILL_INTIMIDATION,
				dnd5ev1alpha1.Skill_SKILL_INVESTIGATION,
				dnd5ev1alpha1.Skill_SKILL_MEDICINE,
				dnd5ev1alpha1.Skill_SKILL_NATURE,
				dnd5ev1alpha1.Skill_SKILL_PERCEPTION,
				dnd5ev1alpha1.Skill_SKILL_PERFORMANCE,
				dnd5ev1alpha1.Skill_SKILL_PERSUASION,
				dnd5ev1alpha1.Skill_SKILL_RELIGION,
				dnd5ev1alpha1.Skill_SKILL_SLEIGHT_OF_HAND,
				dnd5ev1alpha1.Skill_SKILL_STEALTH,
				dnd5ev1alpha1.Skill_SKILL_SURVIVAL,
			}
		} else {
			// Convert specified skill options
			availableSkills = make([]dnd5ev1alpha1.Skill, 0, len(data.SkillChoice.Options))
			for _, skillStr := range data.SkillChoice.Options {
				// Parse skill string to skill enum
				if skill := parseSkillString(skillStr); skill != dnd5ev1alpha1.Skill_SKILL_UNSPECIFIED {
					availableSkills = append(availableSkills, skill)
				}
			}
		}

		skillChoice := &dnd5ev1alpha1.Choice{
			Id:          fmt.Sprintf("%s-skills", strings.ToLower(string(data.ID))),
			Description: data.SkillChoice.Description,
			ChooseCount: int32(data.SkillChoice.Count),
			ChoiceType:  dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SKILLS,
			Options: &dnd5ev1alpha1.Choice_SkillOptions{
				SkillOptions: &dnd5ev1alpha1.SkillOptions{
					Available: availableSkills,
				},
			},
		}
		choices = append(choices, skillChoice)
	}

	// Add tool choice if present (e.g., for Dwarf)
	if data.ToolChoice != nil && data.ToolChoice.Count > 0 {
		var availableTools []dnd5ev1alpha1.Tool

		if len(data.ToolChoice.Options) == 0 {
			// Empty options means "any tool" - populate all tools
			availableTools = []dnd5ev1alpha1.Tool{
				// Artisan's tools
				dnd5ev1alpha1.Tool_TOOL_ALCHEMIST_SUPPLIES,
				dnd5ev1alpha1.Tool_TOOL_BREWER_SUPPLIES,
				dnd5ev1alpha1.Tool_TOOL_CALLIGRAPHER_SUPPLIES,
				dnd5ev1alpha1.Tool_TOOL_CARPENTER_TOOLS,
				dnd5ev1alpha1.Tool_TOOL_CARTOGRAPHER_TOOLS,
				dnd5ev1alpha1.Tool_TOOL_COBBLER_TOOLS,
				dnd5ev1alpha1.Tool_TOOL_COOK_UTENSILS,
				dnd5ev1alpha1.Tool_TOOL_GLASSBLOWER_TOOLS,
				dnd5ev1alpha1.Tool_TOOL_JEWELER_TOOLS,
				dnd5ev1alpha1.Tool_TOOL_LEATHERWORKER_TOOLS,
				dnd5ev1alpha1.Tool_TOOL_MASON_TOOLS,
				dnd5ev1alpha1.Tool_TOOL_PAINTER_SUPPLIES,
				dnd5ev1alpha1.Tool_TOOL_POTTER_TOOLS,
				dnd5ev1alpha1.Tool_TOOL_SMITH_TOOLS,
				dnd5ev1alpha1.Tool_TOOL_TINKER_TOOLS,
				dnd5ev1alpha1.Tool_TOOL_WEAVER_TOOLS,
				dnd5ev1alpha1.Tool_TOOL_WOODCARVER_TOOLS,
				// Other tools
				dnd5ev1alpha1.Tool_TOOL_DISGUISE_KIT,
				dnd5ev1alpha1.Tool_TOOL_FORGERY_KIT,
				dnd5ev1alpha1.Tool_TOOL_HERBALISM_KIT,
				dnd5ev1alpha1.Tool_TOOL_NAVIGATOR_TOOLS,
				dnd5ev1alpha1.Tool_TOOL_POISONER_KIT,
				dnd5ev1alpha1.Tool_TOOL_THIEVES_TOOLS,
			}
		} else {
			// Convert specified tool options
			availableTools = make([]dnd5ev1alpha1.Tool, 0, len(data.ToolChoice.Options))
			for _, toolStr := range data.ToolChoice.Options {
				// Parse tool string to tool enum
				if tool := parseToolString(toolStr); tool != dnd5ev1alpha1.Tool_TOOL_UNSPECIFIED {
					availableTools = append(availableTools, tool)
				}
			}
		}

		toolChoice := &dnd5ev1alpha1.Choice{
			Id:          fmt.Sprintf("%s-tools", strings.ToLower(string(data.ID))),
			Description: data.ToolChoice.Description,
			ChooseCount: int32(data.ToolChoice.Count),
			ChoiceType:  dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_TOOLS,
			Options: &dnd5ev1alpha1.Choice_ToolOptions{
				ToolOptions: &dnd5ev1alpha1.ToolOptions{
					Available: availableTools,
				},
			},
		}
		choices = append(choices, toolChoice)
	}

	// TODO: Convert Size string to proto enum when available
	// TODO: Convert subraces when proto supports them
	// TODO: Convert AbilityChoice for Half-Elf

	return &dnd5ev1alpha1.RaceInfo{
		RaceId:            convertRaceToProtoEnum(data.ID),
		Name:              data.Name(),
		Description:       data.Description(),
		Speed:             int32(data.Speed),
		AbilityBonuses:    abilityBonuses,
		Traits:            convertRaceTraitsToProto(data.Traits),
		Languages:         langs,
		ProficiencyGrants: profGrants,
		Choices:           choices,
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

// convertValidationResultToProto converts toolkit validation to proto ValidationResult
func convertValidationResultToProto(toolkitValidation *choices.ValidationResult, progress toolkitchar.Progress) *dnd5ev1alpha1.ValidationResult {
	if toolkitValidation == nil {
		// If no validation result from toolkit, create one based on progress
		return createValidationFromProgress(progress)
	}

	result := &dnd5ev1alpha1.ValidationResult{
		IsValid: toolkitValidation.Valid,
		Issues:  make([]*dnd5ev1alpha1.ValidationResult_Issue, 0),
	}

	// Convert toolkit validation errors to proto issues
	for _, err := range toolkitValidation.Errors {
		issue := &dnd5ev1alpha1.ValidationResult_Issue{
			Message: err.Message,
		}

		// Map choice category to severity and field
		if err.Category == "" {
			issue.Severity = dnd5ev1alpha1.ValidationResult_SEVERITY_ERROR
			issue.Field = dnd5ev1alpha1.ValidationField_VALIDATION_FIELD_UNSPECIFIED
		} else {
			// For missing choices, use INCOMPLETE severity
			issue.Severity = dnd5ev1alpha1.ValidationResult_SEVERITY_INCOMPLETE

			// Map category to field
			switch err.Category {
			case shared.ChoiceSkills:
				issue.Field = dnd5ev1alpha1.ValidationField_VALIDATION_FIELD_SKILLS
				issue.Source = dnd5ev1alpha1.ValidationSource_VALIDATION_SOURCE_CLASS
			case shared.ChoiceFightingStyle:
				issue.Field = dnd5ev1alpha1.ValidationField_VALIDATION_FIELD_FIGHTING_STYLE
				issue.Source = dnd5ev1alpha1.ValidationSource_VALIDATION_SOURCE_CLASS
			case shared.ChoiceEquipment:
				issue.Field = dnd5ev1alpha1.ValidationField_VALIDATION_FIELD_EQUIPMENT
				issue.Source = dnd5ev1alpha1.ValidationSource_VALIDATION_SOURCE_CLASS
			case shared.ChoiceLanguages:
				issue.Field = dnd5ev1alpha1.ValidationField_VALIDATION_FIELD_LANGUAGES
				// Could be race or background
				if strings.Contains(err.Message, "background") {
					issue.Source = dnd5ev1alpha1.ValidationSource_VALIDATION_SOURCE_BACKGROUND
				} else {
					issue.Source = dnd5ev1alpha1.ValidationSource_VALIDATION_SOURCE_RACE
				}
			default:
				issue.Field = dnd5ev1alpha1.ValidationField_VALIDATION_FIELD_UNSPECIFIED
				issue.Source = dnd5ev1alpha1.ValidationSource_VALIDATION_SOURCE_UNSPECIFIED
			}
		}

		result.Issues = append(result.Issues, issue)

		// Update counts
		switch issue.Severity {
		case dnd5ev1alpha1.ValidationResult_SEVERITY_ERROR:
			result.ErrorCount++
		case dnd5ev1alpha1.ValidationResult_SEVERITY_INCOMPLETE:
			result.IncompleteCount++
		case dnd5ev1alpha1.ValidationResult_SEVERITY_WARNING:
			result.WarningCount++
		}
	}

	return result
}

// createValidationFromProgress creates a ValidationResult based on character progress
func createValidationFromProgress(progress toolkitchar.Progress) *dnd5ev1alpha1.ValidationResult {
	result := &dnd5ev1alpha1.ValidationResult{
		IsValid: progress.IsComplete(),
		Issues:  make([]*dnd5ev1alpha1.ValidationResult_Issue, 0),
	}

	// Check each progress field and add issues for incomplete items
	if !progress.Has(toolkitchar.ProgressName) {
		result.Issues = append(result.Issues, &dnd5ev1alpha1.ValidationResult_Issue{
			Severity: dnd5ev1alpha1.ValidationResult_SEVERITY_INCOMPLETE,
			Source:   dnd5ev1alpha1.ValidationSource_VALIDATION_SOURCE_NAME,
			Field:    dnd5ev1alpha1.ValidationField_VALIDATION_FIELD_NAME,
			Message:  "Character name is required",
		})
		result.IncompleteCount++
	}

	if !progress.Has(toolkitchar.ProgressRace) {
		result.Issues = append(result.Issues, &dnd5ev1alpha1.ValidationResult_Issue{
			Severity: dnd5ev1alpha1.ValidationResult_SEVERITY_INCOMPLETE,
			Source:   dnd5ev1alpha1.ValidationSource_VALIDATION_SOURCE_RACE,
			Field:    dnd5ev1alpha1.ValidationField_VALIDATION_FIELD_RACE,
			Message:  "Race selection is incomplete",
		})
		result.IncompleteCount++
	}

	if !progress.Has(toolkitchar.ProgressClass) {
		result.Issues = append(result.Issues, &dnd5ev1alpha1.ValidationResult_Issue{
			Severity: dnd5ev1alpha1.ValidationResult_SEVERITY_INCOMPLETE,
			Source:   dnd5ev1alpha1.ValidationSource_VALIDATION_SOURCE_CLASS,
			Field:    dnd5ev1alpha1.ValidationField_VALIDATION_FIELD_CLASS,
			Message:  "Class selection or class choices are incomplete",
		})
		result.IncompleteCount++
	}

	if !progress.Has(toolkitchar.ProgressBackground) {
		result.Issues = append(result.Issues, &dnd5ev1alpha1.ValidationResult_Issue{
			Severity: dnd5ev1alpha1.ValidationResult_SEVERITY_INCOMPLETE,
			Source:   dnd5ev1alpha1.ValidationSource_VALIDATION_SOURCE_BACKGROUND,
			Field:    dnd5ev1alpha1.ValidationField_VALIDATION_FIELD_BACKGROUND,
			Message:  "Background selection is incomplete",
		})
		result.IncompleteCount++
	}

	if !progress.Has(toolkitchar.ProgressAbilityScores) {
		result.Issues = append(result.Issues, &dnd5ev1alpha1.ValidationResult_Issue{
			Severity: dnd5ev1alpha1.ValidationResult_SEVERITY_INCOMPLETE,
			Source:   dnd5ev1alpha1.ValidationSource_VALIDATION_SOURCE_ABILITY_SCORES,
			Field:    dnd5ev1alpha1.ValidationField_VALIDATION_FIELD_ABILITY_SCORES,
			Message:  "Ability scores have not been assigned",
		})
		result.IncompleteCount++
	}

	return result
}

// convertCharacterToProto converts toolkit character.Character to proto Character
func convertCharacterToProto(char *toolkitchar.Character) *dnd5ev1alpha1.Character {
	if char == nil {
		return nil
	}
	// Convert to Data first, then use existing converter
	// ToData is now a method on Character
	data := char.ToData()
	return ConvertCharacterDataToProto(data)
}

// ConvertCharacterDataToProto converts toolkit character.Data to proto Character
func ConvertCharacterDataToProto(data *toolkitchar.Data) *dnd5ev1alpha1.Character {
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

	// Convert tool proficiencies
	toolProfs := make([]dnd5ev1alpha1.Tool, 0, len(data.ToolProficiencies))
	for _, tool := range data.ToolProficiencies {
		protoTool := convertToolProficiencyToProto(tool)
		if protoTool != dnd5ev1alpha1.Tool_TOOL_UNSPECIFIED {
			toolProfs = append(toolProfs, protoTool)
		}
	}

	// Convert armor proficiency categories
	armorCats := make([]dnd5ev1alpha1.ArmorProficiencyCategory, 0, len(data.ArmorProficiencies))
	for _, armor := range data.ArmorProficiencies {
		protoCat := convertArmorProficiencyCategoryToProto(armor)
		if protoCat != dnd5ev1alpha1.ArmorProficiencyCategory_ARMOR_PROFICIENCY_CATEGORY_UNSPECIFIED {
			armorCats = append(armorCats, protoCat)
		}
	}

	// Convert weapon proficiencies - separate categories from specific weapons
	weaponCats := make([]dnd5ev1alpha1.WeaponProficiencyCategory, 0)
	specificWeapons := make([]dnd5ev1alpha1.Weapon, 0)
	for _, weapon := range data.WeaponProficiencies {
		cat, specific := convertWeaponProficiencyToProto(weapon)
		if cat == dnd5ev1alpha1.WeaponProficiencyCategory_WEAPON_PROFICIENCY_CATEGORY_SIMPLE ||
			cat == dnd5ev1alpha1.WeaponProficiencyCategory_WEAPON_PROFICIENCY_CATEGORY_MARTIAL {
			weaponCats = append(weaponCats, cat)
		} else if specific != dnd5ev1alpha1.Weapon_WEAPON_UNSPECIFIED {
			specificWeapons = append(specificWeapons, specific)
		}
	}

	// Set proficiencies structure
	char.Proficiencies = &dnd5ev1alpha1.Proficiencies{
		Skills:           skillList,
		SavingThrows:     savingThrows,
		Tools:            toolProfs,
		ArmorCategories:  armorCats,
		WeaponCategories: weaponCats,
		SpecificWeapons:  specificWeapons,
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

	// Convert features
	// Note: ref is stored as string "module:type:id", not an object
	if len(data.Features) > 0 {
		for _, featureJSON := range data.Features {
			var featureData struct {
				Ref  string `json:"ref"`  // Feature type ref "dnd5e:features:rage"
				ID   string `json:"id"`   // Entity ID
				Name string `json:"name"` // Display name
			}
			if err := json.Unmarshal(featureJSON, &featureData); err != nil {
				continue
			}

			// Skip if no ref
			if featureData.Ref == "" {
				continue
			}

			// Extract feature type from ref (e.g., "dnd5e:features:rage" -> "rage")
			featureType := extractIDFromRef(featureData.Ref)
			if featureType == "" {
				continue
			}

			// Map to enum
			featureEnum := featureIDToEnum(featureType)

			displayName := featureData.Name
			if displayName == "" {
				displayName = featureIDToDisplayName(featureType)
			}

			char.Features = append(char.Features, &dnd5ev1alpha1.CharacterFeature{
				Id:          featureEnum,
				Name:        displayName,
				Source:      featureSourceClass,
				ActionType:  getFeatureActionType(featureType),
				FeatureData: featureJSON, // Pass through raw JSON for UI display
			})
		}
	}

	// Convert conditions (active status effects like "raging", "poisoned", etc.)
	if len(data.Conditions) > 0 {
		for _, conditionJSON := range data.Conditions {
			var conditionData struct {
				Ref      string `json:"ref"`    // Toolkit stores ref as string "dnd5e:conditions:raging"
				Source   string `json:"source"` // Source ref string "dnd5e:features:rage"
				Duration int32  `json:"duration"`
			}
			if err := json.Unmarshal(conditionJSON, &conditionData); err != nil {
				continue // Skip invalid conditions
			}

			// Skip if no ref
			if conditionData.Ref == "" {
				continue
			}

			// Extract condition ID from ref string (e.g., "dnd5e:conditions:raging" -> "raging")
			conditionID := extractIDFromRef(conditionData.Ref)
			if conditionID == "" {
				continue
			}

			// Map to enum and derive display name
			conditionEnum := conditionIDToEnum(conditionID)
			conditionName := conditionIDToDisplayName(conditionID)

			char.ActiveConditions = append(char.ActiveConditions, &dnd5ev1alpha1.Condition{
				Id:            conditionEnum,
				Name:          conditionName,
				Source:        conditionData.Source,
				Duration:      conditionData.Duration,
				ConditionData: conditionJSON, // Pass through raw JSON for UI display
			})
		}
	}

	// Convert equipment slots
	char.EquipmentSlots = convertEquipmentSlotsToProto(data.EquipmentSlots)

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

// convertEquipmentSlotToToolkit converts a proto EquipmentSlot enum to toolkit InventorySlot.
// Returns an error for unsupported slots (GLOVES is deprecated and not supported).
func convertEquipmentSlotToToolkit(slot dnd5ev1alpha1.EquipmentSlot) (toolkitchar.InventorySlot, error) {
	switch slot {
	case dnd5ev1alpha1.EquipmentSlot_EQUIPMENT_SLOT_MAIN_HAND:
		return toolkitchar.SlotMainHand, nil
	case dnd5ev1alpha1.EquipmentSlot_EQUIPMENT_SLOT_OFF_HAND:
		return toolkitchar.SlotOffHand, nil
	case dnd5ev1alpha1.EquipmentSlot_EQUIPMENT_SLOT_ARMOR:
		return toolkitchar.SlotArmor, nil
	case dnd5ev1alpha1.EquipmentSlot_EQUIPMENT_SLOT_HELMET:
		return toolkitchar.SlotHelm, nil
	case dnd5ev1alpha1.EquipmentSlot_EQUIPMENT_SLOT_BOOTS:
		return toolkitchar.SlotBoots, nil
	case dnd5ev1alpha1.EquipmentSlot_EQUIPMENT_SLOT_CLOAK:
		return toolkitchar.SlotCloak, nil
	case dnd5ev1alpha1.EquipmentSlot_EQUIPMENT_SLOT_AMULET:
		return toolkitchar.SlotAmulet, nil
	case dnd5ev1alpha1.EquipmentSlot_EQUIPMENT_SLOT_RING_1:
		return toolkitchar.SlotRingLeft, nil
	case dnd5ev1alpha1.EquipmentSlot_EQUIPMENT_SLOT_RING_2:
		return toolkitchar.SlotRingRight, nil
	case dnd5ev1alpha1.EquipmentSlot_EQUIPMENT_SLOT_BELT:
		return toolkitchar.SlotBelt, nil
	case dnd5ev1alpha1.EquipmentSlot_EQUIPMENT_SLOT_GLOVES:
		return "", apierr.InvalidArgument("EQUIPMENT_SLOT_GLOVES is deprecated and not supported")
	default:
		return "", apierr.InvalidArgument("unsupported equipment slot")
	}
}

// convertEquipmentSlotsToProto converts toolkit EquipmentSlots to proto EquipmentSlots
func convertEquipmentSlotsToProto(slots toolkitchar.EquipmentSlots) *dnd5ev1alpha1.EquipmentSlots {
	if len(slots) == 0 {
		return nil
	}

	// Helper to create InventoryItem from item ID
	makeItem := func(itemID string) *dnd5ev1alpha1.InventoryItem {
		if itemID == "" {
			return nil
		}
		return &dnd5ev1alpha1.InventoryItem{
			ItemId:   itemID,
			Quantity: 1,
		}
	}

	return &dnd5ev1alpha1.EquipmentSlots{
		MainHand: makeItem(slots.Get(toolkitchar.SlotMainHand)),
		OffHand:  makeItem(slots.Get(toolkitchar.SlotOffHand)),
		Armor:    makeItem(slots.Get(toolkitchar.SlotArmor)),
		Helmet:   makeItem(slots.Get(toolkitchar.SlotHelm)),
		Boots:    makeItem(slots.Get(toolkitchar.SlotBoots)),
		Cloak:    makeItem(slots.Get(toolkitchar.SlotCloak)),
		Amulet:   makeItem(slots.Get(toolkitchar.SlotAmulet)),
		Ring_1:   makeItem(slots.Get(toolkitchar.SlotRingLeft)),
		Ring_2:   makeItem(slots.Get(toolkitchar.SlotRingRight)),
		Belt:     makeItem(slots.Get(toolkitchar.SlotBelt)),
	}
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
	case ammunition.Arrows20:
		return dnd5ev1alpha1.Weapon_WEAPON_ARROWS_20
	case ammunition.Bolts20:
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

		// Try weapon conversion first
		if weapon := convertWeaponToProto(item); weapon != dnd5ev1alpha1.Weapon_WEAPON_UNSPECIFIED {
			equipItem.Equipment = &dnd5ev1alpha1.EquipmentSelectionItem_Weapon{
				Weapon: weapon,
			}
		} else if armorEnum := convertArmorToProto(item); armorEnum != dnd5ev1alpha1.Armor_ARMOR_UNSPECIFIED {
			// Try armor conversion
			equipItem.Equipment = &dnd5ev1alpha1.EquipmentSelectionItem_Armor{
				Armor: armorEnum,
			}
		} else if tool := convertToolToProto(item); tool != dnd5ev1alpha1.Tool_TOOL_UNSPECIFIED {
			// Try tool conversion
			equipItem.Equipment = &dnd5ev1alpha1.EquipmentSelectionItem_Tool{
				Tool: tool,
			}
		} else {
			// Fall back to OtherEquipmentId for anything we can't identify
			// This includes packs and other items not yet enumerated
			equipItem.Equipment = &dnd5ev1alpha1.EquipmentSelectionItem_OtherEquipmentId{
				OtherEquipmentId: item,
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

// convertProtoToolToToolkit converts proto Tool enum to toolkit SelectionID
func convertProtoToolToToolkit(tool dnd5ev1alpha1.Tool) shared.SelectionID {
	switch tool {
	// Artisan's tools
	case dnd5ev1alpha1.Tool_TOOL_ALCHEMIST_SUPPLIES:
		return refs.Tools.AlchemistSupplies().ID
	case dnd5ev1alpha1.Tool_TOOL_BREWER_SUPPLIES:
		return refs.Tools.BrewerSupplies().ID
	case dnd5ev1alpha1.Tool_TOOL_CALLIGRAPHER_SUPPLIES:
		return refs.Tools.CalligrapherSupplies().ID
	case dnd5ev1alpha1.Tool_TOOL_CARPENTER_TOOLS:
		return refs.Tools.CarpenterTools().ID
	case dnd5ev1alpha1.Tool_TOOL_CARTOGRAPHER_TOOLS:
		return refs.Tools.CartographerTools().ID
	case dnd5ev1alpha1.Tool_TOOL_COBBLER_TOOLS:
		return refs.Tools.CobblerTools().ID
	case dnd5ev1alpha1.Tool_TOOL_COOK_UTENSILS:
		return refs.Tools.CookUtensils().ID
	case dnd5ev1alpha1.Tool_TOOL_GLASSBLOWER_TOOLS:
		return refs.Tools.GlassblowerTools().ID
	case dnd5ev1alpha1.Tool_TOOL_JEWELER_TOOLS:
		return refs.Tools.JewelerTools().ID
	case dnd5ev1alpha1.Tool_TOOL_LEATHERWORKER_TOOLS:
		return refs.Tools.LeatherworkerTools().ID
	case dnd5ev1alpha1.Tool_TOOL_MASON_TOOLS:
		return refs.Tools.MasonTools().ID
	case dnd5ev1alpha1.Tool_TOOL_PAINTER_SUPPLIES:
		return refs.Tools.PainterSupplies().ID
	case dnd5ev1alpha1.Tool_TOOL_POTTER_TOOLS:
		return refs.Tools.PotterTools().ID
	case dnd5ev1alpha1.Tool_TOOL_SMITH_TOOLS:
		return refs.Tools.SmithTools().ID
	case dnd5ev1alpha1.Tool_TOOL_TINKER_TOOLS:
		return refs.Tools.TinkerTools().ID
	case dnd5ev1alpha1.Tool_TOOL_WEAVER_TOOLS:
		return refs.Tools.WeaverTools().ID
	case dnd5ev1alpha1.Tool_TOOL_WOODCARVER_TOOLS:
		return refs.Tools.WoodcarverTools().ID
	// Other tools
	case dnd5ev1alpha1.Tool_TOOL_DISGUISE_KIT:
		return refs.Tools.DisguiseKit().ID
	case dnd5ev1alpha1.Tool_TOOL_FORGERY_KIT:
		return refs.Tools.ForgeryKit().ID
	case dnd5ev1alpha1.Tool_TOOL_HERBALISM_KIT:
		return refs.Tools.HerbalismKit().ID
	case dnd5ev1alpha1.Tool_TOOL_NAVIGATOR_TOOLS:
		return refs.Tools.NavigatorTools().ID
	case dnd5ev1alpha1.Tool_TOOL_POISONER_KIT:
		return refs.Tools.PoisonerKit().ID
	case dnd5ev1alpha1.Tool_TOOL_THIEVES_TOOLS:
		return refs.Tools.ThievesTools().ID
	// Gaming sets
	case dnd5ev1alpha1.Tool_TOOL_DICE_SET:
		return refs.Tools.DiceSet().ID
	case dnd5ev1alpha1.Tool_TOOL_PLAYING_CARD_SET:
		return refs.Tools.PlayingCardSet().ID
	// Musical instruments
	case dnd5ev1alpha1.Tool_TOOL_BAGPIPES:
		return refs.Tools.Bagpipes().ID
	case dnd5ev1alpha1.Tool_TOOL_DRUM:
		return refs.Tools.Drum().ID
	case dnd5ev1alpha1.Tool_TOOL_DULCIMER:
		return refs.Tools.Dulcimer().ID
	case dnd5ev1alpha1.Tool_TOOL_FLUTE:
		return refs.Tools.Flute().ID
	case dnd5ev1alpha1.Tool_TOOL_LUTE:
		return refs.Tools.Lute().ID
	case dnd5ev1alpha1.Tool_TOOL_LYRE:
		return refs.Tools.Lyre().ID
	case dnd5ev1alpha1.Tool_TOOL_HORN:
		return refs.Tools.Horn().ID
	case dnd5ev1alpha1.Tool_TOOL_PAN_FLUTE:
		return refs.Tools.PanFlute().ID
	case dnd5ev1alpha1.Tool_TOOL_SHAWM:
		return refs.Tools.Shawm().ID
	case dnd5ev1alpha1.Tool_TOOL_VIOL:
		return refs.Tools.Viol().ID
	default:
		return ""
	}
}

// convertProtoWeaponToToolkit converts proto Weapon enum to toolkit weapon ID
func convertProtoWeaponToToolkit(weapon dnd5ev1alpha1.Weapon) shared.SelectionID {
	switch weapon {
	// Simple Melee Weapons
	case dnd5ev1alpha1.Weapon_WEAPON_CLUB:
		return refs.Weapons.Club().ID
	case dnd5ev1alpha1.Weapon_WEAPON_DAGGER:
		return refs.Weapons.Dagger().ID
	case dnd5ev1alpha1.Weapon_WEAPON_GREATCLUB:
		return refs.Weapons.Greatclub().ID
	case dnd5ev1alpha1.Weapon_WEAPON_HANDAXE:
		return refs.Weapons.Handaxe().ID
	case dnd5ev1alpha1.Weapon_WEAPON_JAVELIN:
		return refs.Weapons.Javelin().ID
	case dnd5ev1alpha1.Weapon_WEAPON_LIGHT_HAMMER:
		return refs.Weapons.LightHammer().ID
	case dnd5ev1alpha1.Weapon_WEAPON_MACE:
		return refs.Weapons.Mace().ID
	case dnd5ev1alpha1.Weapon_WEAPON_QUARTERSTAFF:
		return refs.Weapons.Quarterstaff().ID
	case dnd5ev1alpha1.Weapon_WEAPON_SICKLE:
		return refs.Weapons.Sickle().ID
	case dnd5ev1alpha1.Weapon_WEAPON_SPEAR:
		return refs.Weapons.Spear().ID
	// Simple Ranged Weapons
	case dnd5ev1alpha1.Weapon_WEAPON_LIGHT_CROSSBOW:
		return refs.Weapons.LightCrossbow().ID
	case dnd5ev1alpha1.Weapon_WEAPON_DART:
		return refs.Weapons.Dart().ID
	case dnd5ev1alpha1.Weapon_WEAPON_SHORTBOW:
		return refs.Weapons.Shortbow().ID
	case dnd5ev1alpha1.Weapon_WEAPON_SLING:
		return refs.Weapons.Sling().ID
	// Martial Melee Weapons
	case dnd5ev1alpha1.Weapon_WEAPON_BATTLEAXE:
		return refs.Weapons.Battleaxe().ID
	case dnd5ev1alpha1.Weapon_WEAPON_FLAIL:
		return refs.Weapons.Flail().ID
	case dnd5ev1alpha1.Weapon_WEAPON_GLAIVE:
		return refs.Weapons.Glaive().ID
	case dnd5ev1alpha1.Weapon_WEAPON_GREATAXE:
		return refs.Weapons.Greataxe().ID
	case dnd5ev1alpha1.Weapon_WEAPON_GREATSWORD:
		return refs.Weapons.Greatsword().ID
	case dnd5ev1alpha1.Weapon_WEAPON_HALBERD:
		return refs.Weapons.Halberd().ID
	case dnd5ev1alpha1.Weapon_WEAPON_LANCE:
		return refs.Weapons.Lance().ID
	case dnd5ev1alpha1.Weapon_WEAPON_LONGSWORD:
		return refs.Weapons.Longsword().ID
	case dnd5ev1alpha1.Weapon_WEAPON_MAUL:
		return refs.Weapons.Maul().ID
	case dnd5ev1alpha1.Weapon_WEAPON_MORNINGSTAR:
		return refs.Weapons.Morningstar().ID
	case dnd5ev1alpha1.Weapon_WEAPON_PIKE:
		return refs.Weapons.Pike().ID
	case dnd5ev1alpha1.Weapon_WEAPON_RAPIER:
		return refs.Weapons.Rapier().ID
	case dnd5ev1alpha1.Weapon_WEAPON_SCIMITAR:
		return refs.Weapons.Scimitar().ID
	case dnd5ev1alpha1.Weapon_WEAPON_SHORTSWORD:
		return refs.Weapons.Shortsword().ID
	case dnd5ev1alpha1.Weapon_WEAPON_TRIDENT:
		return refs.Weapons.Trident().ID
	case dnd5ev1alpha1.Weapon_WEAPON_WAR_PICK:
		return refs.Weapons.WarPick().ID
	case dnd5ev1alpha1.Weapon_WEAPON_WARHAMMER:
		return refs.Weapons.Warhammer().ID
	case dnd5ev1alpha1.Weapon_WEAPON_WHIP:
		return refs.Weapons.Whip().ID
	// Martial Ranged Weapons
	case dnd5ev1alpha1.Weapon_WEAPON_BLOWGUN:
		return refs.Weapons.Blowgun().ID
	case dnd5ev1alpha1.Weapon_WEAPON_HAND_CROSSBOW:
		return refs.Weapons.HandCrossbow().ID
	case dnd5ev1alpha1.Weapon_WEAPON_HEAVY_CROSSBOW:
		return refs.Weapons.HeavyCrossbow().ID
	case dnd5ev1alpha1.Weapon_WEAPON_LONGBOW:
		return refs.Weapons.Longbow().ID
	case dnd5ev1alpha1.Weapon_WEAPON_NET:
		return refs.Weapons.Net().ID
	// Ammunition
	case dnd5ev1alpha1.Weapon_WEAPON_ARROWS_20:
		return shared.SelectionID(ammunition.Arrows20)
	case dnd5ev1alpha1.Weapon_WEAPON_BOLTS_20:
		return shared.SelectionID(ammunition.Bolts20)
	// Category placeholders
	case dnd5ev1alpha1.Weapon_WEAPON_ANY_SIMPLE:
		return refs.Weapons.AnySimpleWeapon().ID
	case dnd5ev1alpha1.Weapon_WEAPON_ANY_MARTIAL:
		return refs.Weapons.AnyMartialWeapon().ID
	case dnd5ev1alpha1.Weapon_WEAPON_ANY:
		return refs.Weapons.AnyWeapon().ID
	default:
		return ""
	}
}

// convertProtoArmorToToolkit converts proto Armor enum to toolkit armor ID
func convertProtoArmorToToolkit(a dnd5ev1alpha1.Armor) shared.SelectionID {
	switch a {
	// Light Armor
	case dnd5ev1alpha1.Armor_ARMOR_PADDED:
		return refs.Armor.Padded().ID
	case dnd5ev1alpha1.Armor_ARMOR_LEATHER:
		return refs.Armor.Leather().ID
	case dnd5ev1alpha1.Armor_ARMOR_STUDDED_LEATHER:
		return refs.Armor.StuddedLeather().ID
	// Medium Armor
	case dnd5ev1alpha1.Armor_ARMOR_HIDE:
		return refs.Armor.Hide().ID
	case dnd5ev1alpha1.Armor_ARMOR_CHAIN_SHIRT:
		return refs.Armor.ChainShirt().ID
	case dnd5ev1alpha1.Armor_ARMOR_SCALE_MAIL:
		return refs.Armor.ScaleMail().ID
	case dnd5ev1alpha1.Armor_ARMOR_BREASTPLATE:
		return refs.Armor.Breastplate().ID
	case dnd5ev1alpha1.Armor_ARMOR_HALF_PLATE:
		return refs.Armor.HalfPlate().ID
	// Heavy Armor
	case dnd5ev1alpha1.Armor_ARMOR_RING_MAIL:
		return refs.Armor.RingMail().ID
	case dnd5ev1alpha1.Armor_ARMOR_CHAIN_MAIL:
		return refs.Armor.ChainMail().ID
	case dnd5ev1alpha1.Armor_ARMOR_SPLINT:
		return refs.Armor.Splint().ID
	case dnd5ev1alpha1.Armor_ARMOR_PLATE:
		return refs.Armor.Plate().ID
	// Shield
	case dnd5ev1alpha1.Armor_ARMOR_SHIELD:
		return refs.Armor.Shield().ID
	default:
		return ""
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

// convertRollAssignmentsToMap converts proto roll assignments to a simple map
func convertRollAssignmentsToMap(assignments *dnd5ev1alpha1.RollAssignments) map[abilities.Ability]string {
	if assignments == nil {
		return nil
	}

	result := make(map[abilities.Ability]string)

	if assignments.StrengthRollId != "" {
		result[abilities.STR] = assignments.StrengthRollId
	}
	if assignments.DexterityRollId != "" {
		result[abilities.DEX] = assignments.DexterityRollId
	}
	if assignments.ConstitutionRollId != "" {
		result[abilities.CON] = assignments.ConstitutionRollId
	}
	if assignments.IntelligenceRollId != "" {
		result[abilities.INT] = assignments.IntelligenceRollId
	}
	if assignments.WisdomRollId != "" {
		result[abilities.WIS] = assignments.WisdomRollId
	}
	if assignments.CharismaRollId != "" {
		result[abilities.CHA] = assignments.CharismaRollId
	}

	return result
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

	// Add tool proficiency choice if present (e.g., Monk, Bard)
	if requirements.Tools != nil && requirements.Tools.Count > 0 {
		toolChoice := createToolChoice(requirements.Tools)
		if toolChoice != nil {
			result = append(result, toolChoice)
		}
	}

	// Add expertise choice if present (e.g., Rogue, Bard)
	if requirements.Expertise != nil && requirements.Expertise.Count > 0 {
		expertiseChoice := createExpertiseChoice(requirements.Expertise)
		if expertiseChoice != nil {
			result = append(result, expertiseChoice)
		}
	}

	// TODO: Add other choice types as needed:
	// - Language choices (requirements.Languages)

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
			equipmentItems = append(equipmentItems, convertEquipmentItemToProto(item))
		}

		// Convert category choices (e.g., "choose a martial weapon")
		categoryChoices := make([]*dnd5ev1alpha1.EquipmentCategoryChoice, 0, len(opt.CategoryChoices))
		for _, catChoice := range opt.CategoryChoices {
			categoryChoice := &dnd5ev1alpha1.EquipmentCategoryChoice{
				Choose:  int32(catChoice.Choose),
				Label:   catChoice.Label,
				Options: convertEquipmentItemsToProto(catChoice.Options),
			}

			// Determine the equipment type and set appropriate categories
			if catChoice.Type == "weapon" {
				// Convert weapon categories from toolkit's specific categories
				// to proto's broader categories (SIMPLE, MARTIAL)
				// TODO: For now we're only mapping melee types while we decide on the design
				weaponCategorySet := make(map[dnd5ev1alpha1.WeaponCategory]bool)
				for _, cat := range catChoice.Categories {
					switch cat {
					case weapons.CategorySimpleMelee:
						weaponCategorySet[dnd5ev1alpha1.WeaponCategory_WEAPON_CATEGORY_SIMPLE] = true
					case weapons.CategoryMartialMelee:
						weaponCategorySet[dnd5ev1alpha1.WeaponCategory_WEAPON_CATEGORY_MARTIAL] = true
					// TODO: Ignoring ranged categories for now
					case weapons.CategorySimpleRanged, weapons.CategoryMartialRanged:
						// Skip ranged for now
					}
				}
				// Convert set to slice
				weaponCategories := make([]dnd5ev1alpha1.WeaponCategory, 0, len(weaponCategorySet))
				for cat := range weaponCategorySet {
					weaponCategories = append(weaponCategories, cat)
				}
				categoryChoice.WeaponCategories = weaponCategories
			}
			// TODO: Handle armor and tool categories when needed

			categoryChoices = append(categoryChoices, categoryChoice)
		}

		equipmentBundles = append(equipmentBundles, &dnd5ev1alpha1.EquipmentBundle{
			Id:              opt.ID,
			Label:           opt.Label,
			Items:           equipmentItems,
			CategoryChoices: categoryChoices,
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

// convertEquipmentItemsToProto maps toolkit-resolved equipment in order. The
// toolkit owns category eligibility; this converter only preserves its result.
func convertEquipmentItemsToProto(items []choices.EquipmentItem) []*dnd5ev1alpha1.EquipmentItem {
	result := make([]*dnd5ev1alpha1.EquipmentItem, 0, len(items))
	for _, item := range items {
		result = append(result, convertEquipmentItemToProto(item))
	}
	return result
}

// convertEquipmentItemToProto maps a concrete toolkit item to its wire shape.
// Detail is resolved upstream by the toolkit; do not derive it here.
func convertEquipmentItemToProto(item choices.EquipmentItem) *dnd5ev1alpha1.EquipmentItem {
	result := &dnd5ev1alpha1.EquipmentItem{
		SelectionId: item.ID,
		Quantity:    int32(item.Quantity),
	}
	setEquipmentItemTypeHint(result, item.ID)
	result.EquipmentDetail = convertEquipmentDetailToProto(item.Detail)
	return result
}

// setEquipmentItemTypeHint sets the type hint on an EquipmentItem based on the item ID
func setEquipmentItemTypeHint(item *dnd5ev1alpha1.EquipmentItem, itemID string) {
	// Common armor pieces
	armorMap := map[string]dnd5ev1alpha1.Armor{
		"shield":          dnd5ev1alpha1.Armor_ARMOR_SHIELD,
		"padded":          dnd5ev1alpha1.Armor_ARMOR_PADDED,
		"leather":         dnd5ev1alpha1.Armor_ARMOR_LEATHER,
		"studded-leather": dnd5ev1alpha1.Armor_ARMOR_STUDDED_LEATHER,
		"hide":            dnd5ev1alpha1.Armor_ARMOR_HIDE,
		"chain-shirt":     dnd5ev1alpha1.Armor_ARMOR_CHAIN_SHIRT,
		"scale-mail":      dnd5ev1alpha1.Armor_ARMOR_SCALE_MAIL,
		"breastplate":     dnd5ev1alpha1.Armor_ARMOR_BREASTPLATE,
		"half-plate":      dnd5ev1alpha1.Armor_ARMOR_HALF_PLATE,
		"ring-mail":       dnd5ev1alpha1.Armor_ARMOR_RING_MAIL,
		"chain-mail":      dnd5ev1alpha1.Armor_ARMOR_CHAIN_MAIL,
		"splint":          dnd5ev1alpha1.Armor_ARMOR_SPLINT,
		"plate":           dnd5ev1alpha1.Armor_ARMOR_PLATE,
	}

	if armor, ok := armorMap[itemID]; ok {
		item.TypeHint = &dnd5ev1alpha1.EquipmentItem_Armor{
			Armor: armor,
		}
		return
	}

	// Common weapons
	weaponMap := map[string]dnd5ev1alpha1.Weapon{
		"club":           dnd5ev1alpha1.Weapon_WEAPON_CLUB,
		"dagger":         dnd5ev1alpha1.Weapon_WEAPON_DAGGER,
		"greatclub":      dnd5ev1alpha1.Weapon_WEAPON_GREATCLUB,
		"handaxe":        dnd5ev1alpha1.Weapon_WEAPON_HANDAXE,
		"javelin":        dnd5ev1alpha1.Weapon_WEAPON_JAVELIN,
		"light-hammer":   dnd5ev1alpha1.Weapon_WEAPON_LIGHT_HAMMER,
		"mace":           dnd5ev1alpha1.Weapon_WEAPON_MACE,
		"quarterstaff":   dnd5ev1alpha1.Weapon_WEAPON_QUARTERSTAFF,
		"sickle":         dnd5ev1alpha1.Weapon_WEAPON_SICKLE,
		"spear":          dnd5ev1alpha1.Weapon_WEAPON_SPEAR,
		"light-crossbow": dnd5ev1alpha1.Weapon_WEAPON_LIGHT_CROSSBOW,
		"dart":           dnd5ev1alpha1.Weapon_WEAPON_DART,
		"shortbow":       dnd5ev1alpha1.Weapon_WEAPON_SHORTBOW,
		"sling":          dnd5ev1alpha1.Weapon_WEAPON_SLING,
		"battleaxe":      dnd5ev1alpha1.Weapon_WEAPON_BATTLEAXE,
		"flail":          dnd5ev1alpha1.Weapon_WEAPON_FLAIL,
		"glaive":         dnd5ev1alpha1.Weapon_WEAPON_GLAIVE,
		"greataxe":       dnd5ev1alpha1.Weapon_WEAPON_GREATAXE,
		"greatsword":     dnd5ev1alpha1.Weapon_WEAPON_GREATSWORD,
		"halberd":        dnd5ev1alpha1.Weapon_WEAPON_HALBERD,
		"lance":          dnd5ev1alpha1.Weapon_WEAPON_LANCE,
		"longsword":      dnd5ev1alpha1.Weapon_WEAPON_LONGSWORD,
		"maul":           dnd5ev1alpha1.Weapon_WEAPON_MAUL,
		"morningstar":    dnd5ev1alpha1.Weapon_WEAPON_MORNINGSTAR,
		"pike":           dnd5ev1alpha1.Weapon_WEAPON_PIKE,
		"rapier":         dnd5ev1alpha1.Weapon_WEAPON_RAPIER,
		"scimitar":       dnd5ev1alpha1.Weapon_WEAPON_SCIMITAR,
		"shortsword":     dnd5ev1alpha1.Weapon_WEAPON_SHORTSWORD,
		"trident":        dnd5ev1alpha1.Weapon_WEAPON_TRIDENT,
		"war-pick":       dnd5ev1alpha1.Weapon_WEAPON_WAR_PICK,
		"warhammer":      dnd5ev1alpha1.Weapon_WEAPON_WARHAMMER,
		"whip":           dnd5ev1alpha1.Weapon_WEAPON_WHIP,
		"blowgun":        dnd5ev1alpha1.Weapon_WEAPON_BLOWGUN,
		"hand-crossbow":  dnd5ev1alpha1.Weapon_WEAPON_HAND_CROSSBOW,
		"heavy-crossbow": dnd5ev1alpha1.Weapon_WEAPON_HEAVY_CROSSBOW,
		"longbow":        dnd5ev1alpha1.Weapon_WEAPON_LONGBOW,
		"net":            dnd5ev1alpha1.Weapon_WEAPON_NET,
	}

	if weapon, ok := weaponMap[itemID]; ok {
		item.TypeHint = &dnd5ev1alpha1.EquipmentItem_Weapon{
			Weapon: weapon,
		}
		return
	}

	if tool := convertToolToProto(tools.ToolID(itemID)); tool != dnd5ev1alpha1.Tool_TOOL_UNSPECIFIED {
		item.TypeHint = &dnd5ev1alpha1.EquipmentItem_Tool{Tool: tool}
	}
}

// parseSkillString converts a skill string to proto Skill enum
func parseSkillString(skillStr string) dnd5ev1alpha1.Skill {
	// Try to parse the string as a skills.Skill first
	switch strings.ToLower(skillStr) {
	case "acrobatics":
		return dnd5ev1alpha1.Skill_SKILL_ACROBATICS
	case "animal_handling", "animal handling":
		return dnd5ev1alpha1.Skill_SKILL_ANIMAL_HANDLING
	case "arcana":
		return dnd5ev1alpha1.Skill_SKILL_ARCANA
	case "athletics":
		return dnd5ev1alpha1.Skill_SKILL_ATHLETICS
	case "deception":
		return dnd5ev1alpha1.Skill_SKILL_DECEPTION
	case "history":
		return dnd5ev1alpha1.Skill_SKILL_HISTORY
	case "insight":
		return dnd5ev1alpha1.Skill_SKILL_INSIGHT
	case "intimidation":
		return dnd5ev1alpha1.Skill_SKILL_INTIMIDATION
	case "investigation":
		return dnd5ev1alpha1.Skill_SKILL_INVESTIGATION
	case "medicine":
		return dnd5ev1alpha1.Skill_SKILL_MEDICINE
	case "nature":
		return dnd5ev1alpha1.Skill_SKILL_NATURE
	case "perception":
		return dnd5ev1alpha1.Skill_SKILL_PERCEPTION
	case "performance":
		return dnd5ev1alpha1.Skill_SKILL_PERFORMANCE
	case "persuasion":
		return dnd5ev1alpha1.Skill_SKILL_PERSUASION
	case "religion":
		return dnd5ev1alpha1.Skill_SKILL_RELIGION
	case "sleight_of_hand", "sleight of hand":
		return dnd5ev1alpha1.Skill_SKILL_SLEIGHT_OF_HAND
	case "stealth":
		return dnd5ev1alpha1.Skill_SKILL_STEALTH
	case "survival":
		return dnd5ev1alpha1.Skill_SKILL_SURVIVAL
	default:
		return dnd5ev1alpha1.Skill_SKILL_UNSPECIFIED
	}
}

// parseToolString converts a tool string to proto Tool enum
func parseToolString(toolStr string) dnd5ev1alpha1.Tool {
	// Normalize the string and handle different formats
	normalized := strings.ToLower(strings.TrimSpace(toolStr))
	normalized = strings.ReplaceAll(normalized, "'", "")
	normalized = strings.ReplaceAll(normalized, "-", " ")

	switch normalized {
	// Artisan's tools
	case "smiths tools", "smith tools":
		return dnd5ev1alpha1.Tool_TOOL_SMITH_TOOLS
	case "brewers supplies", "brewer supplies":
		return dnd5ev1alpha1.Tool_TOOL_BREWER_SUPPLIES
	case "masons tools", "mason tools":
		return dnd5ev1alpha1.Tool_TOOL_MASON_TOOLS
	case "alchemists supplies", "alchemist supplies":
		return dnd5ev1alpha1.Tool_TOOL_ALCHEMIST_SUPPLIES
	case "calligraphers supplies", "calligrapher supplies":
		return dnd5ev1alpha1.Tool_TOOL_CALLIGRAPHER_SUPPLIES
	case "carpenters tools", "carpenter tools":
		return dnd5ev1alpha1.Tool_TOOL_CARPENTER_TOOLS
	case "cartographers tools", "cartographer tools":
		return dnd5ev1alpha1.Tool_TOOL_CARTOGRAPHER_TOOLS
	case "cobblers tools", "cobbler tools":
		return dnd5ev1alpha1.Tool_TOOL_COBBLER_TOOLS
	case "cooks utensils", "cook utensils":
		return dnd5ev1alpha1.Tool_TOOL_COOK_UTENSILS
	case "glassblowers tools", "glassblower tools":
		return dnd5ev1alpha1.Tool_TOOL_GLASSBLOWER_TOOLS
	case "jewelers tools", "jeweler tools":
		return dnd5ev1alpha1.Tool_TOOL_JEWELER_TOOLS
	case "leatherworkers tools", "leatherworker tools":
		return dnd5ev1alpha1.Tool_TOOL_LEATHERWORKER_TOOLS
	case "painters supplies", "painter supplies":
		return dnd5ev1alpha1.Tool_TOOL_PAINTER_SUPPLIES
	case "potters tools", "potter tools":
		return dnd5ev1alpha1.Tool_TOOL_POTTER_TOOLS
	case "tinkers tools", "tinker tools":
		return dnd5ev1alpha1.Tool_TOOL_TINKER_TOOLS
	case "weavers tools", "weaver tools":
		return dnd5ev1alpha1.Tool_TOOL_WEAVER_TOOLS
	case "woodcarvers tools", "woodcarver tools":
		return dnd5ev1alpha1.Tool_TOOL_WOODCARVER_TOOLS
	// Other tools
	case "disguise kit":
		return dnd5ev1alpha1.Tool_TOOL_DISGUISE_KIT
	case "forgery kit":
		return dnd5ev1alpha1.Tool_TOOL_FORGERY_KIT
	case "herbalism kit":
		return dnd5ev1alpha1.Tool_TOOL_HERBALISM_KIT
	case "navigators tools", "navigator tools":
		return dnd5ev1alpha1.Tool_TOOL_NAVIGATOR_TOOLS
	case "poisoners kit", "poisoner kit":
		return dnd5ev1alpha1.Tool_TOOL_POISONER_KIT
	case "thieves tools", "thief tools":
		return dnd5ev1alpha1.Tool_TOOL_THIEVES_TOOLS
	// Gaming sets
	case "dice set":
		return dnd5ev1alpha1.Tool_TOOL_DICE_SET
	case "playing card set", "playing cards":
		return dnd5ev1alpha1.Tool_TOOL_PLAYING_CARD_SET
	// Musical instruments
	case "bagpipes":
		return dnd5ev1alpha1.Tool_TOOL_BAGPIPES
	case "drum":
		return dnd5ev1alpha1.Tool_TOOL_DRUM
	case "dulcimer":
		return dnd5ev1alpha1.Tool_TOOL_DULCIMER
	case "flute":
		return dnd5ev1alpha1.Tool_TOOL_FLUTE
	case "lute":
		return dnd5ev1alpha1.Tool_TOOL_LUTE
	case "lyre":
		return dnd5ev1alpha1.Tool_TOOL_LYRE
	case "horn":
		return dnd5ev1alpha1.Tool_TOOL_HORN
	case "pan flute":
		return dnd5ev1alpha1.Tool_TOOL_PAN_FLUTE
	case "shawm":
		return dnd5ev1alpha1.Tool_TOOL_SHAWM
	case "viol":
		return dnd5ev1alpha1.Tool_TOOL_VIOL
	default:
		return dnd5ev1alpha1.Tool_TOOL_UNSPECIFIED
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

// createToolChoice converts a tool requirement to a proto Choice
func createToolChoice(req *choices.ToolRequirement) *dnd5ev1alpha1.Choice {
	if req == nil || len(req.Options) == 0 {
		return nil
	}

	// Convert tool options to proto enums
	toolOptions := make([]dnd5ev1alpha1.Tool, 0, len(req.Options))
	for _, toolStr := range req.Options {
		if tool := parseToolString(toolStr); tool != dnd5ev1alpha1.Tool_TOOL_UNSPECIFIED {
			toolOptions = append(toolOptions, tool)
		}
	}

	// If no valid tools could be converted, return nil
	if len(toolOptions) == 0 {
		return nil
	}

	return &dnd5ev1alpha1.Choice{
		Id:          string(req.ID),
		Description: req.Label,
		ChooseCount: int32(req.Count),
		ChoiceType:  dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_TOOLS,
		Options: &dnd5ev1alpha1.Choice_ToolOptions{
			ToolOptions: &dnd5ev1alpha1.ToolOptions{
				Available: toolOptions,
			},
		},
	}
}

// createExpertiseChoice converts an expertise requirement to a proto Choice.
//
// ExpertiseRequirement has no fixed option list: expertise can apply to any
// skill the character ends up proficient in (class, race, or background), and
// that set isn't known until the draft's other choices are recorded. So this
// advertises the full skill universe here; the toolkit enforces the real
// subset (proficient skills only) when the choice is submitted and again at
// ValidateChoices.
//
// Tool-based expertise (e.g. thieves' tools) isn't advertised: the toolkit's
// class-choice write path only records skill selections for expertise today
// (see handler.go's CHOICE_CATEGORY_EXPERTISE case).
func createExpertiseChoice(req *choices.ExpertiseRequirement) *dnd5ev1alpha1.Choice {
	if req == nil || req.Count <= 0 {
		return nil
	}

	allSkills := skills.List()
	skillOptions := make([]dnd5ev1alpha1.Skill, 0, len(allSkills))
	for _, skill := range allSkills {
		skillOptions = append(skillOptions, convertSkillToProtoEnum(skill))
	}

	return &dnd5ev1alpha1.Choice{
		Id:          string(req.ID),
		Description: req.Label,
		ChooseCount: int32(req.Count),
		ChoiceType:  dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EXPERTISE,
		Options: &dnd5ev1alpha1.Choice_ExpertiseOptions{
			ExpertiseOptions: &dnd5ev1alpha1.ExpertiseOptions{
				AvailableSkills: skillOptions,
			},
		},
	}
}

// ConvertEquipmentToProto converts a toolkit Equipment interface to proto Equipment message
func ConvertEquipmentToProto(equip equipment.Equipment) *dnd5ev1alpha1.Equipment {
	if equip == nil {
		return nil
	}

	// Base equipment data
	result := &dnd5ev1alpha1.Equipment{
		Id:          equip.EquipmentID(),
		Name:        equip.EquipmentName(),
		Description: equip.EquipmentDescription(),
		Weight: &dnd5ev1alpha1.Weight{
			Quantity: int32(equip.EquipmentWeight()),
			Unit:     "lbs",
		},
	}

	// Set category and type-specific data based on equipment type
	switch equip.EquipmentType() {
	case shared.EquipmentTypeWeapon:
		if weapon, ok := equip.(*weapons.Weapon); ok {
			// Set category based on weapon type
			if weapon.IsSimple() {
				result.Category = dnd5ev1alpha1.EquipmentCategory_EQUIPMENT_CATEGORY_SIMPLE_WEAPON
			} else if weapon.IsMartial() {
				result.Category = dnd5ev1alpha1.EquipmentCategory_EQUIPMENT_CATEGORY_MARTIAL_WEAPON
			}
			result.EquipmentData = &dnd5ev1alpha1.Equipment_WeaponData{
				WeaponData: convertWeaponData(weapon),
			}
		}
	case shared.EquipmentTypeArmor:
		result.Category = dnd5ev1alpha1.EquipmentCategory_EQUIPMENT_CATEGORY_LIGHT_ARMOR
		// TODO: Add armor data conversion when needed
	case shared.EquipmentTypeTool:
		result.Category = dnd5ev1alpha1.EquipmentCategory_EQUIPMENT_CATEGORY_TOOLS
		// TODO: Add tool data conversion when needed
	default:
		result.Category = dnd5ev1alpha1.EquipmentCategory_EQUIPMENT_CATEGORY_ADVENTURING_GEAR
	}

	return result
}

// convertWeaponProperties converts toolkit weapon properties to proto weapon properties.
// Shared by convertWeaponData (concrete weapons.Weapon) and
// convertWeaponDetailToProto (resolved equipment.WeaponDetail) so the mapping
// lives in exactly one place.
func convertWeaponProperties(props []weapons.WeaponProperty) []dnd5ev1alpha1.WeaponProperty {
	properties := make([]dnd5ev1alpha1.WeaponProperty, 0, len(props))
	for _, prop := range props {
		switch prop {
		case weapons.PropertyLight:
			properties = append(properties, dnd5ev1alpha1.WeaponProperty_WEAPON_PROPERTY_LIGHT)
		case weapons.PropertyFinesse:
			properties = append(properties, dnd5ev1alpha1.WeaponProperty_WEAPON_PROPERTY_FINESSE)
		case weapons.PropertyThrown:
			properties = append(properties, dnd5ev1alpha1.WeaponProperty_WEAPON_PROPERTY_THROWN)
		case weapons.PropertyVersatile:
			properties = append(properties, dnd5ev1alpha1.WeaponProperty_WEAPON_PROPERTY_VERSATILE)
		case weapons.PropertyTwoHanded:
			properties = append(properties, dnd5ev1alpha1.WeaponProperty_WEAPON_PROPERTY_TWO_HANDED)
		case weapons.PropertyAmmunition:
			properties = append(properties, dnd5ev1alpha1.WeaponProperty_WEAPON_PROPERTY_AMMUNITION)
		case weapons.PropertyLoading:
			properties = append(properties, dnd5ev1alpha1.WeaponProperty_WEAPON_PROPERTY_LOADING)
		case weapons.PropertyReach:
			properties = append(properties, dnd5ev1alpha1.WeaponProperty_WEAPON_PROPERTY_REACH)
		case weapons.PropertyHeavy:
			properties = append(properties, dnd5ev1alpha1.WeaponProperty_WEAPON_PROPERTY_HEAVY)
		case weapons.PropertySpecial:
			properties = append(properties, dnd5ev1alpha1.WeaponProperty_WEAPON_PROPERTY_SPECIAL)
		}
	}
	return properties
}

// convertWeaponCategoryToProto converts the toolkit's fine-grained weapon
// category (simple-melee, simple-ranged, martial-melee, martial-ranged) to
// the proto's broader SIMPLE/MARTIAL category.
func convertWeaponCategoryToProto(cat weapons.WeaponCategory) dnd5ev1alpha1.WeaponCategory {
	switch {
	case strings.HasPrefix(string(cat), "simple"):
		return dnd5ev1alpha1.WeaponCategory_WEAPON_CATEGORY_SIMPLE
	case strings.HasPrefix(string(cat), "martial"):
		return dnd5ev1alpha1.WeaponCategory_WEAPON_CATEGORY_MARTIAL
	default:
		return dnd5ev1alpha1.WeaponCategory_WEAPON_CATEGORY_UNSPECIFIED
	}
}

// convertArmorCategoryToProto converts the toolkit's armor category
// (light/medium/heavy/shield) to the proto ArmorCategory enum.
func convertArmorCategoryToProto(cat armor.ArmorCategory) dnd5ev1alpha1.ArmorCategory {
	switch cat {
	case armor.CategoryLight:
		return dnd5ev1alpha1.ArmorCategory_ARMOR_CATEGORY_LIGHT
	case armor.CategoryMedium:
		return dnd5ev1alpha1.ArmorCategory_ARMOR_CATEGORY_MEDIUM
	case armor.CategoryHeavy:
		return dnd5ev1alpha1.ArmorCategory_ARMOR_CATEGORY_HEAVY
	case armor.CategoryShield:
		return dnd5ev1alpha1.ArmorCategory_ARMOR_CATEGORY_SHIELD
	default:
		return dnd5ev1alpha1.ArmorCategory_ARMOR_CATEGORY_UNSPECIFIED
	}
}

// convertArmorCategoryToEquipmentCategory converts the toolkit's armor
// category to the proto's top-level EquipmentCategory enum used on
// Equipment.category.
func convertArmorCategoryToEquipmentCategory(cat armor.ArmorCategory) dnd5ev1alpha1.EquipmentCategory {
	switch cat {
	case armor.CategoryLight:
		return dnd5ev1alpha1.EquipmentCategory_EQUIPMENT_CATEGORY_LIGHT_ARMOR
	case armor.CategoryMedium:
		return dnd5ev1alpha1.EquipmentCategory_EQUIPMENT_CATEGORY_MEDIUM_ARMOR
	case armor.CategoryHeavy:
		return dnd5ev1alpha1.EquipmentCategory_EQUIPMENT_CATEGORY_HEAVY_ARMOR
	case armor.CategoryShield:
		return dnd5ev1alpha1.EquipmentCategory_EQUIPMENT_CATEGORY_SHIELD
	default:
		return dnd5ev1alpha1.EquipmentCategory_EQUIPMENT_CATEGORY_UNSPECIFIED
	}
}

// convertWeaponData converts weapon-specific data to proto
func convertWeaponData(weapon *weapons.Weapon) *dnd5ev1alpha1.WeaponData {
	if weapon == nil {
		return nil
	}

	// Determine weapon category
	var weaponCategory dnd5ev1alpha1.WeaponCategory
	if weapon.IsSimple() {
		weaponCategory = dnd5ev1alpha1.WeaponCategory_WEAPON_CATEGORY_SIMPLE
	} else if weapon.IsMartial() {
		weaponCategory = dnd5ev1alpha1.WeaponCategory_WEAPON_CATEGORY_MARTIAL
	}

	result := &dnd5ev1alpha1.WeaponData{
		WeaponCategory: weaponCategory,
		DamageDice:     weapon.Damage,
		DamageType:     convertDamageTypeToProto(string(weapon.DamageType)),
		Properties:     convertWeaponProperties(weapon.Properties),
	}
	applyWeaponRange(result, weapon.Range)

	return result
}

// weaponRangeMelee and weaponRangeRanged are the proto WeaponData.range
// string values. Shared by convertWeaponData and convertEquipmentDetailToProto
// so the literal exists in exactly one place.
const (
	weaponRangeMelee  = "melee"
	weaponRangeRanged = "ranged"
)

// applyWeaponRange sets WeaponData.range/normal_range/long_range from a
// toolkit weapons.Range (nil means melee). Shared by convertWeaponData
// (concrete weapons.Weapon) and convertEquipmentDetailToProto (resolved
// equipment.WeaponDetail).
func applyWeaponRange(weaponData *dnd5ev1alpha1.WeaponData, rng *weapons.Range) {
	if rng != nil {
		weaponData.Range = weaponRangeRanged
		weaponData.NormalRange = int32(rng.Normal)
		weaponData.LongRange = int32(rng.Long)
	} else {
		weaponData.Range = weaponRangeMelee
	}
}

// convertEquipmentDetailToProto converts the toolkit's resolved
// equipment.EquipmentDetail (already produced by
// equipment.ResolveEquipmentDetail, e.g. via choices.EquipmentItem.Detail)
// into the proto Equipment message used by EquipmentItem.equipment_detail.
// This is a pure translation — it does not recompute any D&D rule (damage,
// AC, properties); those values are read straight off the toolkit's
// already-resolved detail.
func convertEquipmentDetailToProto(detail *equipment.EquipmentDetail) *dnd5ev1alpha1.Equipment {
	if detail == nil {
		return nil
	}

	result := &dnd5ev1alpha1.Equipment{
		Name: detail.Name,
		Weight: &dnd5ev1alpha1.Weight{
			Quantity: int32(detail.Weight),
			Unit:     "lbs",
		},
		Cost: convertEquipmentCostToProto(detail.Cost),
	}

	switch {
	case detail.Weapon != nil:
		weaponCategory := convertWeaponCategoryToProto(detail.Weapon.Category)
		if weaponCategory == dnd5ev1alpha1.WeaponCategory_WEAPON_CATEGORY_MARTIAL {
			result.Category = dnd5ev1alpha1.EquipmentCategory_EQUIPMENT_CATEGORY_MARTIAL_WEAPON
		} else {
			result.Category = dnd5ev1alpha1.EquipmentCategory_EQUIPMENT_CATEGORY_SIMPLE_WEAPON
		}
		weaponData := &dnd5ev1alpha1.WeaponData{
			WeaponCategory: weaponCategory,
			DamageDice:     detail.Weapon.Damage,
			DamageType:     convertDamageTypeToProto(string(detail.Weapon.DamageType)),
			Properties:     convertWeaponProperties(detail.Weapon.Properties),
		}
		applyWeaponRange(weaponData, detail.Weapon.Range)
		result.EquipmentData = &dnd5ev1alpha1.Equipment_WeaponData{WeaponData: weaponData}
	case detail.Armor != nil:
		result.Category = convertArmorCategoryToEquipmentCategory(detail.Armor.Category)
		armorData := &dnd5ev1alpha1.ArmorData{
			ArmorCategory:       convertArmorCategoryToProto(detail.Armor.Category),
			BaseAc:              int32(detail.Armor.BaseAC),
			DexBonus:            detail.Armor.DexBonus,
			StrMinimum:          int32(detail.Armor.StrengthRequirement),
			StealthDisadvantage: detail.Armor.StealthDisadvantage,
		}
		if detail.Armor.MaxDexBonus != nil {
			armorData.HasDexLimit = true
			armorData.MaxDexBonus = int32(*detail.Armor.MaxDexBonus)
		}
		result.EquipmentData = &dnd5ev1alpha1.Equipment_ArmorData{ArmorData: armorData}
	case detail.Type == shared.EquipmentTypeTool:
		result.Category = dnd5ev1alpha1.EquipmentCategory_EQUIPMENT_CATEGORY_TOOLS
	default:
		result.Category = dnd5ev1alpha1.EquipmentCategory_EQUIPMENT_CATEGORY_ADVENTURING_GEAR
	}

	return result
}

// convertEquipmentCostToProto maps a toolkit cost string when it is exactly
// representable by the proto's single quantity/unit pair. Multi-denomination
// costs have no lossless representation in the current proto and remain unset.
func convertEquipmentCostToProto(cost string) *dnd5ev1alpha1.Cost {
	parts := strings.Fields(cost)
	if len(parts) != 2 {
		return nil
	}
	quantity, err := strconv.ParseInt(parts[0], 10, 32)
	if err != nil {
		return nil
	}
	return &dnd5ev1alpha1.Cost{Quantity: int32(quantity), Unit: parts[1]}
}

// convertDamageTypeToProto converts toolkit damage type to proto
func convertDamageTypeToProto(damageType string) dnd5ev1alpha1.DamageType {
	switch damageType {
	case "slashing":
		return dnd5ev1alpha1.DamageType_DAMAGE_TYPE_SLASHING
	case "piercing":
		return dnd5ev1alpha1.DamageType_DAMAGE_TYPE_PIERCING
	case "bludgeoning":
		return dnd5ev1alpha1.DamageType_DAMAGE_TYPE_BLUDGEONING
	case "fire":
		return dnd5ev1alpha1.DamageType_DAMAGE_TYPE_FIRE
	case "cold":
		return dnd5ev1alpha1.DamageType_DAMAGE_TYPE_COLD
	case "lightning":
		return dnd5ev1alpha1.DamageType_DAMAGE_TYPE_LIGHTNING
	case "thunder":
		return dnd5ev1alpha1.DamageType_DAMAGE_TYPE_THUNDER
	case "poison":
		return dnd5ev1alpha1.DamageType_DAMAGE_TYPE_POISON
	case "acid":
		return dnd5ev1alpha1.DamageType_DAMAGE_TYPE_ACID
	case "psychic":
		return dnd5ev1alpha1.DamageType_DAMAGE_TYPE_PSYCHIC
	case "necrotic":
		return dnd5ev1alpha1.DamageType_DAMAGE_TYPE_NECROTIC
	case "radiant":
		return dnd5ev1alpha1.DamageType_DAMAGE_TYPE_RADIANT
	case "force":
		return dnd5ev1alpha1.DamageType_DAMAGE_TYPE_FORCE
	default:
		return dnd5ev1alpha1.DamageType_DAMAGE_TYPE_UNSPECIFIED
	}
}

// convertSpellToProtoEnum converts a toolkit spell to proto enum
func convertSpellToProtoEnum(spell spells.Spell) dnd5ev1alpha1.Spell {
	// Map spell IDs to proto enums
	switch spell {
	// Cantrips
	case spells.FireBolt:
		return dnd5ev1alpha1.Spell_SPELL_FIRE_BOLT
	case spells.RayOfFrost:
		return dnd5ev1alpha1.Spell_SPELL_RAY_OF_FROST
	case spells.ShockingGrasp:
		return dnd5ev1alpha1.Spell_SPELL_SHOCKING_GRASP
	case spells.AcidSplash:
		return dnd5ev1alpha1.Spell_SPELL_ACID_SPLASH
	case spells.PoisonSpray:
		return dnd5ev1alpha1.Spell_SPELL_POISON_SPRAY
	case spells.ChillTouch:
		return dnd5ev1alpha1.Spell_SPELL_CHILL_TOUCH
	case spells.SacredFlame:
		return dnd5ev1alpha1.Spell_SPELL_SACRED_FLAME
	case spells.TollTheDead:
		return dnd5ev1alpha1.Spell_SPELL_TOLL_THE_DEAD
	case spells.WordOfRadiance:
		return dnd5ev1alpha1.Spell_SPELL_WORD_OF_RADIANCE
	case spells.EldritchBlast:
		return dnd5ev1alpha1.Spell_SPELL_ELDRITCH_BLAST
	case spells.Frostbite:
		return dnd5ev1alpha1.Spell_SPELL_FROSTBITE
	case spells.PrimalSavagery:
		return dnd5ev1alpha1.Spell_SPELL_PRIMAL_SAVAGERY
	case spells.Thornwhip:
		return dnd5ev1alpha1.Spell_SPELL_THORNWHIP
	case spells.MageHand:
		return dnd5ev1alpha1.Spell_SPELL_MAGE_HAND
	case spells.MinorIllusion:
		return dnd5ev1alpha1.Spell_SPELL_MINOR_ILLUSION
	case spells.Prestidigitation:
		return dnd5ev1alpha1.Spell_SPELL_PRESTIDIGITATION
	case spells.Light:
		return dnd5ev1alpha1.Spell_SPELL_LIGHT
	case spells.Guidance:
		return dnd5ev1alpha1.Spell_SPELL_GUIDANCE
	case spells.Resistance:
		return dnd5ev1alpha1.Spell_SPELL_RESISTANCE
	case spells.Thaumaturgy:
		return dnd5ev1alpha1.Spell_SPELL_THAUMATURGY
	case spells.SpareTheDying:
		return dnd5ev1alpha1.Spell_SPELL_SPARE_THE_DYING

	// Level 1 spells
	case spells.MagicMissile:
		return dnd5ev1alpha1.Spell_SPELL_MAGIC_MISSILE
	case spells.BurningHands:
		return dnd5ev1alpha1.Spell_SPELL_BURNING_HANDS
	case spells.ChromaticOrb:
		return dnd5ev1alpha1.Spell_SPELL_CHROMATIC_ORB
	case spells.Thunderwave:
		return dnd5ev1alpha1.Spell_SPELL_THUNDERWAVE
	case spells.IceKnife:
		return dnd5ev1alpha1.Spell_SPELL_ICE_KNIFE
	case spells.WitchBolt:
		return dnd5ev1alpha1.Spell_SPELL_WITCH_BOLT
	case spells.GuidingBolt:
		return dnd5ev1alpha1.Spell_SPELL_GUIDING_BOLT
	case spells.InflictWounds:
		return dnd5ev1alpha1.Spell_SPELL_INFLICT_WOUNDS
	case spells.HailOfThorns:
		return dnd5ev1alpha1.Spell_SPELL_HAIL_OF_THORNS
	case spells.EnsnaringStrike:
		return dnd5ev1alpha1.Spell_SPELL_ENSNARING_STRIKE
	case spells.HellishRebuke:
		return dnd5ev1alpha1.Spell_SPELL_HELLISH_REBUKE
	case spells.ArmsOfHadar:
		return dnd5ev1alpha1.Spell_SPELL_ARMS_OF_HADAR
	case spells.Hex:
		return dnd5ev1alpha1.Spell_SPELL_HEX
	case spells.SearingSmite:
		return dnd5ev1alpha1.Spell_SPELL_SEARING_SMITE
	case spells.ThunderousSmite:
		return dnd5ev1alpha1.Spell_SPELL_THUNDEROUS_SMITE
	case spells.WrathfulSmite:
		return dnd5ev1alpha1.Spell_SPELL_WRATHFUL_SMITE
	case spells.Shield:
		return dnd5ev1alpha1.Spell_SPELL_SHIELD
	case spells.Sleep:
		return dnd5ev1alpha1.Spell_SPELL_SLEEP
	case spells.CharmPerson:
		return dnd5ev1alpha1.Spell_SPELL_CHARM_PERSON
	case spells.DetectMagic:
		return dnd5ev1alpha1.Spell_SPELL_DETECT_MAGIC
	case spells.Identify:
		return dnd5ev1alpha1.Spell_SPELL_IDENTIFY
	case spells.CureWounds:
		return dnd5ev1alpha1.Spell_SPELL_CURE_WOUNDS
	case spells.HealingWord:
		return dnd5ev1alpha1.Spell_SPELL_HEALING_WORD
	case spells.Bless:
		return dnd5ev1alpha1.Spell_SPELL_BLESS
	case spells.Bane:
		return dnd5ev1alpha1.Spell_SPELL_BANE
	case spells.ShieldOfFaith:
		return dnd5ev1alpha1.Spell_SPELL_SHIELD_OF_FAITH

	// TODO: Add more spell mappings as proto enums are defined

	default:
		return dnd5ev1alpha1.Spell_SPELL_UNSPECIFIED
	}
}

// getFeatureActionType returns the action economy cost for a feature by querying the toolkit.
// Returns ACTION_TYPE_UNSPECIFIED for features not implemented in the toolkit.
func getFeatureActionType(featureID string) dnd5ev1alpha1.ActionType {
	// Construct the full ref string for the feature
	refString := fmt.Sprintf("%s:%s:%s", refs.Module, refs.TypeFeatures, featureID)

	// Try to create the feature from the toolkit
	output, err := features.CreateFromRef(&features.CreateFromRefInput{
		Ref:         refString,
		CharacterID: "action-type-lookup", // Placeholder - not used for ActionType()
	})
	if err != nil {
		// Feature not implemented in toolkit, return unspecified
		return dnd5ev1alpha1.ActionType_ACTION_TYPE_UNSPECIFIED
	}

	// Map toolkit ActionType to proto ActionType
	return convertActionTypeToProto(output.Feature.ActionType())
}

// convertActionTypeToProto maps toolkit combat.ActionType to proto ActionType.
func convertActionTypeToProto(actionType combat.ActionType) dnd5ev1alpha1.ActionType {
	switch actionType {
	case combat.ActionStandard:
		return dnd5ev1alpha1.ActionType_ACTION_TYPE_ACTION
	case combat.ActionBonus:
		return dnd5ev1alpha1.ActionType_ACTION_TYPE_BONUS_ACTION
	case combat.ActionReaction:
		return dnd5ev1alpha1.ActionType_ACTION_TYPE_REACTION
	case combat.ActionFree:
		return dnd5ev1alpha1.ActionType_ACTION_TYPE_FREE
	default:
		return dnd5ev1alpha1.ActionType_ACTION_TYPE_UNSPECIFIED
	}
}

// extractIDFromRef extracts the ID part from a ref string (e.g., "dnd5e:conditions:raging" -> "raging")
func extractIDFromRef(ref string) string {
	parts := strings.Split(ref, ":")
	if len(parts) < 3 {
		return ""
	}
	return parts[len(parts)-1]
}

// toTitleCase converts snake_case to Title Case (e.g., "sneak_attack" -> "Sneak Attack")
func toTitleCase(s string) string {
	words := strings.Split(s, "_")
	for i, word := range words {
		if word != "" {
			words[i] = strings.ToUpper(word[:1]) + strings.ToLower(word[1:])
		}
	}
	return strings.Join(words, " ")
}

// conditionIDToEnum maps a condition ID string to the ConditionId proto enum
// Uses toolkit refs for type-safe comparison - no magic strings
func conditionIDToEnum(id string) dnd5ev1alpha1.ConditionId {
	switch id {
	case refs.Conditions.Raging().ID:
		return dnd5ev1alpha1.ConditionId_CONDITION_ID_RAGING
	case refs.Conditions.BrutalCritical().ID:
		return dnd5ev1alpha1.ConditionId_CONDITION_ID_BRUTAL_CRITICAL
	case refs.Conditions.SneakAttack().ID:
		return dnd5ev1alpha1.ConditionId_CONDITION_ID_SNEAK_ATTACK
	case refs.Features.DivineSmite().ID:
		return dnd5ev1alpha1.ConditionId_CONDITION_ID_DIVINE_SMITE
	case refs.Conditions.FightingStyleDueling().ID:
		return dnd5ev1alpha1.ConditionId_CONDITION_ID_FIGHTING_STYLE_DUELING
	case refs.Conditions.FightingStyleTwoWeaponFighting().ID:
		return dnd5ev1alpha1.ConditionId_CONDITION_ID_FIGHTING_STYLE_TWO_WEAPON_FIGHTING
	case refs.Conditions.FightingStyleGreatWeaponFighting().ID:
		return dnd5ev1alpha1.ConditionId_CONDITION_ID_FIGHTING_STYLE_GREAT_WEAPON_FIGHTING
	case refs.Conditions.FightingStyleArchery().ID:
		return dnd5ev1alpha1.ConditionId_CONDITION_ID_FIGHTING_STYLE_ARCHERY
	case refs.Conditions.FightingStyleDefense().ID:
		return dnd5ev1alpha1.ConditionId_CONDITION_ID_FIGHTING_STYLE_DEFENSE
	case refs.Conditions.FightingStyleProtection().ID:
		return dnd5ev1alpha1.ConditionId_CONDITION_ID_FIGHTING_STYLE_PROTECTION
	case refs.Conditions.UnarmoredDefense().ID:
		return dnd5ev1alpha1.ConditionId_CONDITION_ID_UNARMORED_DEFENSE
	case refs.Conditions.ImprovedCritical().ID:
		return dnd5ev1alpha1.ConditionId_CONDITION_ID_IMPROVED_CRITICAL
	case refs.Conditions.MartialArts().ID:
		return dnd5ev1alpha1.ConditionId_CONDITION_ID_MARTIAL_ARTS
	case refs.Conditions.UnarmoredMovement().ID:
		return dnd5ev1alpha1.ConditionId_CONDITION_ID_UNARMORED_MOVEMENT
	default:
		return dnd5ev1alpha1.ConditionId_CONDITION_ID_UNSPECIFIED
	}
}

// conditionIDToDisplayName converts a condition ID to a human-readable display name
// Uses toolkit refs for type-safe comparison - no magic strings
func conditionIDToDisplayName(id string) string {
	switch id {
	case refs.Conditions.Raging().ID:
		return "Raging"
	case refs.Conditions.BrutalCritical().ID:
		return "Brutal Critical"
	case refs.Conditions.SneakAttack().ID:
		return "Sneak Attack"
	case refs.Features.DivineSmite().ID:
		return "Divine Smite"
	case refs.Conditions.FightingStyleDueling().ID:
		return "Fighting Style: Dueling"
	case refs.Conditions.FightingStyleTwoWeaponFighting().ID:
		return "Fighting Style: Two-Weapon Fighting"
	case refs.Conditions.FightingStyleGreatWeaponFighting().ID:
		return "Fighting Style: Great Weapon Fighting"
	case refs.Conditions.FightingStyleArchery().ID:
		return "Fighting Style: Archery"
	case refs.Conditions.FightingStyleDefense().ID:
		return "Fighting Style: Defense"
	case refs.Conditions.FightingStyleProtection().ID:
		return "Fighting Style: Protection"
	case refs.Conditions.UnarmoredDefense().ID:
		return "Unarmored Defense"
	case refs.Conditions.ImprovedCritical().ID:
		return "Improved Critical"
	case refs.Conditions.MartialArts().ID:
		return "Martial Arts"
	case refs.Conditions.UnarmoredMovement().ID:
		return "Unarmored Movement"
	default:
		// Convert snake_case to Title Case as fallback
		return toTitleCase(id)
	}
}

// featureIDToEnum maps a feature ID string to the FeatureId proto enum
// Uses toolkit refs where available - remaining magic strings are for features
// that don't have toolkit refs yet (breath_weapon, hellish_rebuke, etc.)
func featureIDToEnum(id string) dnd5ev1alpha1.FeatureId {
	switch id {
	// Barbarian features
	case refs.Features.Rage().ID:
		return dnd5ev1alpha1.FeatureId_FEATURE_ID_RAGE
	// Fighter features
	case refs.Features.SecondWind().ID:
		return dnd5ev1alpha1.FeatureId_FEATURE_ID_SECOND_WIND
	case refs.Features.ActionSurge().ID:
		return dnd5ev1alpha1.FeatureId_FEATURE_ID_ACTION_SURGE
	// Rogue features
	case refs.Features.SneakAttack().ID:
		return dnd5ev1alpha1.FeatureId_FEATURE_ID_SNEAK_ATTACK
	// Paladin features
	case refs.Features.DivineSmite().ID:
		return dnd5ev1alpha1.FeatureId_FEATURE_ID_DIVINE_SMITE
	// Monk features
	case refs.Features.FlurryOfBlows().ID:
		return dnd5ev1alpha1.FeatureId_FEATURE_ID_FLURRY_OF_BLOWS
	case refs.Features.PatientDefense().ID:
		return dnd5ev1alpha1.FeatureId_FEATURE_ID_PATIENT_DEFENSE
	case refs.Features.StepOfTheWind().ID:
		return dnd5ev1alpha1.FeatureId_FEATURE_ID_STEP_OF_THE_WIND
	case refs.Features.DeflectMissiles().ID:
		return dnd5ev1alpha1.FeatureId_FEATURE_ID_DEFLECT_MISSILES
	// Features below don't have toolkit refs yet - magic strings until toolkit adds them
	case "breath_weapon":
		return dnd5ev1alpha1.FeatureId_FEATURE_ID_BREATH_WEAPON
	case "hellish_rebuke":
		return dnd5ev1alpha1.FeatureId_FEATURE_ID_HELLISH_REBUKE
	case "radiance_of_dawn":
		return dnd5ev1alpha1.FeatureId_FEATURE_ID_RADIANCE_OF_DAWN
	case "wrath_of_the_storm":
		return dnd5ev1alpha1.FeatureId_FEATURE_ID_WRATH_OF_THE_STORM
	case "destructive_wrath":
		return dnd5ev1alpha1.FeatureId_FEATURE_ID_DESTRUCTIVE_WRATH
	case "starry_form_archer":
		return dnd5ev1alpha1.FeatureId_FEATURE_ID_STARRY_FORM_ARCHER
	default:
		return dnd5ev1alpha1.FeatureId_FEATURE_ID_UNSPECIFIED
	}
}

// featureIDToDisplayName converts a feature ID to a human-readable display name
// Uses toolkit refs for type-safe comparison - no magic strings
func featureIDToDisplayName(id string) string {
	switch id {
	case refs.Features.Rage().ID:
		return "Rage"
	case refs.Features.SecondWind().ID:
		return "Second Wind"
	case refs.Features.ActionSurge().ID:
		return "Action Surge"
	case refs.Features.FlurryOfBlows().ID:
		return "Flurry of Blows"
	case refs.Features.PatientDefense().ID:
		return "Patient Defense"
	case refs.Features.StepOfTheWind().ID:
		return "Step of the Wind"
	case refs.Features.DeflectMissiles().ID:
		return "Deflect Missiles"
	case refs.Features.SneakAttack().ID:
		return "Sneak Attack"
	case refs.Features.DivineSmite().ID:
		return "Divine Smite"
	case refs.Features.BrutalCritical().ID:
		return "Brutal Critical"
	default:
		// Convert snake_case to Title Case as fallback
		return toTitleCase(id)
	}
}

// convertProtoAppearanceToEntity converts proto Appearance to entity Appearance
func convertProtoAppearanceToEntity(proto *dnd5ev1alpha1.Appearance) *entities.Appearance {
	if proto == nil {
		return nil
	}
	return &entities.Appearance{
		SkinTone:       proto.SkinTone,
		PrimaryColor:   proto.PrimaryColor,
		SecondaryColor: proto.SecondaryColor,
		EyeColor:       proto.EyeColor,
	}
}

// convertEntityAppearanceToProto converts entity Appearance to proto Appearance
func convertEntityAppearanceToProto(entity *entities.Appearance) *dnd5ev1alpha1.Appearance {
	if entity == nil {
		return nil
	}
	return &dnd5ev1alpha1.Appearance{
		SkinTone:       entity.SkinTone,
		PrimaryColor:   entity.PrimaryColor,
		SecondaryColor: entity.SecondaryColor,
		EyeColor:       entity.EyeColor,
	}
}
