// Copyright (C) 2026 Kirk Diggler
// SPDX-License-Identifier: GPL-3.0-or-later

package character

import (
	"context"
	"errors"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/sdkerr"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
)

// Sessions is the subset of the toolkit session Manager this service calls.
//
// Declared HERE, at the point of use, rather than depended on as the SDK's own
// concrete *session.Manager — the same shape the session handler's own Manager
// interface takes, and for the same reason: it is what lets these handler
// tests fake a verb's outcome without a real Manager, Redis and dice roller
// behind them. *session.Manager satisfies this structurally; no adapter is
// built at construction.
//
// TWO METHODS, because two is what this service calls. A wider interface would
// be a list of capabilities nobody here uses, and every one of them a thing a
// test has to fake.
type Sessions interface {
	NextLevel(ctx context.Context, in *sdk.NextLevelInput) (*sdk.NextLevelOutput, error)
	LevelUp(ctx context.Context, in *sdk.LevelUpInput) (*sdk.LevelUpOutput, error)
}

// GetNextLevel describes the level this character would take next.
//
// PURE TRANSLATION. Design R6.1, from Kirk's ruling after the walk: *"the API
// is dumb … we added the session package to act as the SDK to the API. So we
// should not need an orchestrator in API anymore and our level up should be
// contained in our session package."* This method binds the caller, calls one
// SDK verb, and projects the answer. It loads nothing, decides nothing, and
// orders no toolkit steps.
//
// The first build of this put the verb in a character orchestrator — load the
// sheet, call Advance, save — which is precisely the tendency the ruling
// names. The rules were the toolkit's even then; the ORCHESTRATION of them was
// not, and that is the part that moved.
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

	out, err := h.sessions.NextLevel(ctx, &sdk.NextLevelInput{Character: req.GetCharacterId()})
	if err != nil {
		return nil, levelUpStatusError(err)
	}
	if out == nil {
		return nil, apierr.ToGRPCError(apierr.Internal("next level returned no output"))
	}

	return &dnd5ev1alpha1.GetNextLevelResponse{
		// CharacterLevel, NOT Level. The SDK's Level is what the sheet holds
		// NOW; CharacterLevel is the one this level would take, which is what
		// the wire field means ("the level the character would take, one above
		// its current level"). Reading the wrong one shipped a response saying
		// "Level 1" for a character about to become 2, and the twelve-class
		// integration test is what caught it.
		Level:    int32(out.CharacterLevel),
		Class:    classFromRef(out.Class),
		Choices:  levelChoicesToProto(out.Choices),
		Features: featureInfosFromRefs(out.Features, out.CharacterLevel, out.ClassName),
		HitDie:   int32(out.HitDie),
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

	// BEFORE the SDK is touched. UNSPECIFIED is not a method anything could
	// pick between -- rolled and averaged are different numbers written into a
	// record that is never corrected -- so the refusal belongs at the wire,
	// where the field that was left unset is.
	method, err := hitPointMethodFromProto(req.GetHitPointMethod())
	if err != nil {
		return nil, err
	}

	if ownErr := h.verifyCallerOwnsCharacter(ctx, req.GetCharacterId()); ownErr != nil {
		return nil, ownErr
	}

	submissions, convertErr := levelChoiceSubmissions(req.GetChoices())
	if convertErr != nil {
		return nil, convertErr
	}

	out, err := h.sessions.LevelUp(ctx, &sdk.LevelUpInput{
		Character:      req.GetCharacterId(),
		HitPointMethod: method,
		Choices:        submissions,
	})
	if err != nil {
		return nil, levelUpStatusError(err)
	}
	if out == nil {
		return nil, apierr.ToGRPCError(apierr.Internal("level up returned no output"))
	}

	// THE SDK RETURNS NO SHEET, by its own boundary law: it reports that it
	// saved and what the level brought, and the host re-reads through its own
	// repository. So the projected Character comes from a read, not from a
	// value handed back -- which also means the client is shown the sheet that
	// is actually stored.
	current, err := h.characterService.GetCharacter(ctx, &character.GetCharacterInput{
		CharacterID: req.GetCharacterId(),
	})
	if err != nil {
		return nil, levelUpStatusError(err)
	}
	if current == nil || current.Character == nil || current.Character.Data == nil {
		return nil, apierr.ToGRPCError(apierr.Internal("level up saved but the character could not be re-read"))
	}

	return &dnd5ev1alpha1.LevelUpResponse{
		Character: ConvertCharacterDataToProto(current.Character.Data),
		// The class name comes from the sheet, because LevelGained does not
		// carry one -- and the sheet is the better source anyway: it is the
		// same stored character being projected one field above, so the two
		// halves of this response cannot disagree about who leveled.
		Gained: levelGainedToProto(out.Gained, classes.Name(current.Character.Data.ClassID)),
	}, nil
}

