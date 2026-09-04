package dungeons_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
)

// heirloomKey is the dungeon the recover-the-artifact scenario is authored
// against (rpg-project#368): the reference tomb plus a concealed vault, a
// holdable heirloom, a captain who knows the way in, an exit, and the
// scenario binding. Shipped beside the plain tomb, which this slice leaves
// exactly as it was.
const heirloomKey = "reference-tomb-heirloom"

// shippedHeirloom is the real content file, so these tests run on what the
// server boots with rather than a fixture that could drift from it.
var shippedHeirloom = filepath.Join("..", "..", "content", heirloomKey+".yaml")

// TestHeirloomFixture_IsByteIdenticalToTheToolkits is the pin the plan asks
// for, and the reason it is bytes rather than behavior: this file and the
// toolkit's dungeonspec testdata copy are ONE fixture kept in two places, and
// two copies of a fixture that are allowed to differ are two fixtures. The
// toolkit proves the scenario's rules against its copy; rpg-api boots the
// game on this one; the walk only means anything if they are the same file.
//
// Read out of the PINNED module (go list resolves go.mod's own version),
// never a local checkout — a checkout routinely sits on an unmerged branch,
// and this must compare against the version actually built.
func (s *RegistrySuite) TestHeirloomFixture_IsByteIdenticalToTheToolkits() {
	ours, err := os.ReadFile(shippedHeirloom)
	s.Require().NoError(err, "the shipped heirloom fixture must exist")

	out, err := exec.CommandContext(s.T().Context(), "go", "list", "-m", "-f", "{{.Dir}}",
		"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter").Output()
	s.Require().NoError(err, "resolve the pinned encounter module's directory via go list")
	theirs, err := os.ReadFile(filepath.Clean(filepath.Join(
		strings.TrimSpace(string(out)), "dungeonspec", "testdata", heirloomKey+".yaml")))
	s.Require().NoError(err, "the pinned encounter module must carry the fixture this one copies")

	s.Equal(string(theirs), string(ours),
		"the shipped heirloom fixture has drifted from the toolkit's — copy the toolkit's over it, "+
			"or change both in the same wave")
}

// TestHeirloomFixture_CompilesAndDeclaresItsScenarioEnding drives the whole
// content path the server boots through: the file compiles, the binding is
// constructed by the rulebook's own scenario package, and the ending it
// declares reaches the world beside the two the host declares.
//
// The ENDING KEY is the scenario's own id, translated by nobody: it is what
// the `ended` beat carries and what a client maps to a sentence.
func (s *RegistrySuite) TestHeirloomFixture_CompilesAndDeclaresItsScenarioEnding() {
	raw, err := os.ReadFile(shippedHeirloom)
	s.Require().NoError(err)
	s.write(heirloomKey+".yaml", raw)

	r := s.open(false)
	entry, err := r.Get(s.ctx, heirloomKey)
	s.Require().NoError(err, "the shipped heirloom fixture must compile")

	keys := make([]string, 0, len(entry.Dungeon.World.Endings))
	for _, e := range entry.Dungeon.World.Endings {
		keys = append(keys, e.Key)
	}
	s.ElementsMatch([]string{sessionworld.EndingWithdrawn, "recover-the-artifact"}, keys,
		"withdrawal always, plus the scenario's own ending — and NO boss-down, "+
			"because this fixture authors no flag (design R8)")
}

// TestSeed_AnEmptyMountGetsEveryShippedDungeon is the walk's runbook made
// mechanical: a fresh box mounts an empty RPG_CONTENT_DIR, and after seeding
// it serves everything the image ships — the tomb AND the heirloom fixture.
// Before rpg-project#368 the seed copied the default alone, so a second
// reference dungeon had to be carried to the box by hand.
func (s *RegistrySuite) TestSeed_AnEmptyMountGetsEveryShippedDungeon() {
	shipped := filepath.Join("..", "..", "content")
	empty := s.T().TempDir()

	seeded, err := dungeons.SeedShipped(empty, shipped)
	s.Require().NoError(err)
	s.Contains(seeded, dungeons.DefaultKey)
	s.Contains(seeded, heirloomKey)

	onDisk, err := os.ReadDir(shipped)
	s.Require().NoError(err)
	s.Len(seeded, len(onDisk), "every shipped dungeon, not a curated subset")

	r, err := dungeons.NewFileRegistry(empty, false, dungeonstest.Projector(s.T()))
	s.Require().NoError(err)
	list, err := r.List(s.ctx)
	s.Require().NoError(err)
	s.Len(list, len(seeded), "and the registry serves every one of them")
}

