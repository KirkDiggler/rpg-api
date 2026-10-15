package dungeons_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/suite"

	tkdungeonspec "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter/dungeonspec"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	"github.com/KirkDiggler/rpg-api/internal/repositories/worlddungeons"
)

// shippedTomb is the real content file, so these tests run on what the server
// boots with rather than a fixture that could drift from it.
var shippedTomb = filepath.Join("..", "..", "content", "reference-tomb.yaml")

type RegistrySuite struct {
	suite.Suite

	ctx   context.Context
	dir   string
	tomb  []byte
	world worlddungeons.Repository
}

func TestRegistrySuite(t *testing.T) {
	suite.Run(t, new(RegistrySuite))
}

func (s *RegistrySuite) SetupTest() {
	s.ctx = context.Background()
	s.dir = s.T().TempDir()

	raw, err := os.ReadFile(shippedTomb)
	s.Require().NoError(err)
	s.tomb = raw
	s.write("reference-tomb.yaml", raw)
	s.world = dungeonstest.WorldStore(s.T())
}

func (s *RegistrySuite) write(name string, raw []byte) {
	s.Require().NoError(os.WriteFile(filepath.Join(s.dir, name), raw, 0o600))
}

// rekeyed is the tomb under another key: the same dungeon, a different file.
func (s *RegistrySuite) rekeyed(key string) []byte {
	out := bytes.Replace(s.tomb, []byte("key: reference-tomb"), []byte("key: "+key), 1)
	s.Require().NotEqual(s.tomb, out, "the fixture's key line must be where this helper expects it")
	return out
}

func (s *RegistrySuite) open() *dungeons.FileRegistry {
	r, err := dungeons.NewFileRegistry(s.dir, dungeonstest.Projector(s.T()))
	s.Require().NoError(err)
	return r
}

// library joins the shipped store over s.dir with the suite's world store,
// as dungeonstest.Caller sees it.
func (s *RegistrySuite) library(authoring bool) dungeons.Registry {
	return s.libraryWith(authoring, dungeonstest.Projector(s.T()))
}

func (s *RegistrySuite) libraryWith(authoring bool, projector dungeons.AtlasProjector) dungeons.Registry {
	shipped, err := dungeons.NewFileRegistry(s.dir, projector)
	s.Require().NoError(err)
	r, err := dungeons.NewRegistry(&dungeons.RegistryConfig{
		Shipped: shipped, World: s.world, Authoring: authoring, Projector: projector,
	})
	s.Require().NoError(err)
	return r
}

// dirNames is the content directory's listing: a request never changes it.
func (s *RegistrySuite) dirNames() []string {
	entries, err := os.ReadDir(s.dir)
	s.Require().NoError(err)
	names := make([]string, 0, len(entries))
	for _, d := range entries {
		names = append(names, d.Name())
	}
	return names
}

func (s *RegistrySuite) TestBoot_LoadsTheShippedTomb() {
	r := s.open()

	list, err := r.List(s.ctx)
	s.Require().NoError(err)
	s.Equal([]dungeons.Summary{{Key: "reference-tomb", Name: "The Reference Tomb", Source: dungeons.SourceShipped}}, list)

	e, err := r.Get(s.ctx, dungeons.DefaultKey)
	s.Require().NoError(err)
	s.Equal(s.tomb, e.YAML, "Get returns the bytes on disk, verbatim")
	s.Require().NotNil(e.Dungeon)
	s.NotEmpty(e.Dungeon.PartySeats, "and the compiled world behind them")
	s.Require().NotNil(e.Atlas, "and the atlas a session on it would serve")
	s.Len(e.Atlas.Cells, 224, "the tomb's 224 cells (design §5)")
	s.Len(e.Atlas.Regions, 3, "entrance, hall, tomb")
	s.Equal("crypt", e.Atlas.Regions[0].Archetype)
}

// TestBoot_RefusesAFileThatDoesNotCompile is the construction-time law: a
// broken file fails the boot naming itself, rather than vanishing from the
// picker.
func (s *RegistrySuite) TestBoot_RefusesAFileThatDoesNotCompile() {
	s.write("broken.yaml", []byte("version: 2\nkey: broken\nvoid: nonsense\n"))

	_, err := dungeons.NewFileRegistry(s.dir, dungeonstest.Projector(s.T()))
	s.Require().Error(err)
	s.Contains(err.Error(), filepath.Join(s.dir, "broken.yaml"), "the error names the file")
	s.Contains(err.Error(), "does not compile")
}

