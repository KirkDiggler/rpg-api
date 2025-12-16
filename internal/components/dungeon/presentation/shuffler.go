package presentation

import (
	"math/rand"
)

// ShuffleConnections randomizes the order of connections to prevent "main path always first" pattern
func ShuffleConnections(connections []*RoomConnection, seed int64) []*RoomConnection {
	if len(connections) <= 1 {
		return connections
	}

	// Create a copy to avoid mutating the input
	shuffled := make([]*RoomConnection, len(connections))
	copy(shuffled, connections)

	// #nosec G404 - Using math/rand for seeded procedural generation, not cryptographic purposes
	r := rand.New(rand.NewSource(seed))

	// Fisher-Yates shuffle
	for i := len(shuffled) - 1; i > 0; i-- {
		j := r.Intn(i + 1)
		shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
	}

	return shuffled
}
