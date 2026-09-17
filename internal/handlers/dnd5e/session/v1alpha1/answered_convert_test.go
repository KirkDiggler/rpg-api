package sessionv1alpha1

import (
	"testing"

	"github.com/stretchr/testify/require"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// answered_convert_test.go is the front room goblin's wire half
// (rpg-project#458): the appeal's own beat, and the WORLD's roll on the
// author's answer table.

// TestVerbPersuadeToProto is the dock's half. Afford emits this row on BOTH
// clocks now (R3), and the world-clock one is the whole panel this slice
// exists to fill -- a client drops a verb it cannot name rather than drawing
// it wrong, so unmapped the row would simply never appear in the front room.
func TestVerbPersuadeToProto(t *testing.T) {
	require.Equal(t, sessionpb.Verb_VERB_PERSUADE, verbToProto(sdk.VerbPersuade))
}

// TestEventPersuadedAndAnsweredKindsToProto is the kind half of both beats. An
// unmapped kind does not fail -- it demotes to EVENT_KIND_UNKNOWN and the body
// stays nil -- so a missing arm here would lose the whole account of a throw
// rather than degrade one field of it.
func TestEventPersuadedAndAnsweredKindsToProto(t *testing.T) {
	require.Equal(t, sessionpb.EventKind_EVENT_KIND_PERSUADED, eventKindToProto(sdk.EventPersuaded))
	require.Equal(t, sessionpb.EventKind_EVENT_KIND_ANSWERED, eventKindToProto(sdk.EventAnswered))
}

// TestPersuadedBodyToProto_CarriesTheWholeRollBothWays mirrors the threat
// beat's own scene, and the missed case is again the one that matters most:
// PersuadeResponse carries no beaten, total or dc, so this beat is the ONLY
// account of the roll and the actor reads it here like every other witness.
//
// FALSE IS AN ANSWER. `beaten: false` is the whole content of a failed appeal,
// and it is the verdict the `persuade_failed` table was read against.
func TestPersuadedBodyToProto_CarriesTheWholeRollBothWays(t *testing.T) {
	for _, tc := range []struct {
		name   string
		beaten bool
		total  int
	}{
		{name: "beaten", beaten: true, total: 13},
		{name: "missed", beaten: false, total: 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			event := &sessionpb.Event{}
			require.NoError(t, setEventBody(event, sdk.PersuadedBody{
				Actor: "char-alice", Target: "front-goblin", DC: 10, Total: tc.total, Beaten: tc.beaten,
			}))

			got := event.GetPersuaded()
			require.NotNil(t, got, "an appeal that reached this seam must reach the wire")
			require.Equal(t, "char-alice", got.GetActor())
			require.Equal(t, "front-goblin", got.GetTarget())
			require.Equal(t, int32(10), got.GetDc())
			require.Equal(t, int32(tc.total), got.GetTotal())
			require.Equal(t, tc.beaten, got.GetBeaten())
		})
	}
}

// TestPersuadedBodyToProto_BeatenIsCopiedNeverDerived pins the reading as the
// provider's. A 20 the rulebook says did not beat a DC 10 crosses as
// beaten=false, because the day a rule changes what beating a DC means, every
// reader that compared total against dc would be wrong at once.
func TestPersuadedBodyToProto_BeatenIsCopiedNeverDerived(t *testing.T) {
	event := &sessionpb.Event{}
	require.NoError(t, setEventBody(event, sdk.PersuadedBody{
		Actor: "char-alice", Target: "front-goblin", DC: 10, Total: 20, Beaten: false,
	}))
	require.False(t, event.GetPersuaded().GetBeaten())
	require.Equal(t, int32(20), event.GetPersuaded().GetTotal())
	require.Equal(t, int32(10), event.GetPersuaded().GetDc())
}

// TestAnsweredBodyToProto_CarriesTheAuthorsLineAndTheWorldsDie is the second
// roll's acceptance case, and it is asserted field by field because EVERY one
// of them is load-bearing somewhere: the die and the weights are R1's debug
// log, the entry index is what a builder highlights in the author's file, the
// line is what the story log prints VERBATIM, and the fact is what a
// disposition or an arrival reads.
func TestAnsweredBodyToProto_CarriesTheAuthorsLineAndTheWorldsDie(t *testing.T) {
	event := &sessionpb.Event{}
	require.NoError(t, setEventBody(event, sdk.AnsweredBody{
		Creature: "front-goblin",
		Verb:     "intimidate",
		Beaten:   true,
		Roll:     42,
		Of:       100,
		Entry:    0,
		Word:     "fact",
		Say:      "Fine! FINE. The cellar door is behind the barrels. Just don't.",
		Fact:     "goblin-cowed",
	}))

	got := event.GetAnswered()
	require.NotNil(t, got)
	require.Equal(t, "front-goblin", got.GetCreature())
	require.Equal(t, sessionpb.Verb_VERB_INTIMIDATE, got.GetVerb(),
		"the verb is typed on the wire, so a client that already branches on Verb branches once")
	require.True(t, got.GetBeaten(), "which table the world rolled on, repeated so the beat stands alone")
	require.Equal(t, int32(42), got.GetRoll())
	require.Equal(t, int32(100), got.GetOf(), "the die SIZE, which is the summed weights and not the entry count")
	require.Equal(t, int32(0), got.GetEntry(), "zero is an answer: the author's first line fired")
	require.Equal(t, sessionpb.AnswerWord_ANSWER_WORD_FACT, got.GetWord())
	require.Equal(t, "Fine! FINE. The cellar door is behind the barrels. Just don't.", got.GetSay(),
		"the author's line VERBATIM; this seam never composes, trims or re-cases one")
	require.Empty(t, got.GetFact(),
		"a fact is per-observer knowledge and never rides a broadcast beat (rpg-project#458)")
}

