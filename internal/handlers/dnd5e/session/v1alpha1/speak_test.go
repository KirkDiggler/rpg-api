package sessionv1alpha1

import (
	"context"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	sessionv1alpha1mock "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1/mock"
)

func TestSpeak_Unauthenticated_Errors(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := &Handler{characters: anyMemberOwnedBy(ctrl, "alice")}
	_, err := h.Speak(context.Background(), &sessionpb.SpeakRequest{
		Session: "sess-1", Member: "char-1", Target: &sessionpb.SpeakRequest_MemberId{MemberId: "goblin-2"}, OptionId: "talk-it-round",
	})
	requireCode(t, err, codes.Unauthenticated)
}

func TestSpeak_EmptyMember_IsRefused(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Speak(ctx, &sessionpb.SpeakRequest{Session: "sess-1", Target: &sessionpb.SpeakRequest_MemberId{MemberId: "goblin-2"}, OptionId: "talk-it-round"})
	requireCode(t, err, codes.InvalidArgument)
}

// TestSpeak_ForeignMember_IsRefusedBeforeTheSDK is the verb's sharpest
// gate, and it has to be made again rather than inherited: Speak SPENDS
// SOMEBODY'S ACTION on the turn clock, and the SDK charges it before it rolls,
// so a client naming another player's character would burn that player's turn.
// The manager mock expects nothing -- the point is the call never reaches the
// SDK at all.
func TestSpeak_ForeignMember_IsRefusedBeforeTheSDK(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)

	h := &Handler{manager: mgr, characters: ownedCharacterRepo(ctrl, "char-bob", "bob")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Speak(ctx, &sessionpb.SpeakRequest{
		Session: "sess-1", Member: "char-bob", Target: &sessionpb.SpeakRequest_MemberId{MemberId: "goblin-2"}, OptionId: "talk-it-round",
	})
	requireCode(t, err, codes.PermissionDenied)
}

// TestSpeak_HappyPath_ReturnsTheAckAndNotTheRoll pins both halves of this
// handler's law at once: session/member/target/option reach the SDK verbatim, and a
// settled attempt answers with the ack alone. The SDK output here carries a
// beaten check, a total and a DC -- the numbers a client would most want --
// and NONE of them may appear on the response, because the `spoke` beat is
// the only account of this roll and the actor reads it there like everybody
// else.
func TestSpeak_HappyPath_ReturnsTheAckAndNotTheRoll(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Speak(gomock.Any(), &sdk.SpeakInput{
		Session: "sess-1", Member: "char-alice", Target: "front-goblin", OptionID: "talk-it-round",
	}).Return(&sdk.SpeakOutput{
		Beaten:   true,
		Total:    13,
		DC:       10,
		Target:   "front-goblin",
		Saved:    sdk.SaveReport{Written: []string{"encounter"}},
		Delivery: sdk.DeliveryReport{},
	}, nil)

	h := &Handler{
		manager: mgr,
		characters: charactersOf(ctrl, map[string]rosterCharacter{
			"char-alice": {owner: "alice", name: "Alice", class: "bard", race: "human"},
		}),
	}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.Speak(ctx, &sessionpb.SpeakRequest{
		Session: "sess-1", Member: "char-alice", Target: &sessionpb.SpeakRequest_MemberId{MemberId: "front-goblin"}, OptionId: "talk-it-round",
	})
	require.NoError(t, err)
	require.Equal(t, []string{"encounter"}, resp.GetSaved().GetWritten())
	require.False(t, resp.GetDelivery().GetFailed())
	require.False(t, resp.GetPaused(), "a settled attempt is not a question")
	require.Nil(t, resp.Roll, "the settled roll rides the beat, not the response")
}

