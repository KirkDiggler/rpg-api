package session_test

import (
	"context"
	"strconv"
	"strings"
	"testing"

	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"

	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

// TestTheSessionSavesThroughTheFence proves the SDK found the Redis
// repository's fenced capability (rpg-project#879 S12a): Launch lands the
// first checkpoint and every verb after it advances the version the store
// keeps beside the world. A plain path would leave no version at all.
func TestTheSessionSavesThroughTheFence(t *testing.T) {
	h := newAcceptanceHarness(t)
	ctx := auth.WithPlayerID(context.Background(), "player-alice")
	_, err := h.charRepo.Create(context.Background(), characterrepo.CreateInput{
		Character: &entities.Character{Data: armedFighter("alice", "player-alice")},
	})
	require.NoError(t, err)

	h.launch(t, "fenced-run", buildThreeRoomTomb(t), seatAt("alice", 1, 1))
	require.Equal(t, uint64(1), checkpointVersion(t, h), "Launch lands checkpoint 1")

	_, err = h.handler.Move(ctx, &sessionpb.MoveRequest{
		Session: "fenced-run", Member: "alice", Path: []*sessionpb.Position{pbAt(2, 1)},
	})
	require.NoError(t, err)
	require.Equal(t, uint64(2), checkpointVersion(t, h), "the Move's one save advanced the checkpoint once")

	// Another writer lands a save between this verb's load and its save --
	// what a lease that expired mid-verb allows. Simulated by a hook that
	// advances the version right before the session's fenced script runs: a
	// plain path would write over it, the fence refuses it.
	h.redis.AddHook(lateWriter{redis: h.redis, key: checkpointKey(t, h)})
	before, err := h.redis.Get(context.Background(), checkpointKey(t, h)).Result()
	require.NoError(t, err)
	_, err = h.handler.Move(ctx, &sessionpb.MoveRequest{
		Session: "fenced-run", Member: "alice", Path: []*sessionpb.Position{pbAt(3, 1)},
	})
	require.Error(t, err, "a save whose checkpoint moved under it is refused")
	after := checkpointVersion(t, h)
	require.Equal(t, mustUint(t, before)+1, after, "only the late writer's save advanced the checkpoint")
}

func mustUint(t *testing.T, s string) uint64 {
	t.Helper()
	v, err := strconv.ParseUint(s, 10, 64)
	require.NoError(t, err)
	return v
}

// lateWriter advances the checkpoint once, just before the first script
// (the fenced save) runs: another writer's save landing mid-verb.
type lateWriter struct {
	redis *goredis.Client
	key   string
}

func (w lateWriter) DialHook(next goredis.DialHook) goredis.DialHook { return next }
func (w lateWriter) ProcessPipelineHook(next goredis.ProcessPipelineHook) goredis.ProcessPipelineHook {
	return next
}
func (w lateWriter) ProcessHook(next goredis.ProcessHook) goredis.ProcessHook {
	fired := false
	return func(ctx context.Context, cmd goredis.Cmder) error {
		name := cmd.Name()
		if !fired && (name == "evalsha" || name == "eval") {
			fired = true
			if err := w.redis.Incr(ctx, w.key).Err(); err != nil {
				return err
			}
		}
		return next(ctx, cmd)
	}
}

func checkpointKey(t *testing.T, h *acceptanceHarness) string {
	t.Helper()
	keys, err := h.redis.Keys(context.Background(), "session-enc-ver:v1alpha1:*").Result()
	require.NoError(t, err)
	require.Len(t, keys, 1)
	return keys[0]
}

// checkpointVersion is the one encounter checkpoint version in the store.
func checkpointVersion(t *testing.T, h *acceptanceHarness) uint64 {
	t.Helper()
	keys, err := h.redis.Keys(context.Background(), "session-enc-ver:v1alpha1:*").Result()
	require.NoError(t, err)
	require.Len(t, keys, 1, "one run, one checkpoint version")
	require.True(t, strings.HasPrefix(keys[0], "session-enc-ver:v1alpha1:"))
	raw, err := h.redis.Get(context.Background(), keys[0]).Result()
	require.NoError(t, err)
	v, err := strconv.ParseUint(raw, 10, 64)
	require.NoError(t, err)
	return v
}
