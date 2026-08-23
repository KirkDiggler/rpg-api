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

	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

	"github.com/KirkDiggler/rpg-api/internal/dungeons"
)

// shippedTomb is the real content file, so these tests run on what the server
// boots with rather than a fixture that could drift from it.
var shippedTomb = filepath.Join("..", "..", "content", "reference-tomb.yaml")

type RegistrySuite struct {
	suite.Suite

	ctx  context.Context
	dir  string
	tomb []byte
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

func (s *RegistrySuite) open(authoring bool) *dungeons.FileRegistry {
	r, err := dungeons.NewFileRegistry(s.dir, authoring, nil)
	s.Require().NoError(err)
	return r
}

func (s *RegistrySuite) TestBoot_LoadsTheShippedTomb() {
	r := s.open(false)

	list, err := r.List(s.ctx)
	s.Require().NoError(err)
	s.Equal([]dungeons.Summary{{Key: "reference-tomb", Name: "The Reference Tomb"}}, list)

	e, err := r.Get(s.ctx, dungeons.DefaultKey)
	s.Require().NoError(err)
	s.Equal(s.tomb, e.YAML, "Get returns the bytes on disk, verbatim")
	s.Require().NotNil(e.Dungeon)
	s.NotEmpty(e.Dungeon.PartySeats, "and the compiled world behind them")
}

// TestBoot_RefusesAFileThatDoesNotCompile is the construction-time law: a
// broken file fails the boot naming itself, rather than vanishing from the
// picker.
func (s *RegistrySuite) TestBoot_RefusesAFileThatDoesNotCompile() {
	s.write("broken.yaml", []byte("version: 2\nkey: broken\nvoid: nonsense\n"))

	_, err := dungeons.NewFileRegistry(s.dir, false, nil)
	s.Require().Error(err)
	s.Contains(err.Error(), filepath.Join(s.dir, "broken.yaml"), "the error names the file")
	s.Contains(err.Error(), "does not compile")
}

func (s *RegistrySuite) TestBoot_RefusesAFileWhoseNameIsNotItsKey() {
	s.write("not-the-key.yaml", s.rekeyed("something-else"))

	_, err := dungeons.NewFileRegistry(s.dir, false, nil)
	s.Require().ErrorIs(err, dungeons.ErrKeyMismatch)
	s.Contains(err.Error(), "not-the-key.yaml")
}

func (s *RegistrySuite) TestBoot_RefusesADirectoryWithoutTheDefaultDungeon() {
	s.Require().NoError(os.Remove(filepath.Join(s.dir, "reference-tomb.yaml")))
	s.write("other.yaml", s.rekeyed("other"))

	_, err := dungeons.NewFileRegistry(s.dir, false, nil)
	s.Require().Error(err)
	s.Contains(err.Error(), dungeons.DefaultKey)
}

func (s *RegistrySuite) TestBoot_IgnoresFilesThatAreNotYAML() {
	s.write("README.md", []byte("not a dungeon"))
	s.write(".reference-tomb.123.tmp", []byte("a leftover temp file"))

	s.open(false)
}

// countingProjector stands in for the session Manager's AtlasOf: it records
// how many worlds it was asked to project and answers a recognizable atlas,
// or fails on demand.
type countingProjector struct {
	calls int
	fail  error
}

func (p *countingProjector) AtlasOf(_ context.Context, world *tkencounter.EncounterData) (*sdk.Atlas, error) {
	p.calls++
	if p.fail != nil {
		return nil, p.fail
	}
	if world == nil {
		return nil, errors.New("nil world")
	}
	return &sdk.Atlas{Grid: sdk.GridHex, Cells: []spatial.Position{{X: 1, Y: 2}}}, nil
}

// TestProjector_EveryEntryCarriesTheAtlasTheSessionWouldServe: the atlas on
// an entry comes from the projector, once per compile, at boot and at Put.
func (s *RegistrySuite) TestProjector_EveryEntryCarriesTheAtlasTheSessionWouldServe() {
	p := &countingProjector{}
	r, err := dungeons.NewFileRegistry(s.dir, true, p)
	s.Require().NoError(err)
	s.Equal(1, p.calls, "the shipped tomb was projected once at boot")

	e, err := r.Get(s.ctx, dungeons.DefaultKey)
	s.Require().NoError(err)
	s.Require().NotNil(e.Atlas)
	s.Equal([]spatial.Position{{X: 1, Y: 2}}, e.Atlas.Cells)

	res, err := r.Put(s.ctx, &dungeons.PutInput{Key: "crypt", YAML: s.rekeyed("crypt"), ValidateOnly: true})
	s.Require().NoError(err)
	s.Require().NotNil(res.Entry.Atlas, "validate_only still answers the atlas — it is the builder's preview")
	s.Equal(2, p.calls)
}

// TestProjector_AWorldThatWillNotLoadIsNotTheAuthorsProblem: a compiled file
// whose world the session refuses is an error (boot refusal / Internal), not
// a FieldError — the stack disagreed with itself, the author did nothing.
func (s *RegistrySuite) TestProjector_AWorldThatWillNotLoadIsNotTheAuthorsProblem() {
	_, err := dungeons.NewFileRegistry(s.dir, true, &countingProjector{fail: errors.New("invalid world")})
	s.Require().Error(err)
	s.Contains(err.Error(), "project atlas")
	s.Contains(err.Error(), "reference-tomb.yaml")

	r, err := dungeons.NewFileRegistry(s.dir, true, nil)
	s.Require().NoError(err)
	_ = r
}

func (s *RegistrySuite) TestGet_UnknownKeyIsNotFound() {
	r := s.open(false)

	_, err := r.Get(s.ctx, "nope")
	s.Require().ErrorIs(err, dungeons.ErrNotFound)
}

func (s *RegistrySuite) TestPut_ValidateOnlyNeverWrites() {
	r := s.open(true)

	res, err := r.Put(s.ctx, &dungeons.PutInput{Key: "crypt", YAML: s.rekeyed("crypt"), ValidateOnly: true})
	s.Require().NoError(err)
	s.Empty(res.Errors)
	s.Require().NotNil(res.Entry, "the compiled entry is answered")

	_, statErr := os.Stat(filepath.Join(s.dir, "crypt.yaml"))
	s.True(os.IsNotExist(statErr), "nothing reached the disk")
	_, getErr := r.Get(s.ctx, "crypt")
	s.ErrorIs(getErr, dungeons.ErrNotFound, "and nothing reached the registry")
}

func (s *RegistrySuite) TestPut_WritesThenGetReturnsTheBytesUnchanged() {
	r := s.open(true)
	raw := append(s.rekeyed("crypt"), []byte("# a trailing comment the author wrote\n")...)

	res, err := r.Put(s.ctx, &dungeons.PutInput{Key: "crypt", YAML: raw})
	s.Require().NoError(err)
	s.Empty(res.Errors)

	e, err := r.Get(s.ctx, "crypt")
	s.Require().NoError(err)
	s.Equal(raw, e.YAML, "verbatim, comment included — never a re-marshal")

	onDisk, err := os.ReadFile(filepath.Join(s.dir, "crypt.yaml"))
	s.Require().NoError(err)
	s.Equal(raw, onDisk, "and the file on disk is the same bytes")

	list, err := r.List(s.ctx)
	s.Require().NoError(err)
	s.Equal([]dungeons.Summary{{Key: "crypt", Name: "The Reference Tomb"}, {Key: "reference-tomb", Name: "The Reference Tomb"}}, list)

	// A fresh registry over the same directory sees the write: the file is
	// the truth, the map is a cache of it.
	again := s.open(false)
	e2, err := again.Get(s.ctx, "crypt")
	s.Require().NoError(err)
	s.Equal(raw, e2.YAML)
}

// TestGet_ReturnsACopyNotTheRegistrysOwnState: mutating what Get (or Put)
// handed back changes nothing the registry serves — Copilot's review point
// on PR #820, kept as a test rather than a comment.
func (s *RegistrySuite) TestGet_ReturnsACopyNotTheRegistrysOwnState() {
	r := s.open(true)
	buf := s.rekeyed("crypt")
	res, err := r.Put(s.ctx, &dungeons.PutInput{Key: "crypt", YAML: buf})
	s.Require().NoError(err)
	buf[0] = '#'
	res.Entry.Key = "tampered"
	res.Entry.YAML[1] = '#'

	got, err := r.Get(s.ctx, "crypt")
	s.Require().NoError(err)
	s.Equal("crypt", got.Key)
	s.Equal(s.rekeyed("crypt"), got.YAML, "neither the caller's buffer nor the returned entry reaches the registry")

	got.YAML[2] = '#'
	again, err := r.Get(s.ctx, "crypt")
	s.Require().NoError(err)
	s.Equal(s.rekeyed("crypt"), again.YAML)
}

func (s *RegistrySuite) TestPut_AFileThatDoesNotCompileIsAnAnswerNotAnError() {
	r := s.open(true)

	res, err := r.Put(s.ctx, &dungeons.PutInput{Key: "crypt", YAML: []byte("version: 2\nkey: crypt\nvoid: nonsense\n")})
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
	r := s.open(true)

	// start on void, intensity out of range: two defects, two paths.
	raw := bytes.Replace(s.rekeyed("crypt"), []byte("start: [1, 3]"), []byte("start: [99, 99]"), 1)
	raw = bytes.Replace(raw, []byte("intensity: 0.6"), []byte("intensity: 1.2"), 1)
	s.Require().NotEqual(s.rekeyed("crypt"), raw)

	res, err := r.Put(s.ctx, &dungeons.PutInput{Key: "crypt", YAML: raw, ValidateOnly: true})
	s.Require().NoError(err)
	paths := make([]string, 0, len(res.Errors))
	for _, fe := range res.Errors {
		s.NotEmpty(fe.Message)
		paths = append(paths, fe.Path)
	}
	s.Contains(paths, "start")
	s.Contains(paths, "regions[0].lighting.intensity")
	s.GreaterOrEqual(len(paths), 2, "every defect, not the first: %v", res.Errors)

	// An unknown key is a decode defect and names its line.
	res, err = r.Put(s.ctx, &dungeons.PutInput{Key: "crypt", YAML: append(s.rekeyed("crypt"), []byte("hieght: 8\n")...), ValidateOnly: true})
	s.Require().NoError(err)
	s.Require().NotEmpty(res.Errors)
	s.Contains(res.Errors[0].Path, "line ")
}

func (s *RegistrySuite) TestPut_KeyMustEqualTheFilesKey() {
	r := s.open(true)

	_, err := r.Put(s.ctx, &dungeons.PutInput{Key: "crypt", YAML: s.tomb})
	s.Require().ErrorIs(err, dungeons.ErrKeyMismatch)

	_, statErr := os.Stat(filepath.Join(s.dir, "crypt.yaml"))
	s.True(os.IsNotExist(statErr))
}

func (s *RegistrySuite) TestPut_RefusesAKeyOutsideTheCharset() {
	r := s.open(true)

	for _, key := range []string{"", "Crypt", "my crypt", "../escape", "crypt.yaml"} {
		_, err := r.Put(s.ctx, &dungeons.PutInput{Key: key, YAML: s.tomb})
		s.ErrorIsf(err, dungeons.ErrInvalidKey, "key %q", key)
	}
}

func (s *RegistrySuite) TestPut_RefusedWhenAuthoringIsOff() {
	r := s.open(false)

	_, err := r.Put(s.ctx, &dungeons.PutInput{Key: "crypt", YAML: s.rekeyed("crypt")})
	s.Require().ErrorIs(err, dungeons.ErrAuthoringDisabled)

	_, statErr := os.Stat(filepath.Join(s.dir, "crypt.yaml"))
	s.True(os.IsNotExist(statErr), "a read-only registry never touches the directory")
}

func (s *RegistrySuite) TestPut_OverwritesInPlaceAndLeavesNoTempFiles() {
	r := s.open(true)
	first := s.rekeyed("crypt")
	second := append(s.rekeyed("crypt"), []byte("# second\n")...)

	_, err := r.Put(s.ctx, &dungeons.PutInput{Key: "crypt", YAML: first})
	s.Require().NoError(err)
	_, err = r.Put(s.ctx, &dungeons.PutInput{Key: "crypt", YAML: second})
	s.Require().NoError(err)

	e, err := r.Get(s.ctx, "crypt")
	s.Require().NoError(err)
	s.Equal(second, e.YAML)

	entries, err := os.ReadDir(s.dir)
	s.Require().NoError(err)
	names := make([]string, 0, len(entries))
	for _, d := range entries {
		names = append(names, d.Name())
	}
	s.ElementsMatch([]string{"reference-tomb.yaml", "crypt.yaml"}, names, "temp files are renamed away, never left behind")
}

// TestPut_ConcurrentPutsOnOneKeySerialise runs under -race: N writers on one
// key must land whole, one after another, and the final state must be one of
// the files written — never a splice of two.
func (s *RegistrySuite) TestPut_ConcurrentPutsOnOneKeySerialise() {
	r := s.open(true)
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
			res, err := r.Put(s.ctx, &dungeons.PutInput{Key: "crypt", YAML: raw})
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

	e, err := r.Get(s.ctx, "crypt")
	s.Require().NoError(err)
	onDisk, err := os.ReadFile(filepath.Join(s.dir, "crypt.yaml"))
	s.Require().NoError(err)
	s.Equal(e.YAML, onDisk, "the served entry is the file on disk")

	whole := false
	for _, v := range variants {
		if bytes.Equal(v, onDisk) {
			whole = true
			break
		}
	}
	s.True(whole, "the final file is exactly one writer's file, not a splice")
}