// TestSpeak_BeatenAndMissed_AnswerIdentically is the ruling made mechanical,
// and it matters MORE here than it did for the threat: a failed appeal is where
// the goblin's bad directions come from, so the temptation to leak the verdict
// onto the response is strongest exactly where the consequence is largest. The
// two SDK outputs differ in the three fields this handler drops, so the two
// responses must be indistinguishable.
//
// A MISS IS NOT AN ERROR. Both arms return nil -- the option's `failure`
// rows fire and the run goes on.
func TestSpeak_BeatenAndMissed_AnswerIdentically(t *testing.T) {
	respond := func(t *testing.T, out *sdk.SpeakOutput) *sessionpb.SpeakResponse {
		t.Helper()
		ctrl := gomock.NewController(t)
		mgr := sessionv1alpha1mock.NewMockManager(ctrl)
		mgr.EXPECT().Speak(gomock.Any(), gomock.Any()).Return(out, nil)

		h := &Handler{
			manager: mgr,
			characters: charactersOf(ctrl, map[string]rosterCharacter{
				"char-alice": {owner: "alice", name: "Alice", class: "bard", race: "human"},
			}),
		}
		ctx := auth.WithPlayerID(context.Background(), "alice")
		resp, err := h.Speak(ctx, &sessionpb.SpeakRequest{
			Session: "sess-1", Member: "char-alice", Target: &sessionpb.SpeakRequest_MemberId{MemberId: "front-goblin"}, OptionId: "talk-it-round",
		})
		require.NoError(t, err, "a failed appeal is an outcome, not an error")
		return resp
	}

	ack := sdk.SaveReport{Written: []string{"encounter"}}
	delivered := sdk.DeliveryReport{}

	beaten := respond(t, &sdk.SpeakOutput{
		Beaten: true, Total: 13, DC: 10, Target: "front-goblin", Saved: ack, Delivery: delivered,
	})
	missed := respond(t, &sdk.SpeakOutput{
		Beaten: false, Total: 4, DC: 10, Target: "front-goblin", Saved: ack, Delivery: delivered,
	})

	require.Equal(t, beaten.GetPaused(), missed.GetPaused())
	require.Equal(t, beaten.Roll, missed.Roll)
	require.Equal(t, beaten.GetSaved().GetWritten(), missed.GetSaved().GetWritten())
	require.Equal(t, beaten.GetDelivery().GetFailed(), missed.GetDelivery().GetFailed())
}

// TestSpeak_Paused_CarriesTheRollAndNothingElse pins the offer window: the
// one moment of this verb that belongs to the caller alone. The d20 is on the
// table and the member is being asked whether to spend a held offer, so the
// roll crosses -- and the DC deliberately does not, so a player weighing the
// offer cannot read off whether it would close the gap.
func TestSpeak_Paused_CarriesTheRollAndNothingElse(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	roll := 9
	mgr.EXPECT().Speak(gomock.Any(), gomock.Any()).Return(&sdk.SpeakOutput{
		Paused:   true,
		Total:    11,
		Roll:     &roll,
		Target:   "front-goblin",
		Saved:    sdk.SaveReport{Written: []string{"encounter"}},
		Delivery: sdk.DeliveryReport{},
	}, nil)

	h := &Handler{
		manager: mgr,
		characters: charactersOf(ctrl, map[string]rosterCharacter{
			"char-alice": {owner: "alice", name: "Alice", class: "bard", race: "human"},
		}),
	}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.Speak(ctx, &sessionpb.SpeakRequest{
		Session: "sess-1", Member: "char-alice", Target: &sessionpb.SpeakRequest_MemberId{MemberId: "front-goblin"}, OptionId: "talk-it-round",
	})
	require.NoError(t, err)
	require.True(t, resp.GetPaused())
	require.NotNil(t, resp.Roll, "the member cannot weigh an offer against a die they cannot see")
	require.Equal(t, int32(9), resp.GetRoll())
}

// TestSpeak_NoRoll_StaysAbsent is speakRollToProto's own law: a nil *int
// is nil on the wire, never a zero that reads as a rolled 0. Every settled
// attempt takes this path.
func TestSpeak_NoRoll_StaysAbsent(t *testing.T) {
	require.Nil(t, speakRollToProto(nil))

	roll := 0
	converted := speakRollToProto(&roll)
	require.NotNil(t, converted, "a rolled zero is still a rolled value, and the SDK never omits one it has")
	require.Equal(t, int32(0), *converted)
}

// TestSpeak_NotYourTurn_IsAWorldRefusal exercises the refusal a player meets
// on the FIGHT clock. It is worth pinning precisely because the world clock no
// longer produces it (R3): a front room has no turn to be out of, so this
// sentinel now means "somebody else's turn" and nothing else, and it is still
// FAILED_PRECONDITION rather than a caller defect.
func TestSpeak_NotYourTurn_IsAWorldRefusal(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Speak(gomock.Any(), gomock.Any()).Return(nil, sdk.ErrNotYourTurn)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Speak(ctx, &sessionpb.SpeakRequest{
		Session: "sess-1", Member: "char-1", Target: &sessionpb.SpeakRequest_MemberId{MemberId: "front-goblin"}, OptionId: "talk-it-round",
	})
	requireCode(t, err, codes.FailedPrecondition)
}

