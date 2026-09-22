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

func TestPersuade_Unauthenticated_Errors(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := &Handler{characters: anyMemberOwnedBy(ctrl, "alice")}
	_, err := h.Persuade(context.Background(), &sessionpb.PersuadeRequest{
		Session: "sess-1", Member: "char-1", Target: "goblin-2",
	})
	requireCode(t, err, codes.Unauthenticated)
}

func TestPersuade_EmptyMember_IsRefused(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Persuade(ctx, &sessionpb.PersuadeRequest{Session: "sess-1", Target: "goblin-2"})
	requireCode(t, err, codes.InvalidArgument)
}

// TestPersuade_ForeignMember_IsRefusedBeforeTheSDK is Intimidate's sharpest
// gate, and it has to be made again rather than inherited: Persuade SPENDS
// SOMEBODY'S ACTION on the turn clock, and the SDK charges it before it rolls,
// so a client naming another player's character would burn that player's turn.
// The manager mock expects nothing -- the point is the call never reaches the
// SDK at all.
func TestPersuade_ForeignMember_IsRefusedBeforeTheSDK(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)

	h := &Handler{manager: mgr, characters: ownedCharacterRepo(ctrl, "char-bob", "bob")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Persuade(ctx, &sessionpb.PersuadeRequest{
		Session: "sess-1", Member: "char-bob", Target: "goblin-2",
	})
	requireCode(t, err, codes.PermissionDenied)
}

// TestPersuade_HappyPath_ReturnsTheAckAndNotTheRoll pins both halves of this
// handler's law at once: session/member/target reach the SDK verbatim, and a
// settled attempt answers with the ack alone. The SDK output here carries a
// beaten check, a total and a DC -- the numbers a client would most want --
// and NONE of them may appear on the response, because the `persuaded` beat is
// the only account of this roll and the actor reads it there like everybody
// else.
func TestPersuade_HappyPath_ReturnsTheAckAndNotTheRoll(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Persuade(gomock.Any(), &sdk.PersuadeInput{
		Session: "sess-1", Member: "char-alice", Target: "front-goblin",
	}).Return(&sdk.PersuadeOutput{
		Beaten:   true,
		Total:    13,
		DC:       10,
		Target:   "front-goblin",
		Seq:      7,
		Saved:    sdk.SaveReport{Written: []string{"encounter"}},
		Delivery: sdk.DeliveryReport{Events: 2},
	}, nil)

	h := &Handler{
		manager: mgr,
		characters: charactersOf(ctrl, map[string]rosterCharacter{
			"char-alice": {owner: "alice", name: "Alice", class: "bard", race: "human"},
		}),
	}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.Persuade(ctx, &sessionpb.PersuadeRequest{
		Session: "sess-1", Member: "char-alice", Target: "front-goblin",
	})
	require.NoError(t, err)
	require.Equal(t, []string{"encounter"}, resp.GetSaved().GetWritten())
	require.Equal(t, int32(2), resp.GetDelivery().GetEvents(),
		"the check beat and the creature's answer are two events, and the report counts both")
	require.False(t, resp.GetPaused(), "a settled attempt is not a question")
	require.Nil(t, resp.Roll, "the settled roll rides the beat, not the response")
}

// TestPersuade_BeatenAndMissed_AnswerIdentically is the ruling made mechanical,
// and it matters MORE here than it did for the threat: a failed appeal is where
// the goblin's bad directions come from, so the temptation to leak the verdict
// onto the response is strongest exactly where the consequence is largest. The
// two SDK outputs differ in the three fields this handler drops, so the two
// responses must be indistinguishable.
//
// A MISS IS NOT AN ERROR. Both arms return nil -- the author's
// `persuade_failed` table fires and the run goes on.
func TestPersuade_BeatenAndMissed_AnswerIdentically(t *testing.T) {
	respond := func(t *testing.T, out *sdk.PersuadeOutput) *sessionpb.PersuadeResponse {
		t.Helper()
		ctrl := gomock.NewController(t)
		mgr := sessionv1alpha1mock.NewMockManager(ctrl)
		mgr.EXPECT().Persuade(gomock.Any(), gomock.Any()).Return(out, nil)

		h := &Handler{
			manager: mgr,
			characters: charactersOf(ctrl, map[string]rosterCharacter{
				"char-alice": {owner: "alice", name: "Alice", class: "bard", race: "human"},
			}),
		}
		ctx := auth.WithPlayerID(context.Background(), "alice")
		resp, err := h.Persuade(ctx, &sessionpb.PersuadeRequest{
			Session: "sess-1", Member: "char-alice", Target: "front-goblin",
		})
		require.NoError(t, err, "a failed appeal is an outcome, not an error")
		return resp
	}

	ack := sdk.SaveReport{Written: []string{"encounter"}}
	delivered := sdk.DeliveryReport{Events: 2}

	beaten := respond(t, &sdk.PersuadeOutput{
		Beaten: true, Total: 13, DC: 10, Target: "front-goblin", Saved: ack, Delivery: delivered,
	})
	missed := respond(t, &sdk.PersuadeOutput{
		Beaten: false, Total: 4, DC: 10, Target: "front-goblin", Saved: ack, Delivery: delivered,
	})

	require.Equal(t, beaten.GetPaused(), missed.GetPaused())
	require.Equal(t, beaten.Roll, missed.Roll)
	require.Equal(t, beaten.GetSaved().GetWritten(), missed.GetSaved().GetWritten())
	require.Equal(t, beaten.GetDelivery().GetEvents(), missed.GetDelivery().GetEvents())
}

