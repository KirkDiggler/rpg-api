// Copyright (C) 2026 Kirk Diggler
// SPDX-License-Identifier: GPL-3.0-or-later

package character

import (
	"context"
	"errors"

	"github.com/KirkDiggler/rpg-toolkit/core"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character/choices"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/proficiencies"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/resources"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
)

// GetNextLevel describes the level this character would take next.
//
// Design R4.13: the screen "MUST render whatever requirements the toolkit
// returns for that level, and MUST contain no class-specific branch." There is
// no class in this method either -- it hands back the level's own requirement
// row in the same Choice message creation already renders, so a class nobody
// has written yet levels the day someone fills its table in.
func (h *Handler) GetNextLevel(
	ctx context.Context,
	req *dnd5ev1alpha1.GetNextLevelRequest,
) (*dnd5ev1alpha1.GetNextLevelResponse, error) {
	if req.GetCharacterId() == "" {
		return nil, apierr.ToGRPCError(apierr.InvalidArgument("character_id is required"))
	}
	if err := h.verifyCallerOwnsCharacter(ctx, req.GetCharacterId()); err != nil {
		return nil, err
	}

	out, err := h.characterService.GetNextLevel(ctx, &character.GetNextLevelInput{
		CharacterID: req.GetCharacterId(),
	})
	if err != nil {
		return nil, levelUpRPCError(err)
	}
	if out == nil {
		return nil, levelUpRPCError(errors.New("get next level returned no output"))
	}

	return &dnd5ev1alpha1.GetNextLevelResponse{
		Level:   int32(out.CharacterLevel),
		Class:   convertClassToProtoEnum(out.ClassID),
		Choices: classRequirementsToProto(out.Requirements),
		Features: featureInfosFromRefs(
			out.FeatureRefs, out.CharacterLevel, classes.Name(out.ClassID),
		),
		HitDie: int32(out.HitDice),
	}, nil
}

// LevelUp takes the level. One call, atomic, no draft (design R4.16).
func (h *Handler) LevelUp(
	ctx context.Context,
	req *dnd5ev1alpha1.LevelUpRequest,
) (*dnd5ev1alpha1.LevelUpResponse, error) {
	if req.GetCharacterId() == "" {
		return nil, apierr.ToGRPCError(apierr.InvalidArgument("character_id is required"))
	}

	// BEFORE the toolkit is touched. UNSPECIFIED is not a method the engine
	// could pick between -- rolled and averaged are different numbers written
	// into a record that is never corrected -- so the refusal belongs at the
	// wire, where the field that was left unset is.
	method, err := hitPointMethodFromProto(req.GetHitPointMethod())
	if err != nil {
		return nil, err
	}

	if ownErr := h.verifyCallerOwnsCharacter(ctx, req.GetCharacterId()); ownErr != nil {
		return nil, ownErr
	}

	selections, convertErr := protoChoicesToToolkit(req.GetChoices())
	if convertErr != nil {
		return nil, convertErr
	}

	out, err := h.characterService.LevelUp(ctx, &character.LevelUpInput{
		CharacterID:    req.GetCharacterId(),
		HitPointMethod: method,
		Choices:        selections,
	})
	if err != nil {
		return nil, levelUpRPCError(err)
	}
	if out == nil || out.Character == nil || out.Character.Data == nil {
		return nil, levelUpRPCError(errors.New("level up returned no persisted character"))
	}

	return &dnd5ev1alpha1.LevelUpResponse{
		Character: ConvertCharacterDataToProto(out.Character.Data),
		Gained: &dnd5ev1alpha1.LevelGained{
			Level:           int32(out.Gained.CharacterLevel),
			HitPointsGained: int32(out.Gained.HitPointGain),
			Features: featureInfosFromRefs(
				refStrings(out.Gained.Features),
				out.Gained.CharacterLevel,
				classes.Name(out.Character.Data.ClassID),
			),
			ResourceChanges: resourceChangesToProto(out.Gained.Resources),
		},
	}, nil
}

