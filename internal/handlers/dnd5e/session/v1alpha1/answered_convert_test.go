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
		Key:      "intimidated",
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
		Creature: "front-goblin", Key: "persuade_failed", Verb: "persuade", Beaten: false,
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
		Creature: "front-goblin", Key: "persuade_failed", Verb: "persuade", Beaten: false,
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
		Creature: "front-goblin", Key: "intimidate_failed", Verb: "intimidate", Beaten: false,
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
		Creature: "front-goblin", Key: "intimidated", Verb: "intimidate", Beaten: false,
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
			Creature: "front-goblin", Key: "persuaded", Verb: "persuade", Roll: 1, Of: 1, Word: "lure",
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
			Creature: "front-goblin", Key: "persuaded", Verb: "persuade", Roll: 1, Of: 1, Word: "pretend",
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

// TestAnswerWordToProto_EveryWordThisBuildShips is the vocabulary itself
// (rpg-project#465). Six words and one empty, each pinned to its exact wire
// value, because this is the only place the seam's spelling and the contract's
// meet and a switch arm added to the wrong case is invisible everywhere else.
//
// THE FOUR TIME WORDS ARE WHY THIS IS TABLE-DRIVEN NOW. `hold`, `attack`,
// `toward` and `away` all arrived in one slice, and a row per word is what
// makes the next one's absence read as a missing row rather than as a passing
// test.
func TestAnswerWordToProto_EveryWordThisBuildShips(t *testing.T) {
	for _, tc := range []struct {
		word string
		want sessionpb.AnswerWord
	}{
		// An entry that only speaks. UNSPECIFIED is its exact spelling and a
		// positive claim, which is what makes demoting an unknown word a lie.
		{"", sessionpb.AnswerWord_ANSWER_WORD_UNSPECIFIED},
		{"fact", sessionpb.AnswerWord_ANSWER_WORD_FACT},
		{"flee", sessionpb.AnswerWord_ANSWER_WORD_FLEE},
		{"hold", sessionpb.AnswerWord_ANSWER_WORD_HOLD},
		{"attack", sessionpb.AnswerWord_ANSWER_WORD_ATTACK},
		{"toward", sessionpb.AnswerWord_ANSWER_WORD_TOWARD},
		{"away", sessionpb.AnswerWord_ANSWER_WORD_AWAY},
	} {
		t.Run(tc.word, func(t *testing.T) {
			got, err := answerWordToProto(tc.word)
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

// TestAnswerKeyToProto_TheFiveKeysAndNothingElse pins which table the world
// rolled on (rpg-project#465 §2) — the one field that replaces `verb` plus
// `beaten`, which between them could spell exactly four.
//
// EMPTY REFUSES, and it is in the table as a row rather than left out. There
// is no such thing as a pick on no key: the SDK fills this on every beat it
// writes, so an empty one is a beat stored by the build BEFORE the table
// existed, and this seam cannot honestly spell it. That is a deliberate choice
// and the row is where somebody reading this file finds it.
func TestAnswerKeyToProto_TheFiveKeysAndNothingElse(t *testing.T) {
	for _, tc := range []struct {
		key     string
		want    sessionpb.AnswerKey
		refused bool
	}{
		{key: "intimidated", want: sessionpb.AnswerKey_ANSWER_KEY_INTIMIDATED},
		{key: "intimidate_failed", want: sessionpb.AnswerKey_ANSWER_KEY_INTIMIDATE_FAILED},
		{key: "persuaded", want: sessionpb.AnswerKey_ANSWER_KEY_PERSUADED},
		{key: "persuade_failed", want: sessionpb.AnswerKey_ANSWER_KEY_PERSUADE_FAILED},
		{key: "time", want: sessionpb.AnswerKey_ANSWER_KEY_TIME},
		{key: "", refused: true},
		// A trigger the design names and this build does not ship. The enum
		// grows one value per use case, so an unknown key means this seam is
		// older than the toolkit that wrote the beat.
		{key: "attacked", refused: true},
	} {
		t.Run("key="+tc.key, func(t *testing.T) {
			got, err := answerKeyToProto(tc.key)
			if tc.refused {
				require.Error(t, err, "an unknown key must not be demoted to UNSPECIFIED")
				require.Equal(t, sessionpb.AnswerKey_ANSWER_KEY_UNSPECIFIED, got)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

// TestTemperToProto_EmptyIsNoneAndAnUnknownWordRefuses is the law slotToProto
// keeps, one enum over (rpg-project#465 §3).
//
// THREE DISTINCT OUTCOMES, and the distinctions are the whole test. Empty is
// TEMPER_NONE — having no temperament is a real answer about a creature, not a
// producer that forgot. `soldier` is TEMPER_SOLDIER, which multiplies
// identically to NONE and is a DIFFERENT value, because somebody named it. And
// a word this build cannot name REFUSES rather than degrading either way:
// UNSPECIFIED would confess the build's failure in a field a client reads as
// the world's answer, and NONE would publish "this creature has no
// temperament" about one whose author gave it one.
//
// "solider" IS THE ROW THAT MATTERS. A typo in a dungeon file is the realistic
// way an unknown word reaches this seam, and it is exactly the case where a
// silent demotion would leave the placement playing perfectly well as a
// soldier forever with nothing anywhere saying the word was dropped.
func TestTemperToProto_EmptyIsNoneAndAnUnknownWordRefuses(t *testing.T) {
	for _, tc := range []struct {
		word    string
		want    sessionpb.Temper
		refused bool
	}{
		{word: "", want: sessionpb.Temper_TEMPER_NONE},
		{word: "soldier", want: sessionpb.Temper_TEMPER_SOLDIER},
		{word: "coward", want: sessionpb.Temper_TEMPER_COWARD},
		{word: "aggressive", want: sessionpb.Temper_TEMPER_AGGRESSIVE},
		{word: "solider", refused: true},
		{word: "berserker", refused: true},
	} {
		t.Run("temper="+tc.word, func(t *testing.T) {
			got, err := temperToProto(tc.word)
			if tc.refused {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.word, "the refusal names the word that caused it")
				require.Equal(t, sessionpb.Temper_TEMPER_UNSPECIFIED, got)
				require.NotEqual(t, sessionpb.Temper_TEMPER_NONE, got,
					"degrading to NONE would present an unknown word as a creature with no temperament")
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}

// TestEventTemperedKindToProto is the dealt temperament's kind half. An
// unmapped kind demotes to EVENT_KIND_UNKNOWN with a nil body, and this beat is
// the ONLY account of the roll that dealt a creature its word — demoted, the
// table would watch four goblins behave differently with nothing saying why.
func TestEventTemperedKindToProto(t *testing.T) {
	require.Equal(t, sessionpb.EventKind_EVENT_KIND_TEMPERED, eventKindToProto(sdk.EventTempered))
}

// TestAnsweredBodyToProto_ATimePickCarriesItsArithmeticAndLeavesTheOldPairUnset
// is the `time` key's acceptance case (rpg-project#465 §6).
//
// THE UNSET PAIR IS THE CLAIM, not an omission. `verb` and `beaten` are
// deprecated and still filled on the four social keys so a reader written
// against the shipped shape keeps working; on `time` nothing spoke and nothing
// was beaten, and VERB_UNSPECIFIED beside `beaten: false` reads as a threat
// that failed — a sentence about an event that never happened.
//
// AND THE WHOLE LOADED TABLE CROSSES, line by line, because the beat has to be
// replayable: add `loaded` across the candidates and you get `of`, and the face
// lands in exactly one of them. A converter that carried the total alone would
// leave a reader taking the engine's sum on trust.
func TestAnsweredBodyToProto_ATimePickCarriesItsArithmeticAndLeavesTheOldPairUnset(t *testing.T) {
	event := &sessionpb.Event{}
	require.NoError(t, setEventBody(event, sdk.AnsweredBody{
		Creature: "front-goblin",
		Key:      "time",
		// The SDK leaves both empty on a time pick, and so must the wire.
		Verb: "", Beaten: false,
		Roll: 900, Of: 1600, Entry: 3, Word: "away", Temper: "coward",
		Candidates: []sdk.AnswerCandidate{
			{Entry: 0, Weight: 3, Percent: 300, Loaded: 900},
			{Entry: 3, Weight: 1, Percent: 700, Loaded: 700},
		},
	}))

	got := event.GetAnswered()
	require.NotNil(t, got)
	require.Equal(t, sessionpb.AnswerKey_ANSWER_KEY_TIME, got.GetKey())
	require.Equal(t, sessionpb.AnswerWord_ANSWER_WORD_AWAY, got.GetWord())
	require.Equal(t, sessionpb.Temper_TEMPER_COWARD, got.GetTemper())
	require.Equal(t, sessionpb.Verb_VERB_UNSPECIFIED, got.GetVerb(),
		"no verb stands behind a time pick, and an unset one is the truth rather than a hole")
	require.False(t, got.GetBeaten(),
		"there was no check to have beaten, which is why `key` supersedes this pair")

	require.Len(t, got.GetCandidates(), 2, "every eligible entry crosses, in the author's order")
	require.Equal(t, int32(0), got.GetCandidates()[0].GetEntry())
	require.Equal(t, int32(3), got.GetCandidates()[0].GetWeight(), "the author's own number, before loading")
	require.Equal(t, int32(300), got.GetCandidates()[0].GetPercent(), "the temperament's multiplier, in percent")
	require.Equal(t, int32(900), got.GetCandidates()[0].GetLoaded(), "weight x percent, copied and never recomputed")
	require.Equal(t, int32(3), got.GetCandidates()[1].GetEntry(),
		"indices are the AUTHOR'S and are not contiguous when a `when` filtered a line out")
	require.Equal(t, int32(700), got.GetCandidates()[1].GetLoaded())

	var summed int32
	for _, c := range got.GetCandidates() {
		summed += c.GetLoaded()
	}
	require.Equal(t, got.GetOf(), summed, "`of` is the sum of `loaded`, so a reader can redo the engine's arithmetic")
}

// TestAnsweredBodyToProto_ASocialPickStillFillsTheDeprecatedPair is the other
// half of the same rule, and it is the compatibility claim: a reader written
// before the table keeps working unchanged on all four social keys.
func TestAnsweredBodyToProto_ASocialPickStillFillsTheDeprecatedPair(t *testing.T) {
	for _, tc := range []struct {
		key    string
		verb   sessionpb.Verb
		beaten bool
	}{
		{key: "intimidated", verb: sessionpb.Verb_VERB_INTIMIDATE, beaten: true},
		{key: "intimidate_failed", verb: sessionpb.Verb_VERB_INTIMIDATE, beaten: false},
		{key: "persuaded", verb: sessionpb.Verb_VERB_PERSUADE, beaten: true},
		{key: "persuade_failed", verb: sessionpb.Verb_VERB_PERSUADE, beaten: false},
	} {
		t.Run(tc.key, func(t *testing.T) {
			verbWord := "intimidate"
			if tc.verb == sessionpb.Verb_VERB_PERSUADE {
				verbWord = "persuade"
			}
			event := &sessionpb.Event{}
			require.NoError(t, setEventBody(event, sdk.AnsweredBody{
				Creature: "front-goblin", Key: tc.key, Verb: verbWord, Beaten: tc.beaten,
				Roll: 4200, Of: 10000, Entry: 0, Word: "fact", Temper: "",
				Candidates: []sdk.AnswerCandidate{{Entry: 0, Weight: 70, Percent: 100, Loaded: 7000}},
			}))

			got := event.GetAnswered()
			require.NotNil(t, got)
			require.Equal(t, tc.verb, got.GetVerb())
			require.Equal(t, tc.beaten, got.GetBeaten())
			require.Equal(t, sessionpb.Temper_TEMPER_NONE, got.GetTemper(),
				"a creature nobody gave a word to is NONE, never UNSPECIFIED and never SOLDIER")
		})
	}
}

// TestAnsweredBodyToProto_NothingEligibleIsTheHoldShape pins the silent table
// (design §2, "fail closed loudly"): a `time` roll where no entry's `when` held
// has nothing on the die, the creature holds, and the beat SAYS SO.
//
// EMPTY CANDIDATES IS A REAL ANSWER, not a missing one — a reader must be able
// to tell a creature with nothing to do apart from a creature that rolled and
// chose to stand there.
func TestAnsweredBodyToProto_NothingEligibleIsTheHoldShape(t *testing.T) {
	event := &sessionpb.Event{}
	require.NoError(t, setEventBody(event, sdk.AnsweredBody{
		Creature: "front-goblin", Key: "time", Roll: 0, Of: 0, Entry: -1, Word: "hold",
	}))

	got := event.GetAnswered()
	require.NotNil(t, got, "a creature that was asked and stood there still gets a beat")
	require.Empty(t, got.GetCandidates())
	require.Equal(t, sessionpb.AnswerWord_ANSWER_WORD_HOLD, got.GetWord())
	require.Equal(t, int32(0), got.GetOf())
}

// TestAnsweredBodyToProto_AnUnknownKeyOrTemperIsRefused is the word refusal's
// twin on the two fields that arrived with it. Each is asserted alone, so a
// seam that started demoting one of them cannot hide behind the other.
func TestAnsweredBodyToProto_AnUnknownKeyOrTemperIsRefused(t *testing.T) {
	t.Run("key", func(t *testing.T) {
		event := &sessionpb.Event{}
		err := setEventBody(event, sdk.AnsweredBody{
			Creature: "front-goblin", Key: "attacked", Roll: 1, Of: 1, Word: "hold",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "attacked")
		require.Nil(t, event.GetAnswered(), "nothing half-built is left for a careless caller to send")
	})

	t.Run("temper", func(t *testing.T) {
		event := &sessionpb.Event{}
		err := setEventBody(event, sdk.AnsweredBody{
			Creature: "front-goblin", Key: "time", Roll: 1, Of: 1, Word: "hold", Temper: "berserker",
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "berserker")
		require.Nil(t, event.GetAnswered())
	})
}

// TestTemperedBodyToProto_CarriesTheDealAndWhoThrewIt is the dealt
// temperament's body (rpg-project#465 §3, R5): which creature was dealt what,
// and the roll that dealt it.
//
// THE FACTION IS THE DIE'S ENTITY and is asserted separately from the member,
// because the thrower and the subject of a throw are different questions
// (rpg-project#463). A converter that filled `faction` from the creature would
// leave a reader unable to say which mix produced the roll or check the sum
// against anything.
func TestTemperedBodyToProto_CarriesTheDealAndWhoThrewIt(t *testing.T) {
	event := &sessionpb.Event{}
	require.NoError(t, setEventBody(event, sdk.TemperedBody{
		Member: "front-goblin-3", Temper: "coward", Roll: 2, Of: 4, Faction: "goblins",
	}))

	got := event.GetTempered()
	require.NotNil(t, got)
	require.Equal(t, "front-goblin-3", got.GetMember(), "who the word landed on")
	require.Equal(t, sessionpb.Temper_TEMPER_COWARD, got.GetTemper())
	require.Equal(t, int32(2), got.GetRoll())
	require.Equal(t, int32(4), got.GetOf(), "the summed shares of the mix, never assumed to be 100")
	require.Equal(t, "goblins", got.GetFaction(), "the faction threw: the mix is its, not the creature's")
	require.NotEqual(t, got.GetMember(), got.GetFaction(),
		"thrower and subject are separate fields and this seam must not fill one from the other")
}

// TestTemperedBodyToProto_AnUnknownWordIsRefused. A deal always produces a word
// the composition could apply, so an unknown one here means this seam is older
// than the rulebook that grew a fourth temperament — and publishing NONE would
// say the creature was dealt nothing, about a creature that was dealt something.
func TestTemperedBodyToProto_AnUnknownWordIsRefused(t *testing.T) {
	event := &sessionpb.Event{}
	err := setEventBody(event, sdk.TemperedBody{
		Member: "front-goblin-3", Temper: "reckless", Roll: 2, Of: 4, Faction: "goblins",
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "reckless")
	require.Nil(t, event.GetTempered())
}
