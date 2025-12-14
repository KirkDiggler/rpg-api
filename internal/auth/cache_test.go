package auth_test

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/KirkDiggler/rpg-api/internal/auth"
)

func TestTokenCache_Get_MissOnEmpty(t *testing.T) {
	cache := auth.NewTokenCache(5 * time.Minute)

	userID, ok := cache.Get("nonexistent-token")

	assert.False(t, ok)
	assert.Empty(t, userID)
}

func TestTokenCache_SetAndGet_Success(t *testing.T) {
	cache := auth.NewTokenCache(5 * time.Minute)

	cache.Set("token-123", "user-456")
	userID, ok := cache.Get("token-123")

	assert.True(t, ok)
	assert.Equal(t, "user-456", userID)
}

func TestTokenCache_Get_ExpiredEntry(t *testing.T) {
	cache := auth.NewTokenCache(10 * time.Millisecond)

	cache.Set("token-123", "user-456")
	time.Sleep(20 * time.Millisecond)

	userID, ok := cache.Get("token-123")

	assert.False(t, ok)
	assert.Empty(t, userID)
}

func TestTokenCache_Set_OverwritesExisting(t *testing.T) {
	cache := auth.NewTokenCache(5 * time.Minute)

	cache.Set("token-123", "user-old")
	cache.Set("token-123", "user-new")

	userID, ok := cache.Get("token-123")

	assert.True(t, ok)
	assert.Equal(t, "user-new", userID)
}

func TestTokenCache_ConcurrentAccess(t *testing.T) {
	cache := auth.NewTokenCache(5 * time.Minute)
	var wg sync.WaitGroup

	// Multiple concurrent writers
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			cache.Set("token", "user")
		}(i)
	}

	// Multiple concurrent readers
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			cache.Get("token")
		}()
	}

	wg.Wait()
	// No panic = success
}