// verifyCallerOwnsCharacter binds the caller to the character before either
// advancement RPC runs.
//
// NOT_FOUND, NEVER PERMISSION_DENIED, for the reason the v2 handler's own
// gate states: PERMISSION_DENIED would itself confirm that SOME character
// exists at that id, which is exactly what "found but not yours" must not
// leak. A caller naming a character it does not control gets the same
// NOT_FOUND a caller naming one that was never created would.
//
// This is a second copy of that gate rather than a shared one because the two
// handlers speak different proto packages and hold different service
// references; the behavior is what has to match, and the test asserts that.
//
// It returns only an error. The v2 gate hands its caller the loaded sheet
// because that handler projects it; both RPCs here go on to call the
// orchestrator, which reads the character itself, so returning the data would
// invite a second, older copy of it into the request.
func (h *Handler) verifyCallerOwnsCharacter(ctx context.Context, characterID string) error {
	playerID := auth.GetPlayerID(ctx)
	if playerID == "" {
		return apierr.ToGRPCError(apierr.Unauthenticated("player not authenticated"))
	}

	out, err := h.characterService.GetCharacter(ctx, &character.GetCharacterInput{
		CharacterID: characterID,
	})
	if err != nil {
		if apierr.IsNotFound(err) {
			return apierr.ToGRPCError(notFoundCharacter(characterID))
		}
		return apierr.ToGRPCError(err)
	}
	if out == nil || out.Character == nil || out.Character.Data == nil ||
		out.Character.Data.PlayerID != playerID {
		return apierr.ToGRPCError(notFoundCharacter(characterID))
	}

	return nil
}

// notFoundCharacter is the ONE NOT_FOUND the gate above ever returns, for
// both refusal reasons. One canonical message, never the repository's: a
// caller comparing the repository's own wording against this one could tell a
// missing character from a foreign one, which is the distinction NOT_FOUND
// exists here to erase.
func notFoundCharacter(characterID string) *apierr.Error {
	return apierr.NotFoundf("character %q not found", characterID)
}

func levelUpRPCError(err error) error {
	var coded *apierr.Error
	if errors.As(err, &coded) && coded.Code != apierr.CodeInternal {
		return apierr.ToGRPCError(err)
	}
	return apierr.ToGRPCError(apierr.WrapWithCode(err, apierr.CodeInternal, character.CharacterDataUnavailableMessage))
}

// hitPointMethodFromProto maps the wire's method onto the toolkit's.
//
// MAX IS NOT ON THE WIRE (design §10: "a hit-point method (ROLLED or AVERAGE;
// the level-1-only MAX is not on the wire)"), so there is no arm for it here
// and the enum has no value to write. UNSPECIFIED is a refusal rather than a
// default: a level's hit points are appended to a record that is never
// corrected, and picking for a client that did not choose would write a
// number nobody asked for.
func hitPointMethodFromProto(method dnd5ev1alpha1.HitPointMethod) (toolkitchar.HitPointMethod, error) {
	switch method {
	case dnd5ev1alpha1.HitPointMethod_HIT_POINT_METHOD_ROLLED:
		return toolkitchar.HitPointMethodRolled, nil
	case dnd5ev1alpha1.HitPointMethod_HIT_POINT_METHOD_AVERAGE:
		return toolkitchar.HitPointMethodAverage, nil
	case dnd5ev1alpha1.HitPointMethod_HIT_POINT_METHOD_UNSPECIFIED:
		return "", apierr.ToGRPCError(apierr.InvalidArgument(
			"hit_point_method is required: rolled or average"))
	default:
		return "", apierr.ToGRPCError(apierr.InvalidArgumentf(
			"unknown hit_point_method %q", method))
	}
}

// featureInfosFromRefs turns canonical feature refs into the wire's
// FeatureInfo.
//
// The DESCRIPTION IS LEFT EMPTY, deliberately. The toolkit's features carry a
// Ref, a Name and an action type and no prose; a description written here
// would be content authored in the API, which is the one thing rpg-api must
// not hold (repo CLAUDE.md: "if it's a game mechanic or calculation ->
// rpg-toolkit"). The level-up screen already renders the description only when
// it is present, so an empty one costs a line of flavor and buys no lie.
func featureInfosFromRefs(refStrings []string, level int, className string) []*dnd5ev1alpha1.FeatureInfo {
	if len(refStrings) == 0 {
		return nil
	}

	infos := make([]*dnd5ev1alpha1.FeatureInfo, 0, len(refStrings))
	for _, ref := range refStrings {
		id := extractIDFromRef(ref)
		if id == "" {
			continue
		}
		infos = append(infos, &dnd5ev1alpha1.FeatureInfo{
			// The canonical ref, not the bare id: it is the vocabulary
			// known_spells and ResourceMaximumChange.key already speak, and
			// the one identity the toolkit owns.
			Id:        ref,
			Name:      featureIDToDisplayName(id),
			Level:     int32(level),
			ClassName: className,
		})
	}
	return infos
}

