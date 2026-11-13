package sessionv1alpha2

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
)

func strp(s string) *string { return &s }

func hopsp(n int) *int { return &n }

func hopsP(n int32) *int32 { return &n }

// TestSpeechToProtoEachPerceiver pins the one speech message
// (rpg-project#934, R6) for each perceiver's version of a spoken thing: every
// part the version holds crosses one-to-one, and AN ABSENT PART STAYS ABSENT
// -- no speaker or spoken-to unless seen (never a zero party), no words
// unless caught (the optional field unset, never ""), no hops when the
// speaker was seen. The act rides FAMILY_ACT and carries its Perceived part.
func TestSpeechToProtoEachPerceiver(t *testing.T) {
	sight, hearing := sdk.SenseSight, sdk.SenseHearing
	sightP, hearingP := sessionpb.Sense_SENSE_SIGHT, sessionpb.Sense_SENSE_HEARING
	shout := "Oi! Pedlar! What have you got to sell?"

	for name, c := range map[string]struct {
		body      sdk.SpeechAct
		senses    []sdk.Sense
		want      *sessionpb.Speech
		wantSense []sessionpb.Sense
	}{
		"seen and heard": {
			body: sdk.SpeechAct{Utterance: "u", Speaker: alice, SpokenTo: gate1, Words: strp("Stand aside."),
				Option: "stand-aside", Place: cell, Loudness: "speak"},
			senses: []sdk.Sense{sight, hearing},
			want: &sessionpb.Speech{Utterance: "u", Speaker: aliceP, SpokenTo: gate1P, Words: strp("Stand aside."),
				Option: "stand-aside", Place: cellP, Loudness: sessionpb.Loudness_LOUDNESS_SPEAK},
			wantSense: []sessionpb.Sense{sightP, hearingP},
		},
		"heard unseen, with someone's roll": {
			body: sdk.SpeechAct{Utterance: "u", Words: strp("We're expected."), Option: "guest", Place: cell,
				Hops: hopsp(0), Loudness: "speak", Skill: "persuasion", Roll: d20},
			senses: []sdk.Sense{hearing},
			want: &sessionpb.Speech{Utterance: "u", Words: strp("We're expected."), Option: "guest", Place: cellP,
				Hops: hopsP(0), Loudness: sessionpb.Loudness_LOUDNESS_SPEAK, Skill: "persuasion", Roll: d20P},
			wantSense: []sessionpb.Sense{hearingP},
		},
		"one room further": {
			body:      sdk.SpeechAct{Utterance: "u", Words: strp(shout), Place: cell, Hops: hopsp(1), Loudness: "shout"},
			senses:    []sdk.Sense{hearing},
			want:      &sessionpb.Speech{Utterance: "u", Words: strp(shout), Place: cellP, Hops: hopsP(1), Loudness: sessionpb.Loudness_LOUDNESS_SHOUT},
			wantSense: []sessionpb.Sense{hearingP},
		},
		"voice only": {
			body:      sdk.SpeechAct{Utterance: "u", Place: cell, Hops: hopsp(0), Loudness: "scream"},
			senses:    []sdk.Sense{hearing},
			want:      &sessionpb.Speech{Utterance: "u", Place: cellP, Hops: hopsP(0), Loudness: sessionpb.Loudness_LOUDNESS_SCREAM},
			wantSense: []sessionpb.Sense{hearingP},
		},
		"a whisper watched": {
			body:      sdk.SpeechAct{Utterance: "u", Speaker: alice, SpokenTo: gate1, Place: cell, Loudness: "whisper"},
			senses:    []sdk.Sense{sight},
			want:      &sessionpb.Speech{Utterance: "u", Speaker: aliceP, SpokenTo: gate1P, Place: cellP, Loudness: sessionpb.Loudness_LOUDNESS_WHISPER},
			wantSense: []sessionpb.Sense{sightP},
		},
		"the speaker themself": {
			body: sdk.SpeechAct{Utterance: "u", Speaker: alice, SpokenTo: gate1, Words: strp(shout), Option: "hail-the-pedlar",
				Place: cell, Loudness: "shout"},
			senses: []sdk.Sense{sight, hearing},
			want: &sessionpb.Speech{Utterance: "u", Speaker: aliceP, SpokenTo: gate1P, Words: strp(shout), Option: "hail-the-pedlar",
				Place: cellP, Loudness: sessionpb.Loudness_LOUDNESS_SHOUT},
			wantSense: []sessionpb.Sense{sightP, hearingP},
		},
		"spoken to a prop": {
			body:      sdk.SpeechAct{Utterance: "u", Speaker: alice, SpokenTo: chest, Words: strp("Open."), Place: cell, Loudness: "speak"},
			senses:    []sdk.Sense{sight, hearing},
			want:      &sessionpb.Speech{Utterance: "u", Speaker: aliceP, SpokenTo: chestP, Words: strp("Open."), Place: cellP, Loudness: sessionpb.Loudness_LOUDNESS_SPEAK},
			wantSense: []sessionpb.Sense{sightP, hearingP},
		},
		"a taught fact": {
			body: sdk.SpeechAct{Utterance: "u", Speaker: gate1, SpokenTo: alice, Words: strp("Feast, tonight."), Place: cell,
				Loudness: "speak", Fact: "feast-tonight"},
			senses: []sdk.Sense{sight, hearing},
			want: &sessionpb.Speech{Utterance: "u", Speaker: gate1P, SpokenTo: aliceP, Words: strp("Feast, tonight."), Place: cellP,
				Loudness: sessionpb.Loudness_LOUDNESS_SPEAK, Fact: "feast-tonight"},
			wantSense: []sessionpb.Sense{sightP, hearingP},
		},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := actToProto(sdk.Act{
				ID: "id-1", Act: "speech:id-1", Family: sdk.FamilyAct, Body: c.body,
				Perceived: &sdk.Perceived{Senses: c.senses},
			})
			require.NoError(t, err)
			require.Equal(t, sessionpb.Family_FAMILY_ACT, got.GetFamily())
			require.True(t, proto.Equal(c.want, got.GetSpeech()), "want %v\n got %v", c.want, got.GetSpeech())
			require.Equal(t, c.wantSense, got.GetPerceived().GetSenses())
			require.Empty(t, got.GetPerceived().GetVia(), "the recipient's own senses name no friend")
			if c.body.Words == nil {
				require.Nil(t, got.GetSpeech().Words, "words not caught stay unset")
			}
			if c.body.Hops == nil {
				require.Nil(t, got.GetSpeech().Hops, "a seen speaker carries no hops")
			}
		})
	}
}