// levelGainedToProto projects what the level brought.
//
// Design R4.7 is why resource changes are on the wire at all: "a slot increase
// is not a question and is applied without asking", so nothing on the screen
// would mention it unless the engine said so. This is what lets a response
// read "1st-level spell slots 2 to 3" instead of leaving the player to notice.
func levelGainedToProto(gained sdk.LevelGained, className string) *dnd5ev1alpha1.LevelGained {
	return &dnd5ev1alpha1.LevelGained{
		Level:           int32(gained.CharacterLevel),
		HitPointsGained: int32(gained.HitPointGain),
		Features:        featureInfosFromRefs(gained.Features, gained.CharacterLevel, className),
		ResourceChanges: resourceChangesToProto(gained.Resources),
	}
}

// classFromRef maps the SDK's canonical class ref onto the wire's enum.
//
// The SDK says "dnd5e:classes:fighter" rather than a classes.Class, because
// its boundary law keeps toolkit types off its signatures. Taking the id after
// the second colon and handing it to the existing converter is VOCABULARY
// TRANSLATION, not a rule: the id IS everything after the second colon, and
// the table that turns an id into an enum already exists and is shared with
// every other class projection on this service.
func classFromRef(ref string) dnd5ev1alpha1.Class {
	id := extractIDFromRef(ref)
	if id == "" {
		return dnd5ev1alpha1.Class_CLASS_UNSPECIFIED
	}
	return convertClassToProtoEnum(id)
}

// levelChoicesToProto renders the SDK's level choices as the same Choice
// message character creation already renders.
//
// R4.13: the screen "MUST render whatever requirements the toolkit returns for
// that level, and MUST contain no class-specific branch." There is no class in
// this function either -- it walks what it was handed.
func levelChoicesToProto(choices []sdk.LevelChoice) []*dnd5ev1alpha1.Choice {
	if len(choices) == 0 {
		return nil
	}

	out := make([]*dnd5ev1alpha1.Choice, 0, len(choices))
	for _, choice := range choices {
		out = append(out, &dnd5ev1alpha1.Choice{
			Id:          choice.ID,
			Description: choice.Label,
			ChooseCount: int32(choice.Count),
			ChoiceType:  levelChoiceCategory(choice.Kind),
			Options: &dnd5ev1alpha1.Choice_SpellOptions{
				SpellOptions: &dnd5ev1alpha1.SpellOptions{
					// Canonical refs, never the deprecated closed enum: "the
					// closed enum cannot name a spell the catalog learned
					// after it was generated" (SpellOptions' own doc).
					AvailableRefs: choice.Options,
					SpellLevel:    int32(choice.SpellLevel),
				},
			},
		})
	}
	return out
}

// levelChoiceCategory maps the SDK's kind onto the wire's category.
//
// The SDK names the KIND of question it asked; the category is how creation's
// renderer already labels the same question, so a level-up choice arrives in
// the vocabulary the screen speaks. An unknown kind is reported as unspecified
// rather than guessed into one of the two: a mislabelled choice would be
// rendered by the wrong control.
func levelChoiceCategory(kind sdk.LevelChoiceKind) dnd5ev1alpha1.ChoiceCategory {
	switch kind {
	case sdk.LevelChoiceSpell:
		return dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SPELLS
	case sdk.LevelChoiceCantrip:
		return dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_CANTRIPS
	default:
		return dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_UNSPECIFIED
	}
}

// levelChoiceSubmissions reads the client's answers into the SDK's submission
// shape.
//
// THE CATEGORY IS NOT SENT. The SDK derives the kind of each answer from the
// question it asked under that id, so a category on the way back would be a
// second name for one fact and free to disagree with the first. What crosses
// is the choice id and the refs chosen under it.
//
// A choice this path cannot read is an ERROR naming it, never a silent drop: a
// dropped answer reaches the engine as a missing one, and the player is told
// they failed to choose something they did choose.
func levelChoiceSubmissions(
	submitted []*dnd5ev1alpha1.ChoiceData,
) ([]sdk.LevelChoiceSubmission, error) {
	if len(submitted) == 0 {
		return nil, nil
	}

	out := make([]sdk.LevelChoiceSubmission, 0, len(submitted))
	for _, choice := range submitted {
		switch choice.GetCategory() {
		case dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SPELLS,
			dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_CANTRIPS:
		default:
			return nil, apierr.ToGRPCError(apierr.InvalidArgumentf(
				"choice %q carries category %s, and level-up answers only spells and cantrips",
				choice.GetChoiceId(), choice.GetCategory()))
		}

		out = append(out, sdk.LevelChoiceSubmission{
			ChoiceID:   choice.GetChoiceId(),
			Selections: choice.GetSpells().GetSpellRefs(),
		})
	}
	return out, nil
}

