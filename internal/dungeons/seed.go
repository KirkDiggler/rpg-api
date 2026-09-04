package dungeons

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ShippedContentDir is where the binary ships its content, relative to its
// working directory: the repo's content/ tree, copied into the image at
// /home/appuser/content. It is the registry's directory when
// RPG_CONTENT_DIR is unset, and the default seed source when it is set —
// the image overrides the source (RPG_SHIPPED_CONTENT_DIR) to an immutable
// copy OUTSIDE the working directory, so a volume mounted over the working
// tree's content/ cannot hide the seed.
const ShippedContentDir = "content"

// SeedShipped makes sure dir holds every dungeon the binary ships before the
// registry loads it: each shipped *.yaml the directory does not already have
// is copied in (same-directory temp + rename, so a crash never leaves half a
// tomb). Returns the keys written, sorted, empty when there was nothing to
// do.
//
// It exists because a deployment mounts an EMPTY directory as
// RPG_CONTENT_DIR on a fresh box (rpg-deployment's ./content volume is
// gitignored), and NewFileRegistry refuses a directory without the default
// dungeon — correctly, since "no key" must always mean the tomb. Tracking a
// second copy of the tomb in the deployment repo would drift from the one
// the image ships; seeding from the image cannot.
//
// # Why EVERY shipped dungeon and not only the default
//
// It seeded only the tomb until rpg-project#368, and that was the narrower
// half of what it is for. The property wanted is "a fresh box has the
// content the image ships"; seeding one file gets that property only while
// the image ships one file. The moment a second reference dungeon exists —
// reference-tomb-heirloom.yaml, the fixture the recover-the-artifact
// scenario is authored against — the one-file version quietly means "a
// fresh box has SOME of the shipped content", and the missing one has to be
// carried to the box by hand before anybody can play it.
//
// The consequence to name: a SHIPPED dungeon deleted from a mounted
// directory comes back on the next boot. That was already true of the tomb
// and is now true of its siblings. A dungeon PutDungeon authored is never
// touched — its key is not one the image ships — and neither is an edited
// copy of a shipped one, because:
//
// It NEVER overwrites. A dir that already has a file for that key —
// authored, edited, or simply different — is left exactly as it is, and if
// that file does not compile, NewFileRegistry refuses it by name as before.
//
// Two refusals, and they say different things because they ARE different
// things: a shipped directory that cannot be read names the directory it
// could not open and the one it was seeding, and a readable shipped
// directory with no default dungeon in it names the file it looked for. A
// target that already has its own tomb makes the second one moot, so it is
// not raised.
func SeedShipped(dir, shipped string) ([]string, error) {
	if same, err := sameDir(dir, shipped); err != nil {
		return nil, err
	} else if same {
		// Nothing to seed FROM: the registry is reading the shipped tree
		// itself. Whether the tomb is there is NewFileRegistry's question.
		return nil, nil
	}

	// TWO DIFFERENT FAILURES, TWO DIFFERENT SENTENCES (Copilot, PR #914):
	// the source directory being unreadable is not the same thing as the
	// tomb being missing from it, and one message covering both sends
	// whoever reads it to the wrong place — it would say the target "has no
	// reference-tomb.yaml" when the target may well have one and it is the
	// SOURCE that could not be opened.
	entries, err := os.ReadDir(shipped)
	if err != nil {
		return nil, fmt.Errorf("dungeons: cannot read the shipped content directory %s while seeding %s: %w",
			shipped, dir, err)
	}
	// The DEFAULT is special and stays special: "no key" must always mean the
	// tomb, so a shipped tree that cannot supply it is named here rather than
	// left to surface as a registry refusal one layer on. Every other shipped
	// dungeon is optional by construction — there is nothing to be missing.
	if _, statErr := os.Stat(filepath.Join(shipped, DefaultKey+yamlExt)); statErr != nil {
		if _, targetErr := os.Stat(filepath.Join(dir, DefaultKey+yamlExt)); errors.Is(targetErr, os.ErrNotExist) {
			return nil, fmt.Errorf("dungeons: %s has no %s%s and the shipped copy at %s cannot be read: %w",
				dir, DefaultKey, yamlExt, filepath.Join(shipped, DefaultKey+yamlExt), statErr)
		}
	}

	var written []string
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != yamlExt {
			continue
		}
		key := strings.TrimSuffix(entry.Name(), yamlExt)

		target := filepath.Join(dir, entry.Name())
		if _, statErr := os.Stat(target); statErr == nil {
			continue
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return nil, fmt.Errorf("dungeons: stat %s: %w", target, statErr)
		}

		source := filepath.Join(shipped, entry.Name())
		raw, readErr := os.ReadFile(filepath.Clean(source))
		if readErr != nil {
			return nil, fmt.Errorf("dungeons: %s has no %s and the shipped copy at %s cannot be read: %w",
				dir, entry.Name(), source, readErr)
		}
		if writeErr := writeAtomic(dir, key, raw); writeErr != nil {
			return nil, fmt.Errorf("dungeons: seed %s: %w", target, writeErr)
		}
		written = append(written, key)
	}
	sort.Strings(written)

	return written, nil
}

// sameDir reports whether a and b name the same directory, by absolute path
// (and by inode when both exist, so a symlinked mount is not mistaken for a
// different place).
func sameDir(a, b string) (bool, error) {
	absA, err := filepath.Abs(a)
	if err != nil {
		return false, fmt.Errorf("dungeons: resolve %s: %w", a, err)
	}
	absB, err := filepath.Abs(b)
	if err != nil {
		return false, fmt.Errorf("dungeons: resolve %s: %w", b, err)
	}
	if absA == absB {
		return true, nil
	}
	infoA, errA := os.Stat(absA)
	infoB, errB := os.Stat(absB)
	if errA != nil || errB != nil {
		return false, nil //nolint:nilerr // a missing side is simply not the same directory
	}
	return os.SameFile(infoA, infoB), nil
}
