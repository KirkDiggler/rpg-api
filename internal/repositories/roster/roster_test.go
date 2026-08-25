package roster_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	rosterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/roster"
)

// Both backings honour the same contract, so one test drives both: a saved
// roster round-trips whole (players and monsters, order preserved), a
// missing encounter is ErrNotFound, and the returned row is a copy the
// caller cannot mutate into the store.
func TestRepositoryContract(t *testing.T) {
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	backings := map[string]rosterrepo.Repository{
		"in_memory": rosterrepo.NewInMemory(),
		"redis":     rosterrepo.NewRedis(client, 24*time.Hour),
	}

	for name, repo := range backings {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()

			_, err := repo.Get(ctx, "enc-none")
			require.ErrorIs(t, err, rosterrepo.ErrNotFound)

			saved := &rosterrepo.Data{
				EncounterID: "enc-1",
				Members: []rosterrepo.Member{
					{ID: "char-alice", Kind: rosterrepo.KindPlayer},
					{ID: "char-bob", Kind: rosterrepo.KindPlayer},
					{ID: "skeleton-1", Kind: rosterrepo.KindMonster, Ref: "dnd5e:monsters:skeleton", Name: "Skeleton"},
				},
			}
			require.NoError(t, repo.Save(ctx, saved))

			got, err := repo.Get(ctx, "enc-1")
			require.NoError(t, err)
			require.Equal(t, saved, got)

			// Mutating the returned row must not reach the store.
			got.Members[0].ID = "mutated"
			again, err := repo.Get(ctx, "enc-1")
			require.NoError(t, err)
			require.Equal(t, "char-alice", again.Members[0].ID)

			require.Error(t, repo.Save(ctx, &rosterrepo.Data{}), "a row with no encounter id must refuse")
		})
	}
}
