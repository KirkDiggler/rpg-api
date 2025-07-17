// Package dice implements the dice orchestrator for handling dice roll sessions
package dice

//go:generate mockgen -destination=mock/mock_service.go -package=dicemock github.com/KirkDiggler/rpg-api/internal/orchestrators/dice Service

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/KirkDiggler/rpg-toolkit/dice"

	"github.com/KirkDiggler/rpg-api/internal/errors"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	"github.com/KirkDiggler/rpg-api/internal/repositories/dice_session"
)

const (
	// Default context for ability score rolling
	ContextAbilityScores = "ability_scores"

	// Default TTL for dice sessions
	DefaultSessionTTL = 15 * time.Minute

	// Dice rolling methods
	MethodStandard = "4d6_drop_lowest"
	MethodClassic  = "3d6"
	MethodHeroic   = "4d6_reroll_1s"
	MethodPointBuy = "point_buy"

	// Standard ability score dice notation
	AbilityScoreNotation = "4d6"
)

var (
	// Regex for parsing simple dice notation like "2d6", "1d20", "3d8"
	diceNotationRegex = regexp.MustCompile(`^(\d+)d(\d+)$`)
)

// Service defines the interface for dice operations
type Service interface {
	// Generic dice rolling
	RollDice(ctx context.Context, input *RollDiceInput) (*RollDiceOutput, error)
	GetRollSession(ctx context.Context, input *GetRollSessionInput) (*GetRollSessionOutput, error)
	ClearRollSession(ctx context.Context, input *ClearRollSessionInput) (*ClearRollSessionOutput, error)

	// Specialized ability score rolling for character creation
	RollAbilityScores(ctx context.Context, input *RollAbilityScoresInput) (*RollAbilityScoresOutput, error)
}

// Config holds the dependencies for the dice orchestrator
type Config struct {
	DiceSessionRepo dice_session.Repository
	IDGenerator     idgen.Generator
}

// Validate ensures all required dependencies are provided
func (c *Config) Validate() error {
	vb := errors.NewValidationBuilder()

	if c.DiceSessionRepo == nil {
		vb.RequiredField("DiceSessionRepo")
	}
	if c.IDGenerator == nil {
		vb.RequiredField("IDGenerator")
	}

	return vb.Build()
}

type orchestrator struct {
	diceSessionRepo dice_session.Repository
	idGen           idgen.Generator
}

// NewOrchestrator creates a new dice orchestrator with the provided dependencies
func NewOrchestrator(cfg *Config) (Service, error) {
	if err := cfg.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid config")
	}

	return &orchestrator{
		diceSessionRepo: cfg.DiceSessionRepo,
		idGen:           cfg.IDGenerator,
	}, nil
}

// parseDiceNotation parses simple dice notation like "2d6" and returns count and size
func (o *orchestrator) parseDiceNotation(notation string) (count, size int, err error) {
	matches := diceNotationRegex.FindStringSubmatch(strings.ToLower(notation))
	if len(matches) != 3 {
		return 0, 0, errors.InvalidArgumentf("invalid dice notation: %s (expected format: XdY)", notation)
	}

	count, err = strconv.Atoi(matches[1])
	if err != nil {
		return 0, 0, errors.InvalidArgumentf("invalid dice count in notation: %s", notation)
	}

	size, err = strconv.Atoi(matches[2])
	if err != nil {
		return 0, 0, errors.InvalidArgumentf("invalid die size in notation: %s", notation)
	}

	if count <= 0 || size <= 0 {
		return 0, 0, errors.InvalidArgumentf("dice count and size must be positive: %s", notation)
	}

	return count, size, nil
}

