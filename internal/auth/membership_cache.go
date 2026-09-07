package auth

import (
	"crypto/sha256"
	"fmt"
	"sync"
	"time"
)

const (
	maximumMembershipTTL     = 30 * time.Second
	maximumMembershipEntries = 1024
)

// MembershipCacheConfig configures the bounded positive membership cache.
type MembershipCacheConfig struct {
	TTL        time.Duration
	MaxEntries int
	Now        func() time.Time
}

// MembershipDecision is a provider-verified world membership decision.
type MembershipDecision struct {
	PlayerID string
	WorldID  string
}

type membershipCacheKey struct {
	tokenDigest [sha256.Size]byte
	guildID     string
}

type membershipCacheEntry struct {
	decision  MembershipDecision
	expiresAt time.Time
	sequence  uint64
}

// MembershipCache stores only successful membership decisions. Keys contain a
// one-way token digest rather than a bearer credential.
type MembershipCache struct {
	mu         sync.Mutex
	entries    map[membershipCacheKey]membershipCacheEntry
	ttl        time.Duration
	maxEntries int
	now        func() time.Time
	sequence   uint64
}

// NewMembershipCache constructs a cache within the approved TTL and size bounds.
func NewMembershipCache(cfg *MembershipCacheConfig) (*MembershipCache, error) {
	if cfg == nil {
		return nil, fmt.Errorf("membership cache config is required")
	}
	if cfg.TTL <= 0 || cfg.TTL > maximumMembershipTTL {
		return nil, fmt.Errorf("membership cache TTL must be between zero and %s", maximumMembershipTTL)
	}
	if cfg.MaxEntries <= 0 || cfg.MaxEntries > maximumMembershipEntries {
		return nil, fmt.Errorf("membership cache maximum entries must be between zero and %d", maximumMembershipEntries)
	}
	if cfg.Now == nil {
		return nil, fmt.Errorf("membership cache clock is required")
	}
	return &MembershipCache{
		entries:    make(map[membershipCacheKey]membershipCacheEntry, cfg.MaxEntries),
		ttl:        cfg.TTL,
		maxEntries: cfg.MaxEntries,
		now:        cfg.Now,
	}, nil
}

// Get returns an unexpired positive decision.
func (c *MembershipCache) Get(token, guildID string) (MembershipDecision, bool) {
	key := newMembershipCacheKey(token, guildID)
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok {
		return MembershipDecision{}, false
	}
	if !c.now().Before(entry.expiresAt) {
		delete(c.entries, key)
		return MembershipDecision{}, false
	}
	return entry.decision, true
}

// Set stores a positive decision without retaining the raw token.
func (c *MembershipCache) Set(token, guildID string, decision MembershipDecision) {
	key := newMembershipCacheKey(token, guildID)
	c.mu.Lock()
	defer c.mu.Unlock()

	now := c.now()
	c.removeExpired(now)
	if _, exists := c.entries[key]; !exists && len(c.entries) >= c.maxEntries {
		c.evictOldest()
	}
	c.sequence++
	c.entries[key] = membershipCacheEntry{
		decision:  decision,
		expiresAt: now.Add(c.ttl),
		sequence:  c.sequence,
	}
}

// DeleteToken removes all guild decisions for a token without a raw-token index.
func (c *MembershipCache) DeleteToken(token string) {
	digest := sha256.Sum256([]byte(token))
	c.mu.Lock()
	defer c.mu.Unlock()
	for key := range c.entries {
		if key.tokenDigest == digest {
			delete(c.entries, key)
		}
	}
}

func newMembershipCacheKey(token, guildID string) membershipCacheKey {
	return membershipCacheKey{
		tokenDigest: sha256.Sum256([]byte(token)),
		guildID:     guildID,
	}
}

func (c *MembershipCache) removeExpired(now time.Time) {
	for key, entry := range c.entries {
		if !now.Before(entry.expiresAt) {
			delete(c.entries, key)
		}
	}
}

func (c *MembershipCache) evictOldest() {
	var oldestKey membershipCacheKey
	var oldest membershipCacheEntry
	first := true
	for key, entry := range c.entries {
		if first || entry.expiresAt.Before(oldest.expiresAt) ||
			(entry.expiresAt.Equal(oldest.expiresAt) && entry.sequence < oldest.sequence) {
			oldestKey = key
			oldest = entry
			first = false
		}
	}
	if !first {
		delete(c.entries, oldestKey)
	}
}
