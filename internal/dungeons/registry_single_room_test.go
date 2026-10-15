package dungeons_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	"github.com/KirkDiggler/rpg-api/internal/repositories/worlddungeons"
)

// workshopKey is the full workshop fixture's own `key:` line.
const workshopKey = dungeonstest.WorkshopRoomKey

// SingleRoomRegistrySuite exercises the released v3 provider through the real
// file registry, on the shared authored workshop fixture (see dungeonstest's
// workshop.go and internal/dungeons/testdata). The fixture is deliberately
// presentation-rich and none of it is judged here any more: the registry
// stores the author's bytes verbatim and compiles only the gameplay keys out
// of them (rpg-project#479).
type SingleRoomRegistrySuite struct {
	suite.Suite
	ctx      context.Context
	dir      string
	registry dungeons.Registry
	world    worlddungeons.Repository
	raw      []byte
}

func TestSingleRoomRegistrySuite(t *testing.T) { suite.Run(t, new(SingleRoomRegistrySuite)) }

func (s *SingleRoomRegistrySuite) SetupTest() {
	s.ctx = context.Background()
	s.dir = dungeonstest.ScratchDir(s.T())
	s.world = dungeonstest.WorldStore(s.T())
	s.registry = s.open()
	s.raw = dungeonstest.WorkshopRoomYAML(s.T())
}

// open joins a fresh shipped store over s.dir with the suite's world store:
// what a restarted server would serve.
func (s *SingleRoomRegistrySuite) open() dungeons.Registry {
	shipped, err := dungeons.NewFileRegistry(s.dir, dungeonstest.Projector(s.T()))
	s.Require().NoError(err)
	r, err := dungeons.NewRegistry(&dungeons.RegistryConfig{
		Shipped: shipped, World: s.world, Authoring: true, Projector: dungeonstest.Projector(s.T()),
	})
	s.Require().NoError(err)
	return r
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
		Caller: dungeonstest.Caller, Key: workshopKey, YAML: s.raw, ValidateOnly: true,
	})
	s.Require().NoError(err)
	s.Empty(out.Errors, "the authored room compiles")
	s.Require().NotNil(out.Entry, "the compiled answer is still the body")

	_, err = s.registry.Get(s.ctx, dungeonstest.Caller, workshopKey)
	s.ErrorIs(err, dungeons.ErrNotFound, "validate-only must not create a registry entry")

	_, statErr := os.Stat(s.workshopFile())
	s.True(os.IsNotExist(statErr), "and no file was written either")
}

// TestSaveGetListAndFreshRegistryPreserveTheAuthoredRoom is the whole save
// loop against the full room: the exact original bytes (comments included)
// are stored and served, the metadata comes from the file's own lines, the
// projected atlas names the entry it was compiled from, and a FRESH registry
// over the same stores serves the same bytes, the same key and the same
// compiled floor.
func (s *SingleRoomRegistrySuite) TestSaveGetListAndFreshRegistryPreserveTheAuthoredRoom() {
	out, err := s.registry.Put(s.ctx, &dungeons.PutInput{Caller: dungeonstest.Caller, Key: workshopKey, YAML: s.raw})
	s.Require().NoError(err)
	s.Require().Empty(out.Errors)
	s.Require().NotNil(out.Entry)
	s.Equal(workshopKey, out.Entry.Key, "the key is the file's own key: line")
	s.Equal("Workshop", out.Entry.Name, "the name is the visual scene's own")
	s.Equal(s.raw, out.Entry.YAML, "stored verbatim, comments and spacing included")
	s.Equal(workshopKey, out.Entry.Atlas.DungeonKey,
		"the projected atlas names the entry it was compiled from -- what a client fetches the room's picture by")

	_, statErr := os.Stat(s.workshopFile())
	s.True(os.IsNotExist(statErr), "the save lands in the world store, never in the content directory")

	got, err := s.registry.Get(s.ctx, dungeonstest.Caller, workshopKey)
	s.Require().NoError(err)
	s.Equal(s.raw, got.YAML, "Get hands back the stored bytes, not a re-marshaled file")
	s.Equal(workshopKey, got.Atlas.DungeonKey)

	mine := dungeons.Summary{
		Key: workshopKey, Name: "Workshop", Owner: dungeonstest.Caller.PlayerID, Source: dungeons.SourceWorld,
	}
	list, err := s.registry.List(s.ctx, dungeonstest.Caller)
	s.Require().NoError(err)
	s.Contains(list, mine)

	// A fresh registry over the same stores — a restarted server — serves
	// the same entry: same bytes, same metadata, same key, same floor.
	reloaded := s.open()
	e, err := reloaded.Get(s.ctx, dungeonstest.Caller, workshopKey)
	s.Require().NoError(err)
	s.Equal("Workshop", e.Name)
	s.Equal(s.raw, e.YAML)
	s.Equal(workshopKey, e.Atlas.DungeonKey,
		"the fresh registry names the same dungeon the first one did")
	s.Equal(out.Entry.Atlas.Cells, e.Atlas.Cells, "and compiles the same floor from the same bytes")

	freshList, err := reloaded.List(s.ctx, dungeonstest.Caller)
	s.Require().NoError(err)
	s.Contains(freshList, mine)
}