// rollDiceWithToolkit uses rpg-toolkit to roll dice and returns individual results
func (o *orchestrator) rollDiceWithToolkit(count, size int, dropLowest int) ([]int32, []int32, int32, error) {
	// Use rpg-toolkit to create the dice roll
	roll, err := dice.NewRoll(count, size)
	if err != nil {
		return nil, nil, 0, errors.Wrapf(err, "failed to create dice roll")
	}

	// Get the total and description
	total := roll.GetValue()
	description := roll.GetDescription()

	// Parse individual dice values from the description
	// Description format: "+2d6[3,4]=7"
	var individualDice []int32

	// Extract dice values from description using regex
	// This is a bit hacky but rpg-toolkit doesn't expose individual dice values directly
	start := strings.Index(description, "[")
	end := strings.Index(description, "]")
	if start >= 0 && end > start {
		diceStr := description[start+1 : end]
		diceStrings := strings.Split(diceStr, ",")
		for _, ds := range diceStrings {
			if d, err := strconv.Atoi(strings.TrimSpace(ds)); err == nil {
				individualDice = append(individualDice, int32(d))
			}
		}
	}

	// Handle drop lowest logic if needed
	var dropped []int32
	if dropLowest > 0 && len(individualDice) > dropLowest {
		// Sort to find lowest dice
		sorted := make([]int32, len(individualDice))
		copy(sorted, individualDice)

		// Simple bubble sort to find lowest
		for i := 0; i < dropLowest; i++ {
			minIdx := i
			for j := i + 1; j < len(sorted); j++ {
				if sorted[j] < sorted[minIdx] {
					minIdx = j
				}
			}
			if minIdx != i {
				sorted[i], sorted[minIdx] = sorted[minIdx], sorted[i]
			}
		}

		// Extract dropped dice and recalculate total
		dropped = sorted[:dropLowest]
		kept := sorted[dropLowest:]

		newTotal := int32(0)
		for _, d := range kept {
			newTotal += d
		}

		return kept, dropped, newTotal, nil
	}

	return individualDice, dropped, int32(total), nil
}

// RollDice rolls dice using the specified notation and stores the result in a session
func (o *orchestrator) RollDice(ctx context.Context, input *RollDiceInput) (*RollDiceOutput, error) {
	if input.EntityID == "" {
		return nil, errors.InvalidArgument("entity ID is required")
	}
	if input.Context == "" {
		return nil, errors.InvalidArgument("context is required")
	}
	if input.Notation == "" {
		return nil, errors.InvalidArgument("dice notation is required")
	}

	// Parse the dice notation
	count, size, err := o.parseDiceNotation(input.Notation)
	if err != nil {
		return nil, err
	}

	// Roll the dice using rpg-toolkit
	individualDice, dropped, total, err := o.rollDiceWithToolkit(count, size, 0)
	if err != nil {
		return nil, errors.Wrap(err, "failed to roll dice")
	}

	// Create the dice roll
	roll := &dice_session.DiceRoll{
		RollID:      o.idGen.Generate(),
		Notation:    input.Notation,
		Dice:        individualDice,
		Total:       total,
		Dropped:     dropped,
		Description: input.Description,
		DiceTotal:   total, // Same as total since no modifiers in basic notation
		Modifier:    0,     // No modifiers in basic notation
	}

	// Try to get existing session first
	getOutput, err := o.diceSessionRepo.Get(ctx, dice_session.GetInput{
		EntityID: input.EntityID,
		Context:  input.Context,
	})

	var session *dice_session.DiceSession
	if err != nil {
		if !errors.IsNotFound(err) {
			return nil, errors.Wrap(err, "failed to check for existing session")
		}

		// No existing session, create a new one
		ttl := input.TTL
		if ttl == 0 {
			ttl = DefaultSessionTTL
		}

		createOutput, err := o.diceSessionRepo.Create(ctx, dice_session.CreateInput{
			EntityID: input.EntityID,
			Context:  input.Context,
			Rolls:    []dice_session.DiceRoll{*roll},
			TTL:      ttl,
		})
		if err != nil {
			return nil, errors.Wrap(err, "failed to create dice session")
		}
		session = createOutput.Session
	} else {
		// Add roll to existing session
		session = getOutput.Session
		session.Rolls = append(session.Rolls, *roll)

		// Update the session
		if err := o.diceSessionRepo.Update(ctx, session); err != nil {
			return nil, errors.Wrap(err, "failed to update dice session")
		}
	}

	slog.Info("Dice rolled successfully",
		"entity_id", input.EntityID,
		"context", input.Context,
		"notation", input.Notation,
		"total", roll.Total,
		"roll_id", roll.RollID,
	)

	return &RollDiceOutput{
		Roll:    roll,
		Session: session,
	}, nil
}

