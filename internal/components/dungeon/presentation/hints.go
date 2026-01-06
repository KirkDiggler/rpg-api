// Package presentation provides presentation layer logic to obscure the main path in dungeons.
// It generates physical hints for connections and randomizes their order to make exploration feel natural.
package presentation

import (
	"math/rand"
)

// Direction represents a cardinal direction for connection placement
type Direction string

// RoomConnection is a local type to avoid import cycles
type RoomConnection struct {
	FromRoom     string
	ToRoom       string
	Type         ConnectionType
	IsMainPath   bool
	Direction    Direction
	PhysicalHint string
}

// ConnectionType defines how rooms are linked
type ConnectionType int

const (
	ConnectionTypeDoor    ConnectionType = 0
	ConnectionTypeStairs  ConnectionType = 1
	ConnectionTypePassage ConnectionType = 2
)

// Theme is a minimal representation for hint generation
type Theme struct {
	ID string
}

// HintGenerator generates physical hints for room connections to obscure the main path
type HintGenerator interface {
	GenerateHints(input *HintInput) *HintOutput
}

// HintInput defines the input for hint generation
type HintInput struct {
	Connections []*RoomConnection
	Theme       Theme
	Seed        int64
}

// HintOutput contains connections with updated physical hints
type HintOutput struct {
	Connections []*RoomConnection
}

// hintGenerator implements HintGenerator
type hintGenerator struct {
	random *rand.Rand
}

// NewHintGenerator creates a new hint generator
func NewHintGenerator() HintGenerator {
	// #nosec G404 - Using math/rand for seeded procedural generation, not cryptographic purposes
	return &hintGenerator{
		random: rand.New(rand.NewSource(0)), // Will be reseeded per generation
	}
}

// GenerateHints assigns physical hints to connections based on theme and randomization
func (g *hintGenerator) GenerateHints(input *HintInput) *HintOutput {
	if input == nil {
		return &HintOutput{Connections: nil}
	}

	if len(input.Connections) == 0 {
		return &HintOutput{Connections: input.Connections}
	}

	// Reseed for deterministic generation
	if input.Seed != 0 {
		g.random.Seed(input.Seed)
	}

	// Get hint vocabulary for this theme
	vocab := getHintVocabulary(input.Theme)

	// Create new connections with generated hints
	connections := make([]*RoomConnection, len(input.Connections))
	for i, conn := range input.Connections {
		newConn := &RoomConnection{
			FromRoom:     conn.FromRoom,
			ToRoom:       conn.ToRoom,
			Type:         conn.Type,
			IsMainPath:   conn.IsMainPath,
			Direction:    conn.Direction, // Preserve direction through presentation layer
			PhysicalHint: g.generateHint(conn, vocab),
		}
		connections[i] = newConn
	}

	return &HintOutput{
		Connections: connections,
	}
}

// generateHint creates a physical hint for a connection
func (g *hintGenerator) generateHint(conn *RoomConnection, vocab *hintVocabulary) string {
	var options []string

	switch conn.Type {
	case ConnectionTypeDoor:
		options = vocab.doors
	case ConnectionTypeStairs:
		options = vocab.stairs
	case ConnectionTypePassage:
		options = vocab.passages
	default:
		options = vocab.doors
	}

	// Randomly select from available options
	if len(options) == 0 {
		return "exit"
	}

	return options[g.random.Intn(len(options))]
}

// hintVocabulary contains themed descriptions for different connection types
type hintVocabulary struct {
	doors    []string
	passages []string
	stairs   []string
}

// getHintVocabulary returns the hint vocabulary for a theme
func getHintVocabulary(theme Theme) *hintVocabulary {
	// Theme-specific vocabularies
	switch theme.ID {
	case "crypt":
		return &hintVocabulary{
			doors: []string{
				"heavy stone door",
				"iron-bound door",
				"ornate bronze door",
				"sealed vault door",
				"crumbling wooden door",
				"arched doorway",
			},
			passages: []string{
				"narrow passage",
				"shadowy corridor",
				"vaulted hallway",
				"torch-lit passage",
				"crumbling archway",
				"low stone tunnel",
			},
			stairs: []string{
				"worn stone steps descending",
				"spiral staircase",
				"cracked marble stairs",
				"narrow stone steps",
			},
		}

	case "cave":
		return &hintVocabulary{
			doors: []string{
				"narrow crevice",
				"boulder-strewn opening",
				"low cave entrance",
				"dark opening",
				"moss-covered entrance",
				"rocky archway",
			},
			passages: []string{
				"winding tunnel",
				"jagged passage",
				"sloping corridor",
				"damp cavern passage",
				"rough-hewn tunnel",
				"natural archway",
			},
			stairs: []string{
				"rough stone steps",
				"slippery descent",
				"natural rock formation",
				"steep rocky slope",
			},
		}

	case "bandit_lair":
		return &hintVocabulary{
			doors: []string{
				"reinforced wooden door",
				"iron gate",
				"barred entrance",
				"heavy oak door",
				"trap door",
				"guard post entrance",
			},
			passages: []string{
				"guard corridor",
				"supply tunnel",
				"narrow passage",
				"rough corridor",
				"fortified hallway",
				"wooden archway",
			},
			stairs: []string{
				"rickety wooden stairs",
				"ladder to lower level",
				"rough-cut steps",
				"rope ladder",
			},
		}

	default:
		// Generic fantasy dungeon vocabulary
		return &hintVocabulary{
			doors: []string{
				"wooden door",
				"stone door",
				"iron door",
				"arched doorway",
				"heavy door",
				"dark entrance",
			},
			passages: []string{
				"corridor",
				"passage",
				"hallway",
				"tunnel",
				"archway",
				"opening",
			},
			stairs: []string{
				"stairs",
				"steps",
				"stairway",
				"descent",
			},
		}
	}
}