// refStrings renders toolkit refs as the strings the wire carries.
func refStrings(refs []core.Ref) []string {
	if len(refs) == 0 {
		return nil
	}
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		out = append(out, ref.String())
	}
	return out
}

// resourceChangesToProto reports every pool whose maximum the level moved.
//
// Design R4.7: "a slot increase is not a question and is applied without
// asking, which means nothing on the level-up screen would mention it unless
// the engine says so. This is what lets a response read '1st-level spell slots
// 2 to 3' instead of leaving the player to notice."
//
// A key the display-name table does not know still ships, named by its key.
// The alternative -- dropping it -- would hide a pool that actually changed.
func resourceChangesToProto(changes []toolkitchar.ResourceChange) []*dnd5ev1alpha1.ResourceMaximumChange {
	if len(changes) == 0 {
		return nil
	}

	out := make([]*dnd5ev1alpha1.ResourceMaximumChange, 0, len(changes))
	for _, change := range changes {
		name, known := resources.DisplayName(change.Key)
		if !known {
			name = string(change.Key)
		}
		out = append(out, &dnd5ev1alpha1.ResourceMaximumChange{
			Key:             string(change.Key),
			Name:            name,
			PreviousMaximum: int32(change.From),
			NewMaximum:      int32(change.To),
		})
	}
	return out
}

// protoChoicesToToolkit reads the level-up submission back into the choice
// vocabulary the engine validates.
//
// This is the REVERSE of convertChoiceToProto, category for category, and it
// reuses that file's enum converters so the two directions cannot drift into
// two different opinions about what a skill is. It could not reuse
// UpdateClass's reader: that one folds a submission into
// character.ClassChoices, a creation-shaped aggregate, while Advance takes the
// choices themselves.
//
// AN UNREADABLE CHOICE IS AN ERROR, never a silent drop. A dropped answer
// reaches the engine as a missing one, and the player is told they failed to
// choose something they did choose.
func protoChoicesToToolkit(submitted []*dnd5ev1alpha1.ChoiceData) ([]choices.ChoiceData, error) {
	if len(submitted) == 0 {
		return nil, nil
	}

	out := make([]choices.ChoiceData, 0, len(submitted))
	for _, choice := range submitted {
		converted, err := protoChoiceToToolkit(choice)
		if err != nil {
			return nil, err
		}
		out = append(out, converted)
	}
	return out, nil
}

//nolint:gocyclo // One arm per wire category; splitting it would hide the mapping it exists to state.
func protoChoiceToToolkit(choice *dnd5ev1alpha1.ChoiceData) (choices.ChoiceData, error) {
	converted := choices.ChoiceData{
		Category: choiceCategoryFromProto(choice.GetCategory()),
		Source:   choiceSourceFromProto(choice.GetSource()),
		ChoiceID: choices.ChoiceID(choice.GetChoiceId()),
		OptionID: choice.GetOptionId(),
	}
	if converted.Category == "" {
		return converted, apierr.ToGRPCError(apierr.InvalidArgumentf(
			"choice %q has an unreadable category", choice.GetChoiceId()))
	}

	switch converted.Category {
	case shared.ChoiceSkills:
		for _, skill := range choice.GetSkills().GetSkills() {
			converted.SkillSelection = append(converted.SkillSelection, convertProtoSkillToToolkit(skill))
		}
	case shared.ChoiceLanguages:
		for _, language := range choice.GetLanguages().GetLanguages() {
			converted.LanguageSelection = append(
				converted.LanguageSelection, convertProtoLanguageToToolkit(language))
		}
	case shared.ChoiceSpells, shared.ChoiceCantrips:
		converted.SpellSelection = selectedSpells(choice.GetSpells())
	case shared.ChoiceToolProficiency:
		// The bare id and the proficiency name are the same string by
		// construction -- refs.Tools.Lute().ID is "lute" and
		// proficiencies.ToolLute is "lute" -- so this reuses UpdateClass's
		// enum reader rather than opening a second, divergeable table.
		for _, tool := range choice.GetTools().GetTools() {
			if id := convertProtoToolToToolkit(tool); id != "" {
				converted.ToolSelection = append(converted.ToolSelection, proficiencies.Tool(id))
			}
		}
	case shared.ChoiceExpertise:
		for _, skill := range choice.GetExpertise().GetSkills() {
			if skill == dnd5ev1alpha1.Skill_SKILL_UNSPECIFIED {
				continue
			}
			converted.ExpertiseSelection = append(
				converted.ExpertiseSelection, convertProtoSkillToToolkit(skill))
		}
	case shared.ChoiceFightingStyle:
		style := convertProtoFightingStyleToToolkit(choice.GetFightingStyle().GetStyle())
		converted.FightingStyleSelection = &style
	case shared.ChoiceEquipment:
		converted.EquipmentSelection = equipmentSelectionFromProto(choice.GetEquipment())
	default:
		return converted, apierr.ToGRPCError(apierr.InvalidArgumentf(
			"choice %q carries category %q, which this level-up path cannot read",
			choice.GetChoiceId(), converted.Category))
	}

	return converted, nil
}

