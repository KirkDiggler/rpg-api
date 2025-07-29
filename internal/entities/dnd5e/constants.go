package dnd5e

import "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/constants"

// Race constants
const (
	RaceHuman      = "RACE_HUMAN"
	RaceDwarf      = "RACE_DWARF"
	RaceElf        = "RACE_ELF"
	RaceHalfling   = "RACE_HALFLING"
	RaceDragonborn = "RACE_DRAGONBORN"
	RaceGnome      = "RACE_GNOME"
	RaceHalfElf    = "RACE_HALF_ELF"
	RaceHalfOrc    = "RACE_HALF_ORC"
	RaceTiefling   = "RACE_TIEFLING"
)

// Subrace constants
const (
	SubraceHighElf           = "SUBRACE_HIGH_ELF"
	SubraceWoodElf           = "SUBRACE_WOOD_ELF"
	SubraceDarkElf           = "SUBRACE_DARK_ELF"
	SubraceHillDwarf         = "SUBRACE_HILL_DWARF"
	SubraceMountainDwarf     = "SUBRACE_MOUNTAIN_DWARF"
	SubraceLightfootHalfling = "SUBRACE_LIGHTFOOT_HALFLING"
	SubraceStoutHalfling     = "SUBRACE_STOUT_HALFLING"
	SubraceForestGnome       = "SUBRACE_FOREST_GNOME"
	SubraceRockGnome         = "SUBRACE_ROCK_GNOME"
)

// Class constants
const (
	ClassBarbarian = "CLASS_BARBARIAN"
	ClassBard      = "CLASS_BARD"
	ClassCleric    = "CLASS_CLERIC"
	ClassDruid     = "CLASS_DRUID"
	ClassFighter   = "CLASS_FIGHTER"
	ClassMonk      = "CLASS_MONK"
	ClassPaladin   = "CLASS_PALADIN"
	ClassRanger    = "CLASS_RANGER"
	ClassRogue     = "CLASS_ROGUE"
	ClassSorcerer  = "CLASS_SORCERER"
	ClassWarlock   = "CLASS_WARLOCK"
	ClassWizard    = "CLASS_WIZARD"
)

// Background constants
const (
	BackgroundAcolyte      = "BACKGROUND_ACOLYTE"
	BackgroundCharlatan    = "BACKGROUND_CHARLATAN"
	BackgroundCriminal     = "BACKGROUND_CRIMINAL"
	BackgroundEntertainer  = "BACKGROUND_ENTERTAINER"
	BackgroundFolkHero     = "BACKGROUND_FOLK_HERO"
	BackgroundGuildArtisan = "BACKGROUND_GUILD_ARTISAN"
	BackgroundHermit       = "BACKGROUND_HERMIT"
	BackgroundNoble        = "BACKGROUND_NOBLE"
	BackgroundOutlander    = "BACKGROUND_OUTLANDER"
	BackgroundSage         = "BACKGROUND_SAGE"
	BackgroundSailor       = "BACKGROUND_SAILOR"
	BackgroundSoldier      = "BACKGROUND_SOLDIER"
	BackgroundUrchin       = "BACKGROUND_URCHIN"
)

// Alignment constants
const (
	AlignmentLawfulGood     = "ALIGNMENT_LAWFUL_GOOD"
	AlignmentNeutralGood    = "ALIGNMENT_NEUTRAL_GOOD"
	AlignmentChaoticGood    = "ALIGNMENT_CHAOTIC_GOOD"
	AlignmentLawfulNeutral  = "ALIGNMENT_LAWFUL_NEUTRAL"
	AlignmentTrueNeutral    = "ALIGNMENT_TRUE_NEUTRAL"
	AlignmentChaoticNeutral = "ALIGNMENT_CHAOTIC_NEUTRAL"
	AlignmentLawfulEvil     = "ALIGNMENT_LAWFUL_EVIL"
	AlignmentNeutralEvil    = "ALIGNMENT_NEUTRAL_EVIL"
	AlignmentChaoticEvil    = "ALIGNMENT_CHAOTIC_EVIL"
)

