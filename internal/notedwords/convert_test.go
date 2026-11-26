package notedwords_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter/dungeonspec"

	"github.com/KirkDiggler/rpg-api/internal/notedwords"
)

// beforeCastle is the shipped castle as dev served it at encounter v0.220.0,
// before rpg-project#1005: last week's file, every old word in it.
const beforeCastle = "testdata/the-mad-king-v0.220.0.yaml"

// beforeCompiled is beforeCastle's compiled form AS ENCOUNTER v0.220.0
// PRODUCED IT: json.MarshalIndent(c, "", " ") plus a newline, where c is
// dungeonspec.Load(beforeCastle). It was generated once from a scratch module
// whose go.mod pinned only github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter
// v0.220.0, running a main that reads the file, calls dungeonspec.Load and
// writes that JSON to stdout. Regenerating it at that version reproduces it
// byte for byte; nothing here may regenerate it at a newer one.
const beforeCompiled = "testdata/the-mad-king-v0.220.0.compiled.json"

// shippedCastle is the castle as this repository ships it now.
var shippedCastle = filepath.Join("..", "..", "content", "the-mad-king.yaml")

type ConvertSuite struct {
	suite.Suite

	before   []byte
	compiled []byte
	shipped  []byte
}

func TestConvertSuite(t *testing.T) {
	suite.Run(t, new(ConvertSuite))
}

func (s *ConvertSuite) SetupTest() {
	var err error
	s.before, err = os.ReadFile(beforeCastle)
	s.Require().NoError(err)
	s.compiled, err = os.ReadFile(beforeCompiled)
	s.Require().NoError(err)
	s.compiled = s.historicalShape(s.compiled)
	s.shipped, err = os.ReadFile(shippedCastle) //nolint:gosec // the repository's own content
	s.Require().NoError(err)
}

func (s *ConvertSuite) convert(raw []byte) *notedwords.ConvertOutput {
	s.T().Helper()
	out, err := notedwords.Convert(&notedwords.ConvertInput{Source: raw})
	s.Require().NoError(err)
	return out
}

// compile is raw's compiled form at the encounter this module pins, in the
// pinned file's JSON shape.
func (s *ConvertSuite) compile(raw []byte) []byte {
	s.T().Helper()
	c, err := dungeonspec.Load(raw)
	s.Require().NoError(err)
	out, err := json.MarshalIndent(c, "", " ")
	s.Require().NoError(err)
	return s.historicalShape(out)
}

// historicalShape compares the same semantic picture without rewriting the
// v0.220.0 oracle. The newer optional wall Sections:null is absence, not a
// changed world. Strip only that absent field at its exact owner; keep every
// nonempty section and every other value so real compiler changes still fail.
// Canonical key order is applied to BOTH sides; numbers retain their spelling.
func (s *ConvertSuite) historicalShape(raw []byte) []byte {
	s.T().Helper()
	var shape map[string]interface{}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	s.Require().NoError(decoder.Decode(&shape))
	field, ok := shape["Field"].(map[string]interface{})
	s.Require().True(ok)
	walls, ok := field["StructuralWalls"].([]interface{})
	s.Require().True(ok)
	for _, value := range walls {
		wall, valid := value.(map[string]interface{})
		s.Require().True(valid)
		if sections, exists := wall["Sections"]; exists && sections == nil {
			delete(wall, "Sections")
		}
	}
	out, err := json.MarshalIndent(shape, "", " ")
	s.Require().NoError(err)
	return append(out, '\n')
}

// TestLastWeeksCastleCompilesAsItDid: the old castle is refused as it is;
// converted, it compiles at today's engine to exactly what it compiled to at
// encounter v0.220.0, before any word changed. The words renamed nothing the
// game runs, so the conversion must not either.
func (s *ConvertSuite) TestLastWeeksCastleCompilesAsItDid() {
	_, err := dungeonspec.Load(s.before)
	s.Require().Error(err, "today's engine refuses last week's castle as it is")

	out := s.convert(s.before)
	s.Equal(map[notedwords.Kind]int{
		notedwords.RecognizedToNoted:  22,
		notedwords.RecognizeToGive:    7,
		notedwords.HolderToTo:         7,
		notedwords.HoldsPropToCarries: 2,
		notedwords.GuardTopicToNoted:  1,
	}, out.Counts())
	s.Empty(out.Ambiguous)

	s.Equal(string(s.compiled), string(s.compile(out.Out)),
		"converted, it compiles to exactly what encounter v0.220.0 compiled last week's castle to")
}

func (s *ConvertSuite) TestAuthoredWallSectionsStillChangeTheHistoricalComparison() {
	converted := s.convert(s.before)
	compiled, err := dungeonspec.Load(converted.Out)
	s.Require().NoError(err)
	s.Require().NotEmpty(compiled.Field.StructuralWalls)
	compiled.Field.StructuralWalls[0].Sections = []encounter.WallSection{{ID: "window", Ref: "test:window", Position: 2, Width: 1}}
	raw, err := json.Marshal(compiled)
	s.Require().NoError(err)
	actual := s.historicalShape(raw)
	s.Contains(string(actual), "test:window")
	s.NotEqual(string(s.compiled), string(actual))
}

// TestTheComparisonCanFail: one changed number in the converted castle is a
// different compiled form, so the comparison above is not vacuous.
func (s *ConvertSuite) TestTheComparisonCanFail() {
	out := s.convert(s.before)
	mutated := bytes.Replace(out.Out, []byte("dc: 8"), []byte("dc: 9"), 1)
	s.Require().NotEqual(out.Out, mutated, "the castle no longer has a `dc: 8`")
	s.NotEqual(string(s.compiled), string(s.compile(mutated)))
}

