package session_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/conditions"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/spells"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

// The cast door, end to end (rpg-project#405). A level-1 bard who chose two
// cantrips is offered a Cast row for each, and casting one produces the beats
// a client draws the fight from.
//
// TWO CANTRIPS, TWO HALVES OF EVERY CANTRIP. True Strike delivers a condition
// with no roll at all; Vicious Mockery rolls a save to decide whether it
// delivers anything. Between them there is no third shape to miss.
//
// EVERY ASSERTION IS THROUGH THE REAL HANDLER, over miniredis, reading the
// event stream a client subscribes to. Nothing here calls the SDK's Manager
// for a verb, and nothing mints a selector: the rows come from Afford, which
// is the only author of them.
const castSessionID = "cast-run"

// castingBard is a level-1 bard who knows exactly the cantrips named.
//
// THE REFS ARE CANONICAL, which is the sheet's own vocabulary
// ("dnd5e:spells:true-strike") and the same strings the creation flow stores.
// Written through the refs package rather than as literals, because here the
// question is whether the CAST DOOR reads what the sheet holds; the creation
// test is where the literal spelling is pinned against drift.
//
// EACH SCENE NAMES ITS OWN CANTRIPS, because what a bard knows is what the
// door is compiled from and the scenes below vary it deliberately: two
// castable, one, or two with no cast content at all.
func castingBard(id, playerID string, known ...spells.Spell) *tkcharacter.Data {
	sheet := levelOneBard(id, playerID, 2)
	sheet.KnownCantrips = make([]string, 0, len(known))
	for _, cantrip := range known {
		ref := refs.Spells.ByID(cantrip)
		if ref == nil {
			panic("the catalog carries no cantrip " + cantrip)
		}
		sheet.KnownCantrips = append(sheet.KnownCantrips, ref.String())
	}

	return sheet
}

// failedSaveDice makes every d20 a 3 and every other die its maximum face.
//
// THE SAVE MUST FAIL, and it must fail for a reason the scene states rather
// than by luck. This bard's spell save DC is 13 -- 8 + proficiency 2 +
// Charisma 16's +3 -- and a skeleton rolling 3 on a negative Wisdom modifier
// cannot reach it from any direction. hittingDice beside this one is the
// opposite choice for the opposite scene: its 15 would SUCCEED here, and a
// successful save delivers nothing at all, so the damage half of the design
// would go unwatched.
type failedSaveDice struct{}

func (failedSaveDice) Roll(_ context.Context, size int) (int, error) {
	if size == 20 {
		return 3, nil
	}

	return size, nil
}

// castScene stands a bard and a fighter next to one skeleton, with the fighter
// having already passed so it is the bard's turn to act.
//
// THE SKELETON NEVER ACTS: pathWalker with no paths passes on every turn, so
// nothing a monster decides can reach the numbers below. Same fake, same
// reason, as the inspiration and interrupt scenes.
func castScene(
	t *testing.T, known ...spells.Spell,
) (*acceptanceHarness, context.Context, context.Context) {
	t.Helper()

	h, bardCtx, fighterCtx := castSceneAtFighterTurn(t, known...)

	// The fighter passes so the door under test is open: a Cast row is priced
	// in actions, and a member who is not the active one is refused before
	// anything else is read.
	_, err := h.handler.EndTurn(fighterCtx, &sessionpb.EndTurnRequest{
		Session: castSessionID, Member: "alice",
		DeclarationId: currentDeclarationID(
			fighterCtx, t, h.handler, castSessionID, "alice", sessionpb.Verb_VERB_END_TURN),
	})
	require.NoError(t, err)

	return h, bardCtx, fighterCtx
}