// Ability constants - use toolkit constants directly
const (
	AbilityStrength     = string(constants.STR) // "str"
	AbilityDexterity    = string(constants.DEX) // "dex"
	AbilityConstitution = string(constants.CON) // "con"
	AbilityIntelligence = string(constants.INT) // "int"
	AbilityWisdom       = string(constants.WIS) // "wis"
	AbilityCharisma     = string(constants.CHA) // "cha"
)

// Ability score map keys for JSON serialization
const (
	AbilityKeyStrength     = "Strength"
	AbilityKeyDexterity    = "Dexterity"
	AbilityKeyConstitution = "Constitution"
	AbilityKeyIntelligence = "Intelligence"
	AbilityKeyWisdom       = "Wisdom"
	AbilityKeyCharisma     = "Charisma"
)

// Skill constants - use toolkit constants directly
const (
	SkillAcrobatics     = string(constants.SkillAcrobatics)     // "acrobatics"
	SkillAnimalHandling = string(constants.SkillAnimalHandling) // "animal-handling"
	SkillArcana         = string(constants.SkillArcana)         // "arcana"
	SkillAthletics      = string(constants.SkillAthletics)      // "athletics"
	SkillDeception      = string(constants.SkillDeception)      // "deception"
	SkillHistory        = string(constants.SkillHistory)        // "history"
	SkillInsight        = string(constants.SkillInsight)        // "insight"
	SkillIntimidation   = string(constants.SkillIntimidation)   // "intimidation"
	SkillInvestigation  = string(constants.SkillInvestigation)  // "investigation"
	SkillMedicine       = string(constants.SkillMedicine)       // "medicine"
	SkillNature         = string(constants.SkillNature)         // "nature"
	SkillPerception     = string(constants.SkillPerception)     // "perception"
	SkillPerformance    = string(constants.SkillPerformance)    // "performance"
	SkillPersuasion     = string(constants.SkillPersuasion)     // "persuasion"
	SkillReligion       = string(constants.SkillReligion)       // "religion"
	SkillSleightOfHand  = string(constants.SkillSleightOfHand)  // "sleight-of-hand"
	SkillStealth        = string(constants.SkillStealth)        // "stealth"
	SkillSurvival       = string(constants.SkillSurvival)       // "survival"
)

// Language constants - use toolkit constants directly
const (
	LanguageCommon      = string(constants.LanguageCommon)      // "common"
	LanguageDwarvish    = string(constants.LanguageDwarvish)    // "dwarvish"
	LanguageElvish      = string(constants.LanguageElvish)      // "elvish"
	LanguageGiant       = string(constants.LanguageGiant)       // "giant"
	LanguageGnomish     = string(constants.LanguageGnomish)     // "gnomish"
	LanguageGoblin      = string(constants.LanguageGoblin)      // "goblin"
	LanguageHalfling    = string(constants.LanguageHalfling)    // "halfling"
	LanguageOrc         = string(constants.LanguageOrc)         // "orc"
	LanguageAbyssal     = string(constants.LanguageAbyssal)     // "abyssal"
	LanguageCelestial   = string(constants.LanguageCelestial)   // "celestial"
	LanguageDraconic    = string(constants.LanguageDraconic)    // "draconic"
	LanguageDeepSpeech  = string(constants.LanguageDeepSpeech)  // "deep speech"
	LanguageInfernal    = string(constants.LanguageInfernal)    // "infernal"
	LanguagePrimordial  = string(constants.LanguagePrimordial)  // "primordial"
	LanguageSylvan      = string(constants.LanguageSylvan)      // "sylvan"
	LanguageUndercommon = string(constants.LanguageUndercommon) // "undercommon"
)

// Creation step constants
const (
	CreationStepName          = "CREATION_STEP_NAME"
	CreationStepRace          = "CREATION_STEP_RACE"
	CreationStepClass         = "CREATION_STEP_CLASS"
	CreationStepBackground    = "CREATION_STEP_BACKGROUND"
	CreationStepAbilityScores = "CREATION_STEP_ABILITY_SCORES"
	CreationStepSkills        = "CREATION_STEP_SKILLS"
	CreationStepLanguages     = "CREATION_STEP_LANGUAGES"
	CreationStepReview        = "CREATION_STEP_REVIEW"
)