// TestPersuade_Paused_CarriesTheRollAndNothingElse pins the offer window: the
// one moment of this verb that belongs to the caller alone. The d20 is on the
// table and the member is being asked whether to spend a held offer, so the
// roll crosses -- and the DC deliberately does not, so a player weighing the
// offer cannot read off whether it would close the gap.
func TestPersuade_Paused_CarriesTheRollAndNothingElse(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	roll := 9
	mgr.EXPECT().Persuade(gomock.Any(), gomock.Any()).Return(&sdk.PersuadeOutput{
		Paused:   true,
		Total:    11,
		Roll:     &roll,
		Target:   "front-goblin",
		Saved:    sdk.SaveReport{Written: []string{"encounter"}},
		Delivery: sdk.DeliveryReport{Events: 1},
	}, nil)

	h := &Handler{
		manager: mgr,
		characters: charactersOf(ctrl, map[string]rosterCharacter{
			"char-alice": {owner: "alice", name: "Alice", class: "bard", race: "human"},
		}),
	}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.Persuade(ctx, &sessionpb.PersuadeRequest{
		Session: "sess-1", Member: "char-alice", Target: "front-goblin",
	})
	require.NoError(t, err)
	require.True(t, resp.GetPaused())
	require.NotNil(t, resp.Roll, "the member cannot weigh an offer against a die they cannot see")
	require.Equal(t, int32(9), resp.GetRoll())
}

// TestPersuade_NoRoll_StaysAbsent is persuadeRollToProto's own law: a nil *int
// is nil on the wire, never a zero that reads as a rolled 0. Every settled
// attempt takes this path.
func TestPersuade_NoRoll_StaysAbsent(t *testing.T) {
	require.Nil(t, persuadeRollToProto(nil))

	roll := 0
	converted := persuadeRollToProto(&roll)
	require.NotNil(t, converted, "a rolled zero is still a rolled value, and the SDK never omits one it has")
	require.Equal(t, int32(0), *converted)
}

// TestPersuade_NotYourTurn_IsAWorldRefusal exercises the refusal a player meets
// on the FIGHT clock. It is worth pinning precisely because the world clock no
// longer produces it (R3): a front room has no turn to be out of, so this
// sentinel now means "somebody else's turn" and nothing else, and it is still
// FAILED_PRECONDITION rather than a caller defect.
func TestPersuade_NotYourTurn_IsAWorldRefusal(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Persuade(gomock.Any(), gomock.Any()).Return(nil, sdk.ErrNotYourTurn)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Persuade(ctx, &sessionpb.PersuadeRequest{
		Session: "sess-1", Member: "char-1", Target: "front-goblin",
	})
	requireCode(t, err, codes.FailedPrecondition)
}

// TestPersuade_Unwitnessed_IsAWorldRefusalNotAnInternalError pins the refusal
// both social verbs own: the creature cannot see who is talking to it.
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
func TestPersuade_Unwitnessed_IsAWorldRefusalNotAnInternalError(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Persuade(gomock.Any(), gomock.Any()).Return(nil, sdk.ErrUnwitnessed)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Persuade(ctx, &sessionpb.PersuadeRequest{
		Session: "sess-1", Member: "char-1", Target: "front-goblin",
	})
	requireCode(t, err, codes.FailedPrecondition)
}

// TestPersuade_ResponseCarriesNoVerdict makes the ruling mechanical at the wire
// type itself rather than only at today's handler code: the message has exactly
// four fields, and none of them is beaten, total or dc. A `beaten` field
// appearing on this message should fail this test and force the deliberate
// conversation the ruling requires, not slip in as a one-line addition.
func TestPersuade_ResponseCarriesNoVerdict(t *testing.T) {
	typ := reflect.TypeOf(sessionpb.PersuadeResponse{})
	var exported []string
	for i := 0; i < typ.NumField(); i++ {
		if f := typ.Field(i); f.IsExported() {
			exported = append(exported, f.Name)
		}
	}
	require.ElementsMatch(t, []string{"Paused", "Roll", "Saved", "Delivery"}, exported)
}

// TestPersuadeAndIntimidateResponsesAreTheSameShape pins the mirroring itself,
// which is the claim the wire made when it spent five field numbers rather than
// sharing one social message: a client that learned one shape has learned the
// other. If a field is ever added to one and not the other, the promise is
// broken and this is where it shows.
func TestPersuadeAndIntimidateResponsesAreTheSameShape(t *testing.T) {
	fields := func(v any) []string {
		typ := reflect.TypeOf(v)
		var out []string
		for i := 0; i < typ.NumField(); i++ {
			if f := typ.Field(i); f.IsExported() {
				out = append(out, f.Name+" "+f.Type.String())
			}
		}
		return out
	}
	require.Equal(t,
		fields(sessionpb.IntimidateResponse{}),
		fields(sessionpb.PersuadeResponse{}),
		"the two social responses mirror field for field, name and type")
}
