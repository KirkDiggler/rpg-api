package dungeons

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"

	"github.com/KirkDiggler/rpg-api/internal/repositories/worlddungeons"
)

// RegistryConfig is what [NewRegistry] joins.
type RegistryConfig struct {
	// Shipped is the read-only reference store. Required.
	Shipped *FileRegistry
	// World is the per-world store authors save to. Required.
	World worlddungeons.Repository
	// Authoring decides whether Put, Rename and Delete WRITE
	// (RPG_AUTHORING_ENABLED). Without it they refuse with
	// ErrAuthoringDisabled; a validate-only Put is still answered.
	Authoring bool
	// Projector produces each world entry's Atlas. Required.
	Projector AtlasProjector
}

// storeRegistry is the Registry: the caller's world store, then shipped.
type storeRegistry struct {
	shipped   *FileRegistry
	world     worlddungeons.Repository
	authoring bool
	projector AtlasProjector

	// putMu guards putLocks, which serialize this process's Puts per world
	// and key: the compile/write of one save never interleaves another's of
	// the same key. Across processes the world store's transaction decides.
	putMu    sync.Mutex
	putLocks map[putLockKey]*sync.Mutex
}

type putLockKey struct{ worldID, key string }

// NewRegistry joins the shipped store and the world store into the Registry.
func NewRegistry(cfg *RegistryConfig) (Registry, error) {
	if cfg == nil {
		return nil, errors.New("dungeons: registry config is required")
	}
	if cfg.Shipped == nil {
		return nil, errors.New("dungeons: shipped store is required")
	}
	if cfg.World == nil {
		return nil, errors.New("dungeons: world store is required")
	}
	if cfg.Projector == nil {
		return nil, errors.New("dungeons: atlas projector is required")
	}
	return &storeRegistry{
		shipped:   cfg.Shipped,
		world:     cfg.World,
		authoring: cfg.Authoring,
		projector: cfg.Projector,
		putLocks:  make(map[putLockKey]*sync.Mutex),
	}, nil
}

// List implements Registry.
func (r *storeRegistry) List(ctx context.Context, caller Caller) ([]Summary, error) {
	if !caller.complete() {
		return nil, ErrNoCaller
	}
	listed, err := r.world.List(ctx, &worlddungeons.ListInput{WorldID: caller.WorldID})
	if err != nil {
		return nil, fmt.Errorf("list dungeons in world %q: %w", caller.WorldID, err)
	}
	held := make(map[string]bool, len(listed.Records))
	out := make([]Summary, 0, len(listed.Records)+len(r.shipped.entries))
	for _, rec := range listed.Records {
		held[rec.Key] = true
		out = append(out, Summary{Key: rec.Key, Name: listedName(rec), Owner: rec.Owner, Source: SourceWorld})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Key < out[j].Key })

	references := make([]Summary, 0, len(r.shipped.entries))
	for _, e := range r.shipped.entries {
		if !e.SingleRoom || held[e.Key] {
			continue
		}
		references = append(references, Summary{Key: e.Key, Name: e.Name, Source: SourceShipped})
	}
	sort.Slice(references, func(i, j int) bool { return references[i].Key < references[j].Key })

	return append(out, references...), nil
}

// listedName is the name stored with a world record when it was saved (the
// compile's own Name, written by Put): List never compiles. A record stored
// without one is listed under its key.
func listedName(rec worlddungeons.Record) string {
	if rec.Name == "" {
		return rec.Key
	}
	return rec.Name
}

// Get implements Registry.
func (r *storeRegistry) Get(ctx context.Context, caller Caller, key string) (*Entry, error) {
	if !caller.complete() {
		return nil, ErrNoCaller
	}
	rec, found, err := r.worldRecord(ctx, caller.WorldID, key)
	if err != nil {
		return nil, err
	}
	if !found {
		return r.shipped.Get(ctx, key)
	}
	entry, ferrs, err := compileEntry(ctx, r.projector, rec.YAML)
	if err != nil {
		return nil, fmt.Errorf("dungeon %q in world %q: %w", key, caller.WorldID, err)
	}
	if len(ferrs) > 0 {
		return nil, fmt.Errorf("dungeon %q in world %q no longer compiles: %s", key, caller.WorldID, joinErrors(ferrs))
	}
	if entry.Key != key {
		return nil, fmt.Errorf("dungeon %q in world %q: stored file's key line says %q: %w",
			key, caller.WorldID, entry.Key, ErrKeyMismatch)
	}
	entry.Source = SourceWorld
	entry.Owner = rec.Owner
	return entry, nil
}

