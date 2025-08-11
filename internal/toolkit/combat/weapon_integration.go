package combat

// WeaponData represents weapon information from external sources
// This allows us to integrate with dnd5e-api or other data sources
type WeaponData struct {
	ID           string
	Name         string
	DamageDice   string   // e.g., "1d8", "2d6"
	DamageType   string   // e.g., "slashing", "piercing", "bludgeoning"
	Properties   []string // e.g., ["finesse", "versatile", "light"]
	VersatileDice string  // For versatile weapons when used two-handed
	Range        struct {
		Normal int
		Long   int
	}
	Thrown bool
	Reach  int // in feet, default 5
}

// SpellData represents spell information for spell attacks
type SpellData struct {
	ID             string
	Name           string
	Level          int    // 0 for cantrips
	DamageDice     string // e.g., "1d10" for fire bolt
	DamageType     string // e.g., "fire", "cold", "necrotic"
	SaveType       string // e.g., "dexterity", "wisdom" - empty for attack spells
	AttackType     string // "melee" or "ranged" for spell attacks
	Range          int    // in feet
	ScalesWithLevel bool   // For cantrips that scale
}

// WeaponProvider is an interface for fetching weapon data
// This can be implemented by different data sources
type WeaponProvider interface {
	GetWeapon(weaponID string) (*WeaponData, error)
}

// SpellProvider is an interface for fetching spell data
type SpellProvider interface {
	GetSpell(spellID string) (*SpellData, error)
}

// EnhancedResolver extends the basic Resolver with weapon/spell integration
type EnhancedResolver struct {
	*Resolver
	WeaponProvider WeaponProvider
	SpellProvider  SpellProvider
}

// ResolveAttackWithWeapon resolves an attack using actual weapon data
func (er *EnhancedResolver) ResolveAttackWithWeapon(input *AttackInput, weapon *WeaponData) (*AttackOutput, error) {
	// This is where we'd integrate real weapon mechanics
	// For now, this is a placeholder showing the structure
	
	// Check for finesse weapons (can use DEX instead of STR)
	useFinesse := false
	for _, prop := range weapon.Properties {
		if prop == "finesse" {
			useFinesse = true
			break
		}
	}
	
	// Determine ability modifier
	// TODO: Use this to calculate attack bonus and damage
	_ = useFinesse // Mark as used for now
	
	// TODO: Complete implementation with weapon data
	// For now, delegate to base resolver
	return er.Resolver.ResolveAttack(input)
}

// ConditionEffect represents a status effect or condition
type ConditionEffect struct {
	Name        string
	Duration    int    // rounds
	SaveType    string // ability save required
	SaveDC      int
	Effects     map[string]interface{} // flexible effects
}

// Common conditions in D&D 5e
const (
	ConditionBlinded       = "blinded"
	ConditionCharmed       = "charmed"
	ConditionDeafened      = "deafened"
	ConditionFrightened    = "frightened"
	ConditionGrappled      = "grappled"
	ConditionIncapacitated = "incapacitated"
	ConditionInvisible     = "invisible"
	ConditionParalyzed     = "paralyzed"
	ConditionPetrified     = "petrified"
	ConditionPoisoned      = "poisoned"
	ConditionProne         = "prone"
	ConditionRestrained    = "restrained"
	ConditionStunned       = "stunned"
	ConditionUnconscious   = "unconscious"
)

// ApplyCondition applies a condition to an entity
// This is a stub for future implementation
func (er *EnhancedResolver) ApplyCondition(entityID string, condition string, duration int) error {
	// TODO: Implement condition tracking
	return nil
}