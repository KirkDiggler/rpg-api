package lobby

import (
	"context"

	"github.com/KirkDiggler/rpg-api/internal/dungeonregistry"
)

// ListDungeonsInput carries the (today empty) ListDungeons request.
type ListDungeonsInput struct{}

// ListDungeonsOutput carries every dungeon key + display name the server
// can StartEncounter from.
type ListDungeonsOutput struct {
	Dungeons []dungeonregistry.DungeonSummary
}

// ListDungeons returns every dungeon key + display name the server can
// StartEncounter from — NOT gated behind authoring (design.md's
// post-approval correction, plan.md S0): it reads content and mutates
// nothing, and the lobby dropdown needs this to work with the authoring
// gate off. Sources directly from o.registry.Keys(), which already
// excludes any load-failed (disabled) key — offering an unplayable
// dungeon in the dropdown would be worse than a temporarily-shorter list.
func (o *Orchestrator) ListDungeons(_ context.Context, _ *ListDungeonsInput) (*ListDungeonsOutput, error) {
	return &ListDungeonsOutput{Dungeons: o.registry.Keys()}, nil
}