// castSceneAtFighterTurn is the same scene stopped one step earlier, with the
// FIGHTER still active.
//
// It exists because "a fighter is offered no Cast row" is only a question
// worth asking on her own turn. Off turn, Afford answers every verb with one
// blocked row carrying "not your turn" -- Attack, Move, Activate, Cast and
// End Turn alike -- without loading a sheet at all, so a Cast row there says
// nothing about what she knows. On her turn the sheet IS read, and its
// silence is the answer.
func castSceneAtFighterTurn(
	t *testing.T, known ...spells.Spell,
) (*acceptanceHarness, context.Context, context.Context) {
	t.Helper()

	h := newAcceptanceHarnessWith(t, failedSaveDice{}, &pathWalker{
		paths:  map[string][]spatial.Position{},
		walked: map[string]bool{},
	})
	bardCtx := auth.WithPlayerID(context.Background(), "player-bella")
	fighterCtx := auth.WithPlayerID(context.Background(), "player-alice")

	for _, sheet := range []*tkcharacter.Data{
		castingBard("bella", "player-bella", known...),
		armedFighter("alice", "player-alice"),
	} {
		_, err := h.charRepo.Create(context.Background(), characterrepo.CreateInput{
			Character: &entities.Character{Data: sheet},
		})
		require.NoError(t, err)
	}

	_, err := h.manager.Manager.StartSession(context.Background(), &sdk.StartSessionInput{
		Session: castSessionID, Encounter: "room-encounter", World: buildOpenRoom(t, 12, 6),
	})
	require.NoError(t, err)

	_, err = h.handler.Join(bardCtx, &sessionpb.JoinRequest{
		Session: castSessionID, Member: "bella", Position: pbAt(2, 0),
	})
	require.NoError(t, err)
	_, err = h.handler.Join(fighterCtx, &sessionpb.JoinRequest{
		Session: castSessionID, Member: "alice", Position: pbAt(3, 0),
	})
	require.NoError(t, err)
	inCombat(t, h.charRepo, "bella", 1)
	inCombat(t, h.charRepo, "alice", 1)

	_, err = h.manager.Manager.Spawn(context.Background(), &sdk.SpawnInput{
		Session: castSessionID, ID: "skel-1", Ref: refs.Monsters.Skeleton().String(),
		Position: at(4, 0),
	})
	require.NoError(t, err)

	turn, err := h.handler.Turn(fighterCtx, &sessionpb.TurnRequest{
		Session: castSessionID, Member: "alice",
	})
	require.NoError(t, err)
	require.Equal(t, []string{"alice", "bella", "skel-1"}, turn.GetOrder(),
		"geometry gate: the fighter acts first, then the bard, then the skeleton")

	return h, bardCtx, fighterCtx
}

// castRows returns every VERB_CAST declaration a member is offered, read
// through the same Afford surface a client uses.
func castRows(
	ctx context.Context, t *testing.T, h *acceptanceHarness, member string,
) []*sessionpb.Declaration {
	t.Helper()

	out, err := h.handler.Afford(ctx, &sessionpb.AffordRequest{
		Session: castSessionID, Member: member,
	})
	require.NoError(t, err)

	var rows []*sessionpb.Declaration
	for _, declaration := range out.GetDeclarations() {
		if declaration.GetVerb() == sessionpb.Verb_VERB_CAST {
			rows = append(rows, declaration)
		}
	}

	return rows
}

// castRowFor returns the Cast row for one named cantrip, found the way a dock
// finds it: by the spell the SERVER put on the row.
//
// This is the field that makes a Cast row self-describing. Before it existed
// a test could only take the row it was given and hope, because one verb
// compiles many rows and nothing on the wire told them apart.
func castRowFor(
	ctx context.Context, t *testing.T, h *acceptanceHarness, member string, cantrip spells.Spell,
) *sessionpb.Declaration {
	t.Helper()

	want := refs.Spells.ByID(cantrip)
	require.NotNil(t, want, "the catalog must carry %q", cantrip)

	for _, row := range castRows(ctx, t, h, member) {
		if row.GetSpell().GetRef() == want.String() {
			return row
		}
	}
	require.FailNow(t, "no Cast row for "+cantrip)

	return nil
}

// beatsOfKind returns every beat of one kind in a captured slice.
func beatsOfKind(events []*sessionpb.Event, kind sessionpb.EventKind) []*sessionpb.Event {
	var out []*sessionpb.Event
	for _, evt := range events {
		if evt.GetKind() == kind {
			out = append(out, evt)
		}
	}

	return out
}

