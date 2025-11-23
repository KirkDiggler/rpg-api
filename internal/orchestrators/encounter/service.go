package encounter

import (
	"context"
)

//go:generate mockgen -destination=mock/mock_service.go -package=encountermock github.com/KirkDiggler/rpg-api/internal/orchestrators/encounter Service

// Service defines the encounter orchestrator interface
type Service interface {
	// ResolveAttack handles a combat attack action
	ResolveAttack(ctx context.Context, input *ResolveAttackInput) (*ResolveAttackOutput, error)

	// CreateDungeon starts a new dungeon encounter
	CreateDungeon(ctx context.Context, input *CreateDungeonInput) (*CreateDungeonOutput, error)

	// MoveCharacter handles character movement in the encounter
	MoveCharacter(ctx context.Context, input *MoveCharacterInput) (*MoveCharacterOutput, error)
}

// ResolveAttackInput contains attack parameters
type ResolveAttackInput struct {
	EncounterID string
	AttackerID  string
	TargetID    string
	WeaponID    string // Optional, uses default weapon if empty
}

// ResolveAttackOutput returns attack results
// TODO: Replace AttackResult with toolkit combat.AttackResult when combat package is published
type ResolveAttackOutput struct {
	Result      *AttackResult // Attack result with full breakdown
	MonsterHP   int           // Updated monster HP
	MonsterDead bool          // Whether monster was defeated
}

// AttackResult contains the outcome of an attack
// TODO: Replace with toolkit combat.AttackResult when available
// This mirrors the toolkit type structure for compatibility
type AttackResult struct {
	// Attack roll details
	AttackRoll      int  // The d20 roll
	AttackBonus     int  // Total bonus applied
	TotalAttack     int  // Roll + bonus
	TargetAC        int  // Target's armor class
	Hit             bool // Did the attack hit?
	Critical        bool // Was it a critical hit?
	IsNaturalTwenty bool // Natural 20
	IsNaturalOne    bool // Natural 1

	// Damage details
	DamageRolls []int  // Individual damage dice rolls
	DamageBonus int    // Total damage bonus
	TotalDamage int    // Final damage dealt
	DamageType  string // Type of damage (slashing, piercing, etc.)

	// Detailed breakdown
	Breakdown *DamageBreakdown // Detailed damage breakdown (nil if attack missed)
}

// DamageBreakdown provides detailed component breakdown of damage calculation
type DamageBreakdown struct {
	Components  []DamageComponent
	AbilityUsed string // Ability used for attack ("STR", "DEX", etc.)
	TotalDamage int    // Sum of all components
}

// DamageComponent represents damage from one source
type DamageComponent struct {
	Source            string        // Type of damage source ("weapon", "ability", "rage", etc.)
	OriginalDiceRolls []int         // Dice values as first rolled
	FinalDiceRolls    []int         // Dice values after all rerolls
	Rerolls           []RerollEvent // History of rerolls
	FlatBonus         int           // Flat modifier (0 if none)
	DamageType        string        // "slashing", "fire", "radiant", etc.
	IsCritical        bool          // Was this component doubled for crit?
}

// RerollEvent tracks a single die reroll
type RerollEvent struct {
	DieIndex int    // Which die was rerolled (0-based index in original_dice_rolls)
	Before   int    // Value before reroll
	After    int    // Value after reroll
	Reason   string // Feature that caused reroll (e.g., "great_weapon_fighting")
}

// CreateDungeonInput contains parameters for creating a dungeon encounter
// Phase 2: Minimal - just tracking who starts the dungeon
type CreateDungeonInput struct {
	PlayerID string // Optional - ID of player starting the dungeon
}

// CreateDungeonOutput returns the created encounter details
// Phase 2: Minimal - just encounter_id, no room yet
type CreateDungeonOutput struct {
	EncounterID string // ID of the created encounter
	// TODO Phase 3: Add Room when implementing spatial
}

// MoveCharacterInput contains movement parameters
// Phase 2: Simple movement to a single target position
type MoveCharacterInput struct {
	EncounterID    string    // ID of the encounter
	EntityID       string    // ID of entity being moved
	TargetPosition *Position // Target position to move to
}

// Position represents a 2D position in the room
// This mirrors spatial.Position for handler layer use
type Position struct {
	X float64
	Y float64
}

// MoveCharacterOutput returns movement results
type MoveCharacterOutput struct {
	Success           bool        // Whether the movement succeeded
	FinalPosition     *Position   // Final position of the entity
	MovementRemaining int32       // Movement points remaining (Phase 3)
	StopReason        string      // Why movement stopped ("completed", "position_occupied", "out_of_bounds", "entity_not_found")
	UpdatedRoom       interface{} // Updated room data (using interface{} until spatial is fixed)
}
