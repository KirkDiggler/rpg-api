package character

import (
	"context"
	"errors"
	"fmt"
	"maps"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/combat"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/currency"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/races"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/resolution"
)

const (
	// CharacterDataUnavailableMessage is the API-safe wording for a strict
	// private character projection failure. The detailed cause stays wrapped
	// internally and is never used as transport text.
	CharacterDataUnavailableMessage = "character data unavailable"

	errViewPlayerIDMissing = "project character identity: player ID is required"
	errViewClassIDMissing  = "project character identity: class ID is required"
	errViewRaceIDMissing   = "project character identity: race ID is required"
	errViewFoldMissing     = "project character equipment: toolkit returned no armour class or equipment view"
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

	// Wallet is the character's persistent coin purse (rpg-toolkit#1533),
	// carried straight off Data.Wallet rather than through StatusView -- the
	// toolkit's own StatusView does not project it (same shape as
	// npcs.StockEntryView not carrying a vendor price), so this is the raw
	// persisted field, not a toolkit computation.
	Wallet currency.Money
}

// ProjectViewInput contains persisted character data to project strictly.
type ProjectViewInput struct {
	Data *tkcharacter.Data
}

// ProjectViewOutput contains the detached projection.
type ProjectViewOutput struct {
	View *View
}

// ProjectLoadedCharacterInput contains an already strictly loaded character.
// Equip and unequip use this after mutating their in-memory sheet so the
// complete post-state is projected before persistence.
type ProjectLoadedCharacterInput struct {
	Character *tkcharacter.Character
}

// ProjectLoadedCharacterOutput contains the detached projection and the folded
// armour class it was computed with.
type ProjectLoadedCharacterOutput struct {
	View *View

	// ArmorClass is the toolkit's folded armour class for the projected sheet —
	// the number equip and unequip persist. View.Equipment's AC total is a second
	// fold of the same sheet under the same installed context (EquipmentView
	// folds again); the two agree because the AC chain is deterministic, not
	// because one value feeds both.
	ArmorClass *combat.ACBreakdown
}

type loadCharacterInput struct {
	Data *tkcharacter.Data
}

type loadCharacterOutput struct {
	Character *tkcharacter.Character
	Data      *tkcharacter.Data
}

type projectLoadedCharacterFunc func(
	context.Context,
	*ProjectLoadedCharacterInput,
) (*ProjectLoadedCharacterOutput, error)

// ProjectView is the one persisted-data projection path. Strict Load refuses
// malformed condition, feature, item, and resource data instead of silently
// dropping it; the folded halves of the view come from the toolkit's
// resolution door, never from a sheet this package attached itself.
func ProjectView(ctx context.Context, input *ProjectViewInput) (*ProjectViewOutput, error) {
	if input == nil {
		return nil, apierr.InvalidArgument("input is required")
	}

	loaded, err := loadCharacter(ctx, &loadCharacterInput{Data: input.Data})
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

// loadCharacter strictly loads persisted data into an inert sheet: no bus, no
// subscriptions. The sheet is what the equip verbs mutate and what StatusView
// reads; anything folded (armour class, the equipment view) is asked of
// resolution.ProjectCharacter, which installs the game context a fold needs.
// Attaching here instead would be a host building rules truth on its own bus,
// and Unarmored Defense refuses that fold outright (gamectx.ErrNotInCast).
func loadCharacter(
	ctx context.Context,
	input *loadCharacterInput,
) (*loadCharacterOutput, error) {
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

	return &loadCharacterOutput{Character: char, Data: workingData}, nil
}

// projectLoadedCharacter composes both detached views from one loaded sheet:
// the equipment view and armour class from the sheet's record through
// resolution.ProjectCharacter, the status view from the sheet itself. Every
// half is fallible and completes before any caller may persist or return a
// partial projection.
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

	data, err := input.Character.ToData()
	if err != nil {
		return nil, fmt.Errorf("serialize character: %w", err)
	}
	if data.PlayerID == "" {
		return nil, errors.New(errViewPlayerIDMissing)
	}
	if data.ClassID == "" {
		return nil, errors.New(errViewClassIDMissing)
	}
	if data.RaceID == "" {
		return nil, errors.New(errViewRaceIDMissing)
	}

	// The equipment view carries a FOLDED armour class rather than the scalar on
	// the sheet, and a fold needs the game context only resolution installs: a
	// monk's Unarmored Defense reads WIS through the cast. A refusal surfaces;
	// the alternative is a projection reporting 10+DEX as though it were the
	// whole answer, which is the bug this path exists to close
	// (rpg-toolkit#1276, #1965).
	projected, err := resolution.ProjectCharacter(ctx, &resolution.ProjectCharacterInput{Character: data})
	if err != nil {
		return nil, fmt.Errorf("project character equipment: %w", err)
	}
	if projected.ArmorClass == nil || projected.Equipment == nil {
		return nil, errors.New(errViewFoldMissing)
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
		Equipment: projected.Equipment,
		Status:    status.View,
		Wallet:    data.Wallet,
	}, ArmorClass: projected.ArmorClass}, nil
}
