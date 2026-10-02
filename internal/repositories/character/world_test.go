package character_test

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	redis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

// characterWorldFixture exposes the raw Redis server alongside the adapter so
// world tests can prove byte-for-byte what was and was not written.
type characterWorldFixture struct {
	t      *testing.T
	ctx    context.Context
	server *miniredis.Miniredis
	client *redis.Client
	repo   characterrepo.Repository
}

func newCharacterWorldFixture(t *testing.T) *characterWorldFixture {
	t.Helper()
	server := miniredis.RunT(t)
	client := redis.NewClient(&redis.Options{Addr: server.Addr(), MaxRetries: -1})
	t.Cleanup(func() { require.NoError(t, client.Close()) })
	repo, err := characterrepo.NewRedis(&characterrepo.RedisConfig{Client: client})
	require.NoError(t, err)
	return &characterWorldFixture{t: t, ctx: context.Background(), server: server, client: client, repo: repo}
}

func (f *characterWorldFixture) create(worldID, id, playerID string) *entities.Character {
	f.t.Helper()
	char := repositoryCharacter(id)
	char.WorldID = worldID
	char.Data.PlayerID = playerID
	_, err := f.repo.Create(f.ctx, characterrepo.CreateInput{Character: char})
	require.NoError(f.t, err)
	return mustGet(f.t, f, worldID, id)
}

func (f *characterWorldFixture) raw(key string) string {
	f.t.Helper()
	value, err := f.server.Get(key)
	require.NoError(f.t, err)
	return value
}

func TestSamePlayerSeparateWorldLists(t *testing.T) {
	f := newCharacterWorldFixture(t)
	// The same player owns distinct characters in A and B; another player owns
	// a third character in A.
	inA := f.create(worldA, "char-a", "shared-player")
	inB := f.create(worldB, "char-b", "shared-player")
	f.create(worldA, "char-other", "second-player")

	listA, err := f.repo.ListByPlayerID(f.ctx, characterrepo.ListByPlayerIDInput{WorldID: worldA, PlayerID: "shared-player"})
	require.NoError(t, err)
	require.Equal(t, []*entities.Character{inA}, listA.Characters)
	listB, err := f.repo.ListByPlayerID(f.ctx, characterrepo.ListByPlayerIDInput{WorldID: worldB, PlayerID: "shared-player"})
	require.NoError(t, err)
	require.Equal(t, []*entities.Character{inB}, listB.Characters)

	// Direct reads are world-scoped: A's ID is invisible from B.
	_, err = f.repo.Get(f.ctx, characterrepo.GetInput{WorldID: worldB, ID: "char-a"})
	require.True(t, apierr.IsNotFound(err))
	_, err = f.repo.Get(f.ctx, characterrepo.GetInput{WorldID: worldA, ID: "char-b"})
	require.True(t, apierr.IsNotFound(err))

	// Deleting A's record leaves B's record and both indexes byte-for-byte.
	beforeB := f.raw("character:" + worldB + ":char-b")
	_, err = f.repo.Delete(f.ctx, characterrepo.DeleteInput{WorldID: worldA, ID: "char-a"})
	require.NoError(t, err)
	require.Equal(t, beforeB, f.raw("character:"+worldB+":char-b"))
	membersB, err := f.server.Members("character:player:" + worldB + ":shared-player")
	require.NoError(t, err)
	require.Equal(t, []string{"char-b"}, membersB)
}

func TestForeignWorldDirectOperationsDoNotWrite(t *testing.T) {
	f := newCharacterWorldFixture(t)
	owned := f.create(worldA, "char-a", "owner-a")
	beforeA := f.raw("character:" + worldA + ":char-a")

	// Every direct operation addressed to B with A's ID misses and writes
	// nothing in B.
	_, err := f.repo.Get(f.ctx, characterrepo.GetInput{WorldID: worldB, ID: "char-a"})
	require.True(t, apierr.IsNotFound(err))
	_, err = f.repo.Delete(f.ctx, characterrepo.DeleteInput{WorldID: worldB, ID: "char-a"})
	require.True(t, apierr.IsNotFound(err))
	foreign := repositoryCharacter("char-a")
	foreign.WorldID = worldB
	_, err = f.repo.Update(f.ctx, characterrepo.UpdateInput{Character: foreign})
	require.True(t, apierr.IsNotFound(err))
	_, err = f.repo.PatchEquipment(f.ctx, characterrepo.PatchEquipmentInput{
		WorldID: worldB, CharacterID: "char-a", ExpectedVersion: "v", ArmorClass: 31,
	})
	require.True(t, apierr.IsNotFound(err))

	// A's record is untouched and no B namespace was created.
	require.Equal(t, beforeA, f.raw("character:"+worldA+":char-a"))
	require.Equal(t, owned, mustGet(t, f, worldA, "char-a"))
	require.False(t, f.server.Exists("character:"+worldB+":char-a"))
	require.False(t, f.server.Exists("character:player:"+worldB+":owner-a"))
}

