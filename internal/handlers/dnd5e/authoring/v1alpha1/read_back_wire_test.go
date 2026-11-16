package authoringv1alpha1_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter/dungeonspec/readback"

	authoringpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/authoring/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	authoringhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/authoring/v1alpha1"
	authoringorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/authoring"
)

// readBackCastleKey is the author's own copy of the castle the read-back
// probes grade: the shipped key is a reference and refuses a grade too.
const readBackCastleKey = "my-mad-king"

// readBackSmallKey is the small site the save probe stores, and the stand-in
// default of the suite's one-file shelf.
const readBackSmallKey = "creature-facts-walk"

// ReadBackWireSuite is the api half of rpg-project#967 (design R25): the
// check carries the ENGINE'S read-back of a file that compiles, and nothing
// of one that does not. Real handler, real orchestrator, real registry over
// the binary's one compiled shelf, so the sentences are the toolkit's own.
type ReadBackWireSuite struct {
	suite.Suite

	ctx     context.Context
	shipped *dungeons.FileRegistry
	castle  string
}

func TestReadBackWireSuite(t *testing.T) { suite.Run(t, new(ReadBackWireSuite)) }

// SetupSuite stages a one-file shelf: the registry refuses to construct
// without a default dungeon, and these probes grade under their own keys, so
// the smallest site stands in for it. Compiling the whole shelf (or the
// castle twice) would spend the CI test budget on nothing this suite reads.
func (s *ReadBackWireSuite) SetupSuite() {
	s.ctx = worldCtx("alice")

	small, err := os.ReadFile(dungeonstest.SitePath(s.T(), readBackSmallKey)) //nolint:gosec // repository fixture
	s.Require().NoError(err)
	dir := s.T().TempDir()
	s.Require().NoError(os.WriteFile(filepath.Join(dir, dungeons.DefaultKey+".yaml"),
		dungeonstest.AsKey(s.T(), small, readBackSmallKey, dungeons.DefaultKey), 0o600))
	s.shipped, err = dungeons.NewFileRegistry(dir, dungeonstest.Projector(s.T()))
	s.Require().NoError(err)

	path := filepath.Join(dungeonstest.ContentDir(s.T()), dungeons.DefaultKey+".yaml")
	raw, err := os.ReadFile(path) //nolint:gosec // the repository's shipped castle
	s.Require().NoError(err)
	s.castle = string(dungeonstest.AsKey(s.T(), raw, dungeons.DefaultKey, readBackCastleKey))
}

// handler is a handler over the shared shelf and a fresh world store;
// authoring decides whether a save writes.
func (s *ReadBackWireSuite) handler(authoring bool) *authoringhandler.Handler {
	s.T().Helper()

	orch, err := authoringorch.New(&authoringorch.Config{
		Dungeons: dungeonstest.Join(s.T(), s.shipped, authoring),
	})
	s.Require().NoError(err)
	h, err := authoringhandler.New(&authoringhandler.HandlerConfig{Orchestrator: orch})
	s.Require().NoError(err)

	return h
}

func (s *ReadBackWireSuite) grade(key, yaml string) *authoringpb.PutDungeonResponse {
	s.T().Helper()

	resp, err := s.handler(false).PutDungeon(s.ctx, &authoringpb.PutDungeonRequest{
		Key: key, Yaml: yaml, ValidateOnly: true,
	})
	s.Require().NoError(err, "a grade is never a status")

	return resp
}

// statements is every statement of the read-back, in reading order.
func statements(rb *authoringpb.ReadBack) []*authoringpb.ReadBackStatement {
	var out []*authoringpb.ReadBackStatement
	for _, sec := range rb.GetSections() {
		for _, g := range sec.GetGroups() {
			out = append(out, g.GetStatements()...)
		}
	}
	return out
}

