package authoringv1alpha1_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	authoringpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/authoring/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	authoringhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/authoring/v1alpha1"
	authoringorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/authoring"
)

// sharing_wire_test.go drives the party's sharing setting (rpg-project#935,
// `party: { sharing: own | view | knowledge }`) through PutDungeon and
// GetDungeon over a real registry: a shelf holding only the castle and a
// fresh world store per test, authoring on, so a test pays only the castle it saves. The setting is
// carried, never read: nothing in the api decides by it.

const (
	sharingCastleKey  = "the-mad-king"
	sharingCastleCopy = "my-sharing-castle"
)

// sharingHandler is an authoring handler over the shared shipped store and
// a fresh world store, and the shipped castle under the copy's key.
func sharingHandler(t *testing.T) (*authoringhandler.Handler, string) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dungeonstest.ContentDir(t), sharingCastleKey+".yaml")) //nolint:gosec // the shipped content directory
	require.NoError(t, err)
	shelf := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(shelf, sharingCastleKey+".yaml"), raw, 0o600))
	castleOnly, err := dungeons.NewFileRegistry(shelf, dungeonstest.Projector(t))
	require.NoError(t, err)
	orch, err := authoringorch.New(&authoringorch.Config{Dungeons: dungeonstest.Join(t, castleOnly, true)})
	require.NoError(t, err)
	h, err := authoringhandler.New(&authoringhandler.HandlerConfig{Orchestrator: orch})
	require.NoError(t, err)
	return h, string(dungeonstest.AsKey(t, raw, sharingCastleKey, sharingCastleCopy))
}

// markedSharing is castle with `party: { sharing: word }` at its root.
func markedSharing(t *testing.T, castle, word string) string {
	t.Helper()
	require.Equal(t, 1, strings.Count(castle, "\nroom:"), "precondition: one root room key")
	return strings.Replace(castle, "\nroom:", "\nparty:\n  sharing: "+word+"\nroom:", 1)
}

// requireSavesAndReloads grades doc, saves it, and reads it back byte for
// byte.
func requireSavesAndReloads(t *testing.T, h *authoringhandler.Handler, doc, what string) {
	t.Helper()
	ctx := worldCtx("alice")
	for _, validateOnly := range []bool{true, false} {
		resp, err := h.PutDungeon(ctx, &authoringpb.PutDungeonRequest{Key: sharingCastleCopy, Yaml: doc, ValidateOnly: validateOnly})
		require.NoError(t, err, "%s (validate only %v)", what, validateOnly)
		require.Empty(t, resp.GetErrors(), "%s compiles", what)
		require.NotNil(t, resp.GetAtlas(), "%s projects its atlas", what)
	}
	saved, err := h.GetDungeon(ctx, &authoringpb.GetDungeonRequest{Key: sharingCastleCopy})
	require.NoError(t, err)
	require.Equal(t, doc, saved.GetYaml(), "%s reloads unchanged", what)
}

// TestSharingEachTheirOwnValidatesSavesAndReloads: `own` grades clean with
// its atlas, saves, and reloads unchanged.
func TestSharingEachTheirOwnValidatesSavesAndReloads(t *testing.T) {
	h, castle := sharingHandler(t)
	requireSavesAndReloads(t, h, markedSharing(t, castle, "own"), "own")
}

// TestSharingSharedViewValidatesSavesAndReloads: so does `view`.
func TestSharingSharedViewValidatesSavesAndReloads(t *testing.T) {
	h, castle := sharingHandler(t)
	requireSavesAndReloads(t, h, markedSharing(t, castle, "view"), "view")
}

// TestSharingUnmarkedCastleIsUnchanged: the castle with no party block
// still grades clean and reloads byte for byte; omitted is the default.
func TestSharingUnmarkedCastleIsUnchanged(t *testing.T) {
	h, castle := sharingHandler(t)
	requireSavesAndReloads(t, h, castle, "unmarked")
}

// TestSharingAnUnknownWordIsRefusedAtItsPath: a word the dial does not have
// is a defect at party.sharing in the engine's own sentence, which names the
// words there are. The passthrough is pinned, not the engine's wording.
func TestSharingAnUnknownWordIsRefusedAtItsPath(t *testing.T) {
	h, castle := sharingHandler(t)
	resp, err := h.PutDungeon(worldCtx("alice"), &authoringpb.PutDungeonRequest{
		Key: sharingCastleCopy, Yaml: markedSharing(t, castle, "shout"), ValidateOnly: true,
	})
	require.NoError(t, err, "a grade is never a status")
	require.Len(t, resp.GetErrors(), 1)
	require.Equal(t, "party.sharing", resp.GetErrors()[0].GetPath())
	require.Contains(t, resp.GetErrors()[0].GetMessage(), "shout", "the refusal quotes the word the author typed")
	for _, word := range []string{"own", "view", "knowledge"} {
		require.Contains(t, resp.GetErrors()[0].GetMessage(), word, "and names the words there are")
	}
	require.Nil(t, resp.GetAtlas())
}