// TestTheNewCastleIsANoOp: the castle this repository ships converts to
// itself, and a converted file converts to itself, so running twice is safe.
func (s *ConvertSuite) TestTheNewCastleIsANoOp() {
	out := s.convert(s.shipped)
	s.Empty(out.Changes)
	s.Empty(out.Ambiguous)
	s.Equal(s.shipped, out.Out)

	twice := s.convert(s.convert(s.before).Out)
	s.Empty(twice.Changes, "a second run over a converted file is a no-op")
}

// TestDialogueIsNeverTouched: the old words inside a line's say are text,
// not keys, and stay as written; the requirement beside them converts.
func (s *ConvertSuite) TestDialogueIsNeverTouched() {
	const src = `room:
  monsterBindings:
    guard:
      options:
        - id: hi
          say: "recognized: you are. recognize: me. holds: purse"
          requires: { recognized: admitted }
          success:
            - say: 'I recognized you, recognize: this'
              recognize: { topic: admitted, holder: spoken_to, about: speaker }
`
	out := s.convert([]byte(src))
	want := strings.NewReplacer(
		"requires: { recognized: admitted }", "requires: { noted: admitted }",
		"recognize: { topic: admitted, holder: spoken_to, about: speaker }",
		"give: { topic: admitted, to: spoken_to, about: speaker }",
	).Replace(src)
	s.Equal(want, string(out.Out))
	s.Len(out.Changes, 3)
}

// TestAHoldsIsNeverGuessed: `holds: <id>` becomes carries only for a prop;
// an id that is both a prop and a topic, or neither, is left and reported.
// A list of records under holds is never touched.
func (s *ConvertSuite) TestAHoldsIsNeverGuessed() {
	const src = `room:
  objects:
    - id: table
      kind: prop
    - id: purse
      kind: prop
  monsterBindings:
    guard:
      holds: [captains-orders]
      options:
        - id: hi
          say: Hi.
          success: [{ say: Fine., recognize: { topic: table, holder: spoken_to } }]
        - id: again
          say: Again.
          requires: { holds: table }
        - id: pay
          say: Pay.
          requires: { holds: purse }
        - id: what
          say: What?
          requires: { holds: nothing-here }
`
	out := s.convert([]byte(src))
	s.Require().Len(out.Ambiguous, 2)
	s.Equal(16, out.Ambiguous[0].Line)
	s.Contains(out.Ambiguous[0].Reason, `"table" is both a prop and a topic`)
	s.Equal(22, out.Ambiguous[1].Line)
	s.Contains(out.Ambiguous[1].Reason, `"nothing-here" is neither a prop nor a topic`)
	got := string(out.Out)
	s.Contains(got, "requires: { holds: table }", "left as written")
	s.Contains(got, "requires: { holds: nothing-here }", "left as written")
	s.Contains(got, "requires: { carries: purse }")
	s.Contains(got, "holds: [captains-orders]", "records are not carrying")
	s.Contains(got, "give: { topic: table, to: spoken_to }")
}

// TestAStatesPostAndAQuotedGuard: a state's holds with a door is a post;
// a guard fact quoted in a flow mapping converts inside its quotes; the beat
// named topic is unchanged.
func (s *ConvertSuite) TestAStatesPostAndAQuotedGuard() {
	const src = `room:
  monsterBindings:
    goblin-1:
      states:
        initial: posted
        posted:
          holds: { door: front-door, against: entering }
behaviorDefinitions:
  patrol:
    body:
      - {on: time, when: {fact: {name: "topic:welcome", op: eq, value: true}}, do: [{pick: move}]}
      - {on: topic, do: [{pick: move}]}
`
	out := s.convert([]byte(src))
	s.Contains(string(out.Out), "post: { door: front-door, against: entering }")
	s.Contains(string(out.Out), `name: "noted.anyone:welcome"`)
	s.Contains(string(out.Out), "{on: topic, do:", "the beat named topic is unchanged")
	s.Len(out.Changes, 2)
}

// TestTheWithdrawnWordsConvertToo: a file written in the words that lived in
// encounter v0.221.0 and v0.222.0 converts to the final ones.
func (s *ConvertSuite) TestTheWithdrawnWordsConvertToo() {
	const src = `room:
  monsterBindings:
    guard:
      options:
        - id: hi
          say: Hi.
          success: [{ say: Fine., give: { holding: admitted, to: spoken_to } }]
        - id: again
          say: Again.
          requires: { holds: admitted }
behaviorDefinitions:
  captain:
    body:
      - {on: time, when: {fact: {name: holds.anyone:admitted, op: eq, value: true}}, do: [{pick: move}]}
      - {on: time, when: {fact: {name: holds:admitted, op: eq, value: true}}, do: [{pick: move}]}
`
	out := s.convert([]byte(src))
	got := string(out.Out)
	s.Contains(got, "give: { topic: admitted, to: spoken_to }")
	s.Contains(got, "requires: { noted: admitted }")
	s.Contains(got, "name: noted.anyone:admitted")
	s.Contains(got, "name: noted:admitted")
	s.Empty(out.Ambiguous)
	s.Len(out.Changes, 4)
	s.Empty(s.convert(out.Out).Changes, "and converts nothing the second time")
}