// requireNoUnknownBeats is the design's own done-when, and it is the reason
// the kinds and the bodies had to land in one change: an unmapped kind does
// not fail, it DEMOTES -- EVENT_KIND_UNKNOWN with a nil body -- so a client
// would receive a beat that happened and could not be read, and nothing
// anywhere would say so.
func requireNoUnknownBeats(t *testing.T, events []*sessionpb.Event) {
	t.Helper()

	for _, evt := range events {
		require.NotEqual(t, sessionpb.EventKind_EVENT_KIND_UNKNOWN, evt.GetKind(),
			"a beat reached the client as UNKNOWN, which is a kind this build failed to map")
		require.NotEqual(t, sessionpb.EventKind_EVENT_KIND_UNSPECIFIED, evt.GetKind(),
			"a beat reached the client with no kind at all, a producer defect")
	}
}

// watchCast opens the bard's own stream and returns everything that arrives
// after the given call, quiesced.
func watchCast(
	ctx context.Context, t *testing.T, h *acceptanceHarness, member string, act func(),
) []*sessionpb.Event {
	t.Helper()

	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream := newRecordingStream(streamCtx)
	done := make(chan error, 1)
	go func() {
		done <- h.handler.StreamEvents(
			&sessionpb.StreamEventsRequest{Session: castSessionID, Member: member}, stream)
	}()
	waitForLive(t, h.manager.Broker, castSessionID, member, stream)
	baseline := len(stream.snapshot())

	act()

	return waitForQuiescence(t, stream, 2*time.Second)[baseline:]
}

// TestAcceptance_TheCastDoorOffersOneRowPerCastableCantrip is the panel this
// slice exists to fill, and the fighter is half the assertion: a Cast row is
// minted from a KNOWN CANTRIP WITH CAST CONTENT, so somebody who knows none
// is offered none rather than an empty menu.
func TestAcceptance_TheCastDoorOffersOneRowPerCastableCantrip(t *testing.T) {
	h, bardCtx, _ := castScene(t, spells.TrueStrike, spells.ViciousMockery)

	rows := castRows(bardCtx, t, h, "bella")
	require.Len(t, rows, 2, "one row per cantrip this bard knows and this build can cast")

	for _, row := range rows {
		require.True(t, row.GetAvailable(), "a bard on her own turn can afford one action")
		require.Equal(t, sessionpb.Slot_SLOT_ACTION, row.GetSlot(),
			"a cantrip costs one action, and Afford shows the price the door charges")
		require.NotEmpty(t, row.GetId(), "every row carries the opaque selector Cast echoes back")
		require.NotEmpty(t, row.GetSpell().GetName(),
			"every row says which cantrip it is, so a dock labels the button rather than saying Cast twice")
	}

	named := []string{rows[0].GetSpell().GetRef(), rows[1].GetSpell().GetRef()}
	require.ElementsMatch(t,
		[]string{refs.Spells.TrueStrike().String(), refs.Spells.ViciousMockery().String()},
		named, "the two rows are the two cantrips, and each names its own")
}

// TestAcceptance_AFighterOnHerOwnTurnIsOfferedNoCastRow is the other half of
// the panel, and the half a bard-only scene would never ask: the door is
// minted from the SHEET'S OWN KNOWN LIST, so somebody who knows nothing is
// offered nothing.
//
// ON HER OWN TURN, deliberately. Off turn every verb answers with one blocked
// "not your turn" row without reading a sheet, so a Cast row there would be
// about the clock and not about her.
func TestAcceptance_AFighterOnHerOwnTurnIsOfferedNoCastRow(t *testing.T) {
	h, _, fighterCtx := castSceneAtFighterTurn(t, spells.TrueStrike, spells.ViciousMockery)

	out, err := h.handler.Afford(fighterCtx, &sessionpb.AffordRequest{
		Session: castSessionID, Member: "alice",
	})
	require.NoError(t, err)

	verbs := make([]sessionpb.Verb, 0, len(out.GetDeclarations()))
	for _, declaration := range out.GetDeclarations() {
		verbs = append(verbs, declaration.GetVerb())
		require.NotEqual(t, sessionpb.Verb_VERB_CAST, declaration.GetVerb(),
			"a fighter knows no cantrips, so the door offers her nothing rather than an empty row")
	}
	require.Contains(t, verbs, sessionpb.Verb_VERB_ATTACK,
		"the panel is live -- she has rows, just not cast ones -- so the absence above is an answer")
}