func (s *RegistrySuite) TestBoot_RefusesAFileWhoseNameIsNotItsKey() {
	s.write("not-the-key.yaml", s.rekeyed("something-else"))

	_, err := dungeons.NewFileRegistry(s.dir, dungeonstest.Projector(s.T()))
	s.Require().ErrorIs(err, dungeons.ErrKeyMismatch)
	s.Contains(err.Error(), "not-the-key.yaml")
}

func (s *RegistrySuite) TestBoot_RefusesADirectoryWithoutTheDefaultDungeon() {
	s.Require().NoError(os.Remove(filepath.Join(s.dir, "reference-tomb.yaml")))
	s.write("other.yaml", s.rekeyed("other"))

	_, err := dungeons.NewFileRegistry(s.dir, dungeonstest.Projector(s.T()))
	s.Require().Error(err)
	s.Contains(err.Error(), dungeons.DefaultKey)
}

func (s *RegistrySuite) TestBoot_IgnoresFilesThatAreNotYAML() {
	s.write("README.md", []byte("not a dungeon"))
	s.write(".reference-tomb.123.tmp", []byte("a leftover temp file"))

	s.open()
}

// countingProjector stands in for the session Manager's AtlasOf: it records
// how many worlds it was asked to project and under which keys, and answers a
// recognizable atlas, or fails on demand.
type countingProjector struct {
	calls int
	keys  []string
	fail  error
}

func (p *countingProjector) AtlasOf(
	_ context.Context, key string, dungeon *tkdungeonspec.Compiled,
) (*sdk.Atlas, error) {
	p.calls++
	p.keys = append(p.keys, key)
	if p.fail != nil {
		return nil, p.fail
	}
	if dungeon == nil {
		return nil, errors.New("nil dungeon")
	}
	return &sdk.Atlas{Grid: sdk.GridHex, Cells: []spatial.Position{{X: 1, Y: 2}}, DungeonKey: key}, nil
}

// TestProjector_EveryEntryCarriesTheAtlasTheSessionWouldServe: the atlas on
// an entry comes from the projector, once per compile, at boot and at Put.
func (s *RegistrySuite) TestProjector_EveryEntryCarriesTheAtlasTheSessionWouldServe() {
	p := &countingProjector{}
	r := s.libraryWith(true, p)
	s.Equal(1, p.calls, "the shipped tomb was projected once at boot")
	s.Equal([]string{dungeons.DefaultKey}, p.keys,
		"and it was projected under its own key -- the projector cannot derive one from a world")

	e, err := r.Get(s.ctx, dungeonstest.Caller, dungeons.DefaultKey)
	s.Require().NoError(err)
	s.Require().NotNil(e.Atlas)
	s.Equal([]spatial.Position{{X: 1, Y: 2}}, e.Atlas.Cells)
	s.Equal(dungeons.DefaultKey, e.Atlas.DungeonKey, "so the entry's atlas names the entry")

	res, err := r.Put(s.ctx, &dungeons.PutInput{Caller: dungeonstest.Caller, Key: "crypt", YAML: s.rekeyed("crypt"), ValidateOnly: true})
	s.Require().NoError(err)
	s.Require().NotNil(res.Entry.Atlas, "validate_only still answers the atlas — it is the builder's preview")
	s.Equal(2, p.calls)
	s.Equal("crypt", p.keys[1], "a Put projects under the key being saved, not the one already loaded")
	s.Equal("crypt", res.Entry.Atlas.DungeonKey,
		"so a builder previewing a draft fetches its scene by the same key a player will")
}

// TestProjector_AWorldThatWillNotLoadIsNotTheAuthorsProblem: a compiled file
// whose world the session refuses is an error (boot refusal / Internal), not
// a FieldError — the stack disagreed with itself, the author did nothing.
func (s *RegistrySuite) TestProjector_AWorldThatWillNotLoadIsNotTheAuthorsProblem() {
	_, err := dungeons.NewFileRegistry(s.dir, &countingProjector{fail: errors.New("invalid world")})
	s.Require().Error(err)
	s.Contains(err.Error(), "project atlas")
	s.Contains(err.Error(), "reference-tomb.yaml")

	_, err = dungeons.NewFileRegistry(s.dir, nil)
	s.Require().Error(err, "a registry without a projector cannot answer PutDungeon and is refused")
	_, err = dungeons.NewRegistry(&dungeons.RegistryConfig{Shipped: s.open(), World: s.world})
	s.Require().Error(err, "nor can the joined registry")
}

