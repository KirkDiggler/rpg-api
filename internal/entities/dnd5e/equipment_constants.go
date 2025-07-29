package dnd5e

// Weapon category constants
const (
	// WeaponCategorySimple represents simple weapons
	WeaponCategorySimple = "simple"
	// WeaponCategoryMartial represents martial weapons
	WeaponCategoryMartial = "martial"
)

// Weapon property constants
const (
	// WeaponPropertyTwoHanded requires two hands to use
	WeaponPropertyTwoHanded = "two-handed"
	// WeaponPropertyVersatile can be used with one or two hands
	WeaponPropertyVersatile = "versatile"
	// WeaponPropertyFinesse allows DEX modifier for attack and damage
	WeaponPropertyFinesse = "finesse"
	// WeaponPropertyLight is a light weapon
	WeaponPropertyLight = "light"
	// WeaponPropertyHeavy is a heavy weapon
	WeaponPropertyHeavy = "heavy"
	// WeaponPropertyReach adds 5 feet to reach
	WeaponPropertyReach = "reach"
	// WeaponPropertyThrown can be thrown
	WeaponPropertyThrown = "thrown"
	// WeaponPropertyAmmunition uses ammunition
	WeaponPropertyAmmunition = "ammunition"
	// WeaponPropertyLoading requires an action to load
	WeaponPropertyLoading = "loading"
	// WeaponPropertySpecial has special rules
	WeaponPropertySpecial = "special"
	// WeaponPropertySilvered is silvered for overcoming resistances
	WeaponPropertySilvered = "silvered"
	// WeaponPropertyMagic indicates a magic item
	WeaponPropertyMagic = "magic"
	// WeaponPropertyRequiresAttunement requires attunement to use
	WeaponPropertyRequiresAttunement = "requires-attunement"
)

// Armor category constants
const (
	// ArmorCategoryLight represents light armor
	ArmorCategoryLight = "light"
	// ArmorCategoryMedium represents medium armor
	ArmorCategoryMedium = "medium"
	// ArmorCategoryHeavy represents heavy armor
	ArmorCategoryHeavy = "heavy"
	// ArmorCategoryShield represents shields
	ArmorCategoryShield = "shield"
)

// Proficiency constants for equipment
const (
	// ProficiencySimpleWeapons for simple weapon proficiency
	ProficiencySimpleWeapons = "simple_weapons"
	// ProficiencyMartialWeapons for martial weapon proficiency
	ProficiencyMartialWeapons = "martial_weapons"
	// ProficiencyLightArmor for light armor proficiency
	ProficiencyLightArmor = "light_armor"
	// ProficiencyMediumArmor for medium armor proficiency
	ProficiencyMediumArmor = "medium_armor"
	// ProficiencyHeavyArmor for heavy armor proficiency
	ProficiencyHeavyArmor = "heavy_armor"
	// ProficiencyShields for shield proficiency
	ProficiencyShields = "shields"
)

// Weapon range types
const (
	// WeaponRangeMelee for melee weapons
	WeaponRangeMelee = "melee"
	// WeaponRangeRanged for ranged weapons
	WeaponRangeRanged = "ranged"
)

// Damage type constants
const (
	// DamageTypeSlashing for slashing damage
	DamageTypeSlashing = "slashing"
	// DamageTypePiercing for piercing damage
	DamageTypePiercing = "piercing"
	// DamageTypeBludgeoning for bludgeoning damage
	DamageTypeBludgeoning = "bludgeoning"
	// DamageTypeFire for fire damage
	DamageTypeFire = "fire"
	// DamageTypeCold for cold damage
	DamageTypeCold = "cold"
	// DamageTypeLightning for lightning damage
	DamageTypeLightning = "lightning"
	// DamageTypeThunder for thunder damage
	DamageTypeThunder = "thunder"
	// DamageTypePoison for poison damage
	DamageTypePoison = "poison"
	// DamageTypeAcid for acid damage
	DamageTypeAcid = "acid"
	// DamageTypePsychic for psychic damage
	DamageTypePsychic = "psychic"
	// DamageTypeNecrotic for necrotic damage
	DamageTypeNecrotic = "necrotic"
	// DamageTypeRadiant for radiant damage
	DamageTypeRadiant = "radiant"
	// DamageTypeForce for force damage
	DamageTypeForce = "force"
)
