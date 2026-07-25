// Copyright (C) 2024 Kirk Diggler
// SPDX-License-Identifier: GPL-3.0-or-later

// Package content hosts dungeon spec YAML files: embedded at build time
// from dungeons/*.yaml, with an optional RPG_CONTENT_DIR directory that
// augments the embedded set at runtime — an author editing a file under
// that directory sees their edit on the next process restart, no rebuild
// (dungeon-authoring design.md §Content hosting).
//
// This package only hosts raw bytes indexed by each spec's declared `key:`
// field. It never decodes beyond that key — validating and compiling a
// spec is rpg-toolkit's encounter/dungeonspec.Load, called by callers (see
// TestEveryEmbeddedSpecLoads).
package content

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed dungeons/*.yaml
var embeddedFS embed.FS

const embeddedDir = "dungeons"

// contentDirEnvVar is the dev-loop override: when set, every *.yaml file in
// this directory is indexed alongside the embedded set. On a key collision
// with an embedded file, the override wins — this is what makes the
// edit-restart authoring loop work (an author's local edit must be seen,
// not the stale embedded copy baked in at the last build).
const contentDirEnvVar = "RPG_CONTENT_DIR"

// specHeader is the minimal shape needed to index a spec by its declared
// key without fully decoding or validating it.
type specHeader struct {
	Key string `yaml:"key"`
}

// AllSpecs returns every known dungeon spec's raw YAML bytes, indexed by
// each file's declared `key:` field (not filename) — the embedded set,
// augmented by RPG_CONTENT_DIR when set (override wins on key collision).
func AllSpecs() map[string][]byte {
	specs := make(map[string][]byte)
	loadEmbedded(specs)
	loadOverrideDir(specs)
	return specs
}

// SpecByKey returns the raw YAML bytes for a dungeon spec by its declared
// key, and whether that key is known.
func SpecByKey(key string) ([]byte, bool) {
	raw, ok := AllSpecs()[key]
	return raw, ok
}

// loadEmbedded indexes every file baked into the binary at build time. A
// file here that fails to yield a key is a broken commit — panicking is
// intentional: this is compiled-in content, not runtime input, and a
// silent skip would defeat the whole point of TestEveryEmbeddedSpecLoads
// (a broken commit must fail CI, not silently vanish from the registry).
func loadEmbedded(specs map[string][]byte) {
	entries, err := embeddedFS.ReadDir(embeddedDir)
	if err != nil {
		panic(fmt.Sprintf("content: embedded %q directory unreadable: %v", embeddedDir, err))
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		raw, err := embeddedFS.ReadFile(filepath.Join(embeddedDir, entry.Name()))
		if err != nil {
			panic(fmt.Sprintf("content: embedded file %q unreadable: %v", entry.Name(), err))
		}
		key, err := headerKey(raw)
		if err != nil {
			panic(fmt.Sprintf("content: embedded file %q has no usable key: %v", entry.Name(), err))
		}
		specs[key] = raw
	}
}

// loadOverrideDir indexes RPG_CONTENT_DIR's *.yaml files when the env var
// is set, overwriting any embedded entry with the same key. Unlike the
// embedded set, a malformed file here is a local dev-loop mistake, not a
// committed regression — it fails loudly (an author will see it
// immediately on the next request) rather than panicking the whole
// registry for every other content-backed key.
func loadOverrideDir(specs map[string][]byte) {
	dir := os.Getenv(contentDirEnvVar)
	if dir == "" {
		return
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		panic(fmt.Sprintf("content: %s=%q unreadable: %v", contentDirEnvVar, dir, err))
	}
	for _, entry := range entries {
		if entry.IsDir() || strings.ToLower(filepath.Ext(entry.Name())) != ".yaml" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dir, entry.Name())) //nolint:gosec // dev-only override dir, operator-controlled path
		if err != nil {
			panic(fmt.Sprintf("content: %s file %q unreadable: %v", contentDirEnvVar, entry.Name(), err))
		}
		key, err := headerKey(raw)
		if err != nil {
			panic(fmt.Sprintf("content: %s file %q has no usable key: %v", contentDirEnvVar, entry.Name(), err))
		}
		specs[key] = raw // override wins on collision — plain overwrite
	}
}

// headerKey decodes just enough of raw to find its declared `key:` field,
// without running the full dungeonspec.Load validation.
func headerKey(raw []byte) (string, error) {
	var header specHeader
	if err := yaml.Unmarshal(raw, &header); err != nil {
		return "", fmt.Errorf("decode key header: %w", err)
	}
	if header.Key == "" {
		return "", fmt.Errorf("empty key field")
	}
	return header.Key, nil
}