func (s *RegistrySuite) TestGet_UnknownKeyIsNotFound() {
	r := s.open()

	_, err := r.Get(s.ctx, "nope")
	s.Require().ErrorIs(err, dungeons.ErrNotFound)
}

func (s *RegistrySuite) TestPut_ValidateOnlyNeverWrites() {
	r := s.library(true)

	res, err := r.Put(s.ctx, &dungeons.PutInput{Caller: dungeonstest.Caller, Key: "crypt", YAML: s.rekeyed("crypt"), ValidateOnly: true})
	s.Require().NoError(err)
	s.Empty(res.Errors)
	s.Require().NotNil(res.Entry, "the compiled entry is answered")

	_, statErr := os.Stat(filepath.Join(s.dir, "crypt.yaml"))
	s.True(os.IsNotExist(statErr), "nothing reached the disk")
	_, getErr := r.Get(s.ctx, dungeonstest.Caller, "crypt")
	s.ErrorIs(getErr, dungeons.ErrNotFound, "and nothing reached the registry")
}

func (s *RegistrySuite) TestPut_WritesThenGetReturnsTheBytesUnchanged() {
	r := s.library(true)
	raw := append(s.rekeyed("crypt"), []byte("# a trailing comment the author wrote\n")...)

	res, err := r.Put(s.ctx, &dungeons.PutInput{Caller: dungeonstest.Caller, Key: "crypt", YAML: raw})
	s.Require().NoError(err)
	s.Empty(res.Errors)

	e, err := r.Get(s.ctx, dungeonstest.Caller, "crypt")
	s.Require().NoError(err)
	s.Equal(raw, e.YAML, "verbatim, comment included — never a re-marshal")

	s.Equal(dungeons.SourceWorld, e.Source)
	s.Equal(dungeonstest.Caller.PlayerID, e.Owner)
	s.Equal([]string{"reference-tomb.yaml"}, s.dirNames(), "the save landed in the world store, not the directory")

	list, err := r.List(s.ctx, dungeonstest.Caller)
	s.Require().NoError(err)
	s.Equal([]dungeons.Summary{{
		Key: "crypt", Name: "The Reference Tomb", Owner: dungeonstest.Caller.PlayerID, Source: dungeons.SourceWorld,
	}}, list, "the world's record; the version 2 tomb is not listed (R23)")

	// A fresh registry over the same world store sees the write: the store
	// is the truth.
	again := s.library(false)
	e2, err := again.Get(s.ctx, dungeonstest.Caller, "crypt")
	s.Require().NoError(err)
	s.Equal(raw, e2.YAML)
}

// TestGet_ReturnsACopyNotTheRegistrysOwnState: mutating what Get (or Put)
// handed back changes nothing the registry serves — Copilot's review point
// on PR #820, kept as a test rather than a comment.
func (s *RegistrySuite) TestGet_ReturnsACopyNotTheRegistrysOwnState() {
	r := s.library(true)
	buf := s.rekeyed("crypt")
	res, err := r.Put(s.ctx, &dungeons.PutInput{Caller: dungeonstest.Caller, Key: "crypt", YAML: buf})
	s.Require().NoError(err)
	buf[0] = '#'
	res.Entry.Key = "tampered"
	res.Entry.YAML[1] = '#'

	got, err := r.Get(s.ctx, dungeonstest.Caller, "crypt")
	s.Require().NoError(err)
	s.Equal("crypt", got.Key)
	s.Equal(s.rekeyed("crypt"), got.YAML, "neither the caller's buffer nor the returned entry reaches the registry")

	got.YAML[2] = '#'
	again, err := r.Get(s.ctx, dungeonstest.Caller, "crypt")
	s.Require().NoError(err)
	s.Equal(s.rekeyed("crypt"), again.YAML)
}

func (s *RegistrySuite) TestPut_AFileThatDoesNotCompileIsAnAnswerNotAnError() {
	r := s.library(true)

	res, err := r.Put(s.ctx, &dungeons.PutInput{Caller: dungeonstest.Caller, Key: "crypt", YAML: []byte("version: 2\nkey: crypt\nvoid: nonsense\n")})
	s.Require().NoError(err, "a bad file is the author's problem, reported as a body")
	s.Require().NotEmpty(res.Errors)
	s.NotEmpty(res.Errors[0].Message)
	s.Nil(res.Entry)

	_, statErr := os.Stat(filepath.Join(s.dir, "crypt.yaml"))
	s.True(os.IsNotExist(statErr), "nothing was written")
}

