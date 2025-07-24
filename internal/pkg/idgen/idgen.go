// Package idgen provides ID generation utilities
package idgen

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

//go:generate mockgen -destination=mock/mock.go -package=idgenmock github.com/KirkDiggler/rpg-api/internal/pkg/idgen Generator

// Generator generates unique identifiers
type Generator interface {
	Generate() string
}

// PrefixedGenerator generates IDs with a specific prefix
type PrefixedGenerator struct {
	prefix string
}

// NewPrefixed creates a new generator with the given prefix
func NewPrefixed(prefix string) *PrefixedGenerator {
	return &PrefixedGenerator{prefix: prefix}
}

// Generate creates a new ID with the format: prefix_timestamp_random
func (g *PrefixedGenerator) Generate() string {
	timestamp := time.Now().UnixNano()
	randomBytes := make([]byte, 4)
	_, err := rand.Read(randomBytes)
	if err != nil {
		// crypto/rand.Read should never fail on a properly configured system
		// If it does, it indicates a catastrophic system failure
		panic(fmt.Sprintf("crypto/rand.Read failed: %v", err))
	}
	random := hex.EncodeToString(randomBytes)

	return fmt.Sprintf("%s_%d_%s", g.prefix, timestamp, random)
}

// SimpleGenerator generates simple IDs without a prefix
type SimpleGenerator struct{}

// Generate creates a new ID with timestamp and random suffix
func (g *SimpleGenerator) Generate() string {
	timestamp := time.Now().UnixNano()
	randomBytes := make([]byte, 8)
	_, err := rand.Read(randomBytes)
	if err != nil {
		// crypto/rand.Read should never fail on a properly configured system
		// If it does, it indicates a catastrophic system failure
		panic(fmt.Sprintf("crypto/rand.Read failed: %v", err))
	}
	random := hex.EncodeToString(randomBytes)

	return fmt.Sprintf("%d_%s", timestamp, random)
}

// SequentialGenerator generates sequential IDs for testing
type SequentialGenerator struct {
	prefix  string
	counter uint64
}

// NewSequential creates a new sequential generator
func NewSequential(prefix string) *SequentialGenerator {
	return &SequentialGenerator{prefix: prefix}
}

// Generate creates a new sequential ID
func (g *SequentialGenerator) Generate() string {
	n := atomic.AddUint64(&g.counter, 1)
	if g.prefix != "" {
		return fmt.Sprintf("%s_%d", g.prefix, n)
	}
	return fmt.Sprintf("%d", n)
}

// UUIDGenerator generates UUIDs with optional prefix
type UUIDGenerator struct {
	prefix string
}

// NewUUID creates a new UUID generator with optional prefix
func NewUUID(prefix string) *UUIDGenerator {
	return &UUIDGenerator{prefix: prefix}
}

// Generate creates a new UUID-based ID
func (g *UUIDGenerator) Generate() string {
	id := uuid.New().String()
	if g.prefix != "" {
		return fmt.Sprintf("%s_%s", g.prefix, id)
	}
	return id
}
