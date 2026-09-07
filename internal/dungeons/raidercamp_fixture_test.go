package dungeons_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
)

// raiderCampKey is the dungeon the hold-out scenario is authored against
// (rpg-project#375): a three-room camp, one faction with its chief as mind,
// hostile to the party until the chief comes to know the fact a letter at
// the gate reveals. Shipped beside the tomb and the heirloom fixture, which
// this slice leaves exactly as they were (A7).
const raiderCampKey = "reference-raider-camp"

// shippedRaiderCamp is the real content file, so these tests run on what
// the server boots with rather than a fixture that could drift from it.
var shippedRaiderCamp = filepath.Join("..", "..", "content", raiderCampKey+".yaml")

// TestRaiderCampFixture_IsByteIdenticalToTheToolkits is the heirloom's pin
// for the camp, for the heirloom's reason: this file and the toolkit's
// dungeonspec testdata copy are ONE fixture kept in two places, and the walk
// only means anything if the rules the toolkit proved on its copy are the
// rules the game boots on this one.
//
// Read out of the PINNED module (go list resolves go.mod's own version),
// never a local checkout — a checkout routinely sits on an unmerged branch,
// and this must compare against the version actually built.
func (s *RegistrySuite) TestRaiderCampFixture_IsByteIdenticalToTheToolkits() {
	ours, err := os.ReadFile(shippedRaiderCamp)
	s.Require().NoError(err, "the shipped raider camp fixture must exist")

	out, err := exec.CommandContext(s.T().Context(), "go", "list", "-m", "-f", "{{.Dir}}",
		"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter").Output()
	s.Require().NoError(err, "resolve the pinned encounter module's directory via go list")
	theirs, err := os.ReadFile(filepath.Clean(filepath.Join(
		strings.TrimSpace(string(out)), "dungeonspec", "testdata", raiderCampKey+".yaml")))
	s.Require().NoError(err, "the pinned encounter module must carry the fixture this one copies")

	s.Equal(string(theirs), string(ours),
		"the shipped raider camp fixture has drifted from the toolkit's — copy the toolkit's over it, "+
			"or change both in the same wave")
}

// TestRaiderCampFixture_CompilesAndDeclaresItsScenarioEnding drives the
// whole content path the server boots through: the file compiles with its
// sides in the world, the hold-out binding is constructed by the rulebook's
// own scenario package, and the ending it declares reaches the world beside
// the one the host always declares.
//
// The ENDING KEY is the scenario's own id, translated by nobody: it is what
// the `ended` beat carries and what a client maps to a sentence.
func (s *RegistrySuite) TestRaiderCampFixture_CompilesAndDeclaresItsScenarioEnding() {
	raw, err := os.ReadFile(shippedRaiderCamp)
	s.Require().NoError(err)
	s.write(raiderCampKey+".yaml", raw)

	r := s.open(false)
	entry, err := r.Get(s.ctx, raiderCampKey)
	s.Require().NoError(err, "the shipped raider camp fixture must compile")

	keys := make([]string, 0, len(entry.Dungeon.World.Endings))
	for _, e := range entry.Dungeon.World.Endings {
		keys = append(keys, e.Key)
	}
	s.ElementsMatch([]string{sessionworld.EndingWithdrawn, "hold-out"}, keys,
		"withdrawal always, plus the scenario's own ending — and NO boss-down, "+
			"because the camp authors no flag: the chief's fall is not how this one ends")
	s.Len(entry.Atlas.Regions, 3, "the gate, the yard and the hut")
}

// TestPut_AHoldOutNobodyCanWinIsAFormFillerAnswer is design §2's scenario
// refusals arriving on this wire: the dungeon ALLOWS an `until` fact no
// record reveals (R8, pre-release: show the cost), and the SCENARIO refuses
// it — "a hold-out nobody can win". Both sentences below are the rulebook's,
// unedited, and reach the builder as an ANSWER on the binding block rather
// than a status, exactly as the heirloom's refusals do.
//
// The two cases split across the two layers that own them, the boundary the
// heirloom's test pins: dungeonspec answers a binding that names NOTHING IN
// THE FILE (design law C1) at the exact key; the scenario package answers a
// binding that is well-formed but unwinnable, in its constructor's words,
// on the block rpg-api was validating.
func (s *RegistrySuite) TestPut_AHoldOutNobodyCanWinIsAFormFillerAnswer() {
	raw, err := os.ReadFile(shippedRaiderCamp)
	s.Require().NoError(err)

	for _, tc := range []struct {
		name, from, to, wantPath, wants string
	}{{
		name:     "a faction this dungeon does not declare",
		from:     "convince: raiders",
		to:       "convince: bandits",
		wantPath: "scenarios.hold-out.convince",
		wants:    "nothing in this dungeon has that id",
	}, {
		name:     "an until fact nothing in the dungeon reveals",
		from:     "until: { fact: saved-wiseman }",
		to:       "until: { fact: spared-the-scout }",
		wantPath: "scenarios.hold-out",
		wants:    "a hold-out nobody can win",
	}} {
		s.Run(tc.name, func() {
			broken := bytes.Replace(raw, []byte(tc.from), []byte(tc.to), 1)
			s.Require().NotEqual(raw, broken, "the fixture line this case edits must be where it expects it")

			r := s.open(true)
			out, putErr := r.Put(s.ctx, &dungeons.PutInput{
				Key: raiderCampKey, YAML: broken, ValidateOnly: true,
			})
			s.Require().NoError(putErr, "a form filled in wrong is an answer, never a status")
			s.Require().Len(out.Errors, 1, "one defect, reported once: %v", out.Errors)
			s.Contains(out.Errors[0].Message, tc.wants)
			s.Equal(tc.wantPath, out.Errors[0].Path, "the refusal names the binding it is about")
		})
	}
}

// TestPut_TheRaiderCampItselfStillCompiles is the control the negatives
// above need: each of them edits one line of this file, so a refusal that
// fired for some unrelated reason would look like a pass.
func (s *RegistrySuite) TestPut_TheRaiderCampItselfStillCompiles() {
	raw, err := os.ReadFile(shippedRaiderCamp)
	s.Require().NoError(err)

	r := s.open(true)
	out, err := r.Put(s.ctx, &dungeons.PutInput{Key: raiderCampKey, YAML: raw, ValidateOnly: true})
	s.Require().NoError(err)
	s.Empty(out.Errors)
}