// equipmentSelectionFromProto reads the item ids out of an equipment
// submission, the same four-arm extraction UpdateClass performs on the same
// message.
func equipmentSelectionFromProto(selection *dnd5ev1alpha1.EquipmentSelection) []shared.SelectionID {
	items := selection.GetItems()
	if len(items) == 0 {
		return nil
	}

	out := make([]shared.SelectionID, 0, len(items))
	for _, item := range items {
		var id string
		switch equipment := item.GetEquipment().(type) {
		case *dnd5ev1alpha1.EquipmentSelectionItem_Weapon:
			id = convertProtoWeaponToToolkit(equipment.Weapon)
		case *dnd5ev1alpha1.EquipmentSelectionItem_Armor:
			id = convertProtoArmorToToolkit(equipment.Armor)
		case *dnd5ev1alpha1.EquipmentSelectionItem_Tool:
			id = convertProtoToolToToolkit(equipment.Tool)
		case *dnd5ev1alpha1.EquipmentSelectionItem_OtherEquipmentId:
			id = equipment.OtherEquipmentId
		}
		if id != "" {
			out = append(out, id)
		}
	}
	return out
}

// choiceCategoryFromProto is convertChoiceCategoryToProto read backwards. An
// unmapped value returns the empty category, which the caller refuses.
func choiceCategoryFromProto(category dnd5ev1alpha1.ChoiceCategory) shared.ChoiceCategory {
	switch category {
	case dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SPELLS:
		return shared.ChoiceSpells
	case dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_CANTRIPS:
		return shared.ChoiceCantrips
	case dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SKILLS:
		return shared.ChoiceSkills
	case dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_LANGUAGES:
		return shared.ChoiceLanguages
	case dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT:
		return shared.ChoiceEquipment
	case dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_FIGHTING_STYLE:
		return shared.ChoiceFightingStyle
	case dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_TOOLS:
		return shared.ChoiceToolProficiency
	case dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EXPERTISE:
		return shared.ChoiceExpertise
	case dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_RACE:
		return shared.ChoiceRace
	case dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_CLASS:
		return shared.ChoiceClass
	case dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_BACKGROUND:
		return shared.ChoiceBackground
	default:
		return ""
	}
}

// choiceSourceFromProto is convertChoiceSourceToProto read backwards.
//
// A level-up choice carries CHOICE_SOURCE_CLASS (design §10): "the record
// entry already says which level they belong to", which is why
// CHOICE_SOURCE_LEVEL_UP is deprecated rather than used. An unspecified source
// maps to the player, which is what a submission with nothing said about its
// origin is.
func choiceSourceFromProto(source dnd5ev1alpha1.ChoiceSource) shared.ChoiceSource {
	switch source {
	case dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_RACE:
		return shared.SourceRace
	case dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_SUBRACE:
		return shared.SourceSubrace
	case dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_CLASS:
		return shared.SourceClass
	case dnd5ev1alpha1.ChoiceSource_CHOICE_SOURCE_BACKGROUND:
		return shared.SourceBackground
	default:
		return shared.SourcePlayer
	}
}
