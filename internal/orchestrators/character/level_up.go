// Copyright (C) 2026 Kirk Diggler
// SPDX-License-Identifier: GPL-3.0-or-later

package character

import (
	"context"
	"errors"
	"fmt"

	"github.com/KirkDiggler/rpg-toolkit/rpgerr"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character/choices"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

// GetNextLevel describes the level this character would take next.
//
// IT IS A READ, AND IT ANSWERS REGARDLESS OF ENTITLEMENT. Design R4.11 puts
// the refusal on the write: "Advance MUST refuse a level the character is not
// entitled to, naming the total and the threshold." A screen that cannot
// describe an unearned level cannot tell a player what they are working
// toward, and the gap between entitlement and level is the signal the client
// already holds from the projected sheet (R4.10).
//
// Every fact it returns comes from a toolkit table indexed by the level, never
// from anything stored: R2.3 says "no per-level fact may be expressible only
// in code", and an orchestrator that remembered what a level brings would be
// exactly that code.
func (o *Orchestrator) GetNextLevel(ctx context.Context, input *GetNextLevelInput) (*GetNextLevelOutput, error) {
	if input == nil {
		return nil, apierr.InvalidArgument("input is required")
	}
	if input.CharacterID == "" {
		return nil, apierr.InvalidArgument("character ID is required")
	}

	current, err := o.characterRepo.Get(ctx, characterrepo.GetInput{ID: input.CharacterID})
	if err != nil {
		return nil, fmt.Errorf("failed to get character: %w", err)
	}
	if current == nil || current.Character == nil || current.Character.Data == nil {
		return nil, characterDataUnavailable(errors.New("character repository returned no character data"))
	}

	loaded, loadErr := loadAttachedCharacter(ctx, &loadAttachedCharacterInput{Data: current.Character.Data})
	if loadErr != nil {
		return nil, characterDataUnavailable(fmt.Errorf("failed to load character: %w", loadErr))
	}
	char := loaded.Character

	// The class is the character's own. Until multiclassing exists a level can
	// only be taken in it (toolkit R2.4), so there is nothing here to choose
	// and nothing for a caller to name.
	classID := current.Character.Data.ClassID

	// TWO DIFFERENT LEVELS, and they are not interchangeable. The character
	// level is what the sheet will read; the class level is what indexes every
	// table below (R4.6: "Grants and class resources are indexed by this,
	// never by CharacterLevel"). They agree only while nobody multiclasses.
	characterLevel := char.GetLevel() + 1
	classLevel := char.ClassLevel(classID) + 1

	// A class with no table is not a class with a d0 hit die. Refusing here
	// keeps a missing row from reaching the screen as a number.
	classData := classes.GetData(classID)
	if classData == nil {
		return nil, characterDataUnavailable(fmt.Errorf("class %q has no class data", classID))
	}

	featureRefs := make([]string, 0)
	for _, grant := range classes.GetGrantsGainedAtLevel(classID, classLevel) {
		for _, feature := range grant.Features {
			featureRefs = append(featureRefs, feature.Ref)
		}
	}

	return &GetNextLevelOutput{
		CharacterLevel: characterLevel,
		ClassID:        classID,
		ClassLevel:     classLevel,
		// GAINED AT, not cumulative. The cumulative set would re-offer every
		// level-1 choice the character already made (toolkit R3.2).
		Requirements: choices.GetClassRequirementsGainedAtLevel(classID, classLevel),
		FeatureRefs:  featureRefs,
		HitDice:      classData.HitDice,
	}, nil
}

// LevelUp takes one level, atomically, and persists the sheet it produced.
//
// The shape is EquipItem's -- load, toolkit verb, project, save
// (orchestrator.go's EquipItem) -- with one deliberate difference: the write
// is the repository's whole-sheet Update rather than the narrow equipment
// patch, because a level moves Level, Levels, MaxHitPoints, ProficiencyBonus,
// Features, Conditions, ClassResources and known spells at once, and
// PatchEquipment is contractually permitted to write only slots and armor
// class.
//
// EVERY RULE IS THE TOOLKIT'S. Whether the level is earned, what it grants,
// how many hit points it adds, whether the submitted choices answer what the
// level asked: all of it is inside Character.Advance, and all this method does
// with a refusal is carry its code and its message out to the client. A check
// here would be a game rule in the API, and an API that can decide when a
// level is earned is one that can grant one (design §4.3).
func (o *Orchestrator) LevelUp(ctx context.Context, input *LevelUpInput) (*LevelUpOutput, error) {
	if input == nil {
		return nil, apierr.InvalidArgument("input is required")
	}
	if input.CharacterID == "" {
		return nil, apierr.InvalidArgument("character ID is required")
	}
	if input.HitPointMethod == "" {
		return nil, apierr.InvalidArgument("hit point method is required")
	}

	current, err := o.characterRepo.Get(ctx, characterrepo.GetInput{ID: input.CharacterID})
	if err != nil {
		return nil, fmt.Errorf("failed to get character: %w", err)
	}
	if current == nil || current.Character == nil || current.Character.Data == nil {
		return nil, characterDataUnavailable(errors.New("character repository returned no character data"))
	}

	loaded, loadErr := loadAttachedCharacter(ctx, &loadAttachedCharacterInput{Data: current.Character.Data})
	if loadErr != nil {
		return nil, characterDataUnavailable(fmt.Errorf("failed to load character: %w", loadErr))
	}
	char := loaded.Character

	advanced, advanceErr := char.Advance(ctx, &tkcharacter.AdvanceInput{
		ClassID:        current.Character.Data.ClassID,
		HitPointMethod: input.HitPointMethod,
		Choices:        input.Choices,
		// Supplied, never left nil. Advance would default a nil roller to its
		// own, and a defaulted source of randomness is one no caller chose --
		// which is the whole reason this orchestrator holds one at all.
		Roller: o.roller,
	})
	if advanceErr != nil {
		return nil, mapAdvanceError(advanceErr)
	}
	if advanced == nil {
		return nil, characterDataUnavailable(errors.New("advance returned no output"))
	}

	// Advance is atomic and has already succeeded, so the sheet in hand is the
	// one to store. loadAttachedCharacter worked from a struct copy, so the
	// repository's own entity was never mutated by a level that then failed.
	updated, updateErr := o.characterRepo.Update(ctx, characterrepo.UpdateInput{
		Character: &entities.Character{Data: char.ToData()},
	})
	if updateErr != nil {
		return nil, fmt.Errorf("failed to save leveled character: %w", updateErr)
	}
	if updated == nil || updated.Character == nil || updated.Character.Data == nil {
		return nil, characterDataUnavailable(errors.New("character repository returned no character after update"))
	}

	return &LevelUpOutput{
		Character: updated.Character,
		Entry:     advanced.Entry,
		Gained:    advanced.Gained,
	}, nil
}

// mapAdvanceError carries the toolkit's refusal out with its own message.
//
// THE MESSAGE IS THE POINT. Advance names the experience total and the
// threshold it fell short of, or the class it is not, or which choice was
// wrong; the client shows that sentence. Replacing it with wording of our own
// would leave the player told only that something was refused.
//
// The split is between a refusal about the REQUEST and a refusal about the
// STATE. A malformed or unanswerable submission is INVALID_ARGUMENT. A
// well-formed request the character's own situation forbids -- not enough
// experience, in combat, no level record, a class it cannot take -- is
// FAILED_PRECONDITION: nothing about the request would change the answer, and
// the same request becomes legal once the situation does.
func mapAdvanceError(err error) error {
	var rpgErr *rpgerr.Error
	if errors.As(err, &rpgErr) {
		switch rpgErr.Code {
		case rpgerr.CodeInvalidArgument:
			return apierr.InvalidArgument(rpgErr.Message)
		case rpgerr.CodePrerequisiteNotMet, rpgerr.CodeTimingRestriction,
			rpgerr.CodeInvalidState, rpgerr.CodeNotAllowed:
			return apierr.FailedPrecondition(rpgErr.Message)
		case rpgerr.CodeNotFound:
			return apierr.NotFound(rpgErr.Message)
		}
	}
	return fmt.Errorf("failed to take a level: %w", err)
}
