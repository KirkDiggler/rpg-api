// Package authoring is the AuthoringService orchestrator: the dungeon
// builder's seam onto the content registry (rpg-api#806, rpg-project#256).
//
// It is thin on purpose. Compilation, validation and the write discipline
// live in internal/dungeons (and through it the toolkit); this package only
// shapes the registry's answers into the verbs the wire speaks. No rule
// lives here and no geometry is computed here.
package authoring

import (
	"context"
	"errors"
	"fmt"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	"github.com/KirkDiggler/rpg-api/internal/dungeons"
)

// Config holds the dependencies for an Orchestrator.
type Config struct {
	// Dungeons is the content registry PutDungeon writes to and GetDungeon
	// reads from. Required.
	Dungeons dungeons.Registry
}

// Orchestrator is the AuthoringService's business logic.
type Orchestrator struct {
	dungeons dungeons.Registry
}

// New constructs an Orchestrator. Returns an error (never a nil
// Orchestrator) when a required dependency is missing.
func New(cfg *Config) (*Orchestrator, error) {
	if cfg == nil {
		return nil, errors.New("authoring orchestrator: Config is required")
	}
	if cfg.Dungeons == nil {
		return nil, errors.New("authoring orchestrator: Config.Dungeons is required")
	}

	return &Orchestrator{dungeons: cfg.Dungeons}, nil
}

// PutDungeonInput is one PutDungeon call.
type PutDungeonInput struct {
	Key          string
	YAML         []byte
	ValidateOnly bool
}

// PutDungeonOutput is PutDungeon's answer on a well-formed request: Errors
// (did not compile, nothing written) or Atlas (compiled; stored unless
// ValidateOnly). Exactly one of the two is meaningful; an empty Errors IS
// success.
type PutDungeonOutput struct {
	Errors []dungeons.FieldError

	// Atlas is the compiled map, the same shape GetAtlas serves.
	//
	// TODO(256): nil until session.AtlasOf lands (plan T3); see
	// dungeons.Entry.Atlas.
	Atlas *sdk.Atlas
}

// PutDungeon compiles and, unless ValidateOnly, stores a dungeon. Registry
// sentinels (dungeons.ErrInvalidKey, ErrKeyMismatch, ErrAuthoringDisabled)
// pass through for the handler to map.
func (o *Orchestrator) PutDungeon(ctx context.Context, in *PutDungeonInput) (*PutDungeonOutput, error) {
	if in == nil {
		return nil, errors.New("authoring orchestrator: PutDungeonInput is required")
	}

	res, err := o.dungeons.Put(ctx, &dungeons.PutInput{
		Key: in.Key, YAML: in.YAML, ValidateOnly: in.ValidateOnly,
	})
	if err != nil {
		return nil, fmt.Errorf("put dungeon: %w", err)
	}
	if len(res.Errors) > 0 {
		return &PutDungeonOutput{Errors: res.Errors}, nil
	}

	return &PutDungeonOutput{Atlas: res.Entry.Atlas}, nil
}

// GetDungeonInput names a stored dungeon.
type GetDungeonInput struct {
	Key string
}

// GetDungeonOutput is the stored file, verbatim.
type GetDungeonOutput struct {
	YAML []byte
}

// GetDungeon returns the stored file for a key. dungeons.ErrNotFound passes
// through for the handler to map.
func (o *Orchestrator) GetDungeon(ctx context.Context, in *GetDungeonInput) (*GetDungeonOutput, error) {
	if in == nil {
		return nil, errors.New("authoring orchestrator: GetDungeonInput is required")
	}

	entry, err := o.dungeons.Get(ctx, in.Key)
	if err != nil {
		return nil, fmt.Errorf("get dungeon: %w", err)
	}

	return &GetDungeonOutput{YAML: entry.YAML}, nil
}
