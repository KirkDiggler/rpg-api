package characterdraft_test

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	"github.com/KirkDiggler/rpg-api/internal/pkg/clock"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	draftrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft"
)

type draftWorldFixture struct {
	t      *testing.T
	ctx    context.Context
	server *miniredis.Miniredis
	client *redis.Client
	repo   draftrepo.Repository
}

func newDraftWorldFixture(t *testing.T) *draftWorldFixture {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr(), MaxRetries: -1})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	repo, err := draftrepo.NewRedis(&draftrepo.Config{Client: client, Clock: clock.New(), IDGenerator: idgen.NewPrefixed("generated-")})
	require.NoError(t, err)
	return &draftWorldFixture{t: t, ctx: context.Background(), server: server, client: client, repo: repo}
}

func (f *draftWorldFixture) create(worldID, id, playerID string) *entities.CharacterDraft {
	f.t.Helper()
	draft := &entities.CharacterDraft{WorldID: worldID, Data: &tkcharacter.DraftData{ID: id, PlayerID: playerID, Name: "Draft " + id}}
	_, err := f.repo.Create(f.ctx, draftrepo.CreateInput{Draft: draft})
	require.NoError(f.t, err)
	return draft
}

func (f *draftWorldFixture) raw(key string) string {
	f.t.Helper()
	value, err := f.server.Get(key)
	require.NoError(f.t, err)
	return value
}

func TestDraftReplacementStaysInWorld(t *testing.T) {
	f := newDraftWorldFixture(t)
	// The same player starts a draft in both worlds.
	f.create(worldA, "draft-a", "shared-player")
	inB := f.create(worldB, "draft-b", "shared-player")
	beforeBRecord := f.raw("draft:" + worldB + ":draft-b")
	beforeBMapping := f.raw("draft:player:" + worldB + ":shared-player")

	// Replacing A's draft must not touch B's record or mapping.
	replacement := &entities.CharacterDraft{WorldID: worldA, Data: &tkcharacter.DraftData{ID: "draft-a-replacement", PlayerID: "shared-player", Name: "New A"}}
	_, err := f.repo.Create(f.ctx, draftrepo.CreateInput{Draft: replacement})
	require.NoError(t, err)

	_, err = f.repo.Get(f.ctx, draftrepo.GetInput{WorldID: worldA, ID: "draft-a"})
	require.True(t, apierr.IsNotFound(err), "old A draft is replaced")
	mappedA, err := f.repo.GetByPlayerID(f.ctx, draftrepo.GetByPlayerIDInput{WorldID: worldA, PlayerID: "shared-player"})
	require.NoError(t, err)
	require.Equal(t, replacement, mappedA.Draft)

	mappedB, err := f.repo.GetByPlayerID(f.ctx, draftrepo.GetByPlayerIDInput{WorldID: worldB, PlayerID: "shared-player"})
	require.NoError(t, err)
	require.Equal(t, inB, mappedB.Draft)
	require.Equal(t, beforeBRecord, f.raw("draft:"+worldB+":draft-b"))
	require.Equal(t, beforeBMapping, f.raw("draft:player:"+worldB+":shared-player"))
}

func TestForeignWorldDirectOperationsDoNotWrite(t *testing.T) {
	f := newDraftWorldFixture(t)
	owned := f.create(worldA, "draft-a", "owner-a")
	beforeA := f.raw("draft:" + worldA + ":draft-a")
	beforeMapping := f.raw("draft:player:" + worldA + ":owner-a")

	_, err := f.repo.Get(f.ctx, draftrepo.GetInput{WorldID: worldB, ID: "draft-a"})
	require.True(t, apierr.IsNotFound(err))
	_, err = f.repo.Delete(f.ctx, draftrepo.DeleteInput{WorldID: worldB, ID: "draft-a"})
	require.True(t, apierr.IsNotFound(err))
	_, err = f.repo.GetByPlayerID(f.ctx, draftrepo.GetByPlayerIDInput{WorldID: worldB, PlayerID: "owner-a"})
	require.True(t, apierr.IsNotFound(err))
	foreign := &entities.CharacterDraft{WorldID: worldB, Data: &tkcharacter.DraftData{ID: "draft-a", PlayerID: "owner-a", Name: "Foreign"}}
	_, err = f.repo.Update(f.ctx, draftrepo.UpdateInput{Draft: foreign})
	require.True(t, apierr.IsNotFound(err))

	require.Equal(t, beforeA, f.raw("draft:"+worldA+":draft-a"))
	require.Equal(t, beforeMapping, f.raw("draft:player:"+worldA+":owner-a"))
	require.Equal(t, owned, mustGetDraft(t, f, worldA, "draft-a").Draft)
	require.False(t, f.server.Exists("draft:"+worldB+":draft-a"))
	require.False(t, f.server.Exists("draft:player:"+worldB+":owner-a"))
}

