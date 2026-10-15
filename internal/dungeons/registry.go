// Package dungeons is the content registry: every authored dungeon the
// server can start an encounter in, keyed by the file's own `key:` line
// (rpg-api#806, rpg-project#256).
//
// # Two stores (rpg-project#585, design R20–R24)
//
// A [FileRegistry] is the SHIPPED store: one directory (RPG_CONTENT_DIR),
// read once at construction and never written by a request. Every *.yaml in
// it compiles through internal/sessionworld; a file that does not compile
// FAILS CONSTRUCTION, naming the file and the problem, rather than being
// skipped: a dungeon that silently vanished from the picker is a worse
// failure than a server that refuses to boot and says why. Its files are
// references: read-only, a starting copy for an author.
//
// The WORLD store (internal/repositories/worlddungeons) holds what authors
// save: per world (the Discord server), owned by the author, a key naming one
// encounter in that world. [NewRegistry] joins the two into the [Registry]
// the lobby and authoring see, for one [Caller] at a time: the world first,
// then the shipped references.
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

	"gopkg.in/yaml.v3"

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

	// ErrAuthoringDisabled means a WRITE was asked of a registry constructed
	// read-only (RPG_AUTHORING_ENABLED unset). The AuthoringService is not
	// registered in that mode, so this is a second fence, not the first.
	//
	// A validate-only Put never returns it (rpg-project#481). Grading a file
	// the caller supplied mutates nothing, and the builder's per-edit preview
	// is exactly that grade.
	ErrAuthoringDisabled = errors.New("authoring is disabled")

	// ErrNoCaller means a registry verb was asked without a world or without
	// a player. Never read as "shipped only": the caller comes from the auth
	// boundary, and its absence is a wiring fault the caller must see.
	ErrNoCaller = errors.New("dungeon caller needs a world and a player")

	// ErrReferenceKey means a save named a key the server ships. References
	// are read-only; the author saves a copy under a key of their own. Its
	// text is the sentence the author reads (design R20).
	ErrReferenceKey = errors.New("that key is a reference; save it under your own key")

	// ErrReferenceReadOnly means a rename or delete named a shipped key that
	// the caller's world does not hold.
	ErrReferenceReadOnly = errors.New("that key is a reference; references cannot be renamed or deleted")

	// ErrOwnedByOther means the key is held in the caller's world by another
	// member: a save, rename or delete of it is refused.
	ErrOwnedByOther = errors.New("that key belongs to another member of this world")

	// ErrKeyExists means a rename's new key already names one of the
	// caller's own encounters (or is the key being renamed).
	ErrKeyExists = errors.New("new key already names one of your encounters")
)

// Source says which store a dungeon came from.
type Source int

const (
	// SourceShipped is a reference the server ships: read-only.
	SourceShipped Source = iota + 1
	// SourceWorld is a record in the caller's world store; Owner says whose.
	SourceWorld
)

// Caller is who is asking, from the auth boundary: the world the boundary
// installed and the authenticated player. Never a request field.
type Caller struct {
	WorldID  string
	PlayerID string
}

// complete reports whether both halves are present.
func (c Caller) complete() bool {
	return c.WorldID != "" && c.PlayerID != ""
}