// GetRollSession retrieves an existing dice roll session
func (o *orchestrator) GetRollSession(ctx context.Context, input *GetRollSessionInput) (*GetRollSessionOutput, error) {
	if input.EntityID == "" {
		return nil, errors.InvalidArgument("entity ID is required")
	}
	if input.Context == "" {
		return nil, errors.InvalidArgument("context is required")
	}

	getOutput, err := o.diceSessionRepo.Get(ctx, dice_session.GetInput{
		EntityID: input.EntityID,
		Context:  input.Context,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get dice session")
	}

	return &GetRollSessionOutput{
		Session: getOutput.Session,
	}, nil
}

// ClearRollSession removes a dice roll session
func (o *orchestrator) ClearRollSession(ctx context.Context, input *ClearRollSessionInput) (*ClearRollSessionOutput, error) {
	if input.EntityID == "" {
		return nil, errors.InvalidArgument("entity ID is required")
	}
	if input.Context == "" {
		return nil, errors.InvalidArgument("context is required")
	}

	deleteOutput, err := o.diceSessionRepo.Delete(ctx, dice_session.DeleteInput{
		EntityID: input.EntityID,
		Context:  input.Context,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to delete dice session")
	}

	slog.Info("Dice session cleared",
		"entity_id", input.EntityID,
		"context", input.Context,
		"rolls_deleted", deleteOutput.RollsDeleted,
	)

	return &ClearRollSessionOutput{
		RollsDeleted: deleteOutput.RollsDeleted,
	}, nil
}

// RollAbilityScores handles specialized ability score rolling for D&D character creation
func (o *orchestrator) RollAbilityScores(ctx context.Context, input *RollAbilityScoresInput) (*RollAbilityScoresOutput, error) {
	if input.EntityID == "" {
		return nil, errors.InvalidArgument("entity ID is required")
	}
	if input.Method == "" {
		input.Method = MethodStandard // Default to 4d6 drop lowest
	}

	// Validate the method
	notation := ""
	dropLowest := false
	switch input.Method {
	case MethodStandard:
		notation = AbilityScoreNotation
		dropLowest = true
	case MethodClassic:
		notation = "3d6"
	case MethodHeroic:
		notation = "4d6r1" // Reroll 1s
	default:
		return nil, errors.InvalidArgumentf("unsupported rolling method: %s", input.Method)
	}

	// Parse the dice notation for ability scores
	count, size, err := o.parseDiceNotation(notation)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse ability score notation")
	}

	// Determine drop lowest count
	dropLowestCount := 0
	if dropLowest {
		dropLowestCount = 1
	}

	// Roll 6 sets of ability scores
	var rolls []*dice_session.DiceRoll
	for i := 0; i < 6; i++ {
		// Roll the dice using rpg-toolkit
		individualDice, droppedDice, total, err := o.rollDiceWithToolkit(count, size, dropLowestCount)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to roll ability score %d", i+1)
		}

		roll := &dice_session.DiceRoll{
			RollID:      o.idGen.Generate(),
			Notation:    notation,
			Dice:        individualDice,
			Total:       total,
			Dropped:     droppedDice,
			Description: fmt.Sprintf("Ability Score %d (%s)", i+1, input.Method),
			DiceTotal:   total, // Same as total since no modifiers
			Modifier:    0,     // No modifiers for ability scores
		}

		rolls = append(rolls, roll)
	}

	// Convert to slice of values for the repository
	rollValues := make([]dice_session.DiceRoll, len(rolls))
	for i, roll := range rolls {
		rollValues[i] = *roll
	}

	// Store in session with ability scores context
	createOutput, err := o.diceSessionRepo.Create(ctx, dice_session.CreateInput{
		EntityID: input.EntityID,
		Context:  ContextAbilityScores,
		Rolls:    rollValues,
		TTL:      DefaultSessionTTL,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to create ability score session")
	}

	slog.Info("Ability scores rolled successfully",
		"entity_id", input.EntityID,
		"method", input.Method,
		"rolls_count", len(rolls),
	)

	return &RollAbilityScoresOutput{
		Rolls:   rolls,
		Session: createOutput.Session,
	}, nil
}