// TestSpeechToProtoRefusesAnUnknownLoudness: a loudness word this build has
// no spelling for refuses the act, never crossing as UNSPECIFIED.
func TestSpeechToProtoRefusesAnUnknownLoudness(t *testing.T) {
	got, err := actToProto(sdk.Act{ID: "id-1", Family: sdk.FamilyAct, Body: sdk.SpeechAct{Utterance: "u", Loudness: "bellow"}})
	require.ErrorIs(t, err, errUnknownValue)
	require.True(t, got.GetRefused())
	require.Nil(t, got.GetSpeech())
}

// TestPerceivedRefusesAnUnknownSense: the envelope's senses fail closed the
// way every other closed set here does.
func TestPerceivedRefusesAnUnknownSense(t *testing.T) {
	got, err := actToProto(sdk.Act{
		ID: "id-1", Family: sdk.FamilyAct, Body: sdk.SpeechAct{Utterance: "u", Loudness: "speak"},
		Perceived: &sdk.Perceived{Senses: []sdk.Sense{"smell"}},
	})
	require.ErrorIs(t, err, errUnknownValue)
	require.True(t, got.GetRefused())
}

// TestAnActWithoutPerceivedCarriesNone: every other act crosses with no
// Perceived part.
func TestAnActWithoutPerceivedCarriesNone(t *testing.T) {
	got, err := actToProto(sdk.Act{ID: "id-1", Family: sdk.FamilyAct, Body: sdk.DoorAct{Door: "gate", State: "open"}})
	require.NoError(t, err)
	require.Nil(t, got.GetPerceived())
}

// TestPerceivedRidesEveryBody pins the shared view on the wire
// (rpg-project#935): the act envelope's Perceived -- senses and via, the
// friends whose senses they were -- is spelled the same on every body; a
// perception keeps the observer the act names (the friend who saw), never
// the reader; and an empty via stays empty.
func TestPerceivedRidesEveryBody(t *testing.T) {
	via := &sdk.Perceived{Senses: []sdk.Sense{sdk.SenseSight}, Via: []sdk.Party{gate1}}
	for name, c := range map[string]struct {
		family sdk.Family
		body   sdk.ActBody
	}{
		"speech":       {sdk.FamilyAct, sdk.SpeechAct{Utterance: "u", Speaker: gate1, Words: strp("Halt."), Place: cell, Loudness: "shout"}},
		"perception":   {sdk.FamilyPerception, sdk.Perception{Observer: gate1, Sense: sdk.SenseSight, Sighted: &sdk.SightChange{Gained: []string{"alice"}}}},
		"walk":         {sdk.FamilyAct, sdk.WalkAct{Mover: gate1, Path: []sdk.WalkStep{{Cell: cell}}}},
		"table answer": {sdk.FamilyTableAnswer, sdk.TableAnswer{Who: gate1}},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := actToProto(sdk.Act{ID: "id-1", Recipient: "alice", Family: c.family, Body: c.body, Perceived: via})
			require.NoError(t, err)
			require.True(t, proto.Equal(&sessionpb.Perceived{Senses: []sessionpb.Sense{sessionpb.Sense_SENSE_SIGHT}, Via: []*sessionpb.Party{gate1P}},
				got.GetPerceived()), "%v", got.GetPerceived())
			if p := got.GetPerception(); p != nil {
				require.True(t, proto.Equal(gate1P, p.GetObserver()), "the friend who saw, not the reader")
			}
		})
	}
	got, err := actToProto(sdk.Act{ID: "id-2", Family: sdk.FamilyAct, Body: sdk.SpeechAct{Utterance: "u", Place: cell, Loudness: "speak"},
		Perceived: &sdk.Perceived{Senses: []sdk.Sense{sdk.SenseHearing}}})
	require.NoError(t, err)
	require.Empty(t, got.GetPerceived().GetVia(), "an empty via stays empty")
}
