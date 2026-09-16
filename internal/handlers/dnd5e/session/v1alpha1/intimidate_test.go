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

func TestIntimidate_Unauthenticated_Errors(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := &Handler{characters: anyMemberOwnedBy(ctrl, "alice")}
	_, err := h.Intimidate(context.Background(), &sessionpb.IntimidateRequest{
		Session: "sess-1", Member: "char-1", Target: "goblin-2",
	})
	requireCode(t, err, codes.Unauthenticated)
}

func TestIntimidate_EmptyMember_IsRefused(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Intimidate(ctx, &sessionpb.IntimidateRequest{Session: "sess-1", Target: "goblin-2"})
	requireCode(t, err, codes.InvalidArgument)
}

// TestIntimidate_ForeignMember_IsRefusedBeforeTheSDK is the sharpest gate on
// a verb that SPENDS SOMEBODY'S ACTION: Intimidate costs the standard action
// and the SDK charges it before it rolls, so a client naming another
// player's character would burn that player's turn. The manager mock expects
// nothing -- the point is the call never reaches the SDK at all.
func TestIntimidate_ForeignMember_IsRefusedBeforeTheSDK(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)

	h := &Handler{manager: mgr, characters: ownedCharacterRepo(ctrl, "char-bob", "bob")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Intimidate(ctx, &sessionpb.IntimidateRequest{
		Session: "sess-1", Member: "char-bob", Target: "goblin-2",
	})
	requireCode(t, err, codes.PermissionDenied)
}

// TestIntimidate_HappyPath_ReturnsTheAckAndNotTheRoll pins BOTH halves of
// this handler's law at once: session/member/target reach the SDK verbatim,
// and a settled attempt answers with the ack alone. The SDK output here
// carries a beaten check, a total and a DC -- the numbers a client would
// most want -- and NONE of them may appear on the response, because the
// `intimidated` beat is the only account of this roll and the actor reads it
// there like everybody else.
func TestIntimidate_HappyPath_ReturnsTheAckAndNotTheRoll(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Intimidate(gomock.Any(), &sdk.IntimidateInput{
		Session: "sess-1", Member: "char-alice", Target: "goblin-2",
	}).Return(&sdk.IntimidateOutput{
		Beaten:   true,
		Total:    14,
		DC:       9,
		Target:   "goblin-2",
		Seq:      7,
		Saved:    sdk.SaveReport{Written: []string{"encounter"}},
		Delivery: sdk.DeliveryReport{Events: 1},
	}, nil)

	h := &Handler{
		manager: mgr,
		characters: charactersOf(ctrl, map[string]rosterCharacter{
			"char-alice": {owner: "alice", name: "Alice", class: "fighter", race: "human"},
		}),
	}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.Intimidate(ctx, &sessionpb.IntimidateRequest{
		Session: "sess-1", Member: "char-alice", Target: "goblin-2",
	})
	require.NoError(t, err)
	require.Equal(t, []string{"encounter"}, resp.GetSaved().GetWritten())
	require.Equal(t, int32(1), resp.GetDelivery().GetEvents())
	require.False(t, resp.GetPaused(), "a settled attempt is not a question")
	require.Nil(t, resp.Roll, "the settled roll rides the beat, not the response")
}

// TestIntimidate_BeatenAndMissed_AnswerIdentically is the ruling made
// mechanical. A beaten threat and a missed one differ on the SDK output in
// exactly the three fields this handler drops, so the two responses must be
// indistinguishable: there is no field left for a future edit to branch the
// outcome onto by accident.
//
// A MISS IS NOT AN ERROR. Both arms return nil -- the goblin shoots on its
// next turn and the member may try again with a new action.
func TestIntimidate_BeatenAndMissed_AnswerIdentically(t *testing.T) {
	respond := func(t *testing.T, out *sdk.IntimidateOutput) *sessionpb.IntimidateResponse {
		t.Helper()
		ctrl := gomock.NewController(t)
		mgr := sessionv1alpha1mock.NewMockManager(ctrl)
		mgr.EXPECT().Intimidate(gomock.Any(), gomock.Any()).Return(out, nil)

		h := &Handler{
			manager: mgr,
			characters: charactersOf(ctrl, map[string]rosterCharacter{
				"char-alice": {owner: "alice", name: "Alice", class: "fighter", race: "human"},
			}),
		}
		ctx := auth.WithPlayerID(context.Background(), "alice")
		resp, err := h.Intimidate(ctx, &sessionpb.IntimidateRequest{
			Session: "sess-1", Member: "char-alice", Target: "goblin-2",
		})
		require.NoError(t, err, "a missed threat is an outcome, not an error")
		return resp
	}

	ack := sdk.SaveReport{Written: []string{"encounter"}}
	delivered := sdk.DeliveryReport{Events: 1}

	beaten := respond(t, &sdk.IntimidateOutput{
		Beaten: true, Total: 14, DC: 9, Target: "goblin-2", Saved: ack, Delivery: delivered,
	})
	missed := respond(t, &sdk.IntimidateOutput{
		Beaten: false, Total: 4, DC: 9, Target: "goblin-2", Saved: ack, Delivery: delivered,
	})

	require.Equal(t, beaten.GetPaused(), missed.GetPaused())
	require.Equal(t, beaten.Roll, missed.Roll)
	require.Equal(t, beaten.GetSaved().GetWritten(), missed.GetSaved().GetWritten())
	require.Equal(t, beaten.GetDelivery().GetEvents(), missed.GetDelivery().GetEvents())
}

