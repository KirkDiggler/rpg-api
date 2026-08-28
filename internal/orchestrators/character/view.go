package character

import (
	"context"
	"errors"
	"fmt"
	"maps"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-toolkit/events"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/races"
)

const (
	// CharacterDataUnavailableMessage is the API-safe wording for a strict
	// private character projection failure. The detailed cause stays wrapped
	// internally and is never used as transport text.
	CharacterDataUnavailableMessage = "character data unavailable"

	errViewPlayerIDMissing = "project character identity: player ID is required"
	errViewClassIDMissing  = "project character identity: class ID is required"
	errViewRaceIDMissing   = "project character identity: race ID is required"
)

// IdentityView is the detached owner and typed class/race identity needed by
// the existing CharacterData contract.
type IdentityView struct {
	PlayerID string
	ClassID  classes.Class
	RaceID   races.Race
}

// View is the detached, owner-private character projection returned across the
// orchestrator boundary. It deliberately exposes no live Character and no
// persistence JSON.
type View struct {
	Identity  IdentityView
	Equipment *tkcharacter.EquipmentView
	Status    *tkcharacter.StatusView
}

// ProjectViewInput contains persisted character data to project strictly.
type ProjectViewInput struct {
	Data *tkcharacter.Data
}

// ProjectViewOutput contains the detached projection.
type ProjectViewOutput struct {
	View *View
}

// ProjectLoadedCharacterInput contains an already strictly loaded and attached
// character. Equip and unequip use this after mutating their in-memory sheet so
// the complete post-state is projected before persistence.
type ProjectLoadedCharacterInput struct {
	Character *tkcharacter.Character
}

// ProjectLoadedCharacterOutput contains the detached projection.
type ProjectLoadedCharacterOutput struct {
	View *View
}

type loadAttachedCharacterInput struct {
	Data *tkcharacter.Data
}

type loadAttachedCharacterOutput struct {
	Character *tkcharacter.Character
	Data      *tkcharacter.Data
}

type projectLoadedCharacterFunc func(
	context.Context,
	*ProjectLoadedCharacterInput,
) (*ProjectLoadedCharacterOutput, error)

// ProjectView is the one persisted-data projection path. Strict Load refuses
// malformed condition, feature, item, and resource data instead of silently
// dropping it; Attach applies every loaded effect before either detached view
// is composed.
func ProjectView(ctx context.Context, input *ProjectViewInput) (*ProjectViewOutput, error) {
	if input == nil {
		return nil, apierr.InvalidArgument("input is required")
	}

	loaded, err := loadAttachedCharacter(ctx, &loadAttachedCharacterInput{Data: input.Data})
	if err != nil {
		return nil, characterDataUnavailable(err)
	}
	projected, err := projectLoadedCharacter(ctx, &ProjectLoadedCharacterInput{Character: loaded.Character})
	if err != nil {
		return nil, characterDataUnavailable(err)
	}

	return &ProjectViewOutput{View: projected.View}, nil
}

func characterDataUnavailable(cause error) *apierr.Error {
	return apierr.WrapWithCode(cause, apierr.CodeInternal, CharacterDataUnavailableMessage)
}

func loadAttachedCharacter(
	ctx context.Context,
	input *loadAttachedCharacterInput,
) (*loadAttachedCharacterOutput, error) {
	if input == nil {
		return nil, apierr.InvalidArgument("input is required")
	}

	// The toolkit retains Data.EquipmentSlots on the loaded sheet and its
	// Equip/Unequip verbs mutate that map in place. Load from a working struct
	// copy with an isolated slots map so a failed projection or repository
	// patch cannot mutate a cached/pointer-returning repository entity. Every
	// other field stays a direct struct copy: opaque JSON, slices, and unrelated
	// maps are preserved without a serialization round trip or reinterpretation
	// because this mutation path does not write them.
	workingData := input.Data
	if input.Data != nil {
		workingCopy := *input.Data
		workingCopy.EquipmentSlots = maps.Clone(input.Data.EquipmentSlots)
		workingData = &workingCopy
	}

	char, err := tkcharacter.Load(ctx, workingData)
	if err != nil {
		return nil, fmt.Errorf("strictly load character: %w", err)
	}
	if err := tkcharacter.Attach(ctx, char, events.NewEventBus()); err != nil {
		return nil, fmt.Errorf("attach character: %w", err)
	}

	return &loadAttachedCharacterOutput{Character: char, Data: workingData}, nil
}

// projectLoadedCharacter composes both detached views from one attached live
// sheet. StatusView is fallible and therefore completes before any caller may
// persist or return a partial projection.
func projectLoadedCharacter(
	ctx context.Context,
	input *ProjectLoadedCharacterInput,
) (*ProjectLoadedCharacterOutput, error) {
	if input == nil {
		return nil, apierr.InvalidArgument("input is required")
	}
	if input.Character == nil {
		return nil, apierr.InvalidArgument("character is required")
	}

	data := input.Character.ToData()
	if data.PlayerID == "" {
		return nil, errors.New(errViewPlayerIDMissing)
	}
	if data.ClassID == "" {
		return nil, errors.New(errViewClassIDMissing)
	}
	if data.RaceID == "" {
		return nil, errors.New(errViewRaceIDMissing)
	}

	// EquipmentView carries a FOLDED armour class rather than the scalar on the
	// sheet, so it can refuse (rpg-toolkit#1276). A refusal must surface: the
	// alternative is a projection reporting base armour as though it were the
	// whole answer, which is the bug this chain exists to close.
	equipment, err := input.Character.EquipmentView(ctx)
	if err != nil {
		return nil, fmt.Errorf("project character equipment: %w", err)
	}

	status, err := input.Character.StatusView(&tkcharacter.StatusViewInput{})
	if err != nil {
		return nil, fmt.Errorf("project character status: %w", err)
	}
	if status == nil || status.View == nil {
		return nil, fmt.Errorf("project character status: toolkit returned no view")
	}

	return &ProjectLoadedCharacterOutput{View: &View{
		Identity: IdentityView{
			PlayerID: data.PlayerID,
			ClassID:  data.ClassID,
			RaceID:   data.RaceID,
		},
		Equipment: equipment,
		Status:    status.View,
	}}, nil
}