// TestPutDungeon_ValidateOnly_CarriesReadBack: the castle's check carries
// the engine's sentences, each with its kind, subject, path and connections,
// and nothing the engine cannot yet say.
func (s *ReadBackWireSuite) TestPutDungeon_ValidateOnly_CarriesReadBack() {
	resp := s.grade(readBackCastleKey, s.castle)

	s.Require().Empty(resp.GetErrors(), "the shipped castle compiles")
	rb := resp.GetReadBack()
	s.Require().NotNil(rb, "a clean check carries the read-back")
	s.Equal(readBackCastleKey, rb.GetKey(), "the read-back names the file's own key")
	s.NotEmpty(rb.GetName())
	s.Empty(rb.GetNotYetSaid(), "the castle has nothing the engine cannot yet say")

	all := statements(rb)
	s.Require().NotEmpty(all)

	var captain *authoringpb.ReadBackStatement
	connected := false
	for _, st := range all {
		if len(st.GetConnects()) > 0 && st.GetConnects()[0].GetRole() != "" {
			connected = true
		}
		if st.GetAbout().GetKind() != "creature" || st.GetAbout().GetId() != "captain" {
			continue
		}
		if strings.HasPrefix(st.GetKind(), "routine") || strings.HasPrefix(st.GetKind(), "creature") {
			captain = st
		}
	}
	s.Require().NotNil(captain, "a statement about creature captain, of a routine or creature kind")
	s.NotEmpty(captain.GetSentence(), "the statement carries its sentence")
	s.NotEmpty(captain.GetPath(), "and the path where it is set")
	s.True(connected, "connections ride the wire with their roles")

	// The folded form rides beside the statements, and loses none of them:
	// every statement's path is carried by some folded item (the engine's
	// own guarantee, asserted at the wire, rpg-project#978).
	s.Require().NotEmpty(rb.GetFolded(), "the check carries the folded read-back")
	carried := map[string]bool{}
	for _, sec := range rb.GetFolded() {
		foldedPaths(sec.GetItems(), carried)
	}
	for _, st := range all {
		s.True(carried[st.GetPath()], "the statement at %q is reachable from a folded item", st.GetPath())
	}

	// The fold is the one-thing fold (rpg-project#980): the studio shows one
	// thing at a time, so no creature points at another's lines, the
	// captain's own lines are said under him, and how far a loudness carries
	// is said once per creature, never on each answer.
	troupe := false
	walkFolded(rb.GetFolded(), func(it *authoringpb.ReadBackFoldedItem) {
		s.NotContains(it.GetText(), "has the same", "a folded item points at another thing's lines")
		if strings.Contains(it.GetText(), "words reach everyone") {
			s.True(strings.HasPrefix(it.GetText(), "At `"),
				"only a loudness item explains what words reach: %q", it.GetText())
		}
		if strings.Contains(it.GetText(), "(`troupe`)") && carriesUnder(it, captainBindings) {
			troupe = true
		}
		said := map[string]bool{}
		for _, child := range it.GetItems() {
			loud, ok := loudnessOf(child.GetText())
			if !ok {
				continue
			}
			s.False(said[loud], "the loudness %q is explained twice under %q", loud, it.GetText())
			said[loud] = true
		}
	})
	s.True(troupe, "the captain's `troupe` line is said under him")

	s.waysOnTheWire(rb, all)
	s.hubsOnTheWire(rb, all)
	s.T().Logf("castle read-back: %d statements, %d folded sections", len(all), len(rb.GetFolded()))
}

// gateJoin is the castle's gate: the door between the courtyard and the
// gatehouse, held by the gate guards and the captain.
const gateJoin = "door:region-courtyard:region-gatehouse:gate-wall-door"

// waysOnTheWire: the castle's ways ride the wire (rpg-project#972). The
// gate join's `troupe` way names its two checks with their difficulties,
// and every way statement is the engine's, field for field: no small test
// site has a way with a difficulty, so the castle's ways are compared to
// the engine's own JSON of the same bytes, a dropped `value` failing here.
func (s *ReadBackWireSuite) waysOnTheWire(rb *authoringpb.ReadBack, all []*authoringpb.ReadBackStatement) {
	asJSON := protojson.MarshalOptions{UseProtoNames: true}
	var wire []any
	troupe := map[string]int32{}
	toolsWay := false
	for _, st := range all {
		if !strings.HasPrefix(st.GetKind(), "place.way.") {
			continue
		}
		raw, err := asJSON.Marshal(st)
		s.Require().NoError(err)
		wire = append(wire, s.decode(raw))

		if s.isKitchenToolsLock(st) {
			toolsWay = true
		}
		if !strings.HasSuffix(st.GetAbout().GetId(), ":line:captain:troupe") {
			continue
		}
		for _, c := range st.GetConnects() {
			if c.GetKind() == "join" {
				s.Equal(gateJoin, c.GetId(), "the captain's troupe way goes through the gate")
			}
			if c.GetKind() == "method" {
				troupe[c.GetId()] = c.GetValue()
				s.Equal(approachBy, c.GetRole(), "each of the troupe way's checks is an alternative")
			}
		}
	}
	s.Require().NotEmpty(wire, "the castle's read-back says its ways")
	s.True(toolsWay, "the kitchen door's lock way rolls Dexterity with thieves' tools as one approach")
	s.Equal(map[string]int32{"skill:performance": 13, "skill:deception": 15}, troupe,
		"the gate's troupe way names each check with its difficulty")

	engine, err := readback.Of([]byte(s.castle))
	s.Require().NoError(err)
	var want []any
	for _, st := range engine.Statements() {
		if !strings.HasPrefix(string(st.Kind), "place.way.") {
			continue
		}
		raw, err := json.Marshal(st)
		s.Require().NoError(err)
		want = append(want, s.decode(raw))
	}
	s.Equal(withoutEmpties(want), withoutEmpties(wire), "the way statements are the engine's, field for field")
}

