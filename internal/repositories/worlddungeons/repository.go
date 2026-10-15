// Package worlddungeons stores each world's authored dungeon files, verbatim,
// with their owners. It does not compile, grade or name the files: the key
// grammar, the compile, the reference refusals and the key-line rewrite
// belong to the dungeon registry. The owner check on a write is this
// store's, inside the write's transaction, so two authors racing for one key
// cannot both win.
package worlddungeons

import (
	"context"
	"errors"
)

//go:generate mockgen -destination=mock/mock_repository.go -package=worlddungeonsmock github.com/KirkDiggler/rpg-api/internal/repositories/worlddungeons Repository

// ErrNotFound reports that the world holds no record under the key.
var ErrNotFound = errors.New("world dungeon not found")

// ErrKeyExists reports that a rename's new key already names one of the
// owner's own records, or equals the key being renamed.
var ErrKeyExists = errors.New("world dungeon key already exists")

// ErrOwnedByOther reports that the key is held in the world by another
// player.
var ErrOwnedByOther = errors.New("world dungeon is owned by another player")

// Record is one authored dungeon file in a world: its key, the player who
// owns it, the name the registry compiled from it when it was saved, and the
// file's bytes exactly as they were saved. The store never reads Name out of
// YAML; it keeps what the writer gave it, so a list needs no compile.
type Record struct {
	Key   string
	Owner string
	Name  string
	YAML  []byte
}

// Repository stores each world's dungeon files under their keys.
type Repository interface {
	// Get returns the record under the key in the world, or ErrNotFound.
	Get(ctx context.Context, in *GetInput) (*GetOutput, error)
	// List returns every record in the world sorted by key; a new world is empty.
	List(ctx context.Context, in *ListInput) (*ListOutput, error)
	// Put creates the record or replaces the owner's own; another owner's key is ErrOwnedByOther.
	Put(ctx context.Context, in *PutInput) (*PutOutput, error)
	// Delete removes the owner's own record; ErrNotFound or ErrOwnedByOther otherwise.
	Delete(ctx context.Context, in *DeleteInput) (*DeleteOutput, error)
	// Rename moves the owner's record to NewKey with the bytes Rewrite returns.
	Rename(ctx context.Context, in *RenameInput) (*RenameOutput, error)
}

// GetInput addresses one record in a world.
type GetInput struct {
	WorldID string
	Key     string
}

// GetOutput carries the record.
type GetOutput struct {
	Record Record
}

// ListInput scopes the listing to one world.
type ListInput struct {
	WorldID string
}

// ListOutput carries the world's records sorted by key.
type ListOutput struct {
	Records []Record
}

// PutInput is one save of a file under a key, by its owner.
type PutInput struct {
	WorldID string
	Owner   string
	Key     string
	// Name is the file's display name as the writer compiled it; empty is
	// stored as absent and read back empty.
	Name string
	YAML []byte
}

// PutOutput confirms the save.
type PutOutput struct{}

// DeleteInput addresses the owner's record to remove.
type DeleteInput struct {
	WorldID string
	Owner   string
	Key     string
}

// DeleteOutput confirms the removal.
type DeleteOutput struct{}

// RenameInput moves the owner's record from Key to NewKey, keeping its owner
// and its stored Name (a rename changes the key line only). Rewrite receives
// the stored bytes and returns the bytes to store under NewKey; an error
// from it is returned and nothing is written. Rewrite runs inside the
// transaction and may be called again when the transaction retries.
type RenameInput struct {
	WorldID string
	Owner   string
	Key     string
	NewKey  string
	Rewrite func(old []byte) ([]byte, error)
}

// RenameOutput confirms the move.
type RenameOutput struct{}