// TestAcceptance_TrueStrikeDeliversAConditionWithNoRoll is the gateless half.
// A cantrip with no save has nothing to resolve, so the whole beat sequence is
// the cast and what it delivered -- and NO SAVED BEAT AT ALL, which is the
// assertion that would catch a save fabricated out of zero values.
func TestAcceptance_TrueStrikeDeliversAConditionWithNoRoll(t *testing.T) {
	h, bardCtx, _ := castScene(t, spells.TrueStrike, spells.ViciousMockery)

	row := castRowFor(bardCtx, t, h, "bella", spells.TrueStrike)

	beats := watchCast(bardCtx, t, h, "bella", func() {
		_, err := h.handler.Cast(bardCtx, &sessionpb.CastRequest{
			Session: castSessionID, Member: "bella",
			DeclarationId: row.GetId(), Target: "skel-1",
		})
		require.NoError(t, err)
	})
	requireNoUnknownBeats(t, beats)

	cast := beatsOfKind(beats, sessionpb.EventKind_EVENT_KIND_CAST)
	require.Len(t, cast, 1, "one cast, one cast beat")
	require.Equal(t, "bella", cast[0].GetCast().GetActor())
	require.Equal(t, refs.Spells.TrueStrike().String(), cast[0].GetCast().GetSpell().GetRef())
	require.Equal(t, "True Strike", cast[0].GetCast().GetSpell().GetName(),
		"the content authors the name, and the client never derives it from the ref")
	require.Equal(t, "skel-1", cast[0].GetCast().GetTarget())

	require.Empty(t, beatsOfKind(beats, sessionpb.EventKind_EVENT_KIND_SAVED),
		"True Strike rolls nothing, so nothing may report a roll")

	// The condition lands on the CASTER, not the target: True Strike gives
	// the bard advantage on her next attack against the skeleton, so the
	// skeleton holds nothing.
	require.Contains(t, conditionRefsOf(t, h.charRepo, "bella"),
		refs.Conditions.TrueStrike().String(),
		"the caster holds the condition her own cantrip granted")
}

// TestAcceptance_ViciousMockeryRollsASaveAndDeliversDamage is the gated half,
// and the whole reason the contest machine grew a damage effect kind rather
// than a second machine.
//
// THE SAVE FAILS BY CONSTRUCTION, not by luck: see failedSaveDice.
func TestAcceptance_ViciousMockeryRollsASaveAndDeliversDamage(t *testing.T) {
	h, bardCtx, _ := castScene(t, spells.TrueStrike, spells.ViciousMockery)

	row := castRowFor(bardCtx, t, h, "bella", spells.ViciousMockery)

	beats := watchCast(bardCtx, t, h, "bella", func() {
		_, err := h.handler.Cast(bardCtx, &sessionpb.CastRequest{
			Session: castSessionID, Member: "bella",
			DeclarationId: row.GetId(), Target: "skel-1",
		})
		require.NoError(t, err)
	})
	requireNoUnknownBeats(t, beats)

	cast := beatsOfKind(beats, sessionpb.EventKind_EVENT_KIND_CAST)
	require.Len(t, cast, 1)
	require.Equal(t, refs.Spells.ViciousMockery().String(), cast[0].GetCast().GetSpell().GetRef())

	saved := beatsOfKind(beats, sessionpb.EventKind_EVENT_KIND_SAVED)
	require.Len(t, saved, 1, "one gate, one save beat")
	body := saved[0].GetSaved()
	require.Equal(t, "skel-1", body.GetSaver())
	require.NotEmpty(t, body.GetAbility(), "the beat says what the save was made with")
	require.Equal(t, int32(3), body.GetRoll(), "the one authoritative d20 face")
	require.Equal(t, int32(13), body.GetDc(),
		"8 + proficiency 2 + Charisma 16's +3, answered by the caster before any machine ran")
	require.False(t, body.GetSucceeded(),
		"a 3 on a skeleton's Wisdom cannot reach 13 from any direction")
	require.Equal(t, refs.Spells.ViciousMockery().String(), body.GetSource().GetRef(),
		"the save names what demanded it, so a client narrates it without holding the cast beat beside it")

	// -- what the failure delivered: damage AND the rider, both as activation
	// results, because that is where delivered effects have always lived --
	var damage *sessionpb.DamageApplied
	conditionsApplied := map[string]bool{}
	for _, evt := range beatsOfKind(beats, sessionpb.EventKind_EVENT_KIND_ACTIVATION_RESULT) {
		result := evt.GetActivationResult()
		if applied := result.GetDamageApplied(); applied != nil {
			damage = applied
		}
		if applied := result.GetConditionApplied(); applied != nil {
			conditionsApplied[applied.GetRef()] = true
		}
	}

	require.NotNil(t, damage, "a failed save against Vicious Mockery costs the target 1d4 psychic")
	require.Equal(t, "skel-1", damage.GetTarget())
	require.Positive(t, damage.GetAmount(), "the skeleton actually lost hit points")
	require.Greater(t, damage.GetHpBefore(), damage.GetHpAfter(),
		"the two hit-point readings are either side of the same blow")
	require.Equal(t, refs.Spells.ViciousMockery().String(), damage.GetSourceRef())
	require.Equal(t, sessionpb.DamageType_DAMAGE_TYPE_PSYCHIC, damage.GetDamageType(),
		"the rulebook says what kind of damage this is; a client never reads the spell's ref to work it out")
	require.NotNil(t, damage.GetCalculation(),
		"the 1d4's own face reaches the client, not only its total")
	require.Equal(t, damage.GetRequested(), damage.GetCalculation().GetTotal(),
		"the calculation's total IS the requested damage, never a second number beside it")

	require.True(t, conditionsApplied[refs.Conditions.ViciousMockery().String()],
		"the rider lands on the target beside the damage")
	require.Equal(t, conditions.ViciousMockeryName, "Vicious Mockery",
		"the condition's display name is the content's, and this pins the spelling the beat carries")
}