const (
	// kitchenDoor is the castle's locked kitchen door.
	kitchenDoor = ":kitchen-wall-door"
	// thievesTools is the tool the kitchen lock is picked with.
	thievesTools = "tool:dnd5e:item:thieves-tools"
	// approachBy marks a check that is an alternative of its own; approachWith
	// marks a part of the approach before it (rpg-toolkit#2445).
	approachBy   = "by"
	approachWith = "with"
)

// isKitchenToolsLock reports whether st is the kitchen door's lock way that
// picks it with thieves' tools, and asserts the tool rides as a PART of the
// Dexterity approach: `{method, ability:dex, by, 13}` immediately followed by
// `{method, thieves-tools, with, 13}` (rpg-project#974). Found by its
// `through` join and its tool, never by index.
func (s *ReadBackWireSuite) isKitchenToolsLock(st *authoringpb.ReadBackStatement) bool {
	if st.GetKind() != "place.way.lock" {
		return false
	}
	through, tool := false, -1
	cs := st.GetConnects()
	for i, c := range cs {
		if c.GetKind() == "join" && c.GetRole() == "through" && strings.HasSuffix(c.GetId(), kitchenDoor) {
			through = true
		}
		if c.GetKind() == "method" && c.GetId() == thievesTools {
			tool = i
		}
	}
	if !through || tool < 0 {
		return false
	}
	s.Require().Positive(tool, "the tool follows the ability it is used with")
	dex, with := cs[tool-1], cs[tool]
	s.Equal([]any{"method", "ability:dex", approachBy, int32(13)},
		[]any{dex.GetKind(), dex.GetId(), dex.GetRole(), dex.GetValue()}, "the approach rolls Dexterity against 13")
	s.Equal([]any{"method", thievesTools, approachWith, int32(13)},
		[]any{with.GetKind(), with.GetId(), with.GetRole(), with.GetValue()}, "with thieves' tools, at the same difficulty")
	return true
}

// hubsOnTheWire: the read-back's Records and Secrets hubs ride the wire
// (rpg-project#973), and a way is said as a way exactly once. A way repeated
// under a hub is a pointer, never a way: the statements that go `through` a
// join are exactly the `place.way.*` statements, and only a way carries a
// `method`, so a consumer counting ways by either rule counts each once.
func (s *ReadBackWireSuite) hubsOnTheWire(rb *authoringpb.ReadBack, all []*authoringpb.ReadBackStatement) {
	s.Equal([]string{"record.tell"}, s.kindsAbout(rb, "record", "kitchen-way", "record.tell"),
		"the record kitchen-way has its own hub, saying who tells it")
	s.Equal([]string{"secret.reveal"}, s.kindsAbout(rb, "concealment", "kitchen-passage", "secret.reveal"),
		"the concealment kitchen-passage has its own hub, saying once what reveals it")

	ways, through := 0, 0
	for _, st := range all {
		isWay := strings.HasPrefix(st.GetKind(), "place.way.")
		if isWay {
			ways++
		}
		goesThrough := false
		for _, c := range st.GetConnects() {
			if c.GetKind() == "join" && c.GetRole() == "through" {
				goesThrough = true
			}
			if c.GetKind() == "method" {
				s.True(isWay, "only a way carries a method; %q at %q does", st.GetKind(), st.GetPath())
			}
		}
		if goesThrough {
			through++
			s.True(isWay, "only a way goes through a join; %q at %q does", st.GetKind(), st.GetPath())
		}
	}
	s.Equal(ways, through, "every way goes through a join, and nothing else does")
	s.T().Logf("castle ways: %d", ways)
}