// Summary is one dungeon as a picker sees it. Owner is the owning player for
// SourceWorld and empty for SourceShipped; ownership is read from Owner, not
// from Source.
type Summary struct {
	Key    string
	Name   string
	Owner  string
	Source Source
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
// Launch and never written by anyone after compile.
type Entry struct {
	Key  string
	Name string

	// Source is the store the entry came from; Owner is the owning player
	// for SourceWorld and empty for SourceShipped.
	Source Source
	Owner  string

	// SingleRoom is the toolkit's dialect rule (dungeonspec.Load): a root
	// `version` of 3 or more is the single-room dialect. Shipped entries
	// below it stay resolvable by key but are left out of every list (R23).
	SingleRoom bool

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
	// (symmetric-bugs-hide-from-roundtrips). Never nil on an Entry.
	Atlas *sdk.Atlas
}

// AtlasProjector turns a compiled world into the atlas a session on it would
// serve: session.Manager.AtlasOf (the same projection Manager.Atlas uses), behind a
// one-method adapter because the SDK's method takes its own input struct.
//
// The key rides along because a projected atlas names its dungeon
// (rpg-project#479) and there is no session record here to ask. A live
// session's atlas reads the key off its record; a preview's is the caller's
// own, echoed back — which is the point, since the builder's preview and the
// game's map have to name their dungeon the same way for one client code
// path to draw both.
type AtlasProjector interface {
	AtlasOf(ctx context.Context, key string, dungeon *tkdungeonspec.Compiled) (*sdk.Atlas, error)
}

// PutInput is one PutDungeon call.
type PutInput struct {
	// Caller is who saves; the record is owned by Caller.PlayerID in
	// Caller.WorldID.
	Caller Caller

	// Key names the dungeon and must equal the file's own `key:` line.
	Key string

	// YAML is the file, verbatim.
	YAML []byte

	// ValidateOnly compiles and answers without writing anything — the
	// builder's per-edit preview. Answered whether or not this registry may
	// write, because a grade mutates nothing (rpg-project#481).
	ValidateOnly bool
}

// PutResult is PutDungeon's answer on a well-formed request: either Errors
// (the file did not compile; nothing was written; Entry is nil) or Entry (it
// did; stored unless ValidateOnly).
type PutResult struct {
	Errors []FieldError
	Entry  *Entry
}

// RenameInput is one RenameDungeon call.
type RenameInput struct {
	Caller Caller
	Key    string
	NewKey string
}

// Registry is what the lobby and authoring orchestrators see: the caller's
// world store joined with the shipped references (see [NewRegistry]).
//
//go:generate mockgen -destination=mock/mock_registry.go -package=dungeonsmock github.com/KirkDiggler/rpg-api/internal/dungeons Registry
type Registry interface {
	// List returns, for the caller, every record in their world sorted by
	// key, then the single-room references sorted by key that no record in
	// the world shadows.
	List(ctx context.Context, caller Caller) ([]Summary, error)

	// Get resolves key for the caller: their world store, then shipped; or
	// ErrNotFound.
	Get(ctx context.Context, caller Caller, key string) (*Entry, error)

	// Put compiles and, unless ValidateOnly, stores a dungeon in the
	// caller's world, owned by the caller. A file that does not compile is a
	// PutResult with Errors, not an error: the author needs the list. An
	// error is a malformed request (ErrInvalidKey, ErrKeyMismatch), a key the
	// caller may not save under (ErrReferenceKey, ErrOwnedByOther; refused
	// for validate-only too), a WRITE asked of a read-only registry
	// (ErrAuthoringDisabled), or I/O.
	Put(ctx context.Context, in *PutInput) (*PutResult, error)

	// Delete removes one of the caller's records.
	Delete(ctx context.Context, caller Caller, key string) error

	// Rename moves one of the caller's records to a new key, rewriting the
	// file's root `key:` line and nothing else.
	Rename(ctx context.Context, in *RenameInput) error
}

// FileRegistry is the shipped store: a directory of *.yaml files, loaded
// once and never written by a request.
type FileRegistry struct {
	dir       string
	projector AtlasProjector
	entries   map[string]*Entry
}

// NewFileRegistry loads every *.yaml under dir and compiles each once.
//
// It fails, naming the file, when a file does not compile, when a file's
// `key:` line disagrees with its filename, when two files claim one key, or
// when DefaultKey is absent — the tomb must load whether or not authoring
// is on, because StartEncounter with no key plays it.
//
// projector produces each Entry's Atlas. Required.
func NewFileRegistry(dir string, projector AtlasProjector) (*FileRegistry, error) {
	if dir == "" {
		return nil, errors.New("dungeons: content directory is required")
	}
	if projector == nil {
		return nil, errors.New("dungeons: atlas projector is required")
	}
	names, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("dungeons: read content directory %s: %w", dir, err)
	}

	r := &FileRegistry{
		dir:       dir,
		projector: projector,
		entries:   make(map[string]*Entry),
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
		entry, ferrs, err := compileEntry(context.Background(), projector, raw)
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
		entry.Source = SourceShipped
		r.entries[entry.Key] = entry
	}

	if _, ok := r.entries[DefaultKey]; !ok {
		return nil, fmt.Errorf("dungeons: %s: no %s%s — the default dungeon must be present",
			dir, DefaultKey, yamlExt)
	}

	return r, nil
}