// worldRecord reads key from the world store; absence is (false, nil).
func (r *storeRegistry) worldRecord(ctx context.Context, worldID, key string) (worlddungeons.Record, bool, error) {
	out, err := r.world.Get(ctx, &worlddungeons.GetInput{WorldID: worldID, Key: key})
	if errors.Is(err, worlddungeons.ErrNotFound) {
		return worlddungeons.Record{}, false, nil
	}
	if err != nil {
		return worlddungeons.Record{}, false, fmt.Errorf("dungeon %q in world %q: %w", key, worldID, err)
	}
	return out.Record, true, nil
}

// Put implements Registry. The refusals that need no compile come first, for
// validate-only too, so the builder's preview names them at the key field.
func (r *storeRegistry) Put(ctx context.Context, in *PutInput) (*PutResult, error) {
	if in == nil {
		return nil, errors.New("dungeons: PutInput is required")
	}
	if !in.Caller.complete() {
		return nil, ErrNoCaller
	}
	if !keyPattern.MatchString(in.Key) {
		return nil, fmt.Errorf("dungeon key %q: %w", in.Key, ErrInvalidKey)
	}
	if r.shipped.has(in.Key) {
		return nil, fmt.Errorf("dungeon %q: %w", in.Key, ErrReferenceKey)
	}

	unlock := r.lockKey(in.Caller.WorldID, in.Key)
	defer unlock()

	rec, found, err := r.worldRecord(ctx, in.Caller.WorldID, in.Key)
	if err != nil {
		return nil, err
	}
	if found && rec.Owner != in.Caller.PlayerID {
		return nil, fmt.Errorf("dungeon %q: %w", in.Key, ErrOwnedByOther)
	}

	// The caller's slice is copied ONCE here and never touched again: what
	// is compiled, stored and served are the same bytes.
	raw := bytes.Clone(in.YAML)
	entry, ferrs, err := compileEntry(ctx, r.projector, raw)
	if err != nil {
		return nil, fmt.Errorf("dungeon %q: %w", in.Key, err)
	}
	if len(ferrs) > 0 {
		return &PutResult{Errors: ferrs}, nil
	}
	if entry.Key != in.Key {
		return nil, fmt.Errorf("dungeon %q: file's key line says %q: %w", in.Key, entry.Key, ErrKeyMismatch)
	}
	entry.Source = SourceWorld
	entry.Owner = in.Caller.PlayerID
	if in.ValidateOnly {
		return &PutResult{Entry: entry}, nil
	}
	// THE GATE IS ON THE WRITE, NOT ON THE GRADE (rpg-project#481).
	if !r.authoring {
		return nil, ErrAuthoringDisabled
	}

	_, err = r.world.Put(ctx, &worlddungeons.PutInput{
		WorldID: in.Caller.WorldID,
		Owner:   in.Caller.PlayerID,
		Key:     in.Key,
		Name:    entry.Name,
		YAML:    raw,
	})
	if errors.Is(err, worlddungeons.ErrOwnedByOther) {
		return nil, fmt.Errorf("dungeon %q: %w", in.Key, ErrOwnedByOther)
	}
	if err != nil {
		return nil, fmt.Errorf("dungeon %q in world %q: %w", in.Key, in.Caller.WorldID, err)
	}

	return &PutResult{Entry: entry.clone()}, nil
}

// lockKey acquires the per-world, per-key Put lock and returns its unlock.
func (r *storeRegistry) lockKey(worldID, key string) func() {
	r.putMu.Lock()
	k := putLockKey{worldID: worldID, key: key}
	l, ok := r.putLocks[k]
	if !ok {
		l = &sync.Mutex{}
		r.putLocks[k] = l
	}
	r.putMu.Unlock()

	l.Lock()
	return l.Unlock
}

