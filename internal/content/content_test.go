// Copyright (C) 2024 Kirk Diggler
// SPDX-License-Identifier: GPL-3.0-or-later

package content_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-toolkit/encounter/dungeonspec"

	"github.com/KirkDiggler/rpg-api/internal/content"
)

type ContentTestSuite struct {
	suite.Suite
}

func TestContentSuite(t *testing.T) {
	suite.Run(t, new(ContentTestSuite))
}

// TestEveryEmbeddedSpecLoads is the CI safety net for content authoring: a
// broken commit to any embedded dungeons/*.yaml file must fail here, not
// surface later at StartEncounter time in prod.
func (s *ContentTestSuite) TestEveryEmbeddedSpecLoads() {
	specs, problems, err := content.AllSpecs()
	s.Require().NoError(err)
	s.Assert().Empty(problems, "no RPG_CONTENT_DIR override is set in this test")
	s.Require().NotEmpty(specs, "at least the reference-tomb fixture must be embedded")
	for key, raw := range specs {
		_, err := dungeonspec.Load(raw)
		s.Require().NoError(err, "embedded spec %q must load — a broken commit fails CI, not prod", key)
	}
}

func (s *ContentTestSuite) TestSpecByKey_UnknownKey() {
	_, ok, err := content.SpecByKey("atlantis")
	s.Require().NoError(err)
	s.Assert().False(ok)
}

func (s *ContentTestSuite) TestSpecByKey_KnownKeyResolves() {
	raw, ok, err := content.SpecByKey("reference-tomb")
	s.Require().NoError(err)
	s.Require().True(ok)
	s.Assert().Contains(string(raw), "key: reference-tomb")
}

// TestSpecByKey_UnsetContentDirLeavesEmbeddedIntact pins that with
// RPG_CONTENT_DIR unset, SpecByKey resolves the embedded reference-tomb
// verbatim — a regression guard against a future implementation
// accidentally reading some override directory (e.g. cwd, or a stale
// value leaking from another test) when the env var is genuinely unset.
func (s *ContentTestSuite) TestSpecByKey_UnsetContentDirLeavesEmbeddedIntact() {
	s.Require().Empty(os.Getenv("RPG_CONTENT_DIR"), "precondition: no override active in this test")

	raw, ok, err := content.SpecByKey("reference-tomb")
	s.Require().NoError(err)
	s.Require().True(ok)
	s.Assert().Contains(string(raw), "key: reference-tomb")
	s.Assert().NotContains(string(raw), "Overridden")
}

// TestContentDirOverride covers the full augment + override-wins-on-
// collision contract: RPG_CONTENT_DIR pointing at a temp dir with one
// yaml file makes that (new) key resolve; embedded keys not present in
// the override dir still resolve (override AUGMENTS, doesn't replace
// wholesale); and on a key collision between the two sources, the
// override wins — the mechanism that makes the edit-restart authoring
// loop work (an author editing their local
// internal/content/dungeons/reference-tomb.yaml via RPG_CONTENT_DIR must
// see their edit, not the stale embedded copy).
func (s *ContentTestSuite) TestContentDirOverride() {
	dir := s.T().TempDir()

	// A wholly new key, not present in the embedded set at all.
	require.NoError(s.T(), os.WriteFile(
		filepath.Join(dir, "atlantis.yaml"),
		[]byte("version: 1\nkey: atlantis\nname: Atlantis\n"),
		0o600,
	))

	// A key that collides with the embedded reference-tomb — distinguishable
	// content so the test can prove which copy won.
	require.NoError(s.T(), os.WriteFile(
		filepath.Join(dir, "reference-tomb.yaml"),
		[]byte("version: 1\nkey: reference-tomb\nname: Overridden Reference Tomb\n"),
		0o600,
	))

	s.T().Setenv("RPG_CONTENT_DIR", dir)

	// New key from the override dir resolves.
	atlantis, ok, err := content.SpecByKey("atlantis")
	s.Require().NoError(err)
	s.Require().True(ok)
	s.Assert().Contains(string(atlantis), "Atlantis")

	// Collision: override wins over the embedded copy.
	tomb, ok, err := content.SpecByKey("reference-tomb")
	s.Require().NoError(err)
	s.Require().True(ok)
	s.Assert().Contains(string(tomb), "Overridden Reference Tomb")

	// AllSpecs reflects both the augmented and the overridden entry, with
	// no problems reported (both override files are well-formed).
	all, problems, err := content.AllSpecs()
	s.Require().NoError(err)
	s.Assert().Empty(problems)
	s.Assert().Contains(all, "atlantis")
	s.Assert().Contains(string(all["reference-tomb"]), "Overridden Reference Tomb")
}

