package dungeons_test

import (
	"bytes"
	"os"
	"path/filepath"

	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
)

// TestSeed_AnEmptyMountGetsTheShippedTombAndLoads is the fresh-box case:
// RPG_CONTENT_DIR is a mounted, empty directory; after seeding, the registry
// constructs and serves the tomb the image ships.
func (s *RegistrySuite) TestSeed_AnEmptyMountGetsTheShippedTombAndLoads() {
	empty := s.T().TempDir()

	seeded, err := dungeons.SeedDefault(empty, s.dir)
	s.Require().NoError(err)
	s.True(seeded, "the tomb was written")

	onDisk, err := os.ReadFile(filepath.Join(empty, "reference-tomb.yaml"))
	s.Require().NoError(err)
	s.Equal(s.tomb, onDisk, "byte for byte the shipped file")

	r, err := dungeons.NewFileRegistry(empty, true, dungeonstest.Projector(s.T()))
	s.Require().NoError(err)
	list, err := r.List(s.ctx)
	s.Require().NoError(err)
	s.Len(list, 1)

	entries, err := os.ReadDir(empty)
	s.Require().NoError(err)
	s.Len(entries, 1, "no temp file left behind")
}

// TestSeed_NeverOverwritesAnExistingTomb: a directory that already has its
// own reference-tomb.yaml — edited by an author — keeps it.
func (s *RegistrySuite) TestSeed_NeverOverwritesAnExistingTomb() {
	custom := append(bytes.Clone(s.tomb), []byte("# the author's own edit\n")...)
	s.write("reference-tomb.yaml", custom)

	seeded, err := dungeons.SeedDefault(s.dir, s.T().TempDir())
	s.Require().NoError(err)
	s.False(seeded)

	onDisk, err := os.ReadFile(filepath.Join(s.dir, "reference-tomb.yaml"))
	s.Require().NoError(err)
	s.Equal(custom, onDisk, "untouched — even though the shipped source here does not exist at all")
}

// TestSeed_ABrokenExistingTombIsStillRefusedByName: seeding does not paper
// over a directory whose own tomb does not compile; the boot refusal stands.
func (s *RegistrySuite) TestSeed_ABrokenExistingTombIsStillRefusedByName() {
	s.write("reference-tomb.yaml", []byte("version: 2\nkey: reference-tomb\nvoid: nonsense\n"))

	seeded, err := dungeons.SeedDefault(s.dir, s.T().TempDir())
	s.Require().NoError(err)
	s.False(seeded)

	_, err = dungeons.NewFileRegistry(s.dir, false, dungeonstest.Projector(s.T()))
	s.Require().Error(err)
	s.Contains(err.Error(), filepath.Join(s.dir, "reference-tomb.yaml"))
}

// TestSeed_MissingShippedFileNamesBothPaths: an empty mount and no shipped
// copy to seed from is a construction-time error that says where it looked.
func (s *RegistrySuite) TestSeed_MissingShippedFileNamesBothPaths() {
	empty := s.T().TempDir()
	nowhere := filepath.Join(s.T().TempDir(), "not-shipped")

	_, err := dungeons.SeedDefault(empty, nowhere)
	s.Require().Error(err)
	s.Contains(err.Error(), empty)
	s.Contains(err.Error(), filepath.Join(nowhere, "reference-tomb.yaml"))

	entries, err := os.ReadDir(empty)
	s.Require().NoError(err)
	s.Empty(entries, "nothing was written")
}

// TestSeed_SameDirectoryIsANoOp: RPG_CONTENT_DIR pointing at the shipped
// tree itself (or a mount over it) has nothing to seed from; the registry's
// own check decides whether the tomb is there.
func (s *RegistrySuite) TestSeed_SameDirectoryIsANoOp() {
	seeded, err := dungeons.SeedDefault(s.dir, s.dir)
	s.Require().NoError(err)
	s.False(seeded)

	empty := s.T().TempDir()
	seeded, err = dungeons.SeedDefault(empty, empty)
	s.Require().NoError(err)
	s.False(seeded, "an empty dir that is its own source is not an error here")
	_, err = dungeons.NewFileRegistry(empty, false, dungeonstest.Projector(s.T()))
	s.Require().Error(err, "the registry still refuses it by name")
}
