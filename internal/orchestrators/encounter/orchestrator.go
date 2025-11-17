// Package encounter provides orchestration for D&D 5e encounter management and combat resolution.
package encounter

import (
	"context"
	"fmt"
	"time"

	"github.com/KirkDiggler/rpg-toolkit/dice"
	"github.com/KirkDiggler/rpg-toolkit/events"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/combat"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/weapons"

	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	encounterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/encounters"
)

// Config holds orchestrator dependencies
type Config struct {
	CharacterRepo characterrepo.Repository
	EncounterRepo encounterrepo.Repository
}

// Validate ensures all required dependencies are present
func (c *Config) Validate() error {
	if c.CharacterRepo == nil {
		return fmt.Errorf("CharacterRepo is required")
	}
	if c.EncounterRepo == nil {
		return fmt.Errorf("EncounterRepo is required")
	}
	return nil
}

// Orchestrator implements the Service interface
type Orchestrator struct {
	charRepo characterrepo.Repository
	encRepo  encounterrepo.Repository
}

// New creates a new encounter orchestrator
func New(cfg *Config) (*Orchestrator, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Orchestrator{
		charRepo: cfg.CharacterRepo,
		encRepo:  cfg.EncounterRepo,
	}, nil
}

// ResolveAttack implements the Service interface
func (o *Orchestrator) ResolveAttack(ctx context.Context, input *ResolveAttackInput) (*ResolveAttackOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}
	if input.EncounterID == "" {
		return nil, fmt.Errorf("encounter ID is required")
	}
	if input.AttackerID == "" {
		return nil, fmt.Errorf("attacker ID is required")
	}
	if input.TargetID == "" {
		return nil, fmt.Errorf("target ID is required")
	}

	// 1. Create EventBus (critical for Rage and other features)
	bus := events.NewEventBus()

	// 2. Load character data from repository
	charOutput, err := o.charRepo.Get(ctx, characterrepo.GetInput{
		ID: input.AttackerID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to load character: %w", err)
	}

	// 3. Load Character from Data (reconstructs features, subscribes to events)
	char, err := character.LoadFromData(ctx, charOutput.CharacterData, bus)
	if err != nil {
		return nil, fmt.Errorf("failed to load character from data: %w", err)
	}

	// 4. Load encounter state
	encOutput, err := o.encRepo.Get(ctx, &encounterrepo.GetInput{
		EncounterID: input.EncounterID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to load encounter: %w", err)
	}

	// Verify encounter exists (even if we don't use its data yet)
	if encOutput.Data == nil {
		return nil, fmt.Errorf("encounter not found: %s", input.EncounterID)
	}

	// 5. Create monster (Phase 2: use NewGoblin factory)
	// TODO: Load monster state from encounter data in future
	goblin := monster.NewGoblin(input.TargetID)

	// 6. Get weapon (Phase 2: use greataxe)
	// TODO: Load from character equipment or use input.WeaponID
	weapon, err := weapons.GetByID(weapons.Greataxe)
	if err != nil {
		return nil, fmt.Errorf("failed to get weapon: %w", err)
	}

	// 7. Call toolkit combat (event-driven, Rage participates here!)
	result, err := combat.ResolveAttack(ctx, &combat.AttackInput{
		Attacker:         char,
		Defender:         goblin,
		Weapon:           &weapon,
		AttackerScores:   charOutput.CharacterData.AbilityScores,
		DefenderAC:       goblin.AC(),
		ProficiencyBonus: char.GetProficiencyBonus(),
		EventBus:         bus,
		Roller:           dice.NewRoller(),
	})
	if err != nil {
		return nil, fmt.Errorf("combat resolution failed: %w", err)
	}

	// 8. Calculate new monster HP
	newHP := goblin.HP()
	if result.Hit {
		newHP = goblin.HP() - result.TotalDamage
		if newHP < 0 {
			newHP = 0
		}
		// TODO: Persist updated HP to encounter repository
	}

	// 9. Convert toolkit result to our output format
	return &ResolveAttackOutput{
		Result: &AttackResult{
			AttackRoll:      result.AttackRoll,
			AttackBonus:     result.AttackBonus,
			TotalAttack:     result.TotalAttack,
			Hit:             result.Hit,
			Critical:        result.Critical,
			IsNaturalTwenty: result.IsNaturalTwenty,
			IsNaturalOne:    result.IsNaturalOne,
			DamageRolls:     result.DamageRolls,
			DamageBonus:     result.DamageBonus,
			TotalDamage:     result.TotalDamage,
			DamageType:      result.DamageType,
		},
		MonsterHP:   newHP,
		MonsterDead: newHP <= 0,
	}, nil
}

// CreateDungeon creates a new encounter and returns the encounter ID
// Phase 2: Minimal implementation - just encounter ID, no room
func (o *Orchestrator) CreateDungeon(ctx context.Context, input *CreateDungeonInput) (*CreateDungeonOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}

	// Generate unique encounter ID using timestamp
	encounterID := fmt.Sprintf("enc-%d", time.Now().UnixNano())

	// Save minimal encounter data (Phase 2)
	// TODO Phase 3: Add RoomData, InitiativeData, etc.
	_, err := o.encRepo.Save(ctx, &encounterrepo.SaveInput{
		EncounterID: encounterID,
		// RoomData, InitiativeData, InitiativeRolls are nil for Phase 2
	})
	if err != nil {
		return nil, fmt.Errorf("failed to save encounter: %w", err)
	}

	// Return encounter ID
	return &CreateDungeonOutput{
		EncounterID: encounterID,
	}, nil
}
