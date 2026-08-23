package lobby

import (
	"context"
	"fmt"

	"github.com/KirkDiggler/rpg-api/internal/dungeons"
)

// ListDungeonsInput is ListDungeons' (empty) request.
type ListDungeonsInput struct{}

// ListDungeonsOutput is every dungeon StartEncounter can build, sorted by
// key.
type ListDungeonsOutput struct {
	Dungeons []dungeons.Summary
}

// ListDungeons reads the content registry. UNGATED — it mutates nothing and
// the lobby's picker needs it with authoring off (design.md §3c).
func (o *Orchestrator) ListDungeons(ctx context.Context, _ *ListDungeonsInput) (*ListDungeonsOutput, error) {
	list, err := o.dungeons.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list dungeons: %w", err)
	}

	return &ListDungeonsOutput{Dungeons: list}, nil
}
