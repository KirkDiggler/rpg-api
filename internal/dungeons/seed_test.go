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

	seeded, err := dungeons.SeedShipped(empty, s.dir)
	s.Require().NoError(err)
	s.Contains(seeded, "reference-tomb", "the tomb was written")

	onDisk, err := os.ReadFile(filepath.Join(empty, "reference-tomb.yaml"))
	s.Require().NoError(err)
	s.Equal(s.tomb, onDisk, "byte for byte the shipped file")

	r, err := dungeons.NewFileRegistry(empty, true, dungeonstest.Projector(s.T()))
	s.Require().NoError(err)
	list, err := r.List(s.ctx)
	s.Require().NoError(err)
	s.Len(list, len(seeded), "every seeded dungeon is served")

	entries, err := os.ReadDir(empty)
	s.Require().NoError(err)
	s.Len(entries, len(seeded), "no temp file left behind")
}

// TestSeed_NeverOverwritesAnExistingTomb: a directory that already has its
// own reference-tomb.yaml — edited by an author — keeps it.
func (s *RegistrySuite) TestSeed_NeverOverwritesAnExistingTomb() {
	custom := append(bytes.Clone(s.tomb), []byte("# the author's own edit\n")...)
	s.write("reference-tomb.yaml", custom)

	seeded, err := dungeons.SeedShipped(s.dir, s.T().TempDir())
	s.Require().NoError(err)
	s.Empty(seeded)

	onDisk, err := os.ReadFile(filepath.Join(s.dir, "reference-tomb.yaml"))
	s.Require().NoError(err)
	s.Equal(custom, onDisk, "untouched — even though the shipped source here does not exist at all")
}

// TestSeed_ABrokenExistingTombIsStillRefusedByName: seeding does not paper
// over a directory whose own tomb does not compile; the boot refusal stands.
func (s *RegistrySuite) TestSeed_ABrokenExistingTombIsStillRefusedByName() {
	s.write("reference-tomb.yaml", []byte("version: 2\nkey: reference-tomb\nvoid: nonsense\n"))

	seeded, err := dungeons.SeedShipped(s.dir, s.T().TempDir())
	s.Require().NoError(err)
	s.Empty(seeded)

	_, err = dungeons.NewFileRegistry(s.dir, false, dungeonstest.Projector(s.T()))
	s.Require().Error(err)
	s.Contains(err.Error(), filepath.Join(s.dir, "reference-tomb.yaml"))
}

// TestSeed_AnUnreadableShippedDirectoryNamesItself: there is nowhere to seed
// FROM. The refusal names the directory it could not open and the one it was
// seeding, and says nothing about the tomb -- the tomb is not what failed
// (Copilot, PR #914).
func (s *RegistrySuite) TestSeed_AnUnreadableShippedDirectoryNamesItself() {
	empty := s.T().TempDir()
	nowhere := filepath.Join(s.T().TempDir(), "not-shipped")

	_, err := dungeons.SeedShipped(empty, nowhere)
	s.Require().Error(err)
	s.Contains(err.Error(), nowhere, "the directory it could not read")
	s.Contains(err.Error(), empty, "and the one it was seeding")
	s.NotContains(err.Error(), "reference-tomb.yaml",
		"and not a file, because a file is not what failed")

	entries, err := os.ReadDir(empty)
	s.Require().NoError(err)
	s.Empty(entries, "nothing was written")
}

// TestSeed_AShippedTreeWithNoTombNamesTheFile is the other refusal, and the
// reason it is separate: the source is readable, it simply cannot supply the
// one dungeon "no key" has to mean. That failure names the file.
func (s *RegistrySuite) TestSeed_AShippedTreeWithNoTombNamesTheFile() {
	empty := s.T().TempDir()
	tombless := s.T().TempDir()

	_, err := dungeons.SeedShipped(empty, tombless)
	s.Require().Error(err)
	s.Contains(err.Error(), filepath.Join(tombless, "reference-tomb.yaml"))
	s.Contains(err.Error(), empty)
}

// TestSeed_AShippedTreeWithNoTombIsFineWhenTheTargetHasOne: the refusal
// above is about the DEFAULT being unavailable, not about the source being
// thin. A target that already has its own tomb needs nothing from the
// source, and every other shipped dungeon is optional by construction.
func (s *RegistrySuite) TestSeed_AShippedTreeWithNoTombIsFineWhenTheTargetHasOne() {
	tombless := s.T().TempDir()

	seeded, err := dungeons.SeedShipped(s.dir, tombless)
	s.Require().NoError(err)
	s.Empty(seeded)
}

// TestSeed_SameDirectoryIsANoOp: RPG_CONTENT_DIR pointing at the shipped
// tree itself (or a mount over it) has nothing to seed from; the registry's
// own check decides whether the tomb is there.
func (s *RegistrySuite) TestSeed_SameDirectoryIsANoOp() {
	seeded, err := dungeons.SeedShipped(s.dir, s.dir)
	s.Require().NoError(err)
	s.Empty(seeded)

	empty := s.T().TempDir()
	seeded, err = dungeons.SeedShipped(empty, empty)
	s.Require().NoError(err)
	s.Empty(seeded, "an empty dir that is its own source is not an error here")
	_, err = dungeons.NewFileRegistry(empty, false, dungeonstest.Projector(s.T()))
	s.Require().Error(err, "the registry still refuses it by name")
}