// TestAcceptance_AKnownCantripWithNoContentMintsNoRow is ruling R9's stated
// consequence, asserted rather than assumed: a bard who knows only cantrips
// this build has no cast content for is offered NO CAST ROWS AT ALL.
//
// Fail closed. The alternative -- a row per known cantrip, some of which
// resolve to nothing -- is an affordance with nothing behind it, which is
// exactly the shape slice one's walk kept finding. The gate on the choice
// list means a real bard cannot reach this state today; the door is asserted
// to hold anyway, because the day a tenth cantrip is known and a ninth still
// has no profile, this is what decides whether the panel lies.
func TestAcceptance_AKnownCantripWithNoContentMintsNoRow(t *testing.T) {
	h, bardCtx, _ := castScene(t, spells.MageHand, spells.Light)

	require.Empty(t, castRows(bardCtx, t, h, "bella"),
		"Mage Hand and Light carry no cast content, so they mint no rows rather than rows that resolve to nothing")
}

// TestAcceptance_ACastWithNoTargetIsRefusedAsACallerMistake walks the refusal
// the whole seam was rewired for: resolution's own ErrBadAction is translated
// to session's ErrBadCast at the seam, and statusError turns that into
// INVALID_ARGUMENT.
//
// A CALLER MISTAKE, not a world state. Vicious Mockery names one creature, so
// a request that names nobody is malformed in its own shape -- the same
// bucket ErrBadActivation sits in, and deliberately NOT the bucket a target
// that drifted out of range lands in, which is a stale declaration and
// FAILED_PRECONDITION. A client can tell "I built this wrong" from "the world
// moved" only because the two codes differ.
func TestAcceptance_ACastWithNoTargetIsRefusedAsACallerMistake(t *testing.T) {
	h, bardCtx, _ := castScene(t, spells.TrueStrike, spells.ViciousMockery)

	row := castRowFor(bardCtx, t, h, "bella", spells.ViciousMockery)
	require.Equal(t, sessionpb.TargetKind_TARGET_KIND_MEMBER, row.GetTargetKind(),
		"the row itself says a target is required, before any click")

	_, err := h.handler.Cast(bardCtx, &sessionpb.CastRequest{
		Session: castSessionID, Member: "bella",
		DeclarationId: row.GetId(),
	})
	require.Error(t, err)
	require.Equal(t, codes.InvalidArgument, status.Code(err),
		"a cast that names nobody is malformed, not refused by the world")

	// AND NOTHING MOVED. A refusal at the door is all-or-none: the action is
	// still hers to spend, so the row is still there to click.
	require.Len(t, castRows(bardCtx, t, h, "bella"), 2,
		"the refused cast cost nothing, so both rows are still there to click")
}