func TestDraftOwnershipCannotMove(t *testing.T) {
	f := newDraftWorldFixture(t)
	owned := f.create(worldA, "draft-a", "owner-a")
	beforeRecord := f.raw("draft:" + worldA + ":draft-a")
	beforeMapping := f.raw("draft:player:" + worldA + ":owner-a")

	moved := &entities.CharacterDraft{WorldID: worldA, Data: &tkcharacter.DraftData{ID: "draft-a", PlayerID: "owner-b", Name: "Moved"}}
	_, err := f.repo.Update(f.ctx, draftrepo.UpdateInput{Draft: moved})
	require.True(t, apierr.IsInvalidArgument(err), "%v", err)

	require.Equal(t, beforeRecord, f.raw("draft:"+worldA+":draft-a"))
	require.Equal(t, beforeMapping, f.raw("draft:player:"+worldA+":owner-a"))
	require.Equal(t, owned, mustGetDraft(t, f, worldA, "draft-a").Draft)
	require.False(t, f.server.Exists("draft:player:"+worldA+":owner-b"))
}

func TestDraftLegacyKeysAreNotFallback(t *testing.T) {
	f := newDraftWorldFixture(t)
	legacy := `{"data":{"id":"draft-legacy","player_id":"owner-a","name":"Old"}}`
	require.NoError(t, f.server.Set("draft:draft-legacy", legacy))
	require.NoError(t, f.server.Set("draft:player:owner-a", "draft-legacy"))

	_, err := f.repo.Get(f.ctx, draftrepo.GetInput{WorldID: worldA, ID: "draft-legacy"})
	require.True(t, apierr.IsNotFound(err))
	_, err = f.repo.GetByPlayerID(f.ctx, draftrepo.GetByPlayerIDInput{WorldID: worldA, PlayerID: "owner-a"})
	require.True(t, apierr.IsNotFound(err))

	require.Equal(t, legacy, f.raw("draft:draft-legacy"))
	require.Equal(t, "draft-legacy", f.raw("draft:player:owner-a"))
}

func TestDraftCorruptOwnershipDoesNotLeak(t *testing.T) {
	f := newDraftWorldFixture(t)

	// A world-scoped key holding a record that claims another world.
	mismatched := `{"world_id":"` + worldB + `","data":{"id":"draft-a","player_id":"owner-a","name":"poisoned"}}`
	require.NoError(t, f.server.Set("draft:"+worldA+":draft-a", mismatched))
	_, err := f.repo.Get(f.ctx, draftrepo.GetInput{WorldID: worldA, ID: "draft-a"})
	require.True(t, apierr.IsInternal(err), "%v", err)
	_, err = f.repo.Delete(f.ctx, draftrepo.DeleteInput{WorldID: worldA, ID: "draft-a"})
	require.True(t, apierr.IsInternal(err), "%v", err)
	require.Equal(t, mismatched, f.raw("draft:"+worldA+":draft-a"))

	// A mapping that resolves to a different player's draft is corruption and
	// is not projected, and the mapping is not deleted.
	f.create(worldA, "draft-b", "owner-b")
	require.NoError(t, f.server.Set("draft:player:"+worldA+":owner-a", "draft-b"))
	_, err = f.repo.GetByPlayerID(f.ctx, draftrepo.GetByPlayerIDInput{WorldID: worldA, PlayerID: "owner-a"})
	require.True(t, apierr.IsInternal(err), "%v", err)
	require.Equal(t, "draft-b", f.raw("draft:player:"+worldA+":owner-a"))
}

func mustGetDraft(t *testing.T, f *draftWorldFixture, worldID, id string) *draftrepo.GetOutput {
	t.Helper()
	out, err := f.repo.Get(f.ctx, draftrepo.GetInput{WorldID: worldID, ID: id})
	require.NoError(t, err)
	return out
}
