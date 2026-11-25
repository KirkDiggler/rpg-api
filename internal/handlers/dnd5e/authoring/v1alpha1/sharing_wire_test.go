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
// GetDungeon over a real registry: a shelf holding only the small kitchen
// site and a fresh world store per test, authoring on, so a test pays only
// the site it saves. The setting is a root key that is carried and never
// read, so the castle's size proves nothing more (rpg-api#1274).

const (
	sharingSiteKey  = dungeonstest.CastleKitchenKey
	sharingSiteCopy = "my-sharing-site"
)

// sharingHandler is an authoring handler over a shelf and a fresh world
// store, and the kitchen site under the copy's key. The registry refuses a
// shelf with no default dungeon, so the shelf's default file is the kitchen
// standing in for the castle, rekeyed to dungeons.DefaultKey; the copy the
// tests save is the same kitchen under its own key.
func sharingHandler(t *testing.T) (*authoringhandler.Handler, string) {
	t.Helper()
	raw := dungeonstest.CastleKitchenYAML(t)
	shelf := t.TempDir()
	standIn := dungeonstest.AsKey(t, raw, sharingSiteKey, dungeons.DefaultKey)
	require.NoError(t, os.WriteFile(filepath.Join(shelf, dungeons.DefaultKey+".yaml"), standIn, 0o600))
	siteOnly, err := dungeons.NewFileRegistry(shelf, dungeonstest.Projector(t))
	require.NoError(t, err)
	orch, err := authoringorch.New(&authoringorch.Config{Dungeons: dungeonstest.Join(t, siteOnly, true)})
	require.NoError(t, err)
	h, err := authoringhandler.New(&authoringhandler.HandlerConfig{Orchestrator: orch})
	require.NoError(t, err)
	return h, string(dungeonstest.AsKey(t, raw, sharingSiteKey, sharingSiteCopy))
}

// markedSharing is site with `party: { sharing: word }` at its root.
func markedSharing(t *testing.T, site, word string) string {
	t.Helper()
	require.Equal(t, 1, strings.Count(site, "\nroom:"), "precondition: one root room key")
	return strings.Replace(site, "\nroom:", "\nparty:\n  sharing: "+word+"\nroom:", 1)
}

// requireSavesAndReloads grades doc, saves it, and reads it back byte for
// byte.
func requireSavesAndReloads(t *testing.T, h *authoringhandler.Handler, doc, what string) {
	t.Helper()
	ctx := worldCtx("alice")
	for _, validateOnly := range []bool{true, false} {
		resp, err := h.PutDungeon(ctx, &authoringpb.PutDungeonRequest{Key: sharingSiteCopy, Yaml: doc, ValidateOnly: validateOnly})
		require.NoError(t, err, "%s (validate only %v)", what, validateOnly)
		require.Empty(t, resp.GetErrors(), "%s compiles", what)
		require.NotNil(t, resp.GetAtlas(), "%s projects its atlas", what)
	}
	saved, err := h.GetDungeon(ctx, &authoringpb.GetDungeonRequest{Key: sharingSiteCopy})
	require.NoError(t, err)
	require.Equal(t, doc, saved.GetYaml(), "%s reloads unchanged", what)
}

// TestSharingEachTheirOwnValidatesSavesAndReloads: `own` grades clean with
// its atlas, saves, and reloads unchanged.
func TestSharingEachTheirOwnValidatesSavesAndReloads(t *testing.T) {
	h, site := sharingHandler(t)
	requireSavesAndReloads(t, h, markedSharing(t, site, "own"), "own")
}

// TestSharingSharedViewValidatesSavesAndReloads: so does `view`.
func TestSharingSharedViewValidatesSavesAndReloads(t *testing.T) {
	h, site := sharingHandler(t)
	requireSavesAndReloads(t, h, markedSharing(t, site, "view"), "view")
}

// TestSharingAnUnmarkedSiteIsUnchanged: the site with no party block
// still grades clean and reloads byte for byte; omitted is the default.
func TestSharingAnUnmarkedSiteIsUnchanged(t *testing.T) {
	h, site := sharingHandler(t)
	requireSavesAndReloads(t, h, site, "unmarked")
}

// TestSharingAnUnknownWordIsRefusedAtItsPath: a word the dial does not have
// is a defect at party.sharing in the engine's own sentence, which names the
// words there are. The passthrough is pinned, not the engine's wording.
func TestSharingAnUnknownWordIsRefusedAtItsPath(t *testing.T) {
	h, site := sharingHandler(t)
	resp, err := h.PutDungeon(worldCtx("alice"), &authoringpb.PutDungeonRequest{
		Key: sharingSiteCopy, Yaml: markedSharing(t, site, "shout"), ValidateOnly: true,
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

// TestSharingSharedKnowledgeValidatesSavesAndReloads: `knowledge` is designed
// and not built (rpg-project#937). It is valid content: it grades clean with
// its atlas, saves, and reloads unchanged; only a launch refuses it.
func TestSharingSharedKnowledgeValidatesSavesAndReloads(t *testing.T) {
	h, site := sharingHandler(t)
	requireSavesAndReloads(t, h, markedSharing(t, site, "knowledge"), "knowledge")
}
