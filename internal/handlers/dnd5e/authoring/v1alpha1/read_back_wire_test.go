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
	s.T().Logf("castle read-back: %d statements", len(all))
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
// site has nothing unsaid, so not_yet_said is compared empty.
func (s *ReadBackWireSuite) TestPutDungeon_ReadBackMapsFieldForField() {
	raw, err := os.ReadFile(dungeonstest.SitePath(s.T(), readBackSmallKey)) //nolint:gosec // repository fixture
	s.Require().NoError(err)
	yaml := dungeonstest.AsKey(s.T(), raw, readBackSmallKey, "my-creature-facts")

	resp := s.grade("my-creature-facts", string(yaml))
	s.Require().NotNil(resp.GetReadBack())
	wire, err := protojson.MarshalOptions{UseProtoNames: true}.Marshal(resp.GetReadBack())
	s.Require().NoError(err)

	engine, err := readback.Of(yaml)
	s.Require().NoError(err)
	want, err := json.Marshal(engine)
	s.Require().NoError(err)

	s.Equal(withoutEmpties(s.decode(want)), withoutEmpties(s.decode(wire)))
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
