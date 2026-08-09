package authoring

import (
	"context"
	"errors"

	"github.com/KirkDiggler/rpg-toolkit/encounter/dungeonspec"
)

// toolkitCompiler adapts the released v0.3 provider until rpg-toolkit#897
// publishes the native CompileDungeon contract. Its supported authored canvas
// source is bounds only: a regions candidate is rejected by toolkit decode,
// never stripped or downgraded here. The Wave A provider must replace this
// adapter's load/build pair and populate the resolved source itself.
type toolkitCompiler struct{}

func (toolkitCompiler) CompileDungeon(
	ctx context.Context,
	in *CompileDungeonInput,
) (*CompileDungeonOutput, error) {
	if in == nil {
		return nil, errors.New("authoring compiler: CompileDungeonInput is required")
	}

	config := dungeonspec.LoadConfig{PartyStartSeatCount: in.PartyStartSeatCount}
	var (
		compiled dungeonspec.CompiledDungeon
		err      error
	)
	if in.Previous != nil {
		compiled, err = dungeonspec.LoadWithPrevious(in.Source, config, *in.Previous)
	} else {
		compiled, err = dungeonspec.LoadWithConfig(in.Source, config)
	}
	if err != nil {
		return &CompileDungeonOutput{FieldErrors: []FieldError{{Message: err.Error()}}}, nil
	}

	floorPlan, err := buildFloorPlan(ctx, compiled, in.PreviewSeed)
	if err != nil {
		return &CompileDungeonOutput{FieldErrors: []FieldError{{Message: err.Error()}}}, nil
	}
	// The released adapter only accepts the legacy topology contract. It does
	// not inspect dimensions/cells and cannot turn an unsupported regions
	// source into bounds; toolkit decode has already rejected such a source.
	floorPlan.FloorSource = FloorSourceBounds

	return &CompileDungeonOutput{
		Compiled:  compiled,
		FloorPlan: floorPlan,
		Name:      captureName(in.Source, ""),
	}, nil
}