// List returns every shipped dungeon, both dialects, sorted by key. The
// two-store registry's list leaves out the ones that are not single-room.
func (r *FileRegistry) List(_ context.Context) ([]Summary, error) {
	out := make([]Summary, 0, len(r.entries))
	for _, e := range r.entries {
		out = append(out, Summary{Key: e.Key, Name: e.Name, Source: SourceShipped})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })

	return out, nil
}

// Get returns the shipped entry under key, or ErrNotFound.
func (r *FileRegistry) Get(_ context.Context, key string) (*Entry, error) {
	e, ok := r.entries[key]
	if !ok {
		return nil, fmt.Errorf("dungeon %q: %w", key, ErrNotFound)
	}

	return e.clone(), nil
}

// has reports whether key is a shipped reference.
func (r *FileRegistry) has(key string) bool {
	_, ok := r.entries[key]
	return ok
}

// clone is the copy Get returns: a fresh struct and a fresh YAML slice, so
// a caller editing what it got back edits nothing the registry serves.
func (e *Entry) clone() *Entry {
	out := *e
	out.YAML = bytes.Clone(e.YAML)
	return &out
}

// writeAtomic writes dir/<key>.yaml through a temp file in the same
// directory and a rename, so a reader (or a crash) never sees a half-written
// dungeon: the old file is intact until the new one is whole.
func writeAtomic(dir, key string, raw []byte) error {
	tmp, err := os.CreateTemp(dir, "."+key+".*.tmp")
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
	if err := os.Rename(tmpName, filepath.Join(dir, key+yamlExt)); err != nil {
		cleanup()
		return fmt.Errorf("rename into place: %w", err)
	}

	return nil
}

// compileEntry compiles raw into an Entry. A file that is not a dungeon comes
// back as FieldErrors (nil entry, nil error); anything else is an error.
func compileEntry(ctx context.Context, projector AtlasProjector, raw []byte) (*Entry, []FieldError, error) {
	d, err := sessionworld.Compile(raw)
	if err != nil {
		if errors.Is(err, tkdungeonspec.ErrBadSpec) {
			return nil, compileErrors(err), nil
		}
		return nil, nil, err
	}

	// The FILE's own key, which is the one this entry is stored and served
	// under. A request key that disagrees with it is refused just after this
	// (ErrKeyMismatch, on both the Put path and the load-from-disk path), so
	// an atlas built here can only ever reach a caller naming the same string
	// the caller asked for.
	atlas, err := projector.AtlasOf(ctx, d.Key, d.Spec)
	if err != nil {
		// Mostly the stack disagreeing with itself: a dungeon that compiled but
		// will not project. Not always: a file whose own endings collide with
		// the ones a launch declares (a duplicate ending key) is an authoring
		// error that surfaces here as a status until it becomes a field error.
		return nil, nil, fmt.Errorf("project atlas: %w", err)
	}

	return &Entry{
		Key:        d.Key,
		Name:       d.Name,
		YAML:       raw,
		Dungeon:    d,
		Atlas:      atlas,
		SingleRoom: isSingleRoom(raw),
	}, nil, nil
}

// singleRoomVersion is the first root `version` of the single-room dialect.
const singleRoomVersion = 3

// isSingleRoom is the toolkit's own dispatch rule (dungeonspec.Load): a root
// `version` of 3 or more is a single-room site document.
func isSingleRoom(raw []byte) bool {
	var root struct {
		Version int `yaml:"version"`
	}
	return yaml.Unmarshal(raw, &root) == nil && root.Version >= singleRoomVersion
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