// TestPut_ErrorsNameTheThingTheyAreAbout pins the path contract the builder
// draws from: each defect arrives with the YAML path it is about, and a file
// with several defects answers all of them at once.
func (s *RegistrySuite) TestPut_ErrorsNameTheThingTheyAreAbout() {
	r := s.library(true)

	// start on void, intensity out of range: two defects, two paths.
	raw := bytes.Replace(s.rekeyed("crypt"), []byte("start: [1, 3]"), []byte("start: [99, 99]"), 1)
	raw = bytes.Replace(raw, []byte("intensity: 0.6"), []byte("intensity: 1.2"), 1)
	s.Require().NotEqual(s.rekeyed("crypt"), raw)

	res, err := r.Put(s.ctx, &dungeons.PutInput{Caller: dungeonstest.Caller, Key: "crypt", YAML: raw, ValidateOnly: true})
	s.Require().NoError(err)
	paths := make([]string, 0, len(res.Errors))
	for _, fe := range res.Errors {
		s.NotEmpty(fe.Message)
		paths = append(paths, fe.Path)
	}
	s.Contains(paths, "start")
	s.Contains(paths, "regions[0].lighting.intensity")
	s.GreaterOrEqual(len(paths), 2, "every defect, not the first: %v", res.Errors)

	// An unknown key names ITSELF. It used to be a decode defect carrying a
	// `line N` and the name of a Go type; encounter v0.94.1 walks the shape
	// instead (rpg-project#481 slice 1), so the one grammar holds for every
	// refusal the builder renders.
	//
	// The list of legal keys is deliberately NOT pinned: it grows every time
	// the dialect gains a field, and a test that fails on new content says
	// nothing about this one. What is pinned is what the fix was about — the
	// path is the key, and nothing machine-facing survives in the sentence.
	const misspelledKey = "hieght" //nolint:misspell // the typo IS the test
	res, err = r.Put(s.ctx, &dungeons.PutInput{
		Caller: dungeonstest.Caller, Key: "crypt", YAML: append(s.rekeyed("crypt"), []byte(misspelledKey+": 8\n")...), ValidateOnly: true,
	})
	s.Require().NoError(err)
	s.Require().Len(res.Errors, 1)
	s.Equal(misspelledKey, res.Errors[0].Path)
	s.Contains(res.Errors[0].Message, `"`+misspelledKey+`"`, "the refusal quotes the key the author typed")
	s.NotContains(res.Errors[0].Message, "line ", "no line number")
	s.NotContains(res.Errors[0].Message, "dungeonspec.", "and no Go type name")
}

func (s *RegistrySuite) TestPut_KeyMustEqualTheFilesKey() {
	r := s.library(true)

	_, err := r.Put(s.ctx, &dungeons.PutInput{Caller: dungeonstest.Caller, Key: "crypt", YAML: s.tomb})
	s.Require().ErrorIs(err, dungeons.ErrKeyMismatch)

	_, statErr := os.Stat(filepath.Join(s.dir, "crypt.yaml"))
	s.True(os.IsNotExist(statErr))
}

func (s *RegistrySuite) TestPut_RefusesAKeyOutsideTheCharset() {
	r := s.library(true)

	for _, key := range []string{"", "Crypt", "my crypt", "../escape", "crypt.yaml"} {
		_, err := r.Put(s.ctx, &dungeons.PutInput{Caller: dungeonstest.Caller, Key: key, YAML: s.tomb})
		s.ErrorIsf(err, dungeons.ErrInvalidKey, "key %q", key)
	}
}

func (s *RegistrySuite) TestPut_RefusedWhenAuthoringIsOff() {
	r := s.library(false)

	_, err := r.Put(s.ctx, &dungeons.PutInput{Caller: dungeonstest.Caller, Key: "crypt", YAML: s.rekeyed("crypt")})
	s.Require().ErrorIs(err, dungeons.ErrAuthoringDisabled)

	_, statErr := os.Stat(filepath.Join(s.dir, "crypt.yaml"))
	s.True(os.IsNotExist(statErr), "a read-only registry never touches the directory")
}

