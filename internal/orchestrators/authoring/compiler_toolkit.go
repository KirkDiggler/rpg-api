package authoring

import (
	"context"
	"errors"

	"github.com/KirkDiggler/rpg-toolkit/encounter/dungeonspec"
)

// toolkitCompiler is the production adapter to the protobuf-free toolkit
// provider. It makes exactly one complete-candidate CompileDungeon call and
// maps only provider-owned compiled state, floor projection, and field errors.
type toolkitCompiler struct{}

func (toolkitCompiler) CompileDungeon(
	ctx context.Context,
	in *CompileDungeonInput,
) (*CompileDungeonOutput, error) {
	if in == nil {
		return nil, errors.New("authoring compiler: CompileDungeonInput is required")
	}

	var mode dungeonspec.CompileMode
	switch in.Mode {
	case CompileModeDraft:
		mode = dungeonspec.CompileModeDraft
	case CompileModeStrict:
		mode = dungeonspec.CompileModeStrict
	default:
		return nil, errors.New("authoring compiler: compile mode must be draft or strict")
	}

	providerOutput, err := dungeonspec.CompileDungeon(ctx, &dungeonspec.CompileDungeonInput{
		Source:              in.Source,
		Mode:                mode,
		PartyStartSeatCount: in.PartyStartSeatCount,
		PreviewSeed:         in.PreviewSeed,
	})
	if err != nil {
		return nil, err
	}
	if providerOutput == nil {
		return nil, errors.New("authoring compiler: provider returned no output")
	}

	output := &CompileDungeonOutput{
		Compiled:  providerOutput.Compiled,
		FloorPlan: mapFloorPlan(providerOutput.FloorPlan),
	}
	if providerOutput.FieldErrors != nil {
		output.FieldErrors = make([]FieldError, len(providerOutput.FieldErrors))
		for index, fieldError := range providerOutput.FieldErrors {
			output.FieldErrors[index] = FieldError{
				Field: fieldError.Field, Message: fieldError.Message, Code: fieldError.Code,
			}
		}
	}
	return output, nil
}