// TestAnsweredBodyToProto_TheFactNeverRidesTheBeat is the ruling made
// mechanical (Kirk, rpg-project#458), and it is asserted on the word that
// TEACHES one, because that is the only case where a fact exists to leak.
//
// THE BEAT IS BROADCAST AND A FACT IS NOT. Every witness of the creature gets
// this beat; what any one of them then knows is the intel log's answer, held
// per observer. An id on the beat would hand the whole table a fact the world
// may have taught only some of them, and no second field could take it back.
//
// The SDK still carries it, and that is the point of testing here rather than
// trusting the absence: this seam reads a populated field and deliberately
// drops it, so a future edit that "fixes the missing mapping" fails this.
func TestAnsweredBodyToProto_TheFactNeverRidesTheBeat(t *testing.T) {
	event := &sessionpb.Event{}
	require.NoError(t, setEventBody(event, sdk.AnsweredBody{
		Creature: "front-goblin", Verb: "persuade", Beaten: false,
		Roll: 40, Of: 100, Entry: 0, Word: "fact",
		Say:  "Cellar's empty, friend.",
		Fact: "cellar-is-clear",
	}))

	got := event.GetAnswered()
	require.NotNil(t, got)
	require.Empty(t, got.GetFact(), "the id the SDK carried must not reach the wire")
	// EVERYTHING ELSE STILL CROSSES, so this is a dropped field and not a
	// dropped beat: the word still says the creature taught something, and the
	// line the player actually receives is untouched.
	require.Equal(t, sessionpb.AnswerWord_ANSWER_WORD_FACT, got.GetWord())
	require.Equal(t, "Cellar's empty, friend.", got.GetSay())
	require.Equal(t, int32(40), got.GetRoll())
}

// TestAnsweredBodyToProto_FleeAndTheFailureTable is the other half of the
// table: the entry that makes the goblin run, read off the FAILURE table after
// a missed appeal, with a nonzero entry index. Nothing here is derivable from
// the scene above.
func TestAnsweredBodyToProto_FleeAndTheFailureTable(t *testing.T) {
	event := &sessionpb.Event{}
	require.NoError(t, setEventBody(event, sdk.AnsweredBody{
		Creature: "front-goblin", Verb: "persuade", Beaten: false,
		Roll: 3, Of: 4, Entry: 1, Word: "flee", Say: "Boss! BOSS!",
	}))

	got := event.GetAnswered()
	require.NotNil(t, got)
	require.Equal(t, sessionpb.Verb_VERB_PERSUADE, got.GetVerb())
	require.False(t, got.GetBeaten(), "the failure table was read, and the beat says so on its own")
	require.Equal(t, int32(1), got.GetEntry())
	require.Equal(t, sessionpb.AnswerWord_ANSWER_WORD_FLEE, got.GetWord())
	require.Empty(t, got.GetFact(), "a creature that runs teaches nobody anything")
}

// TestAnsweredBodyToProto_AnEntryThatOnlySpeaks pins the distinction the whole
// error return exists to protect. An author may write an entry with a line and
// NO word, and the SDK carries that as an empty string; UNSPECIFIED is its
// exact wire spelling and a legitimate value. That is what makes demoting an
// UNKNOWN word to the same value a lie rather than a degradation.
func TestAnsweredBodyToProto_AnEntryThatOnlySpeaks(t *testing.T) {
	event := &sessionpb.Event{}
	require.NoError(t, setEventBody(event, sdk.AnsweredBody{
		Creature: "front-goblin", Verb: "intimidate", Beaten: false,
		Roll: 1, Of: 1, Entry: 0, Word: "",
		Say: "Big talk, for someone standing in my doorway.",
	}))

	got := event.GetAnswered()
	require.NotNil(t, got)
	require.Equal(t, sessionpb.AnswerWord_ANSWER_WORD_UNSPECIFIED, got.GetWord(),
		"an entry that only speaks is UNSPECIFIED, and that is a positive claim")
	require.Equal(t, "Big talk, for someone standing in my doorway.", got.GetSay())
}