// TestIntimidate_Paused_CarriesTheRollAndNothingElse pins the offer window:
// the one moment of this verb that belongs to the caller alone. The d20 is
// on the table and the member is being asked whether to spend a held offer,
// so the roll crosses -- and the DC deliberately does not, so a player
// weighing the offer cannot read off whether it would close the gap.
func TestIntimidate_Paused_CarriesTheRollAndNothingElse(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	roll := 11
	mgr.EXPECT().Intimidate(gomock.Any(), gomock.Any()).Return(&sdk.IntimidateOutput{
		Paused:   true,
		Total:    13,
		Roll:     &roll,
		Target:   "goblin-2",
		Saved:    sdk.SaveReport{Written: []string{"encounter"}},
		Delivery: sdk.DeliveryReport{Events: 1},
	}, nil)

	h := &Handler{
		manager: mgr,
		characters: charactersOf(ctrl, map[string]rosterCharacter{
			"char-alice": {owner: "alice", name: "Alice", class: "fighter", race: "human"},
		}),
	}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.Intimidate(ctx, &sessionpb.IntimidateRequest{
		Session: "sess-1", Member: "char-alice", Target: "goblin-2",
	})
	require.NoError(t, err)
	require.True(t, resp.GetPaused())
	require.NotNil(t, resp.Roll, "the member cannot weigh an offer against a die they cannot see")
	require.Equal(t, int32(11), resp.GetRoll())
}

// TestIntimidate_NoRoll_StaysAbsent is intimidateRollToProto's own law: a nil
// *int is nil on the wire, never a zero that reads as "they rolled a 1... no,
// a 0". Every settled attempt takes this path.
func TestIntimidate_NoRoll_StaysAbsent(t *testing.T) {
	require.Nil(t, intimidateRollToProto(nil))

	roll := 0
	converted := intimidateRollToProto(&roll)
	require.NotNil(t, converted, "a rolled zero is still a rolled value, and the SDK never omits one it has")
	require.Equal(t, int32(0), *converted)
}

// TestIntimidate_ManagerError_TranslatesViaErrorTable exercises the refusal a
// player meets most: the threat is priced as an action and refused off their
// own turn, which is FAILED_PRECONDITION here rather than a caller defect.
func TestIntimidate_ManagerError_TranslatesViaErrorTable(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Intimidate(gomock.Any(), gomock.Any()).Return(nil, sdk.ErrNotYourTurn)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Intimidate(ctx, &sessionpb.IntimidateRequest{
		Session: "sess-1", Member: "char-1", Target: "goblin-2",
	})
	requireCode(t, err, codes.FailedPrecondition)
}

// TestIntimidate_Unwitnessed_IsAWorldRefusalNotAnInternalError pins the
// refusal this verb owns: the target cannot see who is threatening them.
//
// FAILED_PRECONDITION, AND THE CODE IS THE POINT. The threat is well-formed
// and names two real members; it is the sightline between them that refuses
// it. Internal would tell a client this server broke, and a dock that showed
// "something went wrong" for an ordinary tactical fact is how a player stops
// trusting the panel.
//
// SEPARATE FROM ErrOutOfReach ON PURPOSE. Reach is a distance and this is a
// sightline: a threat has no distance cap at all, so the remedy is to be SEEN
// -- step out from behind the pillar, open the door -- and never to step
// closer. The toolkit split this onto its own sentinel for that reason
// (rpg-toolkit#1790) and this test is what stops the two being folded back
// together at this seam.
func TestIntimidate_Unwitnessed_IsAWorldRefusalNotAnInternalError(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Intimidate(gomock.Any(), gomock.Any()).Return(nil, sdk.ErrUnwitnessed)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Intimidate(ctx, &sessionpb.IntimidateRequest{
		Session: "sess-1", Member: "char-1", Target: "goblin-2",
	})
	requireCode(t, err, codes.FailedPrecondition)
}

// TestIntimidate_ResponseCarriesNoVerdict makes the ruling mechanical at the
// wire type itself rather than only at today's handler code: the message has
// exactly four fields, and none of them is beaten, total or dc. A `beaten`
// field appearing on this message should fail this test and force the
// deliberate conversation the ruling requires, not slip in as a one-line
// addition -- the same guard TestSearch_ResponseCarriesNoOutcome keeps for
// the secrecy law one verb over.
func TestIntimidate_ResponseCarriesNoVerdict(t *testing.T) {
	typ := reflect.TypeOf(sessionpb.IntimidateResponse{})
	var exported []string
	for i := 0; i < typ.NumField(); i++ {
		if f := typ.Field(i); f.IsExported() {
			exported = append(exported, f.Name)
		}
	}
	require.ElementsMatch(t, []string{"Paused", "Roll", "Saved", "Delivery"}, exported)
}