// TestSpeak_Unwitnessed_IsAWorldRefusalNotAnInternalError pins the refusal
// the verb owns: the creature cannot see who is talking to it.
//
// FAILED_PRECONDITION, AND THE CODE IS THE POINT. The call is well-formed and
// names two real members; it is the sightline between them that refuses it.
// Internal would tell a client this server broke, and a dock showing
// "something went wrong" for an ordinary tactical fact is how a player stops
// trusting the panel.
//
// SEPARATE FROM ErrOutOfReach ON PURPOSE, and the reasoning survives the move
// from threatening to talking unchanged: reach is a distance and this is a
// sightline, so the remedy is to be SEEN -- step out from behind the pillar,
// open the door -- and never to step closer.
func TestSpeak_Unwitnessed_IsAWorldRefusalNotAnInternalError(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Speak(gomock.Any(), gomock.Any()).Return(nil, sdk.ErrUnwitnessed)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Speak(ctx, &sessionpb.SpeakRequest{
		Session: "sess-1", Member: "char-1", Target: &sessionpb.SpeakRequest_MemberId{MemberId: "front-goblin"}, OptionId: "talk-it-round",
	})
	requireCode(t, err, codes.FailedPrecondition)
}

// TestSpeak_ResponseCarriesNoVerdict makes the ruling mechanical at the wire
// type itself rather than only at today's handler code: the message has exactly
// four fields, and none of them is beaten, total or dc. A `beaten` field
// appearing on this message should fail this test and force the deliberate
// conversation the ruling requires, not slip in as a one-line addition.
func TestSpeak_ResponseCarriesNoVerdict(t *testing.T) {
	typ := reflect.TypeOf(sessionpb.SpeakResponse{})
	var exported []string
	for i := 0; i < typ.NumField(); i++ {
		if f := typ.Field(i); f.IsExported() {
			exported = append(exported, f.Name)
		}
	}
	require.ElementsMatch(t, []string{"Paused", "Roll", "Saved", "Delivery"}, exported)
}

// TestSpeak_ToAProp_ReachesTheSDKOnThePropArm pins the other target arm: the
// prop id crosses as SpeakInput.Prop and Target stays empty (rpg-project#569).
func TestSpeak_ToAProp_ReachesTheSDKOnThePropArm(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Speak(gomock.Any(), &sdk.SpeakInput{
		Session: "sess-1", Member: "char-alice", Prop: "altar", OptionID: "pray",
	}).Return(&sdk.SpeakOutput{Prop: "altar", OptionID: "pray"}, nil)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Speak(ctx, &sessionpb.SpeakRequest{
		Session: "sess-1", Member: "char-alice",
		Target: &sessionpb.SpeakRequest_PropId{PropId: "altar"}, OptionId: "pray",
	})
	require.NoError(t, err)
}

// TestSpeak_NoTarget_IsRefusedBeforeTheSDK pins the exactly-one rule's
// handler half: the oneof forbids both arms, and an absent or empty arm is the
// request's own defect, refused as InvalidArgument without calling the SDK.
func TestSpeak_NoTarget_IsRefusedBeforeTheSDK(t *testing.T) {
	for name, target := range map[string]any{
		"absent":       nil,
		"empty member": &sessionpb.SpeakRequest_MemberId{MemberId: ""},
		"empty prop":   &sessionpb.SpeakRequest_PropId{PropId: ""},
	} {
		t.Run(name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mgr := sessionv1alpha1mock.NewMockManager(ctrl)
			h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
			ctx := auth.WithPlayerID(context.Background(), "alice")
			req := &sessionpb.SpeakRequest{Session: "sess-1", Member: "char-1", OptionId: "talk-it-round"}
			switch arm := target.(type) {
			case *sessionpb.SpeakRequest_MemberId:
				req.Target = arm
			case *sessionpb.SpeakRequest_PropId:
				req.Target = arm
			}
			_, err := h.Speak(ctx, req)
			requireCode(t, err, codes.InvalidArgument)
		})
	}
}

// TestSpeak_NoOption_IsAWorldRefusal pins ErrNoOption's code: a target in
// plain sight that offers this speaker no such line is the world's present
// state refusing a well-formed call, not a missing member.
func TestSpeak_NoOption_IsAWorldRefusal(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Speak(gomock.Any(), gomock.Any()).Return(nil, sdk.ErrNoOption)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Speak(ctx, &sessionpb.SpeakRequest{
		Session: "sess-1", Member: "char-1",
		Target: &sessionpb.SpeakRequest_MemberId{MemberId: "front-goblin-2"}, OptionId: "lean-on-it",
	})
	requireCode(t, err, codes.FailedPrecondition)
}
