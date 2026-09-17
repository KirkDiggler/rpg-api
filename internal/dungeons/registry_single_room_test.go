package dungeons_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"

	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"

	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
)

// workshopKey is the full workshop fixture's own `key:` line.
const workshopKey = dungeonstest.WorkshopRoomKey

// SingleRoomRegistrySuite exercises the released v3 provider through the real
// file registry, on the shared authored workshop fixture (see dungeonstest's
// workshop.go and internal/dungeons/testdata). The fixture is deliberately
// presentation-rich but uses only public asset references: the registry never
// reads model bytes.
type SingleRoomRegistrySuite struct {
	suite.Suite
	ctx      context.Context
	dir      string
	registry *dungeons.FileRegistry
	raw      []byte
}

func TestSingleRoomRegistrySuite(t *testing.T) { suite.Run(t, new(SingleRoomRegistrySuite)) }

func (s *SingleRoomRegistrySuite) SetupTest() {
	s.ctx = context.Background()
	s.registry, s.dir = dungeonstest.Scratch(s.T())
	s.raw = dungeonstest.WorkshopRoomYAML(s.T())
}

// decodeScene decodes one RoomSceneJSON into the toolkit's presentation type,
// so assertions compare DECODED SOURCE VALUES against the independently
// spelled expected graph rather than two strings of the same converter.
func (s *SingleRoomRegistrySuite) decodeScene(raw string) *tkencounter.RoomScenePresentation {
	s.T().Helper()
	var scene tkencounter.RoomScenePresentation
	s.Require().NoError(json.Unmarshal([]byte(raw), &scene))
	return &scene
}

// workshopFile is the registry file the full room is stored under.
func (s *SingleRoomRegistrySuite) workshopFile() string {
	return filepath.Join(s.dir, workshopKey+".yaml")
}

// TestValidateOnlyWritesNothing pins the authoring form's draft path: a
// validate-only Put compiles the whole room (it answers an entry) and writes
// NOTHING — no registry entry a Get can find, and no file on disk.
func (s *SingleRoomRegistrySuite) TestValidateOnlyWritesNothing() {
	out, err := s.registry.Put(s.ctx, &dungeons.PutInput{
		Key: workshopKey, YAML: s.raw, ValidateOnly: true,
	})
	s.Require().NoError(err)
	s.Empty(out.Errors, "the authored room compiles")
	s.Require().NotNil(out.Entry, "the compiled answer is still the body")

	_, err = s.registry.Get(s.ctx, workshopKey)
	s.ErrorIs(err, dungeons.ErrNotFound, "validate-only must not create a registry entry")

	_, statErr := os.Stat(s.workshopFile())
	s.True(os.IsNotExist(statErr), "and no file was written either")
}

// TestSaveGetListAndFreshRegistryPreserveTheAuthoredRoom is the whole save
// loop against the full room: the exact original bytes (comments included)
// are stored and served, the metadata comes from the file's own lines, the
// scene is the complete authored graph, and a FRESH registry over the saved
// directory serves the same bytes and the same decoded scene.
func (s *SingleRoomRegistrySuite) TestSaveGetListAndFreshRegistryPreserveTheAuthoredRoom() {
	out, err := s.registry.Put(s.ctx, &dungeons.PutInput{Key: workshopKey, YAML: s.raw})
	s.Require().NoError(err)
	s.Require().Empty(out.Errors)
	s.Require().NotNil(out.Entry)
	s.Equal(workshopKey, out.Entry.Key, "the key is the file's own key: line")
	s.Equal("Workshop", out.Entry.Name, "the name is the visual scene's own")
	s.Equal(s.raw, out.Entry.YAML, "stored verbatim, comments and spacing included")
	s.NotEmpty(out.Entry.Atlas.RoomSceneJSON, "a v3 room carries its scene")
	s.Equal(dungeonstest.WorkshopRoomScene(), *s.decodeScene(out.Entry.Atlas.RoomSceneJSON),
		"PutDungeon's scene is the complete authored graph")

	onDisk, err := os.ReadFile(s.workshopFile())
	s.Require().NoError(err)
	s.Equal(s.raw, onDisk, "and the disk file is exactly the bytes the author sent")

	got, err := s.registry.Get(s.ctx, workshopKey)
	s.Require().NoError(err)
	s.Equal(s.raw, got.YAML, "Get hands back the stored bytes, not a re-marshaled file")
	s.Equal(out.Entry.Atlas.RoomSceneJSON, got.Atlas.RoomSceneJSON)

	list, err := s.registry.List(s.ctx)
	s.Require().NoError(err)
	s.Contains(list, dungeons.Summary{Key: workshopKey, Name: "Workshop"})

	// A fresh registry over the saved directory — a restarted server — serves
	// the same entry: same bytes, same metadata, same decoded scene.
	reloaded, err := dungeons.NewFileRegistry(s.dir, false, dungeonstest.Projector(s.T()))
	s.Require().NoError(err)
	e, err := reloaded.Get(s.ctx, workshopKey)
	s.Require().NoError(err)
	s.Equal("Workshop", e.Name)
	s.Equal(s.raw, e.YAML)
	s.Equal(out.Entry.Atlas.RoomSceneJSON, e.Atlas.RoomSceneJSON)
	s.Equal(dungeonstest.WorkshopRoomScene(), *s.decodeScene(e.Atlas.RoomSceneJSON),
		"the fresh registry's scene is the complete authored graph, not a thumbnail")

	freshList, err := reloaded.List(s.ctx)
	s.Require().NoError(err)
	s.Contains(freshList, dungeons.Summary{Key: workshopKey, Name: "Workshop"})
}

