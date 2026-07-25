// Copyright (C) 2024 Kirk Diggler
// SPDX-License-Identifier: GPL-3.0-or-later

// Package content hosts dungeon spec YAML files, indexed by each file's
// declared key: field (not filename). The embedded dungeons/*.yaml set is
// compiled into the binary; an optional RPG_CONTENT_DIR directory
// augments it at runtime with *.yaml/*.yml files of its own, winning over
// an embedded file on a key collision.
//
// This package only reads and indexes raw bytes -- it never validates a
// spec's contents; that's rpg-toolkit's encounter/dungeonspec.Load,
// called by callers (see TestEveryEmbeddedSpecLoads). It also only reads
// RPG_CONTENT_DIR fresh on every call -- whether an edit there takes
// effect immediately or requires a process restart is a property of
// whoever calls AllSpecs/SpecByKey and how often they cache the result,
// not of this package.
package content

import (
	"embed"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
)

//go:embed dungeons/*.yaml
var embeddedFS embed.FS

const embeddedDir = "dungeons"

// contentDirEnvVar is the dev-loop override directory. See the package
// doc and AllSpecs for the augment/override-wins contract.
const contentDirEnvVar = "RPG_CONTENT_DIR"

// AllSpecs returns every known dungeon spec's raw YAML bytes, indexed by
// key, plus any RPG_CONTENT_DIR file that couldn't be indexed -- a
// malformed/missing key: field, or a key colliding with another override
// file. Those are non-fatal: reported in problems for the caller to log,
// the affected file simply doesn't participate in the registry (an
// author's dev-loop mistake, not a reason to fail the whole registry).
//
// Returns an error only when RPG_CONTENT_DIR is set but its directory
// itself can't be read -- an operator/config mistake that must fail
// construction loudly rather than silently falling back to
// embedded-only content.
//
// Builds the registry fresh on every call -- a caller invoking this
// per-request rather than once at startup should cache the result.
func AllSpecs() (specs map[string][]byte, problems []string, err error) {
	specs, _ = buildRegistry(embeddedFiles(), true) // fatal mode never returns problems -- it panics instead

	dir := os.Getenv(contentDirEnvVar)
	if dir == "" {
		return specs, nil, nil
	}

	overrideFiles, err := readOverrideDir(dir)
	if err != nil {
		return nil, nil, err
	}
	overrides, problems := buildRegistry(overrideFiles, false)
	for k, v := range overrides {
		specs[k] = v // override wins over embedded on collision
	}
	return specs, problems, nil
}

// SpecByKey returns the raw YAML bytes for a dungeon spec by its declared
// key, and whether that key is known. Propagates the same
// RPG_CONTENT_DIR unreadable-directory error AllSpecs does.
func SpecByKey(key string) (raw []byte, ok bool, err error) {
	specs, _, err := AllSpecs()
	if err != nil {
		return nil, false, err
	}
	raw, ok = specs[key]
	return raw, ok, nil
}

// embeddedFiles reads every file under the embedded dungeons/ directory.
// A read failure here means the embed itself is broken -- panicking is
// intentional, compiled-in content should never fail to read.
func embeddedFiles() []fileEntry {
	entries, err := embeddedFS.ReadDir(embeddedDir)
	if err != nil {
		panic(fmt.Sprintf("content: embedded %q directory unreadable: %v", embeddedDir, err))
	}
	files := make([]fileEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		// embed.FS paths are always forward-slash regardless of OS --
		// path.Join, not filepath.Join (which would use the OS separator).
		raw, err := embeddedFS.ReadFile(path.Join(embeddedDir, entry.Name()))
		if err != nil {
			panic(fmt.Sprintf("content: embedded file %q unreadable: %v", entry.Name(), err))
		}
		files = append(files, fileEntry{name: entry.Name(), raw: raw})
	}
	return files
}

// readOverrideDir reads every *.yaml/*.yml file in dir. Returns a hard
// error if dir itself can't be listed (RPG_CONTENT_DIR pointing at a path
// that doesn't exist or isn't readable) or if the OS refuses to read a
// file it just listed (rare -- e.g. a permissions problem specific to one
// file). Both are operator/environment problems, distinct from a file
// whose YAML *content* is the issue -- that's buildRegistry's non-fatal
// problems path.
func readOverrideDir(dir string) ([]fileEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("content: %s=%q unreadable: %w", contentDirEnvVar, dir, err)
	}
	files := make([]fileEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, entry.Name())) //nolint:gosec // dev-only override dir, operator-controlled path
		if err != nil {
			return nil, fmt.Errorf("content: %s file %q unreadable: %w", contentDirEnvVar, entry.Name(), err)
		}
		files = append(files, fileEntry{name: entry.Name(), raw: raw})
	}
	return files, nil
}