// TestPut_ValidateOnlyIsAnsweredWhenAuthoringIsOff is the gate's scope
// (rpg-project#481). The refusal above is the SAVE's; a grade writes nothing,
// so the builder's per-edit preview is answered by a registry that may not
// store a byte — with the engine's own defects for a broken file and the map
// the game would play for a good one.
func (s *RegistrySuite) TestPut_ValidateOnlyIsAnsweredWhenAuthoringIsOff() {
	r := s.library(false)

	// A broken file: the engine's defects, not ErrAuthoringDisabled.
	broken := bytes.Replace(s.rekeyed("crypt"), []byte("start: [1, 3]"), []byte("start: [99, 99]"), 1)
	s.Require().NotEqual(s.rekeyed("crypt"), broken, "the fixture's start line must be where this test expects it")

	res, err := r.Put(s.ctx, &dungeons.PutInput{Caller: dungeonstest.Caller, Key: "crypt", YAML: broken, ValidateOnly: true})
	s.Require().NoError(err, "a read-only registry grades the file rather than refusing it")
	s.Require().NotEmpty(res.Errors)
	paths := make([]string, 0, len(res.Errors))
	for _, fe := range res.Errors {
		s.NotEmpty(fe.Message)
		paths = append(paths, fe.Path)
	}
	s.Contains(paths, "start")

	// A good file: the atlas, same as a writable registry answers.
	res, err = r.Put(s.ctx, &dungeons.PutInput{Caller: dungeonstest.Caller, Key: "crypt", YAML: s.rekeyed("crypt"), ValidateOnly: true})
	s.Require().NoError(err)
	s.Empty(res.Errors)
	s.Require().NotNil(res.Entry)
	s.Require().NotNil(res.Entry.Atlas, "the grade carries the map a session on this file would serve")
	s.Equal("crypt", res.Entry.Atlas.DungeonKey)

	// And neither grade left a trace: no file, no entry.
	_, statErr := os.Stat(filepath.Join(s.dir, "crypt.yaml"))
	s.True(os.IsNotExist(statErr), "a grade writes nothing")
	_, err = r.Get(s.ctx, dungeonstest.Caller, "crypt")
	s.Require().ErrorIs(err, dungeons.ErrNotFound, "and registers nothing")
}

func (s *RegistrySuite) TestPut_OverwritesInPlaceAndNeverWritesTheDirectory() {
	r := s.library(true)
	first := s.rekeyed("crypt")
	second := append(s.rekeyed("crypt"), []byte("# second\n")...)

	_, err := r.Put(s.ctx, &dungeons.PutInput{Caller: dungeonstest.Caller, Key: "crypt", YAML: first})
	s.Require().NoError(err)
	_, err = r.Put(s.ctx, &dungeons.PutInput{Caller: dungeonstest.Caller, Key: "crypt", YAML: second})
	s.Require().NoError(err)

	e, err := r.Get(s.ctx, dungeonstest.Caller, "crypt")
	s.Require().NoError(err)
	s.Equal(second, e.YAML)

	s.Equal([]string{"reference-tomb.yaml"}, s.dirNames(), "a save never writes the content directory")
}

// TestPut_ConcurrentPutsOnOneKeySerialise runs under -race: N writers on one
// key must land whole, one after another, and the final state must be one of
// the files written — never a splice of two.
func (s *RegistrySuite) TestPut_ConcurrentPutsOnOneKeySerialise() {
	r := s.library(true)
	const writers = 16

	variants := make([][]byte, writers)
	for i := range variants {
		variants[i] = append(s.rekeyed("crypt"), []byte(fmt.Sprintf("# writer %d\n", i))...)
	}

	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(raw []byte) {
			defer wg.Done()
			res, err := r.Put(s.ctx, &dungeons.PutInput{Caller: dungeonstest.Caller, Key: "crypt", YAML: raw})
			if err != nil {
				errs <- err
				return
			}
			if len(res.Errors) > 0 {
				errs <- fmt.Errorf("unexpected compile errors: %v", res.Errors)
			}
		}(variants[i])
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		s.Require().NoError(err)
	}

	e, err := r.Get(s.ctx, dungeonstest.Caller, "crypt")
	s.Require().NoError(err)
	stored := e.YAML

	whole := false
	for _, v := range variants {
		if bytes.Equal(v, stored) {
			whole = true
			break
		}
	}
	s.True(whole, "the stored file is exactly one writer's file, not a splice")
}
