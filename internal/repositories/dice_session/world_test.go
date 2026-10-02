package dicesession_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	dicesession "github.com/KirkDiggler/rpg-api/internal/repositories/dice_session"
)

type diceWorldFixture struct {
	t      *testing.T
	ctx    context.Context
	server *miniredis.Miniredis
	client *redis.Client
	clock  *controlledClock
	repo   dicesession.Repository
}

func newDiceWorldFixture(t *testing.T) *diceWorldFixture {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr(), MaxRetries: -1})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	clock := &controlledClock{now: time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)}
	repo, err := dicesession.NewRedisRepository(&dicesession.Config{Client: client, Clock: clock})
	require.NoError(t, err)
	return &diceWorldFixture{t: t, ctx: context.Background(), server: server, client: client, clock: clock, repo: repo}
}

func (f *diceWorldFixture) create(worldID, entity, scope string, rolls []dicesession.DiceRoll) *dicesession.DiceSession {
	f.t.Helper()
	out, err := f.repo.Create(f.ctx, dicesession.CreateInput{WorldID: worldID, EntityID: entity, Context: scope, Rolls: rolls, TTL: time.Hour})
	require.NoError(f.t, err)
	return out.Session
}

func (f *diceWorldFixture) raw(key string) string {
	f.t.Helper()
	value, err := f.server.Get(key)
	require.NoError(f.t, err)
	return value
}

func TestAbilityRollsStayInWorld(t *testing.T) {
	f := newDiceWorldFixture(t)
	// The same player rolls ability scores in both worlds under the same
	// entity+context names.
	rollsA := []dicesession.DiceRoll{{RollID: "roll-a", Total: 15}}
	rollsB := []dicesession.DiceRoll{{RollID: "roll-b", Total: 8}}
	f.create(worldA, "shared-player", "ability_scores", rollsA)
	f.create(worldB, "shared-player", "ability_scores", rollsB)

	gotA, err := f.repo.Get(f.ctx, dicesession.GetInput{WorldID: worldA, EntityID: "shared-player", Context: "ability_scores"})
	require.NoError(t, err)
	require.Equal(t, rollsA, gotA.Session.Rolls)
	gotB, err := f.repo.Get(f.ctx, dicesession.GetInput{WorldID: worldB, EntityID: "shared-player", Context: "ability_scores"})
	require.NoError(t, err)
	require.Equal(t, rollsB, gotB.Session.Rolls)

	// A's roll IDs are not available to B: the roll ID itself is not a key,
	// but the world-scoped session that holds it is invisible from B.
	_, err = f.repo.Get(f.ctx, dicesession.GetInput{WorldID: worldB, EntityID: "other-player", Context: "ability_scores"})
	require.True(t, apierr.IsNotFound(err))
	require.Contains(t, gotA.Session.Rolls[0].RollID, "roll-a")
	require.NotEqual(t, gotA.Session.Rolls[0].RollID, gotB.Session.Rolls[0].RollID)

	// Deleting A's session leaves B's bytes unchanged.
	beforeB := f.raw("dice_session:" + worldB + ":shared-player:ability_scores")
	_, err = f.repo.Delete(f.ctx, dicesession.DeleteInput{WorldID: worldA, EntityID: "shared-player", Context: "ability_scores"})
	require.NoError(t, err)
	require.Equal(t, beforeB, f.raw("dice_session:"+worldB+":shared-player:ability_scores"))
	require.False(t, f.server.Exists("dice_session:"+worldA+":shared-player:ability_scores"))
}

func TestDiceForeignWorldDeleteDoesNotTouchOtherWorld(t *testing.T) {
	f := newDiceWorldFixture(t)
	f.create(worldA, "entity-a", "scope-a", []dicesession.DiceRoll{{RollID: "roll-a", Total: 1}})
	beforeA := f.raw("dice_session:" + worldA + ":entity-a:scope-a")

	// Deleting under B is an idempotent no-op that must not reach A's key.
	out, err := f.repo.Delete(f.ctx, dicesession.DeleteInput{WorldID: worldB, EntityID: "entity-a", Context: "scope-a"})
	require.NoError(t, err)
	require.Zero(t, out.RollsDeleted)
	require.Equal(t, beforeA, f.raw("dice_session:"+worldA+":entity-a:scope-a"))
}

func TestDiceLegacyKeysAreNotFallback(t *testing.T) {
	f := newDiceWorldFixture(t)
	legacy := `{"EntityID":"entity-a","Context":"scope-a","Rolls":[],"CreatedAt":"2026-09-24T12:00:00Z","ExpiresAt":"2026-09-24T13:00:00Z"}`
	require.NoError(t, f.server.Set("dice_session:entity-a:scope-a", legacy))

	_, err := f.repo.Get(f.ctx, dicesession.GetInput{WorldID: worldA, EntityID: "entity-a", Context: "scope-a"})
	require.True(t, apierr.IsNotFound(err))
	require.Equal(t, legacy, f.raw("dice_session:entity-a:scope-a"))
}

func TestDiceCorruptOwnershipDoesNotLeak(t *testing.T) {
	f := newDiceWorldFixture(t)
	mismatched := `{"world_id":"` + worldB + `","EntityID":"entity-a","Context":"scope-a","Rolls":[],"CreatedAt":"2026-09-24T12:00:00Z","ExpiresAt":"2026-09-25T12:00:00Z"}`
	require.NoError(t, f.server.Set("dice_session:"+worldA+":entity-a:scope-a", mismatched))
	// A healthy record in the other world must survive every operation below.
	f.create(worldB, "entity-a", "scope-a", []dicesession.DiceRoll{{RollID: "other-roll"}})
	beforeOther := f.raw("dice_session:" + worldB + ":entity-a:scope-a")

	// Get refuses to project the contradictory envelope.
	_, err := f.repo.Get(f.ctx, dicesession.GetInput{WorldID: worldA, EntityID: "entity-a", Context: "scope-a"})
	require.True(t, apierr.IsInternal(err), "%v", err)

	// Update must not overwrite an envelope whose persisted ownership
	// contradicts the write.
	session := &dicesession.DiceSession{WorldID: worldA, EntityID: "entity-a", Context: "scope-a", Rolls: []dicesession.DiceRoll{{RollID: "new"}}, ExpiresAt: f.clock.now.Add(time.Hour)}
	require.True(t, apierr.IsInternal(f.repo.Update(f.ctx, session)))
	require.Equal(t, mismatched, f.raw("dice_session:"+worldA+":entity-a:scope-a"))

	// Delete keeps its documented best-effort semantics: the corrupt
	// world-scoped key is removed with a zero count, and the other world is
	// untouched.
	out, err := f.repo.Delete(f.ctx, dicesession.DeleteInput{WorldID: worldA, EntityID: "entity-a", Context: "scope-a"})
	require.NoError(t, err)
	require.Zero(t, out.RollsDeleted)
	require.False(t, f.server.Exists("dice_session:"+worldA+":entity-a:scope-a"))
	require.Equal(t, beforeOther, f.raw("dice_session:"+worldB+":entity-a:scope-a"))
}
