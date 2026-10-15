package dungeons

import (
	"fmt"
	"regexp"
)

// rewriteKeyLine renames a file by changing its one root-level `key:` line
// from oldKey to newKey and nothing else (S7-Q4). The line is matched at
// column 0, with optional quotes around the key; whatever follows the key on
// that line (spacing, a comment, a carriage return) is kept verbatim. A file
// with zero such lines, or more than one, is an error: it cannot be renamed
// without guessing which line names it.
func rewriteKeyLine(raw []byte, oldKey, newKey string) ([]byte, error) {
	line := regexp.MustCompile(`(?m)^key:[ \t]*(["']?)` + regexp.QuoteMeta(oldKey) +
		`(["']?)([ \t]*(?:#[^\r\n]*)?)(\r?)$`)
	matches := line.FindAllIndex(raw, -1)
	if len(matches) != 1 {
		return nil, fmt.Errorf("dungeon %q: want exactly one root key line, found %d", oldKey, len(matches))
	}
	return line.ReplaceAll(raw, []byte("key: "+newKey+"${3}${4}")), nil
}