// TestAnsweredBodyToProto_AnUnknownWordIsRefused is the point of the error
// return, and the scene that keeps it honest.
//
// `alarm` IS A REAL WORD OF THE DESIGN that this build does not ship: the enum
// says it grows a value per slice, so the day the toolkit lands it against an
// api nobody rebuilt, the choice is between saying "the creature only spoke"
// about a goblin that ran for the guards, and refusing. It refuses, and the
// message names the word so somebody is sent to the pins rather than to the
// renderer.
//
// NO BODY IS SET on the refusal either: a half-built Answered on a returned
// error is a thing a careless caller sends anyway.
func TestAnsweredBodyToProto_AnUnknownWordIsRefused(t *testing.T) {
	event := &sessionpb.Event{}
	err := setEventBody(event, sdk.AnsweredBody{
		Creature: "front-goblin", Verb: "intimidate", Beaten: false,
		Roll: 1, Of: 1, Word: "alarm", Say: "Boss! BOSS!",
	})
	require.Error(t, err, "a word this build cannot spell must not be demoted to UNSPECIFIED")
	require.Contains(t, err.Error(), "alarm", "the refusal names the word that caused it")
	require.Nil(t, event.GetAnswered(), "nothing half-built is left on the event for a caller to send")
}

// TestEventToProto_AnUnknownWordFailsTheWholeEvent pins that the refusal
// actually escapes the converter rather than being swallowed one frame up, and
// that the wrapper names the seq and kind -- which is what turns "something is
// wrong" into "beat 91 on the answered stream".
func TestEventToProto_AnUnknownWordFailsTheWholeEvent(t *testing.T) {
	_, err := eventToProto(sdk.Event{
		Session: "sess-1", Seq: 91, Kind: sdk.EventAnswered,
		Body: sdk.AnsweredBody{
			Creature: "front-goblin", Verb: "persuade", Roll: 1, Of: 1, Word: "lure",
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "91", "the refusal names which beat could not be projected")
	require.Contains(t, err.Error(), "lure")
}

// TestEventsToProto_OneBadBeatFailsTheWholeRead is the catch-up half. A read
// that silently dropped the beat it could not spell would hand a reconnecting
// client a story with a hole in it and no seq gap to notice -- and that read
// exists precisely so a client can trust that what it got is what happened.
func TestEventsToProto_OneBadBeatFailsTheWholeRead(t *testing.T) {
	_, err := eventsToProto([]sdk.Event{
		{Session: "sess-1", Seq: 1, Kind: sdk.EventDowned, Body: sdk.DownedBody{Member: "goblin-1"}},
		{Session: "sess-1", Seq: 2, Kind: sdk.EventAnswered, Body: sdk.AnsweredBody{
			Creature: "front-goblin", Verb: "persuade", Roll: 1, Of: 1, Word: "pretend",
		}},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "pretend")
}

// TestSightingStanceIsCarriedPerViewer is the ring's own seam
// (rpg-project#458). `session.Sighting` gained a believed stance beside Name
// and Kind, filled from `encounter.BelievedStance` per viewer, and this
// converter copies it verbatim.
//
// IT REPLACED A GAP MARKER. Until the seam carried the field this test asserted
// its ABSENCE, so that its arrival would be noticed rather than sit unwired.
// It arrived; the marker is gone and this is what took its place.
func TestSightingStanceIsCarriedPerViewer(t *testing.T) {
	for _, word := range []string{"hostile", "neutral", "allied"} {
		got := sightingToProto(sdk.Sighting{
			Subject: "front-goblin", Name: "Goblin", Kind: sdk.KindMonster,
			Status: "current", Stance: word,
		})
		require.Equal(t, word, got.GetStance(),
			"the seam's own word crosses verbatim; this converter knows no vocabulary")
	}
}

// TestSightingStanceCrossesEmptyRatherThanBorrowingTheFaction pins the value
// itself, because "we could not fill it" and "we filled it with something
// plausible" look identical from the outside and are opposite decisions. The
// wire says empty means the observer has no word; a faction color smuggled in
// here would be shared truth wearing a per-viewer field's clothes, and the
// first authored `pretend` would have to undo it.
func TestSightingStanceEmptyStaysEmptyForACreatureInNoFaction(t *testing.T) {
	got := sightingToProto(sdk.Sighting{
		Subject: "world-npc-1",
		Name:    "Merchant",
		Kind:    sdk.KindWorld,
		Status:  "current",
		// The seam leaves it empty when the run cannot answer: a subject who
		// is not a member, or one in NO FACTION AT ALL, which a world NPC is.
	})
	require.Equal(t, "world-npc-1", got.GetSubject(), "the fields that DO cross still cross")
	require.Empty(t, got.GetStance(),
		"empty is the wire's own \"no word for it\"; mapping it to neutral would "+
			"invent a belief nobody holds, and the client has a roster color to fall back to")
}
