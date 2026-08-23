// Package dungeons is the content registry: every authored dungeon the
// server can start an encounter in, keyed by the file's own `key:` line
// (rpg-api#806, rpg-project#256).
//
// # One directory, loaded once, written through
//
// A [FileRegistry] owns one directory (RPG_CONTENT_DIR). At construction it
// reads every *.yaml in it and compiles each through internal/sessionworld.
// A file that does not compile FAILS CONSTRUCTION, naming the file and the
// problem, rather than being skipped: a dungeon that silently vanished from
// the picker is a worse failure than a server that refuses to boot and says
// why. Put compiles first, writes the file atomically (temp + rename), then
// swaps the in-memory entry, so what is on disk and what is served never
// disagree and a crash mid-write leaves the previous file intact.
//
// # What the registry does NOT do
//
// It holds no geometry opinion. Compilation is sessionworld's (and through it
// rpg-toolkit's); the registry only decides which bytes are which key. It
// never re-marshals a file: GetDungeon hands back exactly the bytes that were
// Put, comments and spacing included.
package dungeons

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
	tkdungeonspec "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter/dungeonspec"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
)

// DefaultKey is the dungeon StartEncounter plays when the caller names none.
// It is the shipped content/reference-tomb.yaml, and the registry refuses to
// construct without it (see NewFileRegistry) so "no key" can never mean "no
// dungeon".
const DefaultKey = "reference-tomb"

// yamlExt is the one file extension the registry reads and writes.
const yamlExt = ".yaml"

// keyPattern is the charset a dungeon key may use: it becomes a filename and
// a door-ID prefix inside the toolkit, so it is deliberately narrow.
var keyPattern = regexp.MustCompile(`^[a-z0-9-]+$`)

var (
	// ErrNotFound means no dungeon is registered under the key.
	ErrNotFound = errors.New("dungeon not found")

	// ErrInvalidKey means the key is outside [a-z0-9-].
	ErrInvalidKey = errors.New("dungeon key must match [a-z0-9-]+")

	// ErrKeyMismatch means the request named one key and the file's own
	// `key:` line names another. Refused rather than reconciled: the file
	// names itself, and a registry that stored it under a different name
	// would be lying to the next reader.
	ErrKeyMismatch = errors.New("request key does not match the file's key")

	// ErrAuthoringDisabled means Put was called on a registry constructed
	// read-only (RPG_AUTHORING_ENABLED unset). The AuthoringService is not
	// registered in that mode, so this is a second fence, not the first.
	ErrAuthoringDisabled = errors.New("authoring is disabled")
)

// Summary is one dungeon as the lobby's picker sees it.
type Summary struct {
	Key  string
	Name string
}

// FieldError is one compile problem and the YAML path it is about
// (authoring.v1alpha1.FieldError on the wire): "regions[1].cells[0][3]",
// "walls[3]", "place[7].blocks_los", "start"; a decode failure (unknown key,
// second document) carries "line N"; empty means the file as a whole.
type FieldError struct {
	Path    string
	Message string
}

// Entry is one registered dungeon: the bytes as stored, and what they
// compile to.
//
// Get hands out a COPY (struct and YAML slice), and Put copies the caller's
// YAML before keeping it, so no caller holds a reference into the registry's
// own state. Dungeon is shared: it is the compiled world, read by
// StartSession and never written by anyone after compile.
type Entry struct {
	Key  string
	Name string

	// YAML is the stored file, verbatim.
	YAML []byte

	// Dungeon is the compiled world plus the party seats and monster
	// placements StartEncounter seeds from.
	Dungeon *sessionworld.Dungeon

	// Atlas is the map the game plays from, as GetAtlas would report it for a
	// session started on this dungeon — PutDungeon's answer. Produced by the
	// registry's AtlasProjector (the session Manager), never computed here:
	// the projection from the composition's map to the wire's is the
	// toolkit's and lives in exactly one place there
	// (symmetric-bugs-hide-from-roundtrips).
	//
	// TODO(256): nil while the registry is built without a projector — until
	// rulebooks/dnd5e/session's Manager.AtlasOf lands (plan T3,
	// feat/256-atlas-regions) and is pinned here.
	Atlas *sdk.Atlas
}

// AtlasProjector turns a compiled world into the atlas a session on it would
// serve. *session.Manager satisfies it structurally once plan T3 lands
// (Manager.AtlasOf, same validation-load path as StartSession and the same
// projection Manager.Atlas uses).
type AtlasProjector interface {
	AtlasOf(ctx context.Context, world *tkencounter.EncounterData) (*sdk.Atlas, error)
}

// PutInput is one PutDungeon call.
type PutInput struct {
	// Key names the dungeon and must equal the file's own `key:` line.
	Key string

	// YAML is the file, verbatim.
	YAML []byte

	// ValidateOnly compiles and answers without writing anything.
	ValidateOnly bool
}

// PutResult is PutDungeon's answer on a well-formed request: either Errors
// (the file did not compile; nothing was written; Entry is nil) or Entry (it
// did; stored unless ValidateOnly).
type PutResult struct {
	Errors []FieldError
	Entry  *Entry
}