// TestContentDirOverride_AcceptsYmlExtension covers the should-fix: a
// .yml file (not just .yaml) in the override dir must be picked up — the
// dev loop's most likely typo/variant shouldn't be a silent no-op.
func (s *ContentTestSuite) TestContentDirOverride_AcceptsYmlExtension() {
	dir := s.T().TempDir()
	require.NoError(s.T(), os.WriteFile(
		filepath.Join(dir, "shortform.yml"),
		[]byte("version: 1\nkey: shortform\nname: Short Form\n"),
		0o600,
	))
	s.T().Setenv("RPG_CONTENT_DIR", dir)

	raw, ok, err := content.SpecByKey("shortform")
	s.Require().NoError(err)
	s.Require().True(ok)
	s.Assert().Contains(string(raw), "Short Form")
}

// TestContentDirOverride_IndexedByDeclaredKeyNotFilename is the black-box
// pin for BLOCKING 2: an override file whose name doesn't match its
// declared key resolves ONLY by that key, never by its filename stem —
// the same contract registry_internal_test.go verifies directly against
// buildRegistry, exercised here through the real SpecByKey entry point.
func (s *ContentTestSuite) TestContentDirOverride_IndexedByDeclaredKeyNotFilename() {
	dir := s.T().TempDir()
	require.NoError(s.T(), os.WriteFile(
		filepath.Join(dir, "zzz-scratch.yaml"),
		[]byte("version: 1\nkey: my-dungeon\nname: Scratch\n"),
		0o600,
	))
	s.T().Setenv("RPG_CONTENT_DIR", dir)

	raw, ok, err := content.SpecByKey("my-dungeon")
	s.Require().NoError(err)
	s.Require().True(ok, "must resolve by its declared key field")
	s.Assert().Contains(string(raw), "Scratch")

	_, ok, err = content.SpecByKey("zzz-scratch")
	s.Require().NoError(err)
	s.Assert().False(ok, "must not resolve by its filename stem")
}

// TestContentDirOverride_MalformedHeaderFileSkippedAndReported covers
// BLOCKING 1(a): a malformed/undecodable-header file in the override dir
// must not panic the whole registry — it's excluded, reported in
// problems (loggable, for Task E2's startup registry), and every other
// key still resolves.
func (s *ContentTestSuite) TestContentDirOverride_MalformedHeaderFileSkippedAndReported() {
	dir := s.T().TempDir()
	require.NoError(s.T(), os.WriteFile(
		filepath.Join(dir, "broken.yaml"),
		[]byte("this: is: not: valid: yaml:"),
		0o600,
	))
	require.NoError(s.T(), os.WriteFile(
		filepath.Join(dir, "atlantis.yaml"),
		[]byte("version: 1\nkey: atlantis\nname: Atlantis\n"),
		0o600,
	))
	s.T().Setenv("RPG_CONTENT_DIR", dir)

	specs, problems, err := content.AllSpecs()
	s.Require().NoError(err, "a malformed override FILE must not become a hard error")
	s.Require().Len(problems, 1)
	s.Assert().Contains(problems[0], "broken.yaml")
	s.Assert().Contains(specs, "atlantis", "other override files still resolve")
	s.Assert().Contains(specs, "reference-tomb", "the embedded set is untouched")
}

// TestContentDirOverride_UnreadablePathReturnsError covers BLOCKING 1(b):
// RPG_CONTENT_DIR pointing at a path that can't be listed at all (an
// operator/config mistake, not a single bad file) must fail construction
// loudly via a returned error — never a panic, since this is runtime
// input, not compiled-in content.
func (s *ContentTestSuite) TestContentDirOverride_UnreadablePathReturnsError() {
	missing := filepath.Join(s.T().TempDir(), "does-not-exist")
	s.T().Setenv("RPG_CONTENT_DIR", missing)

	specs, problems, err := content.AllSpecs()
	s.Require().Error(err)
	s.Assert().Nil(specs)
	s.Assert().Nil(problems)

	_, _, err = content.SpecByKey("reference-tomb")
	s.Require().Error(err, "SpecByKey must propagate the same unreadable-path error")
}
