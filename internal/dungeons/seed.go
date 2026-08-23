package dungeons

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// ShippedContentDir is where the binary ships its content, relative to its
// working directory: the repo's content/ tree, copied into the image at
// /home/appuser/content. It is the registry's directory when
// RPG_CONTENT_DIR is unset, and the default seed source when it is set —
// the image overrides the source (RPG_SHIPPED_CONTENT_DIR) to an immutable
// copy OUTSIDE the working directory, so a volume mounted over the working
// tree's content/ cannot hide the seed.
const ShippedContentDir = "content"

// SeedDefault makes sure dir holds the default dungeon before the registry
// loads it: if dir has no DefaultKey file, the shipped one is copied in
// (same-directory temp + rename, so a crash never leaves half a tomb).
//
// It exists because a deployment mounts an EMPTY directory as
// RPG_CONTENT_DIR on a fresh box (rpg-deployment's ./content volume is
// gitignored), and NewFileRegistry refuses a directory without the default
// dungeon — correctly, since "no key" must always mean the tomb. Tracking a
// second copy of the tomb in the deployment repo would drift from the one
// the image ships; seeding from the image cannot.
//
// It NEVER overwrites: a dir that already has a reference-tomb.yaml —
// authored, edited, or simply different — is left exactly as it is, and if
// that file does not compile, NewFileRegistry refuses it by name as before.
// Returns whether a file was written. A missing shipped file is an error
// naming both paths.
func SeedDefault(dir, shipped string) (bool, error) {
	if same, err := sameDir(dir, shipped); err != nil {
		return false, err
	} else if same {
		// Nothing to seed FROM: the registry is reading the shipped tree
		// itself. Whether the tomb is there is NewFileRegistry's question.
		return false, nil
	}

	target := filepath.Join(dir, DefaultKey+yamlExt)
	if _, err := os.Stat(target); err == nil {
		return false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("dungeons: stat %s: %w", target, err)
	}

	source := filepath.Join(shipped, DefaultKey+yamlExt)
	raw, err := os.ReadFile(filepath.Clean(source))
	if err != nil {
		return false, fmt.Errorf("dungeons: %s has no %s%s and the shipped copy at %s cannot be read: %w",
			dir, DefaultKey, yamlExt, source, err)
	}

	if err := writeAtomic(dir, DefaultKey, raw); err != nil {
		return false, fmt.Errorf("dungeons: seed %s: %w", target, err)
	}

	return true, nil
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
