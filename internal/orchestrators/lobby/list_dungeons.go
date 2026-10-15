package lobby

import (
	"context"
	"errors"
	"fmt"

	"github.com/KirkDiggler/rpg-api/internal/dungeons"
)

// ListDungeonsInput is ListDungeons' request: who is asking, from the auth
// boundary.
type ListDungeonsInput struct {
	Caller dungeons.Caller
}

// ListDungeonsOutput is every dungeon the caller's StartEncounter can build:
// their world's encounters by key, then the single-room references by key.
type ListDungeonsOutput struct {
	Dungeons []dungeons.Summary
}

// ListDungeons reads the content registry. UNGATED — it mutates nothing and
// the lobby's picker needs it with authoring off (design.md §3c).
func (o *Orchestrator) ListDungeons(ctx context.Context, in *ListDungeonsInput) (*ListDungeonsOutput, error) {
	if in == nil {
		return nil, errors.New("list dungeons input is required")
	}
	list, err := o.dungeons.List(ctx, in.Caller)
	if err != nil {
		return nil, fmt.Errorf("list dungeons: %w", err)
	}

	return &ListDungeonsOutput{Dungeons: list}, nil
}
