package holdingwords_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter/dungeonspec"

	"github.com/KirkDiggler/rpg-api/internal/holdingwords"
)

// beforeCastle is the shipped castle as it stood on dev before
// rpg-project#1005: last week's file, every old word in it.
const beforeCastle = "testdata/the-mad-king-before-holding-words.yaml"

// shippedCastle is the castle in the new words, as this repository ships it.
var shippedCastle = filepath.Join("..", "..", "content", "the-mad-king.yaml")

type ConvertSuite struct {
	suite.Suite

	before []byte
	after  []byte
}

func TestConvertSuite(t *testing.T) {
	suite.Run(t, new(ConvertSuite))
}

func (s *ConvertSuite) SetupTest() {
	var err error
	s.before, err = os.ReadFile(beforeCastle)
	s.Require().NoError(err)
	s.after, err = os.ReadFile(shippedCastle) //nolint:gosec // the repository's own content
	s.Require().NoError(err)
}

func (s *ConvertSuite) convert(raw []byte) *holdingwords.ConvertOutput {
	s.T().Helper()
	out, err := holdingwords.Convert(&holdingwords.ConvertInput{Source: raw})
	s.Require().NoError(err)
	return out
}

// TestLastWeeksCastleCompilesAsTheNewOne: the old castle, converted, is
// refused nowhere and compiles to exactly what the shipped castle compiles
// to; the only text that still differs is comments.
func (s *ConvertSuite) TestLastWeeksCastleCompilesAsTheNewOne() {
	_, err := dungeonspec.Load(s.before)
	s.Require().Error(err, "the engine refuses last week's castle as it is")

	out := s.convert(s.before)
	s.Equal(map[holdingwords.Kind]int{
		holdingwords.RecognizedToHolds:   22,
		holdingwords.RecognizeToGive:     7,
		holdingwords.TopicToHolding:      7,
		holdingwords.HolderToTo:          7,
		holdingwords.HoldsPropToCarries:  2,
		holdingwords.GuardTopicToHoldsAn: 1,
	}, out.Counts())
	s.Empty(out.Ambiguous)

	converted, err := dungeonspec.Load(out.Out)
	s.Require().NoError(err, "the converted castle compiles")
	shipped, err := dungeonspec.Load(s.after)
	s.Require().NoError(err)
	s.Equal(shipped, converted, "and compiles to exactly the shipped castle")

	s.Equal(stripComments(s.after), stripComments(out.Out),
		"comments are the only text the conversion does not reach")
}

// TestTheNewCastleIsANoOp: converting a converted file changes nothing, so
// running twice is safe.
func (s *ConvertSuite) TestTheNewCastleIsANoOp() {
	out := s.convert(s.after)
	s.Empty(out.Changes)
	s.Empty(out.Ambiguous)
	s.Equal(s.after, out.Out)

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
		"requires: { recognized: admitted }", "requires: { holds: admitted }",
		"recognize: { topic: admitted, holder: spoken_to, about: speaker }",
		"give: { holding: admitted, to: spoken_to, about: speaker }",
	).Replace(src)
	s.Equal(want, string(out.Out))
	s.Len(out.Changes, 4)
}

// TestAPropThatIsAlsoAHoldingIsReportedAndLeft: `holds: table` reads two
// ways when table is a prop and a holding something gives; it is left as
// written and reported. A list of records under holds is never touched.
func (s *ConvertSuite) TestAPropThatIsAlsoAHoldingIsReportedAndLeft() {
	const src = `room:
  objects:
    - id: table
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
`
	out := s.convert([]byte(src))
	s.Require().Len(out.Ambiguous, 1)
	s.Equal(14, out.Ambiguous[0].Line)
	s.Contains(out.Ambiguous[0].Reason, `"table" is both a prop and a holding`)
	s.Contains(string(out.Out), "requires: { holds: table }", "left as written")
	s.Contains(string(out.Out), "holds: [captains-orders]", "records are not carrying")
	s.Contains(string(out.Out), "give: { holding: table, to: spoken_to }")
}

// TestAStatesPostAndAQuotedGuard: a state's holds with a door is a post;
// a guard fact quoted in a flow mapping converts inside its quotes.
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
	s.Contains(string(out.Out), `name: "holds.anyone:welcome"`)
	s.Contains(string(out.Out), "{on: topic, do:", "the beat named topic is unchanged")
	s.Len(out.Changes, 2)
}

// stripComments drops full-line comments and trailing comments, for
// comparing what a file says rather than what it remarks.
func stripComments(raw []byte) string {
	var b strings.Builder
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if i := strings.Index(line, " #"); i >= 0 {
			line = strings.TrimRight(line[:i], " ")
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}
