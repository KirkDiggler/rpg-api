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
// ONE CAPTURE THAT NEEDED A STRIPPED COPY: reference-hearing.atlas.json
// (rpg-project#569) was captured from origin/dev at 1dd537bb from the hearing
// room with every `on:` table removed, because that build cannot compile the
// `heard`, `path` and time-row `fact` words. The atlas holds no table, and the
// full file at the hearing branch's head answers the identical atlas.
//
// AND the-mad-king.atlas.json (rpg-project#605, the whole castle as one
// studio document) was captured at the commit that added it, because no
// earlier build carries the castle file: a change detector from that commit
// on, not proof of correctness. It was recaptured
// when the castle took rpg-toolkit d428cf40's twelve outer wall lines: twelve
// structural walls with no openings, and their twelve wall props and twelve
// spans in placed, are the only change to the captured atlas. It was
// recaptured again at rpg-toolkit 1d590c5b. Two partition walls close the
// kitchen and chapel off as their own regions. The chapel wall and its door
// move onto the hex boundary, which frees the four sealed cells the wall
// stood on. It was recaptured at rpg-toolkit 8c9c4495, where the
// kitchen wall and its door do the same. Its four sealed cells and the
// one-cell space beside it join the kitchen, and nothing is sealed. It was
// recaptured at rpg-toolkit d39830fa, the fold (rpg-project#611): the cells
// room and the kitchen's orders join the castle and the south front moves
// four rows north. Cells go from 1563 to 1447, structural walls from 19 to
// 23, doors from 5 to 6, placed from 51 to 62, props from 3 to 4 and
// regions from 6 to 7, and the cells block takes the id room-1-region.
// It was recaptured for the QA castle (rpg-project#639): the corner wall off
// the gate wall is its one change, a twenty-fourth structural wall and its
// wall prop and span in placed (62 to 64). The stranger, the patrols and the
// far room's new arrival are not in an atlas. It was recaptured again for
// the operator's outside start (rpg-project#654): his open ground round the
// gatehouse takes the cells from 1447 to 1586, its doorway in the
// gatehouse's south wall is a seventh structural door with its wall prop and
// span in placed (64 to 66), the outside is an eighth region, and the six
// cells his ground paints under the gatehouse's side walls answer as sealed.
// It was recaptured for site format v5 (rpg-project#725), where regions are
// the floor: the eight wall-bounded partitions become the ten authored
// regions (the nine named rooms and the implicit room-1-region for the open
// ground), each with its authored name and intensity 1, over the same 1580
// cells. The castle copy at that pin also carries rpg-toolkit 93b323e6
// (rpg-project#716), whose front-door exit is the atlas's second exit. Every
// other field is unchanged, and the other four converted files answer their
// fixtures unchanged.
//
// AND reference-pursuit.atlas.json (smarter monsters S5, rpg-project#672) was
// captured at the commit that added it, for the castle's reason: no earlier
// build carries the file. A change detector from that commit on.
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
