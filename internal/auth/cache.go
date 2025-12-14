// Package auth provides Discord OAuth authentication for gRPC services.
package auth

import (
	"sync"
	"time"
)

type cacheEntry struct {
	userID    string
	expiresAt time.Time
}

// TokenCache provides an in-memory cache for Discord token -> userID mappings.
type TokenCache struct {
	mu      sync.RWMutex
	entries map[string]*cacheEntry
	ttl     time.Duration
}

// NewTokenCache creates a new token cache with the specified TTL.
func NewTokenCache(ttl time.Duration) *TokenCache {
	return &TokenCache{
		entries: make(map[string]*cacheEntry),
		ttl:     ttl,
	}
}

// Get retrieves a userID for the given token.
// Returns the userID and true if found and not expired.
// Lazily removes expired entries.
func (c *TokenCache) Get(token string) (string, bool) {
	c.mu.RLock()
	entry, exists := c.entries[token]
	c.mu.RUnlock()

	if !exists {
		return "", false
	}

	if time.Now().After(entry.expiresAt) {
		// Lazy cleanup - upgrade to write lock
		c.mu.Lock()
		delete(c.entries, token)
		c.mu.Unlock()
		return "", false
	}

	return entry.userID, true
}

// Set stores a userID for the given token with the configured TTL.
func (c *TokenCache) Set(token, userID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[token] = &cacheEntry{
		userID:    userID,
		expiresAt: time.Now().Add(c.ttl),
	}
}