// Delete implements Registry.
func (r *storeRegistry) Delete(ctx context.Context, caller Caller, key string) error {
	if !caller.complete() {
		return ErrNoCaller
	}
	if !r.authoring {
		return ErrAuthoringDisabled
	}
	if !keyPattern.MatchString(key) {
		return fmt.Errorf("dungeon key %q: %w", key, ErrInvalidKey)
	}
	_, err := r.world.Delete(ctx, &worlddungeons.DeleteInput{
		WorldID: caller.WorldID,
		Owner:   caller.PlayerID,
		Key:     key,
	})
	switch {
	case err == nil:
		return nil
	case errors.Is(err, worlddungeons.ErrOwnedByOther):
		return fmt.Errorf("dungeon %q: %w", key, ErrOwnedByOther)
	case errors.Is(err, worlddungeons.ErrNotFound):
		return r.absent(key)
	default:
		return fmt.Errorf("dungeon %q in world %q: %w", key, caller.WorldID, err)
	}
}

// absent is the refusal for a rename or delete of a key the world does not
// hold: a reference cannot be renamed or deleted; anything else is not found.
func (r *storeRegistry) absent(key string) error {
	if r.shipped.has(key) {
		return fmt.Errorf("dungeon %q: %w", key, ErrReferenceReadOnly)
	}
	return fmt.Errorf("dungeon %q: %w", key, ErrNotFound)
}

// Rename implements Registry.
func (r *storeRegistry) Rename(ctx context.Context, in *RenameInput) error {
	if in == nil {
		return errors.New("dungeons: RenameInput is required")
	}
	if !in.Caller.complete() {
		return ErrNoCaller
	}
	if !r.authoring {
		return ErrAuthoringDisabled
	}
	for _, key := range []string{in.Key, in.NewKey} {
		if !keyPattern.MatchString(key) {
			return fmt.Errorf("dungeon key %q: %w", key, ErrInvalidKey)
		}
	}
	if r.shipped.has(in.NewKey) {
		return fmt.Errorf("dungeon %q: %w", in.NewKey, ErrReferenceKey)
	}

	// The source is checked first so the store's ErrOwnedByOther below can
	// only be about the new key.
	rec, found, err := r.worldRecord(ctx, in.Caller.WorldID, in.Key)
	if err != nil {
		return err
	}
	if !found {
		return r.absent(in.Key)
	}
	if rec.Owner != in.Caller.PlayerID {
		return fmt.Errorf("dungeon %q: %w", in.Key, ErrOwnedByOther)
	}

	_, err = r.world.Rename(ctx, &worlddungeons.RenameInput{
		WorldID: in.Caller.WorldID,
		Owner:   in.Caller.PlayerID,
		Key:     in.Key,
		NewKey:  in.NewKey,
		Rewrite: func(old []byte) ([]byte, error) { return r.rewrite(ctx, old, in.Key, in.NewKey) },
	})
	switch {
	case err == nil:
		return nil
	case errors.Is(err, worlddungeons.ErrKeyExists):
		return fmt.Errorf("dungeon %q: %w", in.NewKey, ErrKeyExists)
	case errors.Is(err, worlddungeons.ErrOwnedByOther):
		return fmt.Errorf("dungeon %q: %w", in.NewKey, ErrOwnedByOther)
	case errors.Is(err, worlddungeons.ErrNotFound):
		return r.absent(in.Key)
	default:
		return fmt.Errorf("rename dungeon %q to %q in world %q: %w", in.Key, in.NewKey, in.Caller.WorldID, err)
	}
}

// rewrite is Rename's transform: the one root key line changed, the result
// compiled, and refused unless it names the new key.
func (r *storeRegistry) rewrite(ctx context.Context, old []byte, key, newKey string) ([]byte, error) {
	next, err := rewriteKeyLine(old, key, newKey)
	if err != nil {
		return nil, err
	}
	entry, ferrs, err := compileEntry(ctx, r.projector, next)
	if err != nil {
		return nil, fmt.Errorf("renamed dungeon %q: %w", newKey, err)
	}
	if len(ferrs) > 0 {
		return nil, fmt.Errorf("renamed dungeon %q does not compile: %s", newKey, joinErrors(ferrs))
	}
	if entry.Key != newKey {
		return nil, fmt.Errorf("renamed dungeon %q: file's key line says %q: %w", newKey, entry.Key, ErrKeyMismatch)
	}
	return next, nil
}
