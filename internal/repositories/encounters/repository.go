package encounters

//go:generate mockgen -destination=mock/mock_repository.go -package=encountermock github.com/KirkDiggler/rpg-api/internal/repositories/encounters Repository

import (
	"context"
	"crypto/rand"
	"math/big"
	"time"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/initiative"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster"
	// "github.com/KirkDiggler/rpg-toolkit/tools/spatial" // Temporarily disabled
)

// EncounterState represents the current state of an encounter
type EncounterState string

const (
	// StateWaiting indicates the encounter is in lobby/waiting for players
	StateWaiting EncounterState = "waiting"
	// StateActive indicates combat is active
	StateActive EncounterState = "active"
	// StatePaused indicates combat is paused (e.g., disconnection)
	StatePaused EncounterState = "paused"
	// StateCompleted indicates the encounter is finished (victory/TPK)
	StateCompleted EncounterState = "completed"
)

// Player represents a player in an encounter
type Player struct {
	PlayerID    string
	CharacterID string
	IsReady     bool
	IsConnected bool
	JoinedAt    time.Time
}

// GenerateJoinCode creates a 6-character alphanumeric join code
// Excludes confusing characters: I, l, O, 0, 1
func GenerateJoinCode() string {
	const chars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"
	const codeLength = 6

	code := make([]byte, codeLength)
	maxValue := big.NewInt(int64(len(chars)))

	for i := range code {
		n, err := rand.Int(rand.Reader, maxValue)
		if err != nil {
			// Fallback to a simpler method if crypto/rand fails
			// This should never happen in practice
			n = big.NewInt(time.Now().UnixNano() % int64(len(chars)))
		}
		code[i] = chars[n.Int64()]
	}
	return string(code)
}

// Repository defines the storage interface for encounters
type Repository interface {
	// Save stores an encounter
	Save(ctx context.Context, input *SaveInput) (*SaveOutput, error)

	// Get retrieves an encounter by ID
	Get(ctx context.Context, input *GetInput) (*GetOutput, error)

	// GetByJoinCode retrieves an encounter by its join code
	GetByJoinCode(ctx context.Context, input *GetByJoinCodeInput) (*GetOutput, error)

	// Update modifies an existing encounter
	Update(ctx context.Context, input *UpdateInput) (*UpdateOutput, error)

	// Delete removes an encounter
	Delete(ctx context.Context, input *DeleteInput) (*DeleteOutput, error)
}

// EncounterData represents the persistent state of an encounter
type EncounterData struct {
	ID                string
	RoomData          interface{} // Temporarily using interface{} until spatial is fixed
	InitiativeData    *initiative.TrackerData
	InitiativeRolls   []initiative.Roll // Store what was rolled for each entity
	MovementRemaining int32             // Movement remaining for active turn
	Monsters          []*monster.Data   // Monster state for this encounter

	// Multiplayer fields
	State     EncounterState     // Current state (waiting/active/paused/completed)
	JoinCode  string             // 6-char code for players to join
	HostID    string             // ID of the player who created the encounter
	Players   map[string]*Player // Map of PlayerID to Player
	CreatedAt time.Time          // When the encounter was created
}

// SaveInput defines the request for saving an encounter
type SaveInput struct {
	EncounterID       string
	RoomData          interface{} // Temporarily using interface{} until spatial is fixed
	InitiativeData    *initiative.TrackerData
	InitiativeRolls   []initiative.Roll
	MovementRemaining int32           // Movement remaining for active turn
	Monsters          []*monster.Data // Monster state for this encounter

	// Multiplayer fields
	State     EncounterState     // Current state (waiting/active/paused/completed)
	JoinCode  string             // 6-char code for players to join
	HostID    string             // ID of the player who created the encounter
	Players   map[string]*Player // Map of PlayerID to Player
	CreatedAt time.Time          // When the encounter was created
}

// SaveOutput defines the response for saving an encounter
type SaveOutput struct {
	Success bool
}

// GetInput defines the request for retrieving an encounter
type GetInput struct {
	EncounterID string
}

// GetOutput defines the response for retrieving an encounter
type GetOutput struct {
	Data *EncounterData
}

// GetByJoinCodeInput defines the request for retrieving an encounter by join code
type GetByJoinCodeInput struct {
	JoinCode string
}

// UpdateInput defines the request for updating an encounter
type UpdateInput struct {
	EncounterID       string
	InitiativeData    *initiative.TrackerData // Turn order changes
	RoomData          interface{}             // Position changes - temporarily using interface{}
	MovementRemaining *int32                  // Movement remaining for active turn (optional - only update if provided)
	Monsters          []*monster.Data         // Monster state updates (optional - only update if provided)

	// Multiplayer fields (optional - only update if provided)
	State   *EncounterState    // State transition (waiting->active, etc.)
	HostID  *string            // New host ID (for host transfer when host leaves)
	Players map[string]*Player // Updated player map (ready status, connection status)
}

// UpdateOutput defines the response for updating an encounter
type UpdateOutput struct {
	Success bool
}

// DeleteInput defines the request for deleting an encounter
type DeleteInput struct {
	EncounterID string
}

// DeleteOutput defines the response for deleting an encounter
type DeleteOutput struct {
	Success bool
}
