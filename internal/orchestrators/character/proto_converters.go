package character

import (
	pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/languages"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/races"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/skills"
)

// convertProtoRaceToToolkit converts proto Race enum to toolkit Race type
func convertProtoRaceToToolkit(race pb.Race) races.Race {
	switch race {
	case pb.Race_RACE_HUMAN:
		return races.Human
	case pb.Race_RACE_ELF:
		return races.Elf
	case pb.Race_RACE_DWARF:
		return races.Dwarf
	case pb.Race_RACE_HALFLING:
		return races.Halfling
	case pb.Race_RACE_DRAGONBORN:
		return races.Dragonborn
	case pb.Race_RACE_GNOME:
		return races.Gnome
	case pb.Race_RACE_HALF_ELF:
		return races.HalfElf
	case pb.Race_RACE_HALF_ORC:
		return races.HalfOrc
	case pb.Race_RACE_TIEFLING:
		return races.Tiefling
	default:
		return races.Invalid
	}
}

// convertProtoClassToToolkit converts proto Class enum to toolkit Class type
func convertProtoClassToToolkit(class pb.Class) classes.Class {
	switch class {
	case pb.Class_CLASS_BARBARIAN:
		return classes.Barbarian
	case pb.Class_CLASS_BARD:
		return classes.Bard
	case pb.Class_CLASS_CLERIC:
		return classes.Cleric
	case pb.Class_CLASS_DRUID:
		return classes.Druid
	case pb.Class_CLASS_FIGHTER:
		return classes.Fighter
	case pb.Class_CLASS_MONK:
		return classes.Monk
	case pb.Class_CLASS_PALADIN:
		return classes.Paladin
	case pb.Class_CLASS_RANGER:
		return classes.Ranger
	case pb.Class_CLASS_ROGUE:
		return classes.Rogue
	case pb.Class_CLASS_SORCERER:
		return classes.Sorcerer
	case pb.Class_CLASS_WARLOCK:
		return classes.Warlock
	case pb.Class_CLASS_WIZARD:
		return classes.Wizard
	default:
		return classes.Invalid
	}
}

// convertProtoSkillToToolkit converts proto Skill enum to toolkit Skill type
func convertProtoSkillToToolkit(skill pb.Skill) skills.Skill {
	switch skill {
	case pb.Skill_SKILL_ACROBATICS:
		return skills.Acrobatics
	case pb.Skill_SKILL_ANIMAL_HANDLING:
		return skills.AnimalHandling
	case pb.Skill_SKILL_ARCANA:
		return skills.Arcana
	case pb.Skill_SKILL_ATHLETICS:
		return skills.Athletics
	case pb.Skill_SKILL_DECEPTION:
		return skills.Deception
	case pb.Skill_SKILL_HISTORY:
		return skills.History
	case pb.Skill_SKILL_INSIGHT:
		return skills.Insight
	case pb.Skill_SKILL_INTIMIDATION:
		return skills.Intimidation
	case pb.Skill_SKILL_INVESTIGATION:
		return skills.Investigation
	case pb.Skill_SKILL_MEDICINE:
		return skills.Medicine
	case pb.Skill_SKILL_NATURE:
		return skills.Nature
	case pb.Skill_SKILL_PERCEPTION:
		return skills.Perception
	case pb.Skill_SKILL_PERFORMANCE:
		return skills.Performance
	case pb.Skill_SKILL_PERSUASION:
		return skills.Persuasion
	case pb.Skill_SKILL_RELIGION:
		return skills.Religion
	case pb.Skill_SKILL_SLEIGHT_OF_HAND:
		return skills.SleightOfHand
	case pb.Skill_SKILL_STEALTH:
		return skills.Stealth
	case pb.Skill_SKILL_SURVIVAL:
		return skills.Survival
	default:
		return skills.Invalid
	}
}

// convertProtoLanguageToToolkit converts proto Language enum to toolkit Language type
func convertProtoLanguageToToolkit(language pb.Language) languages.Language {
	switch language {
	case pb.Language_LANGUAGE_COMMON:
		return languages.Common
	case pb.Language_LANGUAGE_DWARVISH:
		return languages.Dwarvish
	case pb.Language_LANGUAGE_ELVISH:
		return languages.Elvish
	case pb.Language_LANGUAGE_GIANT:
		return languages.Giant
	case pb.Language_LANGUAGE_GNOMISH:
		return languages.Gnomish
	case pb.Language_LANGUAGE_GOBLIN:
		return languages.Goblin
	case pb.Language_LANGUAGE_HALFLING:
		return languages.Halfling
	case pb.Language_LANGUAGE_ORC:
		return languages.Orc
	case pb.Language_LANGUAGE_ABYSSAL:
		return languages.Abyssal
	case pb.Language_LANGUAGE_CELESTIAL:
		return languages.Celestial
	case pb.Language_LANGUAGE_DRACONIC:
		return languages.Draconic
	case pb.Language_LANGUAGE_DEEP_SPEECH:
		return languages.DeepSpeech
	case pb.Language_LANGUAGE_INFERNAL:
		return languages.Infernal
	case pb.Language_LANGUAGE_PRIMORDIAL:
		return languages.Primordial
	case pb.Language_LANGUAGE_SYLVAN:
		return languages.Sylvan
	case pb.Language_LANGUAGE_UNDERCOMMON:
		return languages.Undercommon
	default:
		return languages.Invalid
	}
}

// convertProtoBackgroundToString converts proto Background enum to string
func convertProtoBackgroundToString(bg pb.Background) string {
	switch bg {
	case pb.Background_BACKGROUND_ACOLYTE:
		return "acolyte"
	case pb.Background_BACKGROUND_CHARLATAN:
		return "charlatan"
	case pb.Background_BACKGROUND_CRIMINAL:
		return "criminal"
	case pb.Background_BACKGROUND_ENTERTAINER:
		return "entertainer"
	case pb.Background_BACKGROUND_FOLK_HERO:
		return "folk-hero"
	case pb.Background_BACKGROUND_GUILD_ARTISAN:
		return "guild-artisan"
	case pb.Background_BACKGROUND_HERMIT:
		return "hermit"
	case pb.Background_BACKGROUND_NOBLE:
		return "noble"
	case pb.Background_BACKGROUND_OUTLANDER:
		return "outlander"
	case pb.Background_BACKGROUND_SAGE:
		return "sage"
	case pb.Background_BACKGROUND_SAILOR:
		return "sailor"
	case pb.Background_BACKGROUND_SOLDIER:
		return "soldier"
	case pb.Background_BACKGROUND_URCHIN:
		return "urchin"
	default:
		return ""
	}
}