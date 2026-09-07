package auth_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/KirkDiggler/rpg-api/internal/auth"
)

func TestMembershipCacheExpiryAndDeleteToken(t *testing.T) {
	now := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	cache, err := auth.NewMembershipCache(&auth.MembershipCacheConfig{
		TTL:        30 * time.Second,
		MaxEntries: 1024,
		Now:        func() time.Time { return now },
	})
	require.NoError(t, err)
	decision := auth.MembershipDecision{PlayerID: "player-1", WorldID: "123456789012345678"}
	cache.Set("credential-a", "123456789012345678", decision)
	cache.Set("credential-a", "223456789012345678", auth.MembershipDecision{PlayerID: "player-1", WorldID: "223456789012345678"})

	got, ok := cache.Get("credential-a", "123456789012345678")
	require.True(t, ok)
	require.Equal(t, decision, got)

	now = now.Add(30 * time.Second)
	_, ok = cache.Get("credential-a", "123456789012345678")
	require.False(t, ok, "an entry must expire at exactly its TTL")

	now = now.Add(time.Second)
	cache.Set("credential-a", "123456789012345678", decision)
	cache.Set("other-credential", "123456789012345678", decision)
	cache.DeleteToken("credential-a")
	_, ok = cache.Get("credential-a", "123456789012345678")
	require.False(t, ok)
	_, ok = cache.Get("credential-a", "223456789012345678")
	require.False(t, ok)
	_, ok = cache.Get("other-credential", "123456789012345678")
	require.True(t, ok)
}

func TestMembershipCacheBoundsAndDeterministicEviction(t *testing.T) {
	now := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	cache, err := auth.NewMembershipCache(&auth.MembershipCacheConfig{
		TTL:        30 * time.Second,
		MaxEntries: 1024,
		Now:        func() time.Time { return now },
	})
	require.NoError(t, err)

	for i := 0; i < 1024; i++ {
		cache.Set(fmt.Sprintf("credential-%04d", i), "123456789012345678", auth.MembershipDecision{
			PlayerID: fmt.Sprintf("player-%04d", i),
			WorldID:  "123456789012345678",
		})
		now = now.Add(time.Millisecond)
	}
	cache.Set("credential-new", "123456789012345678", auth.MembershipDecision{PlayerID: "player-new", WorldID: "123456789012345678"})

	_, ok := cache.Get("credential-0000", "123456789012345678")
	require.False(t, ok, "oldest-expiring entry must be evicted")
	_, ok = cache.Get("credential-0001", "123456789012345678")
	require.True(t, ok)
	_, ok = cache.Get("credential-new", "123456789012345678")
	require.True(t, ok)
}

func TestMembershipCacheConcurrentAccess(t *testing.T) {
	cache, err := auth.NewMembershipCache(&auth.MembershipCacheConfig{
		TTL:        30 * time.Second,
		MaxEntries: 1024,
		Now:        time.Now,
	})
	require.NoError(t, err)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(3)
		go func(i int) {
			defer wg.Done()
			token := fmt.Sprintf("credential-%d", i)
			cache.Set(token, "123456789012345678", auth.MembershipDecision{PlayerID: "player", WorldID: "123456789012345678"})
		}(i)
		go func(i int) {
			defer wg.Done()
			cache.Get(fmt.Sprintf("credential-%d", i), "123456789012345678")
		}(i)
		go func(i int) {
			defer wg.Done()
			cache.DeleteToken(fmt.Sprintf("credential-%d", i))
		}(i)
	}
	wg.Wait()
}

func TestMembershipCacheConfigValidation(t *testing.T) {
	_, err := auth.NewMembershipCache(nil)
	require.Error(t, err)
	_, err = auth.NewMembershipCache(&auth.MembershipCacheConfig{})
	require.Error(t, err)
	_, err = auth.NewMembershipCache(&auth.MembershipCacheConfig{TTL: 31 * time.Second, MaxEntries: 1, Now: time.Now})
	require.Error(t, err)
	_, err = auth.NewMembershipCache(&auth.MembershipCacheConfig{TTL: time.Second, MaxEntries: 1025, Now: time.Now})
	require.Error(t, err)
}