// TestPut_ABadScenarioBindingIsAFormFillerAnswer is the design §8 row this
// repo owns: "the form refuses a missing or non-holdable artifact in
// form-filler words."
//
// It is an ANSWER, not a status. PutDungeon's contract is that a well-formed
// request whose file will not compile comes back OK with `errors` populated,
// and a form filled in wrong is exactly that kind of failure — the builder
// has to render the sentence beside the field it is about.
//
// EVERY SENTENCE BELOW IS THE RULEBOOK'S, UNEDITED, and the four cases split
// across the two layers that own them, which is the boundary worth pinning:
//
//   - dungeonspec answers a binding that names NOTHING IN THE FILE, because
//     that is a reference check it can make alone (design law C1), and its
//     path reaches the exact key;
//   - the scenario package answers a binding that names the WRONG KIND of
//     thing, in the words its own constructor uses, and rpg-api names the
//     binding block it was validating — the field key is inside the
//     rulebook's sentence, which rpg-api may repeat and must not parse;
//   - rpg-api itself answers only "this build has no such scenario", which
//     is a fact about the binary and nobody else's to know.
func (s *RegistrySuite) TestPut_ABadScenarioBindingIsAFormFillerAnswer() {
	raw, err := os.ReadFile(shippedHeirloom)
	s.Require().NoError(err)

	for _, tc := range []struct {
		name, from, to, wantPath, wants string
	}{{
		name:     "an artifact this dungeon does not place",
		from:     "artifact: heirloom",
		to:       "artifact: nothing-by-that-name",
		wantPath: "scenarios.recover-the-artifact.artifact",
		wants:    "nothing in this dungeon has that id",
	}, {
		name:     "an artifact nobody can pick up",
		from:     "artifact: heirloom",
		to:       "artifact: captain",
		wantPath: "scenarios.recover-the-artifact",
		wants:    "this scenario needs an artifact — which placed thing is the party here to recover",
	}, {
		name:     "a way out this dungeon does not declare",
		from:     "exit: entrance",
		to:       "exit: back-door",
		wantPath: "scenarios.recover-the-artifact.exit",
		wants:    "nothing in this dungeon has that id",
	}, {
		name:     "a scenario this build does not have",
		from:     "recover-the-artifact:",
		to:       "rescue-the-princess:",
		wantPath: "scenarios.rescue-the-princess",
		wants:    "no scenario named",
	}} {
		s.Run(tc.name, func() {
			broken := bytes.Replace(raw, []byte(tc.from), []byte(tc.to), 1)
			s.Require().NotEqual(raw, broken, "the fixture line this case edits must be where it expects it")

			r := s.open(true)
			out, putErr := r.Put(s.ctx, &dungeons.PutInput{
				Key: heirloomKey, YAML: broken, ValidateOnly: true,
			})
			s.Require().NoError(putErr, "a form filled in wrong is an answer, never a status")
			s.Require().Len(out.Errors, 1)
			s.Contains(out.Errors[0].Message, tc.wants)
			s.Equal(tc.wantPath, out.Errors[0].Path, "the refusal names the binding it is about")
		})
	}
}

// TestPut_TheHeirloomFixtureItselfStillCompiles is the control the four
// negatives above need: each of them edits one line of this file, so a
// refusal that fired for some unrelated reason would look like a pass.
func (s *RegistrySuite) TestPut_TheHeirloomFixtureItselfStillCompiles() {
	raw, err := os.ReadFile(shippedHeirloom)
	s.Require().NoError(err)

	r := s.open(true)
	out, err := r.Put(s.ctx, &dungeons.PutInput{Key: heirloomKey, YAML: raw, ValidateOnly: true})
	s.Require().NoError(err)
	s.Empty(out.Errors)
}

// TestPut_TheOldSpellingOfKnowledgeIsRefusedByName is R1 arriving at the wire
// (rpg-project#372): one spelling of knowledge, and the retired one is
// refused rather than ignored.
//
// IGNORING IT WOULD BE THE DANGEROUS OUTCOME. A file that still said `knows:`
// and compiled would store, spawn a captain holding nothing, and play exactly
// like a dungeon whose author never authored intel at all — a scenario that
// cannot be won by its second path, with nothing anywhere saying why. So this
// asserts the refusal is an ANSWER the builder can render, on the field it is
// about, in the compiler's own words.
func (s *RegistrySuite) TestPut_TheOldSpellingOfKnowledgeIsRefusedByName() {
	raw, err := os.ReadFile(shippedHeirloom)
	s.Require().NoError(err)

	const holds = "holds: [vault-map]"
	s.Require().Contains(string(raw), holds, "the fixture's holds line must be where this test expects it")
	legacy := []byte(strings.Replace(string(raw), holds, "knows: [vault-map]", 1))

	r := s.open(true)
	out, err := r.Put(s.ctx, &dungeons.PutInput{
		Key: heirloomKey, YAML: legacy, ValidateOnly: true,
	})
	s.Require().NoError(err, "a retired spelling is a form filled in wrong, not a status")
	s.Require().Len(out.Errors, 1)
	s.Contains(out.Errors[0].Message, "knows",
		"the refusal names the word the author used, so they can find it")
}

// TestPut_TheShippedFixtureAuthorsItsKnowledgeAsARecord is the positive half,
// and it is about the FILE rather than the compiler: the dungeon this slice
// ships must be the one the panel will round-trip, so it declares an `intel:`
// record and places it with `holds:`, and says `knows:` nowhere.
func (s *RegistrySuite) TestPut_TheShippedFixtureAuthorsItsKnowledgeAsARecord() {
	raw, err := os.ReadFile(shippedHeirloom)
	s.Require().NoError(err)

	s.Contains(string(raw), "intel:", "the record is declared")
	s.Contains(string(raw), "reveals: { door: vault }", "and says what it reveals")
	s.Contains(string(raw), "holds: [vault-map]", "and the captain is where it was placed")
	s.NotContains(string(raw), "knows:", "and nothing spells knowledge the retired way")
}