// Registry is what the lobby and authoring orchestrators see.
//
//go:generate mockgen -destination=mock/mock_registry.go -package=dungeonsmock github.com/KirkDiggler/rpg-api/internal/dungeons Registry
type Registry interface {
	// List returns every registered dungeon, sorted by key.
	List(ctx context.Context) ([]Summary, error)

	// Get returns the entry under key, or ErrNotFound.
	Get(ctx context.Context, key string) (*Entry, error)

	// Put compiles and, unless ValidateOnly, stores a dungeon. A file that
	// does not compile is a PutResult with Errors, not an error: the author
	// needs the list. An error is a malformed request (ErrInvalidKey,
	// ErrKeyMismatch), a read-only registry (ErrAuthoringDisabled), or I/O.
	Put(ctx context.Context, in *PutInput) (*PutResult, error)
}

// FileRegistry is the one Registry: a directory of *.yaml files.
type FileRegistry struct {
	dir       string
	authoring bool
	projector AtlasProjector

	// mu guards entries. Reads take it shared; Put swaps under it exclusively.
	mu      sync.RWMutex
	entries map[string]*Entry

	// putMu serializes Puts per key: two authors saving the same key land
	// one after the other, never interleaving the compile/write/swap of one
	// with the other's. Different keys do not contend.
	putMu    sync.Mutex
	putLocks map[string]*sync.Mutex
}

var _ Registry = (*FileRegistry)(nil)

// NewFileRegistry loads every *.yaml under dir and compiles each once.
//
// It fails, naming the file, when a file does not compile, when a file's
// `key:` line disagrees with its filename, when two files claim one key, or
// when DefaultKey is absent — the tomb must load whether or not authoring
// is on, because StartEncounter with no key plays it.
//
// authoring decides whether Put writes; with it false Put returns
// ErrAuthoringDisabled and the directory is never written to.
//
// projector produces each Entry's Atlas. TODO(256): nil is accepted (Atlas
// stays nil) only until the session Manager's AtlasOf is pinned; it then
// becomes required.
func NewFileRegistry(dir string, authoring bool, projector AtlasProjector) (*FileRegistry, error) {
	if dir == "" {
		return nil, errors.New("dungeons: content directory is required")
	}
	names, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("dungeons: read content directory %s: %w", dir, err)
	}

	r := &FileRegistry{
		dir:       dir,
		authoring: authoring,
		projector: projector,
		entries:   make(map[string]*Entry),
		putLocks:  make(map[string]*sync.Mutex),
	}

	for _, d := range names {
		if d.IsDir() || filepath.Ext(d.Name()) != yamlExt {
			continue
		}
		path := filepath.Join(dir, d.Name())
		raw, err := os.ReadFile(path) //nolint:gosec // path is dir + a ReadDir entry
		if err != nil {
			return nil, fmt.Errorf("dungeons: read %s: %w", path, err)
		}
		entry, ferrs, err := r.compileEntry(context.Background(), raw)
		if err != nil {
			return nil, fmt.Errorf("dungeons: %s: %w", path, err)
		}
		if len(ferrs) > 0 {
			return nil, fmt.Errorf("dungeons: %s does not compile: %s", path, joinErrors(ferrs))
		}
		want := strings.TrimSuffix(d.Name(), yamlExt)
		if entry.Key != want {
			return nil, fmt.Errorf("dungeons: %s: file is named %q but its key line says %q: %w",
				path, want, entry.Key, ErrKeyMismatch)
		}
		if _, dup := r.entries[entry.Key]; dup {
			return nil, fmt.Errorf("dungeons: %s: key %q is already registered", path, entry.Key)
		}
		r.entries[entry.Key] = entry
	}

	if _, ok := r.entries[DefaultKey]; !ok {
		return nil, fmt.Errorf("dungeons: %s: no %s%s — the default dungeon must be present",
			dir, DefaultKey, yamlExt)
	}

	return r, nil
}

// List implements Registry.
func (r *FileRegistry) List(_ context.Context) ([]Summary, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Summary, 0, len(r.entries))
	for _, e := range r.entries {
		out = append(out, Summary{Key: e.Key, Name: e.Name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })

	return out, nil
}

// Get implements Registry.
func (r *FileRegistry) Get(_ context.Context, key string) (*Entry, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	e, ok := r.entries[key]
	if !ok {
		return nil, fmt.Errorf("dungeon %q: %w", key, ErrNotFound)
	}

	return e.clone(), nil
}

// clone is the copy Get returns: a fresh struct and a fresh YAML slice, so
// a caller editing what it got back edits nothing the registry serves.
func (e *Entry) clone() *Entry {
	out := *e
	out.YAML = bytes.Clone(e.YAML)
	return &out
}

