// Copyright (C) 2024 Kirk Diggler
// SPDX-License-Identifier: GPL-3.0-or-later

package content

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBuildRegistry_DuplicateKeyAcrossFilesPanicsWhenFatal covers the
// embedded set's posture: a broken commit that ships two files declaring
// the same key must fail CI loudly, not let the lexically-last file
// silently shadow the earlier one (a shadowed file is invisible to
// TestEveryEmbeddedSpecLoads since that test only ever sees whatever
// buildRegistry actually returns). This can't be exercised through the
// real embedded fixture without committing a second, deliberately broken
// content file -- hence a direct unit test on the extracted helper with
// two in-memory byte slices instead.
func TestBuildRegistry_DuplicateKeyAcrossFilesPanicsWhenFatal(t *testing.T) {
	files := []fileEntry{
		{name: "a.yaml", raw: []byte("version: 1\nkey: same-key\n")},
		{name: "b.yaml", raw: []byte("version: 1\nkey: same-key\n")},
	}

	assert.Panics(t, func() {
		buildRegistry(files, true)
	})
}

// TestBuildRegistry_DuplicateKeyNonFatalExcludesAndReports covers the
// RPG_CONTENT_DIR override posture: a duplicate key within the override
// set is a dev-loop mistake, not a reason to fail construction -- the
// second file is excluded and reported, the first wins, no panic.
func TestBuildRegistry_DuplicateKeyNonFatalExcludesAndReports(t *testing.T) {
	files := []fileEntry{
		{name: "a.yaml", raw: []byte("version: 1\nkey: same-key\nname: First\n")},
		{name: "b.yaml", raw: []byte("version: 1\nkey: same-key\nname: Second\n")},
	}

	registry, problems := buildRegistry(files, false)

	require.Contains(t, registry, "same-key")
	assert.Contains(t, string(registry["same-key"]), "First")
	require.Len(t, problems, 1)
	assert.Contains(t, problems[0], "b.yaml")
	assert.Contains(t, problems[0], "same-key")
}

// TestBuildRegistry_MalformedHeaderPanicsWhenFatal mirrors the duplicate-
// key case for an embedded file with no usable key: field at all
// (invalid YAML, or an empty key) -- also a broken commit, also must
// panic rather than be silently dropped from the registry.
func TestBuildRegistry_MalformedHeaderPanicsWhenFatal(t *testing.T) {
	files := []fileEntry{
		{name: "broken.yaml", raw: []byte("not: valid: yaml: at: all:")},
	}

	assert.Panics(t, func() {
		buildRegistry(files, true)
	})
}

// TestBuildRegistry_MalformedHeaderNonFatalExcludesAndReports is the
// override-dir posture for the same malformed input: excluded, reported,
// no panic -- an author's local mistake surfaces as a loggable problem
// the next time they look, not a crash.
func TestBuildRegistry_MalformedHeaderNonFatalExcludesAndReports(t *testing.T) {
	files := []fileEntry{
		{name: "broken.yaml", raw: []byte("not: valid: yaml: at: all:")},
		{name: "empty-key.yaml", raw: []byte("version: 1\nkey: \"\"\n")},
	}

	registry, problems := buildRegistry(files, false)

	assert.Empty(t, registry)
	require.Len(t, problems, 2)
	assert.Contains(t, problems[0], "broken.yaml")
	assert.Contains(t, problems[1], "empty-key.yaml")
}

// TestBuildRegistry_IndexedByDeclaredKeyNotFilename is the direct,
// mutation-verified pin for BLOCKING 2: a file whose name doesn't match
// its declared key must be found ONLY by that key, never by its filename
// stem. Verified by temporarily swapping buildRegistry's indexing to use
// the filename stem instead of the decoded key and confirming this test
// fails -- see the E1 fix report for that mutation's before/after output.
func TestBuildRegistry_IndexedByDeclaredKeyNotFilename(t *testing.T) {
	files := []fileEntry{
		{name: "zzz-scratch.yaml", raw: []byte("version: 1\nkey: my-dungeon\nname: Scratch\n")},
	}

	registry, problems := buildRegistry(files, false)

	assert.Empty(t, problems)
	_, foundByFilenameStem := registry["zzz-scratch"]
	assert.False(t, foundByFilenameStem, "must not be indexed by filename stem")
	raw, foundByDeclaredKey := registry["my-dungeon"]
	require.True(t, foundByDeclaredKey, "must be indexed by its declared key field")
	assert.Contains(t, string(raw), "Scratch")
}

// --- FilenameForKey: write-through's "overwrite the originating file"
// targeting (plan.md S1's Key rules) ---

// TestFilenameForKey_KeyFound_ReturnsDeclaringFilename proves the base
// case PutDungeon's write-through needs: given a key that some file in
// dir declares, return that file's exact name (not a filename derived
// from the key itself — TestBuildRegistry_IndexedByDeclaredKeyNotFilename
// already pins that a file's name and its declared key can differ).
func TestFilenameForKey_KeyFound_ReturnsDeclaringFilename(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "zzz-scratch.yaml"),
		[]byte("version: 1\nkey: my-dungeon\nname: Scratch\n"),
		0o600,
	))

	filename, ok, err := FilenameForKey(dir, "my-dungeon")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "zzz-scratch.yaml", filename)
}

// TestFilenameForKey_KeyNotFound_ReturnsOkFalseNoError proves an unknown
// key is reported as ok=false with a nil error — a NEW key PutDungeon is
// about to create for the first time, not a failure of any kind.
func TestFilenameForKey_KeyNotFound_ReturnsOkFalseNoError(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "other.yaml"),
		[]byte("version: 1\nkey: other-key\nname: Other\n"),
		0o600,
	))

	filename, ok, err := FilenameForKey(dir, "no-such-key")
	require.NoError(t, err)
	require.False(t, ok)
	assert.Empty(t, filename)
}

// TestFilenameForKey_UnreadableDir_PropagatesWrappedError proves dir
// itself being unreadable propagates the same wrapped error
// readOverrideDir already produces for AllSpecs/SpecByKey — an
// operator/config mistake, not a "key not found" case.
func TestFilenameForKey_UnreadableDir_PropagatesWrappedError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist")

	_, ok, err := FilenameForKey(missing, "any-key")
	require.Error(t, err)
	assert.False(t, ok)
	assert.Contains(t, err.Error(), contentDirEnvVar)
}
