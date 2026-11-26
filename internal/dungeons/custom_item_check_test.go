package dungeons_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	"github.com/KirkDiggler/rpg-api/internal/repositories/worlddungeons"
)

// THE CONTENT CHECK ON A CUSTOM ITEM (rpg-project#850): an item the rules
// refuse is refused when the author checks or saves the site, at its place
// under `customItems:`, the same way a refused status is; nothing is stored.

const customItemsKey = "custom-items-site"

func customItemsSite(t *testing.T, slot string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "custom-items", "site.yaml"))
	require.NoError(t, err)
	if slot == "cloak" {
		return raw
	}
	require.Equal(t, 1, bytes.Count(raw, []byte("slot: cloak")), "precondition: the item's slot is written once")
	return bytes.Replace(raw, []byte("slot: cloak"), []byte("slot: "+slot), 1)
}

func TestPutRefusesACustomItemTheRulesRefuseAndStoresNothing(t *testing.T) {
	ctx := context.Background()
	world := dungeonstest.WorldStore(t)
	r := joinShelf(t, world, true)

	good, err := r.Put(ctx, &dungeons.PutInput{Caller: alice, Key: customItemsKey, YAML: customItemsSite(t, "cloak"), ValidateOnly: true})
	require.NoError(t, err)
	require.Empty(t, good.Errors, "control: a cloak item on the cloak slot is taken")

	saved, err := r.Put(ctx, &dungeons.PutInput{Caller: alice, Key: customItemsKey, YAML: customItemsSite(t, "cloak")})
	require.NoError(t, err)
	require.Empty(t, saved.Errors, "control: the same site saves")
	_, err = world.Delete(ctx, &worlddungeons.DeleteInput{WorldID: alice.WorldID, Owner: alice.PlayerID, Key: customItemsKey})
	require.NoError(t, err)

	for _, validateOnly := range []bool{true, false} {
		out, putErr := r.Put(ctx, &dungeons.PutInput{Caller: alice, Key: customItemsKey, YAML: customItemsSite(t, "cape"), ValidateOnly: validateOnly})
		require.NoErrorf(t, putErr, "validate-only %v: a refusal is an answer, not an error", validateOnly)
		require.Lenf(t, out.Errors, 1, "validate-only %v", validateOnly)
		require.Equal(t, "customItems.lieutenants-cloak", out.Errors[0].Path, "at the author's place")
		require.Contains(t, out.Errors[0].Message, "cape", "the rules' sentence names the word")
		require.Nil(t, out.Entry)
	}
	_, err = world.Get(ctx, &worlddungeons.GetInput{WorldID: alice.WorldID, Key: customItemsKey})
	require.ErrorIs(t, err, worlddungeons.ErrNotFound, "nothing was stored")
}

// THE AUTHOR SAYS AN ITEM IS UNIQUE (rpg-project#850): `unique: true` on an
// item is taken by the site format and by the rules; a value that is not true
// or false is refused at the item's own word, and nothing is stored.

func customItemsSiteUnique(t *testing.T, value string) []byte {
	t.Helper()
	raw := customItemsSite(t, "cloak")
	require.Equal(t, 1, bytes.Count(raw, []byte("slot: cloak\n")), "precondition: the item's slot line is written once")
	return bytes.Replace(raw, []byte("slot: cloak\n"), []byte("slot: cloak\n    unique: "+value+"\n"), 1)
}

func TestPutTakesAnItemTheAuthorSaysIsUniqueAndRefusesAWrongValue(t *testing.T) {
	ctx := context.Background()
	world := dungeonstest.WorldStore(t)
	r := joinShelf(t, world, true)

	checked, err := r.Put(ctx, &dungeons.PutInput{Caller: alice, Key: customItemsKey, YAML: customItemsSiteUnique(t, "true"), ValidateOnly: true})
	require.NoError(t, err)
	require.Empty(t, checked.Errors, "unique: true is accepted on validate-only")

	saved, err := r.Put(ctx, &dungeons.PutInput{Caller: alice, Key: customItemsKey, YAML: customItemsSiteUnique(t, "true")})
	require.NoError(t, err)
	require.Empty(t, saved.Errors, "unique: true is accepted on save")
	_, err = world.Delete(ctx, &worlddungeons.DeleteInput{WorldID: alice.WorldID, Owner: alice.PlayerID, Key: customItemsKey})
	require.NoError(t, err)

	for _, validateOnly := range []bool{true, false} {
		out, putErr := r.Put(ctx, &dungeons.PutInput{Caller: alice, Key: customItemsKey, YAML: customItemsSiteUnique(t, "sometimes"), ValidateOnly: validateOnly})
		require.NoErrorf(t, putErr, "validate-only %v: a refusal is an answer, not an error", validateOnly)
		require.Lenf(t, out.Errors, 1, "validate-only %v", validateOnly)
		require.Equal(t, "customItems.lieutenants-cloak.unique", out.Errors[0].Path, "at the item's own word")
		require.Contains(t, out.Errors[0].Message, "true or false")
		require.Nil(t, out.Entry)
	}
	_, err = world.Get(ctx, &worlddungeons.GetInput{WorldID: alice.WorldID, Key: customItemsKey})
	require.ErrorIs(t, err, worlddungeons.ErrNotFound, "nothing was stored")
}
