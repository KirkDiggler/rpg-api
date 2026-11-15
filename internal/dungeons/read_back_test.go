package dungeons_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter/dungeonspec/readback"

	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
)

const (
	readBackSiteKey   = "creature-facts-walk"
	readBackAuthorKey = "my-creature-facts"
)

// ReadBackFailureSuite reaches the read-back's failure and panic branches
// through the seam (export_test.go): after a clean compile, neither turns
// the check into a refusal. A save never asks for a read-back (R25), so a
// failing or panicking one cannot touch it: stored, a success, none carried
// (rpg-project#967).
type ReadBackFailureSuite struct {
	suite.Suite

	ctx     context.Context
	shipped *dungeons.FileRegistry
	yaml    []byte
}

func TestReadBackFailureSuite(t *testing.T) { suite.Run(t, new(ReadBackFailureSuite)) }

// SetupSuite stages a one-file shelf: the registry needs a default dungeon,
// and the small site stands in for it.
func (s *ReadBackFailureSuite) SetupSuite() {
	s.ctx = context.Background()
	raw, err := os.ReadFile(dungeonstest.SitePath(s.T(), readBackSiteKey)) //nolint:gosec // repository fixture
	s.Require().NoError(err)
	dir := s.T().TempDir()
	s.Require().NoError(os.WriteFile(filepath.Join(dir, dungeons.DefaultKey+".yaml"),
		dungeonstest.AsKey(s.T(), raw, readBackSiteKey, dungeons.DefaultKey), 0o600))
	s.shipped, err = dungeons.NewFileRegistry(dir, dungeonstest.Projector(s.T()))
	s.Require().NoError(err)
	s.yaml = dungeonstest.AsKey(s.T(), raw, readBackSiteKey, readBackAuthorKey)
}

func failing([]byte) (*readback.ReadBack, error) { return nil, errors.New("read-back broke") }

func panicking([]byte) (*readback.ReadBack, error) { panic("read-back panicked") }

// put sends the small site through a registry whose read-back is fn.
func (s *ReadBackFailureSuite) put(fn func([]byte) (*readback.ReadBack, error), validateOnly bool) (dungeons.Registry, *dungeons.PutResult) {
	s.T().Helper()

	r := dungeonstest.Join(s.T(), s.shipped, true)
	dungeons.SetReadBack(r, fn)
	res, err := r.Put(s.ctx, &dungeons.PutInput{
		Caller: dungeonstest.Caller, Key: readBackAuthorKey, YAML: s.yaml, ValidateOnly: validateOnly,
	})
	s.Require().NoError(err, "a read-back failure is never an error to the caller")
	s.Require().Empty(res.Errors, "nor a refusal")
	s.Require().NotNil(res.Entry, "the clean compile is still answered")
	s.Nil(res.ReadBack, "with no read-back")

	return r, res
}

func (s *ReadBackFailureSuite) TestCheck_ReadBackFails_StaysClean() {
	s.put(failing, true)
}

func (s *ReadBackFailureSuite) TestCheck_ReadBackPanics_StaysClean() {
	s.put(panicking, true)
}

func (s *ReadBackFailureSuite) TestSave_ReadBackFails_IsStoredWithNone() {
	r, _ := s.put(failing, false)
	got, err := r.Get(s.ctx, dungeonstest.Caller, readBackAuthorKey)
	s.Require().NoError(err, "the save was written")
	s.Equal(dungeons.SourceWorld, got.Source)
}

func (s *ReadBackFailureSuite) TestSave_ReadBackPanics_IsStoredWithNone() {
	r, _ := s.put(panicking, false)
	got, err := r.Get(s.ctx, dungeonstest.Caller, readBackAuthorKey)
	s.Require().NoError(err, "the save was written")
	s.Equal(dungeons.SourceWorld, got.Source)
}
