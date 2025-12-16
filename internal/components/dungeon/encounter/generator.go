// Package encounter provides CR-scaled monster generation for dungeon rooms.
package encounter

import (
	"context"
	"fmt"

	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
)

// EncounterGenerator creates monster encounters for dungeon rooms
type EncounterGenerator interface {
	Generate(ctx context.Context, input *GenerateInput) (*GenerateOutput, error)
}

// GenerateInput defines parameters for encounter generation
type GenerateInput struct {
	// MonsterPool contains regular monsters from theme
	MonsterPool []dungeon.MonsterRef
	// BossPool contains boss monsters from theme
	BossPool []dungeon.MonsterRef
	// CRBudget is the allocated CR for this room
	CRBudget float64
	// IsBossRoom indicates if this is the final boss room
	IsBossRoom bool
	// Seed for deterministic random generation
	Seed int64
}

// GenerateOutput contains the generated encounter
type GenerateOutput struct {
	// Encounter contains the monster composition
	Encounter *dungeon.Encounter
}

// generator implements EncounterGenerator
type generator struct {
	selector MonsterSelector
}

// NewGenerator creates a new EncounterGenerator
func NewGenerator() EncounterGenerator {
	return &generator{
		selector: NewMonsterSelector(),
	}
}

// Generate creates a monster encounter based on the input parameters
func (g *generator) Generate(ctx context.Context, input *GenerateInput) (*GenerateOutput, error) {
	if input == nil {
		return nil, fmt.Errorf("input is required")
	}

	if input.CRBudget <= 0 {
		return nil, fmt.Errorf("CRBudget must be greater than 0")
	}

	if len(input.MonsterPool) == 0 {
		return nil, fmt.Errorf("MonsterPool cannot be empty")
	}

	if input.IsBossRoom && len(input.BossPool) == 0 {
		return nil, fmt.Errorf("BossPool cannot be empty for boss room")
	}

	// Select monsters based on CR budget
	selectOutput := g.selector.SelectMonsters(&SelectInput{
		Pool:     input.MonsterPool,
		BossPool: input.BossPool,
		Budget:   input.CRBudget,
		IsBoss:   input.IsBossRoom,
		Seed:     input.Seed,
	})

	return &GenerateOutput{
		Encounter: &dungeon.Encounter{
			Monsters: selectOutput.Monsters,
			TotalCR:  selectOutput.TotalCR,
		},
	}, nil
}