// Put implements Registry. See the package comment for the write discipline.
func (r *FileRegistry) Put(ctx context.Context, in *PutInput) (*PutResult, error) {
	if in == nil {
		return nil, errors.New("dungeons: PutInput is required")
	}
	if !keyPattern.MatchString(in.Key) {
		return nil, fmt.Errorf("dungeon key %q: %w", in.Key, ErrInvalidKey)
	}
	if !r.authoring {
		return nil, ErrAuthoringDisabled
	}

	unlock := r.lockKey(in.Key)
	defer unlock()

	// The caller's slice is copied ONCE here and never touched again: what
	// is compiled, written and served are the same bytes, and a caller
	// reusing its buffer after Put returns cannot reach any of them.
	raw := bytes.Clone(in.YAML)
	entry, ferrs, err := r.compileEntry(ctx, raw)
	if err != nil {
		return nil, fmt.Errorf("dungeon %q: %w", in.Key, err)
	}
	if len(ferrs) > 0 {
		return &PutResult{Errors: ferrs}, nil
	}
	if entry.Key != in.Key {
		return nil, fmt.Errorf("dungeon %q: file's key line says %q: %w", in.Key, entry.Key, ErrKeyMismatch)
	}
	if in.ValidateOnly {
		return &PutResult{Entry: entry}, nil
	}

	if err := r.writeAtomic(in.Key, raw); err != nil {
		return nil, fmt.Errorf("dungeon %q: %w", in.Key, err)
	}

	r.mu.Lock()
	r.entries[in.Key] = entry
	r.mu.Unlock()

	return &PutResult{Entry: entry.clone()}, nil
}

// lockKey acquires the per-key Put lock and returns its unlock.
func (r *FileRegistry) lockKey(key string) func() {
	r.putMu.Lock()
	l, ok := r.putLocks[key]
	if !ok {
		l = &sync.Mutex{}
		r.putLocks[key] = l
	}
	r.putMu.Unlock()

	l.Lock()
	return l.Unlock
}

// writeAtomic writes dir/<key>.yaml through a temp file in the same
// directory and a rename, so a reader (or a crash) never sees a half-written
// dungeon: the old file is intact until the new one is whole.
func (r *FileRegistry) writeAtomic(key string, raw []byte) error {
	tmp, err := os.CreateTemp(r.dir, "."+key+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }

	if _, err := tmp.Write(raw); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpName, filepath.Join(r.dir, key+yamlExt)); err != nil {
		cleanup()
		return fmt.Errorf("rename into place: %w", err)
	}

	return nil
}

// compileEntry compiles raw into an Entry. A file that is not a dungeon comes
// back as FieldErrors (nil entry, nil error); anything else is an error.
func (r *FileRegistry) compileEntry(ctx context.Context, raw []byte) (*Entry, []FieldError, error) {
	d, err := sessionworld.Compile(raw)
	if err != nil {
		if errors.Is(err, tkdungeonspec.ErrBadSpec) {
			return nil, compileErrors(err), nil
		}
		return nil, nil, err
	}

	entry := &Entry{Key: d.Key, Name: d.Name, YAML: raw, Dungeon: d}
	if r.projector != nil {
		atlas, err := r.projector.AtlasOf(ctx, d.World)
		if err != nil {
			// A world that compiled but will not load is not the author's
			// file being wrong; it is the stack disagreeing with itself.
			return nil, nil, fmt.Errorf("project atlas: %w", err)
		}
		entry.Atlas = atlas
	}

	return entry, nil, nil
}

// compileErrors turns a compile refusal into the wire's list: a validation
// failure carries every defect with its path (dungeonspec v2's
// ValidationError), one-to-one; a decode failure, or a refusal the
// composition made at construction, is one entry whose path is whatever the
// toolkit put in front of the message (a `line N` for decode; empty
// otherwise).
func compileErrors(err error) []FieldError {
	var verr *tkdungeonspec.ValidationError
	if errors.As(err, &verr) {
		out := make([]FieldError, len(verr.Errors))
		for i, fe := range verr.Errors {
			out[i] = FieldError{Path: fe.Path, Message: fe.Message}
		}
		return out
	}

	var fe tkdungeonspec.FieldError
	if errors.As(err, &fe) {
		return []FieldError{{Path: fe.Path, Message: fe.Message}}
	}

	return []FieldError{{Path: "", Message: err.Error()}}
}

func joinErrors(ferrs []FieldError) string {
	parts := make([]string, len(ferrs))
	for i, fe := range ferrs {
		if fe.Path == "" {
			parts[i] = fe.Message
			continue
		}
		parts[i] = fe.Path + ": " + fe.Message
	}
	return strings.Join(parts, "; ")
}

// FindContentDir walks up from start looking for a content directory that
// holds the default dungeon, and returns it. For tests and harnesses that run
// from a package directory; the server reads RPG_CONTENT_DIR instead.
func FindContentDir(start string) (string, error) {
	dir := start
	for {
		candidate := filepath.Join(dir, "content")
		if _, err := os.Stat(filepath.Join(candidate, DefaultKey+yamlExt)); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("dungeons: no content/%s%s above %s", DefaultKey, yamlExt, start)
		}
		dir = parent
	}
}
