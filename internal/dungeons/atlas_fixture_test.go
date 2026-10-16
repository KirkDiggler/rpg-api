package dungeons_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
)

// TestPut_EveryShippedDungeonAnswersTheAtlasItAnsweredBeforeTheWave pins that
// projecting the atlas from the compiled dungeon (rpg-toolkit#1965 tier 2 D)
// changed nothing a client sees.
//
// The fixtures under testdata/atlas were captured from rpg-api origin/dev at
// 541f91bd, BEFORE the atlas moved off the hand-built world, by Putting each
// shipped content file validate-only and marshaling the answer's atlas. A
// shipped dungeon with no fixture fails here: add one by capturing it from a
// build whose atlas you trust, never from the build under test.
//
// ONE EXCEPTION, said out loud: castle-kitchen.atlas.json was captured at
// rpg-api#1090's own head, because no earlier build can compile a dungeon
// that places templates (rpg-project#555). It proves nothing about
// correctness — it is a change detector from that head on, vouched for by
// the #555 walk rather than by an earlier build. It holds nothing derived
// from a template: the atlas is cells, the orders prop, the region and the
// party start.
//
// AND ONE CAPTURE THAT NEEDED A STRIPPED COPY: reference-hearing.atlas.json
// (rpg-project#569) was captured from origin/dev at 1dd537bb from the hearing
// room with every `on:` table removed, because that build cannot compile the
// `heard`, `path` and time-row `fact` words. The atlas holds no table, and the
// full file at the hearing branch's head answers the identical atlas.
//
// AND the-mad-king-cells.atlas.json (rpg-project#569, the first studio-authored
// castle room) was captured at its own content commit's base, origin/dev
// 8f22676a, with no Go change in the commit: a change detector from there on,
// vouched for by the operator's play of the room on 2026-10-15. It was
// recaptured at rpg-api#1111's own head for fog of war R9, which adds
// footprint_cells to its four structural walls. That field is the only
// change to the captured atlas.
func (s *RegistrySuite) TestPut_EveryShippedDungeonAnswersTheAtlasItAnsweredBeforeTheWave() {
	files, err := filepath.Glob(filepath.Join(dungeonstest.ContentDir(s.T()), "*.yaml"))
	s.Require().NoError(err)
	s.Require().NotEmpty(files)

	// Each shipped file's atlas is read off the shipped store's entry: the
	// same compile and projection a validate-only Put ran when the fixtures
	// were captured. A Put under a shipped key is now the reference refusal
	// (design R20), so the store's own entry is the answer to compare.
	r := dungeonstest.ShippedStore(s.T())
	for _, f := range files {
		key := strings.TrimSuffix(filepath.Base(f), ".yaml")
		raw, err := os.ReadFile(f)
		s.Require().NoError(err)

		want, err := os.ReadFile(filepath.Join("testdata", "atlas", key+".atlas.json"))
		s.Require().NoErrorf(err, "%s has no atlas fixture", key)

		entry, err := r.Get(s.ctx, key)
		s.Require().NoErrorf(err, "%s must compile", key)
		s.Require().Equalf(raw, entry.YAML, "%s is served verbatim", key)

		got, err := json.MarshalIndent(entry.Atlas, "", " ")
		s.Require().NoError(err)
		s.JSONEqf(string(want), string(got), "%s: the atlas PutDungeon answers moved", key)
	}
}
