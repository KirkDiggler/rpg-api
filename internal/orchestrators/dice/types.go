package dice

import (
	"time"

	dicesession "github.com/KirkDiggler/rpg-api/internal/repositories/dice_session"
)

// RollDiceInput defines the request for rolling dice
type RollDiceInput struct {
	EntityID    string
	Context     string
	Notation    string
	Description string
	TTL         time.Duration
}

// RollDiceOutput defines the response for rolling dice
type RollDiceOutput struct {
	Roll    *dicesession.DiceRoll
	Session *dicesession.DiceSession
}

// GetRollSessionInput defines the request for getting a roll session
type GetRollSessionInput struct {
	EntityID string
	Context  string
}

// GetRollSessionOutput defines the response for getting a roll session
type GetRollSessionOutput struct {
	Session *dicesession.DiceSession
}

// ClearRollSessionInput defines the request for clearing a roll session
type ClearRollSessionInput struct {
	EntityID string
	Context  string
}

// ClearRollSessionOutput defines the response for clearing a roll session
type ClearRollSessionOutput struct {
	RollsDeleted int32
}

// RollAbilityScoresInput defines the request for rolling ability scores for character creation
type RollAbilityScoresInput struct {
	EntityID string
	Method   string // "4d6_drop_lowest", "3d6", "point_buy", etc.
}

// RollAbilityScoresOutput defines the response for rolling ability scores
type RollAbilityScoresOutput struct {
	Rolls   []*dicesession.DiceRoll
	Session *dicesession.DiceSession
}
