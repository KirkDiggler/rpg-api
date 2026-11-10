package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

// TestBoot_ShippedSitesCompileOnceThroughTheProvider is the boot half of
// rpg-project#879 S5: the content registry projects every shipped dungeon
// through the session Manager, so with the process's provider injected the
// shelf's sites compile at boot, one logged compile per hash, and the warm-up
// that names each key's site compiles nothing more.
func TestBoot_ShippedSitesCompileOnceThroughTheProvider(t *testing.T) {
	var mu sync.Mutex
	var lines []string
	logf := func(format string, args ...any) {
		mu.Lock()
		defer mu.Unlock()
		lines = append(lines, fmt.Sprintf(format, args...))
	}
	sites, err := sessionorch.NewSites(&sessionorch.SitesConfig{Logf: logf})
	require.NoError(t, err)

	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	chars, err := characterrepo.NewRedis(&characterrepo.RedisConfig{Client: client})
	require.NoError(t, err)
	orch, err := sessionorch.New(sessionorch.Config{
		Redis: client, Characters: chars, TTL: time.Hour, Sites: sites,
		PresentationIDs: idgen.NewSequential("presentation"),
	})
	require.NoError(t, err)

	store, err := dungeons.NewFileRegistry(dungeonstest.ContentDir(t), registryProjector{orch.Manager})
	require.NoError(t, err)
	shipped, err := store.List(context.Background())
	require.NoError(t, err)
	require.NotEmpty(t, shipped)
	compiledAtLoad := sites.Compiles()
	require.Positive(t, compiledAtLoad, "the registry's projection compiled the shelf's sites")
	require.LessOrEqual(t, compiledAtLoad, len(shipped), "at most one compile per shipped dungeon")

	require.NoError(t, warmShippedSites(context.Background(), store, sites))
	require.Equal(t, compiledAtLoad, sites.Compiles(), "the warm-up after the registry compiles nothing")

	mu.Lock()
	defer mu.Unlock()
	compiled := 0
	for _, l := range lines {
		if strings.HasPrefix(l, "site compiled: ") {
			compiled++
		}
	}
	require.Equal(t, compiledAtLoad, compiled, "one log line per compile")
}