// kindsAbout is the kinds equal to kind among the statements of the groups
// about {aboutKind, id}, and fails when there is no such group.
func (s *ReadBackWireSuite) kindsAbout(rb *authoringpb.ReadBack, aboutKind, id, kind string) []string {
	s.T().Helper()

	var out []string
	found := false
	for _, sec := range rb.GetSections() {
		for _, g := range sec.GetGroups() {
			if g.GetAbout().GetKind() != aboutKind || g.GetAbout().GetId() != id {
				continue
			}
			found = true
			for _, st := range g.GetStatements() {
				if st.GetKind() == kind {
					out = append(out, st.GetKind())
				}
			}
		}
	}
	s.True(found, "a group about {%s, %s}", aboutKind, id)
	return out
}

// captainBindings is where the castle sets the guard captain's own lines.
const captainBindings = "room.room.monsterBindings.captain."

// walkFolded calls fn on every folded item at every depth.
func walkFolded(secs []*authoringpb.ReadBackFoldedSection, fn func(*authoringpb.ReadBackFoldedItem)) {
	var walk func([]*authoringpb.ReadBackFoldedItem)
	walk = func(items []*authoringpb.ReadBackFoldedItem) {
		for _, it := range items {
			fn(it)
			walk(it.GetItems())
		}
	}
	for _, sec := range secs {
		walk(sec.GetItems())
	}
}

// carriesUnder reports whether the item carries a path under prefix.
func carriesUnder(it *authoringpb.ReadBackFoldedItem, prefix string) bool {
	for _, p := range it.GetPaths() {
		if strings.HasPrefix(p, prefix) {
			return true
		}
	}
	return false
}

// loudnessOf is the loudness a loudness item explains ("At `speak`, …").
func loudnessOf(text string) (string, bool) {
	rest, ok := strings.CutPrefix(text, "At `")
	if !ok {
		return "", false
	}
	loud, _, ok := strings.Cut(rest, "`")
	return loud, ok
}

// foldedPaths marks every path any item at any depth carries.
func foldedPaths(items []*authoringpb.ReadBackFoldedItem, into map[string]bool) {
	for _, it := range items {
		for _, p := range it.GetPaths() {
			into[p] = true
		}
		foldedPaths(it.GetItems(), into)
	}
}

// TestPutDungeon_Save_CarriesNoReadBack: the read-back rides the check
// alone (R25); a clean save is stored and answered with its map and no
// read-back. The same file's check carries one, so the nil is the save's.
func (s *ReadBackWireSuite) TestPutDungeon_Save_CarriesNoReadBack() {
	const authorKey = "my-creature-facts"

	raw, err := os.ReadFile(dungeonstest.SitePath(s.T(), readBackSmallKey)) //nolint:gosec // repository fixture
	s.Require().NoError(err)
	yaml := string(dungeonstest.AsKey(s.T(), raw, readBackSmallKey, authorKey))
	s.Require().NotNil(s.grade(authorKey, yaml).GetReadBack(), "the check of the same file carries one")

	h := s.handler(true)
	resp, err := h.PutDungeon(s.ctx, &authoringpb.PutDungeonRequest{Key: authorKey, Yaml: yaml})
	s.Require().NoError(err)
	s.Require().Empty(resp.GetErrors())
	s.Require().NotNil(resp.GetAtlas(), "a clean save is answered with its map")
	s.Nil(resp.GetReadBack(), "and no read-back")

	got, err := h.GetDungeon(s.ctx, &authoringpb.GetDungeonRequest{Key: authorKey})
	s.Require().NoError(err, "the save was stored")
	s.Equal(yaml, got.GetYaml())
}

// TestPutDungeon_Refused_CarriesNoReadBack pins the end behavior: a line
// that both speaks and acts is refused, and the refused check carries no
// read-back. It does not prove which guard holds it: without the registry's
// early return the read-back would refuse the same bytes and still be nil.
// A small site takes the break, so the castle compiles once in this suite.
func (s *ReadBackWireSuite) TestPutDungeon_Refused_CarriesNoReadBack() {
	const fixtureKey = "reference-npc-pacing"
	const authorKey = "my-npc-pacing"
	const line = "            say: Begin the pacing drill.\n"

	raw, err := os.ReadFile(dungeonstest.SitePath(s.T(), fixtureKey)) //nolint:gosec // repository fixture
	s.Require().NoError(err)
	yaml := string(dungeonstest.AsKey(s.T(), raw, fixtureKey, authorKey))
	s.Require().Contains(yaml, line, "the site no longer holds the drill line")
	s.Require().NotNil(s.grade(authorKey, yaml).GetReadBack(), "unbroken, the site's check carries a read-back")

	resp := s.grade(authorKey, strings.Replace(yaml, line, line+"            do: You start the drill.\n", 1))

	s.Require().NotEmpty(resp.GetErrors(), "a line with say and do is refused")
	s.Nil(resp.GetReadBack(), "a refusal carries no read-back")
	s.Nil(resp.GetAtlas())
}

