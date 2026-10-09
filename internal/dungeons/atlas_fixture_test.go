package dungeons_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
)

// TestPut_EveryShippedDungeonAnswersTheAtlasItAnsweredBeforeTheWave pins that
// projecting the atlas from the compiled dungeon (rpg-toolkit#1965 tier 2 D)
// changed nothing a client sees.
//
// The fixtures under testdata/atlas were captured from rpg-api origin/dev at
// 541f91bd, BEFORE the atlas moved off the hand-built world, by Putting each
// shipped content file validate-only and marshalling the answer's atlas. A
// shipped dungeon with no fixture fails here: add one by capturing it from a
// build whose atlas you trust, never from the build under test.
func (s *RegistrySuite) TestPut_EveryShippedDungeonAnswersTheAtlasItAnsweredBeforeTheWave() {
	files, err := filepath.Glob(filepath.Join(dungeonstest.ContentDir(s.T()), "*.yaml"))
	s.Require().NoError(err)
	s.Require().NotEmpty(files)

	r := dungeonstest.Shipped(s.T())
	for _, f := range files {
		key := strings.TrimSuffix(filepath.Base(f), ".yaml")
		raw, err := os.ReadFile(f)
		s.Require().NoError(err)

		want, err := os.ReadFile(filepath.Join("testdata", "atlas", key+".atlas.json"))
		s.Require().NoErrorf(err, "%s has no atlas fixture", key)

		res, err := r.Put(s.ctx, &dungeons.PutInput{Key: key, YAML: raw, ValidateOnly: true})
		s.Require().NoError(err)
		s.Require().NotNilf(res.Entry, "%s must compile: %+v", key, res.Errors)

		got, err := json.MarshalIndent(res.Entry.Atlas, "", " ")
		s.Require().NoError(err)
		s.JSONEqf(string(want), string(got), "%s: the atlas PutDungeon answers moved", key)
	}
}
