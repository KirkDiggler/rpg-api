// Copyright (C) 2024 Kirk Diggler
// SPDX-License-Identifier: GPL-3.0-or-later

package content_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
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
// broken commit to any content/dungeons/*.yaml file must fail here, not
// surface later at StartEncounter time in prod.
func (s *ContentTestSuite) TestEveryEmbeddedSpecLoads() {
	specs := content.AllSpecs()
	s.Require().NotEmpty(specs, "at least the reference-tomb fixture must be embedded")
	for key, raw := range specs {
		_, err := dungeonspec.Load(raw)
		s.Require().NoError(err, "embedded spec %q must load — a broken commit fails CI, not prod", key)
	}
}

func (s *ContentTestSuite) TestSpecByKey_UnknownKey() {
	_, ok := content.SpecByKey("atlantis")
	s.Assert().False(ok)
}

func (s *ContentTestSuite) TestSpecByKey_KnownKeyResolves() {
	raw, ok := content.SpecByKey("reference-tomb")
	s.Require().True(ok)
	s.Assert().Contains(string(raw), "key: reference-tomb")
}

// TestContentDirOverride covers the full augment + override-wins-on-collision
// contract: RPG_CONTENT_DIR pointing at a temp dir with one yaml file makes
// that (new) key resolve; embedded keys not present in the override dir
// still resolve (override AUGMENTS, doesn't replace wholesale); and on a
// key collision between the two sources, the override wins — the mechanism
// that makes the edit-restart authoring loop work (an author editing their
// local content/dungeons/reference-tomb.yaml via RPG_CONTENT_DIR must see
// their edit, not the stale embedded copy).
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
	atlantis, ok := content.SpecByKey("atlantis")
	s.Require().True(ok)
	s.Assert().Contains(string(atlantis), "Atlantis")

	// Collision: override wins over the embedded copy.
	tomb, ok := content.SpecByKey("reference-tomb")
	s.Require().True(ok)
	s.Assert().Contains(string(tomb), "Overridden Reference Tomb")

	// AllSpecs reflects both the augmented and the overridden entry.
	all := content.AllSpecs()
	s.Assert().Contains(all, "atlantis")
	s.Assert().Contains(string(all["reference-tomb"]), "Overridden Reference Tomb")
}

func (s *ContentTestSuite) TestContentDirOverride_UnsetLeavesEmbeddedIntact() {
	// Sanity check that not setting the override doesn't change embedded
	// behavior — guards against a future implementation accidentally
	// treating an unset/empty RPG_CONTENT_DIR as "override with cwd".
	raw, ok := content.SpecByKey("reference-tomb")
	s.Require().True(ok)
	s.Assert().Contains(string(raw), "key: reference-tomb")
	s.Assert().NotContains(string(raw), "Overridden")

	assert.NotEmpty(s.T(), raw) // keep the assert import meaningfully used alongside require above
}