func TestOwnershipCannotMove(t *testing.T) {
	f := newCharacterWorldFixture(t)
	owned := f.create(worldA, "char-a", "owner-a")
	before := f.raw("character:" + worldA + ":char-a")

	for _, player := range []string{"owner-b", ""} {
		moved := repositoryCharacter("char-a")
		moved.WorldID = worldA
		moved.Data.PlayerID = player
		_, err := f.repo.Update(f.ctx, characterrepo.UpdateInput{Character: moved})
		require.True(t, apierr.IsInvalidArgument(err), "player %q: %v", player, err)
	}
	require.Equal(t, before, f.raw("character:"+worldA+":char-a"))
	require.Equal(t, owned, mustGet(t, f, worldA, "char-a"))

	membersA, err := f.server.Members("character:player:" + worldA + ":owner-a")
	require.NoError(t, err)
	require.Equal(t, []string{"char-a"}, membersA)
	require.False(t, f.server.Exists("character:player:"+worldA+":owner-b"))

	// PatchEquipment never touches ownership either.
	version := mustGetOutput(t, f, worldA, "char-a").Version
	_, err = f.repo.PatchEquipment(f.ctx, characterrepo.PatchEquipmentInput{
		WorldID: worldA, CharacterID: "char-a", ExpectedVersion: version, ArmorClass: 42,
	})
	require.NoError(t, err)
	after := mustGet(t, f, worldA, "char-a")
	require.Equal(t, "owner-a", after.Data.PlayerID)
}

func TestLegacyKeysAreNotFallback(t *testing.T) {
	f := newCharacterWorldFixture(t)
	legacy := `{"data":{"id":"char-legacy","player_id":"owner-a","name":"Old","hit_points":1}}`
	require.NoError(t, f.server.Set("character:char-legacy", legacy))
	require.NoError(t, f.server.Set("character:player:owner-a", "char-legacy"))

	// A world-scoped miss never falls back to the ownerless legacy key.
	_, err := f.repo.Get(f.ctx, characterrepo.GetInput{WorldID: worldA, ID: "char-legacy"})
	require.True(t, apierr.IsNotFound(err))
	list, err := f.repo.ListByPlayerID(f.ctx, characterrepo.ListByPlayerIDInput{WorldID: worldA, PlayerID: "owner-a"})
	require.NoError(t, err)
	require.Empty(t, list.Characters)

	// The legacy keys are neither read nor deleted.
	require.Equal(t, legacy, f.raw("character:char-legacy"))
	require.Equal(t, "char-legacy", f.raw("character:player:owner-a"))

	// A fresh world-scoped create with the same ID is a new record; the legacy
	// key is left behind untouched.
	created := f.create(worldA, "char-legacy", "owner-a")
	require.Equal(t, created, mustGet(t, f, worldA, "char-legacy"))
	require.Equal(t, legacy, f.raw("character:char-legacy"))
}

func TestCorruptOwnershipDoesNotLeak(t *testing.T) {
	f := newCharacterWorldFixture(t)

	// Record ownership mismatch: a B-keyed envelope claiming world A.
	mismatched := `{"world_id":"` + worldA + `","data":{"id":"char-a","player_id":"owner-a","name":"poisoned"}}`
	require.NoError(t, f.server.Set("character:"+worldB+":char-a", mismatched))
	_, err := f.repo.Get(f.ctx, characterrepo.GetInput{WorldID: worldB, ID: "char-a"})
	require.True(t, apierr.IsInternal(err), "%v", err)
	_, err = f.repo.Delete(f.ctx, characterrepo.DeleteInput{WorldID: worldB, ID: "char-a"})
	require.True(t, apierr.IsInternal(err), "%v", err)
	require.Equal(t, mismatched, f.raw("character:"+worldB+":char-a"))

	// Poisoned player index: world A's owner-a index references owner-b's
	// record. The list errors without projecting or deleting the member.
	f.create(worldA, "char-b", "owner-b")
	_, err = f.server.SAdd("character:player:"+worldA+":owner-a", "char-b")
	require.NoError(t, err)
	_, err = f.repo.ListByPlayerID(f.ctx, characterrepo.ListByPlayerIDInput{WorldID: worldA, PlayerID: "owner-a"})
	require.True(t, apierr.IsInternal(err), "%v", err)
	members, err := f.server.Members("character:player:" + worldA + ":owner-a")
	require.NoError(t, err)
	require.Equal(t, []string{"char-b"}, members)
	require.True(t, f.server.Exists("character:"+worldA+":char-b"))

	// Poisoned session index: a world A index referencing a world A record is
	// fine, but a B-keyed record cannot resolve from A's index.
	_, err = f.server.SAdd("character:session:"+worldA+":session-a", "char-cross")
	require.NoError(t, err)
	require.NoError(t, f.server.Set("character:"+worldA+":char-cross", `{"world_id":"`+worldB+`","data":{"id":"char-cross","player_id":"owner-a"}}`))
	_, err = f.repo.ListBySessionID(f.ctx, characterrepo.ListBySessionIDInput{WorldID: worldA, SessionID: "session-a"})
	require.True(t, apierr.IsInternal(err), "%v", err)
}

func mustGet(t *testing.T, f *characterWorldFixture, worldID, id string) *entities.Character {
	t.Helper()
	return mustGetOutput(t, f, worldID, id).Character
}

func mustGetOutput(t *testing.T, f *characterWorldFixture, worldID, id string) *characterrepo.GetOutput {
	t.Helper()
	out, err := f.repo.Get(f.ctx, characterrepo.GetInput{WorldID: worldID, ID: id})
	require.NoError(t, err)
	return out
}