// TestAFailingRealSaveKeepsThePreviousBytesAndEntry is the durability half:
// a REAL save (not validate-only, which would not persist anyway) of a file
// whose monster names a ref no rulebook definition has is refused, and the
// prior entry — its bytes, its metadata and its compiled atlas — survives
// untouched, as served and as stored alike.
func (s *SingleRoomRegistrySuite) TestAFailingRealSaveKeepsThePreviousBytesAndEntry() {
	_, err := s.registry.Put(s.ctx, &dungeons.PutInput{Caller: dungeonstest.Caller, Key: workshopKey, YAML: s.raw})
	s.Require().NoError(err)
	before, err := s.registry.Get(s.ctx, dungeonstest.Caller, workshopKey)
	s.Require().NoError(err)

	bad := bytes.Replace(s.raw, []byte("dnd5e:monsters:skeleton"), []byte("dnd5e:monsters:not-real"), 1)
	s.Require().NotEqual(s.raw, bad, "the fixture's monster ref must be where this test expects it")
	out, err := s.registry.Put(s.ctx, &dungeons.PutInput{Caller: dungeonstest.Caller, Key: workshopKey, YAML: bad})
	s.Require().NoError(err, "a file that does not compile is a body, not a status")
	s.Require().NotEmpty(out.Errors, "the unknown monster ref is the refusal")
	s.Nil(out.Entry, "and nothing was accepted")

	after, err := s.registry.Get(s.ctx, dungeonstest.Caller, workshopKey)
	s.Require().NoError(err, "the prior entry still answers")
	s.Equal(before.YAML, after.YAML, "the prior file bytes survive the failed save")
	s.Equal(before.Atlas.Cells, after.Atlas.Cells, "so does its compiled floor")
	s.Equal(workshopKey, after.Atlas.DungeonKey, "and its key")
	s.Equal("Workshop", after.Name, "and its metadata")

	stored, err := s.world.Get(s.ctx, &worlddungeons.GetInput{WorldID: dungeonstest.Caller.WorldID, Key: workshopKey})
	s.Require().NoError(err)
	s.Equal(s.raw, stored.Record.YAML, "the stored record keeps the prior bytes too")
}

// TestAnUnknownMonsterRefIsRefusedBeforeAnyEntryExists pins the preflight's
// place: content resolution happens at compile, BEFORE a dungeon is accepted,
// so a file whose monster names an unavailable ref never becomes a registry
// entry — not even a broken one a picker could list or a launch could start.
func (s *SingleRoomRegistrySuite) TestAnUnknownMonsterRefIsRefusedBeforeAnyEntryExists() {
	broken := bytes.Replace(s.raw, []byte("key: workshop-room"), []byte("key: broken-room"), 1)
	broken = bytes.Replace(broken, []byte("dnd5e:monsters:skeleton"), []byte("dnd5e:monsters:not-real"), 1)
	s.Require().NotEqual(s.raw, broken, "the fixture's key and ref lines must be where this test expects them")

	out, err := s.registry.Put(s.ctx, &dungeons.PutInput{Caller: dungeonstest.Caller, Key: "broken-room", YAML: broken})
	s.Require().NoError(err)
	s.Require().NotEmpty(out.Errors)
	s.Contains(out.Errors[0].Message,
		`monster "skeleton-a" references unknown monster "dnd5e:monsters:not-real"`,
		"the refusal names the placement and the ref")

	_, err = s.registry.Get(s.ctx, dungeonstest.Caller, "broken-room")
	s.ErrorIs(err, dungeons.ErrNotFound, "no entry was accepted")

	_, statErr := os.Stat(filepath.Join(s.dir, "broken-room.yaml"))
	s.True(os.IsNotExist(statErr), "and nothing was written under the new key either")
}
