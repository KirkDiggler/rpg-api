package dungeonstest

import (
	"path/filepath"
	"testing"
)

// CastleKitchenKey is the castle kitchen's own `key:` line. The kitchen left
// the shelf when the castle absorbed it (rpg-project#611); its file stays as
// test content, the one dungeon whose creatures' numbers are authored
// templates (rpg-project#555), under internal/dungeons/testdata.
const CastleKitchenKey = "castle-kitchen"

// CastleKitchenYAML is the kitchen, read from the shared fixture directory.
func CastleKitchenYAML(t testing.TB) []byte {
	t.Helper()
	return roomFixture(t, CastleKitchenKey)
}

// StageCastleKitchen writes the kitchen into dir, a scratch content
// directory, so a registry over dir serves it as a shipped reference, and
// returns the bytes it wrote. A suite about what a reference refuses stages
// it before building its registry.
func StageCastleKitchen(t testing.TB, dir string) []byte {
	t.Helper()

	raw := CastleKitchenYAML(t)
	writeFile(t, filepath.Join(dir, CastleKitchenKey+".yaml"), raw)

	return raw
}