// verifyCallerOwnsCharacter binds the caller to the character before either
// advancement RPC runs.
//
// NOT_FOUND, NEVER PERMISSION_DENIED, for the reason the v2 handler's own gate
// states: PERMISSION_DENIED would itself confirm that SOME character exists at
// that id, which is exactly what "found but not yours" must not leak. A caller
// naming a character it does not control gets the same NOT_FOUND a caller
// naming one that was never created would.
//
// This is the one thing the SDK cannot do for us. Ownership of the CALLING
// PLAYER is transport's, not the rules engine's: the SDK is handed a character
// id and has no notion of who is holding the connection.
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

// notFoundCharacter is the ONE NOT_FOUND the gate above ever returns, for both
// refusal reasons. One canonical message, never the repository's: a caller
// comparing the repository's own wording against this one could tell a missing
// character from a foreign one, which is the distinction NOT_FOUND exists here
// to erase.
func notFoundCharacter(characterID string) *apierr.Error {
	return apierr.NotFoundf("character %q not found", characterID)
}

// hitPointMethodFromProto maps the wire's method onto the SDK's.
//
// MAX IS NOT ON THE WIRE (design §10: "a hit-point method (ROLLED or AVERAGE;
// the level-1-only MAX is not on the wire)"), so there is no arm for it here
// and the enum has no value to write. UNSPECIFIED is a refusal rather than a
// default: a level's hit points are appended to a record that is never
// corrected, and picking for a client that did not choose would write a number
// nobody asked for.
func hitPointMethodFromProto(method dnd5ev1alpha1.HitPointMethod) (sdk.LevelUpHitPointMethod, error) {
	switch method {
	case dnd5ev1alpha1.HitPointMethod_HIT_POINT_METHOD_ROLLED:
		return sdk.HitPointsRolled, nil
	case dnd5ev1alpha1.HitPointMethod_HIT_POINT_METHOD_AVERAGE:
		return sdk.HitPointsAverage, nil
	case dnd5ev1alpha1.HitPointMethod_HIT_POINT_METHOD_UNSPECIFIED:
		return "", apierr.ToGRPCError(apierr.InvalidArgument(
			"hit_point_method is required: rolled or average"))
	default:
		return "", apierr.ToGRPCError(apierr.InvalidArgumentf(
			"unknown hit_point_method %q", method))
	}
}

// levelUpStatusError carries a refusal out with its own code and message.
//
// The SDK's sentinels are translated by the session handler's statusError,
// which is THE tested error-translation table for this SDK (design rule 7).
// This service is a second caller of the same SDK, so it routes through the
// same function rather than opening a second table free to disagree about what
// ErrNotEntitled means.
//
// THE MESSAGE IS THE POINT. The engine names the experience total and the
// threshold it fell short of, or the spell already known; the client shows
// that sentence. Replacing it with wording of our own would leave the player
// told only that something was refused.
func levelUpStatusError(err error) error {
	if err == nil {
		return nil
	}

	// An apierr from this service's own repository read keeps its own code;
	// only SDK errors go to the SDK's table.
	var coded *apierr.Error
	if errors.As(err, &coded) {
		return apierr.ToGRPCError(err)
	}
	return sdkerr.StatusError(err)
}

// featureInfosFromRefs turns canonical feature refs into the wire's
// FeatureInfo.
//
// The DESCRIPTION IS LEFT EMPTY, deliberately. The toolkit's features carry a
// Ref, a Name and an action type and no prose; a description written here
// would be content authored in the API, which is the one thing rpg-api must
// not hold. The level-up screen renders the description only when it is
// present, so an empty one costs a line of flavor and buys no lie.
func featureInfosFromRefs(refStrs []string, level int, className string) []*dnd5ev1alpha1.FeatureInfo {
	if len(refStrs) == 0 {
		return nil
	}

	infos := make([]*dnd5ev1alpha1.FeatureInfo, 0, len(refStrs))
	for _, ref := range refStrs {
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

// resourceChangesToProto reports every pool whose maximum the level moved.
//
// The SDK names the pool as well as keying it, so there is no lookup here and
// no table in this package that could disagree with the engine about what
// "spell_slot_level_1" is called. Key and name both cross verbatim.
func resourceChangesToProto(changes []sdk.ResourceMaximumChange) []*dnd5ev1alpha1.ResourceMaximumChange {
	if len(changes) == 0 {
		return nil
	}

	out := make([]*dnd5ev1alpha1.ResourceMaximumChange, 0, len(changes))
	for _, change := range changes {
		out = append(out, &dnd5ev1alpha1.ResourceMaximumChange{
			Key:             change.Key,
			Name:            change.Name,
			PreviousMaximum: int32(change.From),
			NewMaximum:      int32(change.To),
		})
	}
	return out
}
