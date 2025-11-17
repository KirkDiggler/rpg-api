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
	Hit             bool // Did the attack hit?
	Critical        bool // Was it a critical hit?
	IsNaturalTwenty bool // Natural 20
	IsNaturalOne    bool // Natural 1

	// Damage details
	DamageRolls []int  // Individual damage dice rolls
	DamageBonus int    // Total damage bonus
	TotalDamage int    // Final damage dealt
	DamageType  string // Type of damage (slashing, piercing, etc.)
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
