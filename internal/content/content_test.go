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

// TestFogLabSpec_CompilesWithExpectedFixtureShape validates the fog-lab.yaml
// fixture (rpg-project#733 follow-up: a purpose-built dungeon so fog-of-war
// playtesting isn't fighting the procedural crypt generator) both generically
// (TestEveryEmbeddedSpecLoads already covers "does it load at all") and
// structurally: the specific room shape the fixture exists to guarantee. A
// fixture that loads but silently drifts from "4 pillars, no monsters in
// room 1; one skeleton in room 2, door unlocked" would be worse than an
// obviously-broken one — this pins the shape, not just the load.
//
// Also pins the lighting follow-up (Kirk playtest finding: the fixture was
// "essentially unlit" — 2 dim candles + 2 unlit bone-piles in room 1, ZERO
// props in room 2 — which both looked wrong and defeated the fixture's own
// purpose, since it's hard to judge a remembered-shadow transition when the
// whole room is already dark): 2 braziers in room 1, 1 in room 2, every one
// EXPLICITLY non-blocking (dungeonspec's own compile-time default for an
// omitted blocks_los is true — see fog-lab.yaml's comments and this
// package's earlier lesson on the same point for the flanking props).
func (s *ContentTestSuite) TestFogLabSpec_CompilesWithExpectedFixtureShape() {
	raw, ok, err := content.SpecByKey("fog-lab")
	s.Require().NoError(err)
	s.Require().True(ok, "fog-lab must be embedded and resolve by its declared key")

	compiled, err := dungeonspec.Load(raw)
	s.Require().NoError(err, "fog-lab.yaml must compile through dungeonspec — a fixture that fails to load is worse than no fixture")

	s.Require().Len(compiled.Params.Regions, 2, "exactly two rooms: the pillar room and the skeleton room")
	room1, room2 := compiled.Params.Regions[0], compiled.Params.Regions[1]

	// Room 1: four pillars, no monsters, four explicitly non-blocking
	// flanking props, and two explicitly non-blocking braziers for light.
	s.Assert().Empty(room1.Obstacles, "room 1 uses static place: entries, not rolled obstacles")
	pillars, flankers, braziers := 0, 0, 0
	for _, o := range room1.PlacedObstacles {
		switch o.Ref {
		case "dnd5e:props:pillar":
			pillars++
			s.Assert().True(o.BlocksLoS, "pillars must block line of sight (the geometry-fog test depends on this)")
		case "dnd5e:props:candles", "dnd5e:props:bone-pile":
			flankers++
			s.Assert().False(o.BlocksLoS, "props next to pillars must be EXPLICITLY non-blocking, not relying on any ref default")
		case "dnd5e:props:brazier":
			braziers++
			s.Assert().False(o.BlocksLoS, "light props must be EXPLICITLY non-blocking, not relying on any ref default")
		}
	}
	s.Assert().Equal(4, pillars, "room 1 must have exactly 4 pillars")
	s.Assert().Equal(4, flankers, "room 1 must have exactly 4 non-blocking flanking props, one per pillar")
	s.Assert().Equal(2, braziers, "room 1 must have exactly 2 braziers lighting the pillar layout")
	s.Assert().Len(room1.PlacedObstacles, pillars+flankers+braziers, "no unexpected extra props in room 1")

	// No monster spawns target room 1 at all — entity-fog is tested
	// separately, in room 2, per the fixture's whole point.
	for _, spawn := range compiled.Spawns {
		s.Assert().NotEqual(room1.ID, spawn.RoomID, "room 1 must have zero monsters")
	}

	// Room 2: exactly one monster (the mandatory boss slot, filled by the
	// fixture's single skeleton rather than a separate non-boss monster in a
	// third room) and exactly one non-blocking brazier so the skeleton isn't
	// sitting in a pitch-dark room.
	s.Require().Len(compiled.Spawns, 1, "fog-lab must spawn exactly one monster: the skeleton in room 2")
	s.Assert().Equal(room2.ID, compiled.Spawns[0].RoomID)
	s.Assert().Equal("dnd5e:monsters:skeleton", compiled.Spawns[0].MonsterRef)
	s.Require().Len(room2.PlacedObstacles, 1, "room 2 has exactly one placed prop: its light source")
	s.Assert().Equal("dnd5e:props:brazier", room2.PlacedObstacles[0].Ref)
	s.Assert().False(room2.PlacedObstacles[0].BlocksLoS, "room 2's light prop must be EXPLICITLY non-blocking")

	// The connector between them must be unlocked — Kirk needs to open the
	// door, look, and step back repeatedly without a DC check in the way.
	s.Require().Len(compiled.Params.Connectors, 1)
	s.Assert().False(compiled.Params.Connectors[0].Locked, "the pillars->sentry door must be unlocked")
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

// TestOverriddenKeys_UnsetIsEmpty covers the no-override baseline: with
// RPG_CONTENT_DIR unset, nothing is shadowed.
func (s *ContentTestSuite) TestOverriddenKeys_UnsetIsEmpty() {
	shadowed, err := content.OverriddenKeys()
	s.Require().NoError(err)
	s.Assert().Empty(shadowed)
}

// TestOverriddenKeys_DistinguishesShadowedFromNewKeys is the E2 debug-log
// enabler: a caller (the lobby orchestrator's startup registry) needs to
// tell "an override REPLACED an embedded key" (worth a debug line — an
// author's confirmation their edit took effect) apart from "an override
// merely ADDED a new key" (nothing was shadowed, no such confirmation is
// meaningful). reference-tomb collides with the embedded set; atlantis is
// wholly new and must NOT appear here even though it resolves fine via
// SpecByKey.
func (s *ContentTestSuite) TestOverriddenKeys_DistinguishesShadowedFromNewKeys() {
	dir := s.T().TempDir()
	require.NoError(s.T(), os.WriteFile(
		filepath.Join(dir, "reference-tomb.yaml"),
		[]byte("version: 1\nkey: reference-tomb\nname: Overridden Reference Tomb\n"),
		0o600,
	))
	require.NoError(s.T(), os.WriteFile(
		filepath.Join(dir, "atlantis.yaml"),
		[]byte("version: 1\nkey: atlantis\nname: Atlantis\n"),
		0o600,
	))
	s.T().Setenv("RPG_CONTENT_DIR", dir)

	shadowed, err := content.OverriddenKeys()
	s.Require().NoError(err)
	s.Assert().Equal([]string{"reference-tomb"}, shadowed)
}

// TestOverriddenKeys_UnreadablePathReturnsError mirrors AllSpecs'/
// SpecByKey's posture: an unreadable RPG_CONTENT_DIR is a hard error here
// too, never silently treated as "nothing shadowed."
func (s *ContentTestSuite) TestOverriddenKeys_UnreadablePathReturnsError() {
	missing := filepath.Join(s.T().TempDir(), "does-not-exist")
	s.T().Setenv("RPG_CONTENT_DIR", missing)

	_, err := content.OverriddenKeys()
	s.Require().Error(err)
}