// TestAFailingRealSaveKeepsThePreviousBytesAndEntry is the durability half:
// a REAL save (not validate-only, which would not persist anyway) of a file
// whose monster names a ref no rulebook definition has is refused, and the
// prior entry — its bytes, its metadata and its scene — survives untouched,
// on the in-memory registry and on disk alike.
func (s *SingleRoomRegistrySuite) TestAFailingRealSaveKeepsThePreviousBytesAndEntry() {
	_, err := s.registry.Put(s.ctx, &dungeons.PutInput{Key: workshopKey, YAML: s.raw})
	s.Require().NoError(err)
	before, err := s.registry.Get(s.ctx, workshopKey)
	s.Require().NoError(err)

	bad := bytes.Replace(s.raw, []byte("dnd5e:monsters:skeleton"), []byte("dnd5e:monsters:not-real"), 1)
	s.Require().NotEqual(s.raw, bad, "the fixture's monster ref must be where this test expects it")
	out, err := s.registry.Put(s.ctx, &dungeons.PutInput{Key: workshopKey, YAML: bad})
	s.Require().NoError(err, "a file that does not compile is a body, not a status")
	s.Require().NotEmpty(out.Errors, "the unknown monster ref is the refusal")
	s.Nil(out.Entry, "and nothing was accepted")

	after, err := s.registry.Get(s.ctx, workshopKey)
	s.Require().NoError(err, "the prior entry still answers")
	s.Equal(before.YAML, after.YAML, "the prior file bytes survive the failed save")
	s.Equal(before.Atlas.RoomSceneJSON, after.Atlas.RoomSceneJSON, "so does its scene")
	s.Equal("Workshop", after.Name, "and its metadata")

	onDisk, err := os.ReadFile(s.workshopFile())
	s.Require().NoError(err)
	s.Equal(s.raw, onDisk, "the on-disk file keeps the prior bytes too")
}

// TestAnUnknownMonsterRefIsRefusedBeforeAnyEntryExists pins the preflight's
// place: content resolution happens at compile, BEFORE a dungeon is accepted,
// so a file whose monster names an unavailable ref never becomes a registry
// entry — not even a broken one a picker could list or a launch could start.
func (s *SingleRoomRegistrySuite) TestAnUnknownMonsterRefIsRefusedBeforeAnyEntryExists() {
	broken := bytes.Replace(s.raw, []byte("key: workshop-room"), []byte("key: broken-room"), 1)
	broken = bytes.Replace(broken, []byte("dnd5e:monsters:skeleton"), []byte("dnd5e:monsters:not-real"), 1)
	s.Require().NotEqual(s.raw, broken, "the fixture's key and ref lines must be where this test expects them")

	out, err := s.registry.Put(s.ctx, &dungeons.PutInput{Key: "broken-room", YAML: broken})
	s.Require().NoError(err)
	s.Require().NotEmpty(out.Errors)
	s.Contains(out.Errors[0].Message,
		`monster "skeleton-a" references unknown monster "dnd5e:monsters:not-real"`,
		"the refusal names the placement and the ref")

	_, err = s.registry.Get(s.ctx, "broken-room")
	s.ErrorIs(err, dungeons.ErrNotFound, "no entry was accepted")

	_, statErr := os.Stat(filepath.Join(s.dir, "broken-room.yaml"))
	s.True(os.IsNotExist(statErr), "and nothing was written under the new key either")
}