// TestPutDungeon_OlderDialect_CarriesNoReadBack pins the end behavior: a
// version 2 file compiles, is answered with its map and no error, and
// carries no read-back. It does not prove the ErrNotASite branch: without
// it the failure branch would also answer nil (and log a warn).
func (s *ReadBackWireSuite) TestPutDungeon_OlderDialect_CarriesNoReadBack() {
	raw, err := os.ReadFile(dungeonstest.SitePath(s.T(), frontRoomKey)) //nolint:gosec // repository fixture
	s.Require().NoError(err)

	resp := s.grade(frontRoomCopyKey, string(dungeonstest.AsKey(s.T(), raw, frontRoomKey, frontRoomCopyKey)))

	s.Require().Empty(resp.GetErrors(), "the version 2 front room compiles")
	s.Require().NotNil(resp.GetAtlas(), "and is answered with its map")
	s.Nil(resp.GetReadBack(), "an older dialect carries no read-back")
}

// TestPutDungeon_ReadBackMapsFieldForField: the wire read-back is the
// toolkit's, every field. The toolkit's JSON names are the proto's field
// names, so the two encodings, empties dropped on both sides, are equal: a
// field the mapping drops, blanks, adds or reorders fails here. The small
// site has nothing unsaid, so not_yet_said is compared empty. The folded
// form is the engine's one-thing fold, FoldedEach(), which its JSON of the read-back does not
// carry, so it is compared on its own: an item blanked, dropped or
// reordered at any depth fails.
func (s *ReadBackWireSuite) TestPutDungeon_ReadBackMapsFieldForField() {
	raw, err := os.ReadFile(dungeonstest.SitePath(s.T(), readBackSmallKey)) //nolint:gosec // repository fixture
	s.Require().NoError(err)
	yaml := dungeonstest.AsKey(s.T(), raw, readBackSmallKey, "my-creature-facts")

	resp := s.grade("my-creature-facts", string(yaml))
	s.Require().NotNil(resp.GetReadBack())
	asJSON := protojson.MarshalOptions{UseProtoNames: true}
	folded := resp.GetReadBack().GetFolded()
	wireFolded, err := asJSON.Marshal(&authoringpb.ReadBack{Folded: folded})
	s.Require().NoError(err)
	resp.GetReadBack().Folded = nil
	wire, err := asJSON.Marshal(resp.GetReadBack())
	s.Require().NoError(err)

	engine, err := readback.Of(yaml)
	s.Require().NoError(err)
	want, err := json.Marshal(engine)
	s.Require().NoError(err)
	wantFolded, err := json.Marshal(map[string]any{"folded": engine.FoldedEach()})
	s.Require().NoError(err)

	s.Require().NotEmpty(folded, "the small site's check carries the folded read-back")
	s.Equal(withoutEmpties(s.decode(want)), withoutEmpties(s.decode(wire)))
	s.Equal(withoutEmpties(s.decode(wantFolded)), withoutEmpties(s.decode(wireFolded)))
}

func (s *ReadBackWireSuite) decode(raw []byte) any {
	s.T().Helper()

	var out any
	s.Require().NoError(json.Unmarshal(raw, &out))
	return out
}

// withoutEmpties drops null, "", empty lists and empty objects at every
// depth: protojson leaves out what the toolkit writes as null or omits, and
// neither side's empty is a fact.
func withoutEmpties(v any) any {
	switch t := v.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, e := range t {
			if e = withoutEmpties(e); e != nil {
				out[k] = e
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	case []any:
		out := make([]any, 0, len(t))
		for _, e := range t {
			out = append(out, withoutEmpties(e))
		}
		if len(out) == 0 {
			return nil
		}
		return out
	case string:
		if t == "" {
			return nil
		}
		return t
	default:
		return t
	}
}
