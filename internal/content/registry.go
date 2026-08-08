// Copyright (C) 2024 Kirk Diggler
// SPDX-License-Identifier: GPL-3.0-or-later

package content

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// fileEntry is one file's name and raw content, prior to being indexed by
// its declared key: field.
type fileEntry struct {
	name string
	raw  []byte
}

// specHeader is the minimal shape needed to index a spec by its declared
// key without fully decoding or validating it -- that's
// encounter/dungeonspec.Load's job downstream (see TestEveryEmbeddedSpecLoads).
type specHeader struct {
	Key string `yaml:"key"`
}

// buildRegistry indexes files by each one's declared key: field, NOT
// filename -- an author's file named zzz-scratch.yaml with key: my-dungeon
// inside must resolve as "my-dungeon", never as "zzz-scratch" (pinned by
// TestSpecByKey_IndexedByDeclaredKeyNotFilename).
//
// A file whose key can't be decoded (invalid YAML, or an empty/missing
// key field) is always excluded from the returned registry; so is the
// second (and any later) file in this same call to declare an
// already-used key. panicOnProblem controls how those exclusions are
// reported:
//
//   - true (the embedded set): panics immediately. Embedded content is
//     compiled in -- a bad key or a key collision between two embedded
//     files is unambiguously a broken commit, and panicking is what makes
//     it fail CI instead of silently shadowing one file behind another
//     (TestEveryEmbeddedSpecLoads can only see whatever this function
//     actually returns, so a silent drop would defeat its whole purpose).
//   - false (the RPG_CONTENT_DIR override set): non-fatal. Each excluded
//     file is instead appended to problems as a loggable line -- an
//     author's dev-loop mistake, not a reason to fail the whole registry.
func buildRegistry(files []fileEntry, panicOnProblem bool) (registry map[string][]byte, problems []string) {
	registry = make(map[string][]byte, len(files))
	for _, f := range files {
		key, err := headerKey(f.raw)
		if err != nil {
			if panicOnProblem {
				panic(fmt.Sprintf("content: file %q has no usable key: %v", f.name, err))
			}
			problems = append(problems, fmt.Sprintf("%s: %v", f.name, err))
			continue
		}
		if _, exists := registry[key]; exists {
			if panicOnProblem {
				panic(fmt.Sprintf(
					"content: file %q declares key %q, already used by another file in this set", f.name, key))
			}
			problems = append(problems,
				fmt.Sprintf("%s: key %q already used by another file in this directory", f.name, key))
			continue
		}
		registry[key] = f.raw
	}
	return registry, problems
}

// headerKey decodes just enough of raw to find its declared key: field,
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

// FilenameForKey returns the exact filename, within dir, whose declared
// key: field equals key -- the write-through targeting PutDungeon's
// "overwrite the originating file" rule needs (design.md's Key rules):
// AllSpecs/SpecByKey only ever return key -> bytes, discarding the
// filename inside buildRegistry, so there was no existing way to answer
// "which FILE currently declares this key" until now. ok=false with a nil
// error means key isn't declared by anything in dir today -- a brand new
// key PutDungeon is about to create for the first time, not a failure.
// dir itself being unreadable propagates the same wrapped error
// readOverrideDir already produces for AllSpecs/SpecByKey.
//
// Reuses readOverrideDir + headerKey verbatim -- the same header-only
// decode buildRegistry already performs, just returning the filename
// instead of discarding it, so this never introduces a second, competing
// notion of "which key does this file declare."
func FilenameForKey(dir, key string) (filename string, ok bool, err error) {
	files, err := readOverrideDir(dir)
	if err != nil {
		return "", false, err
	}
	for _, f := range files {
		fileKey, keyErr := headerKey(f.raw)
		if keyErr != nil {
			continue // malformed/missing key: header -- not a match, not this function's problem to report
		}
		if fileKey == key {
			return f.name, true, nil
		}
	}
	return "", false, nil
}
