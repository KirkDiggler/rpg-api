package sessionv1alpha1

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	sessionv1alpha1mock "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1/mock"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
)

// ownedBy answers each character's owner from the map; an absent id is owned
// by somebody else.
func ownedBy(ctrl *gomock.Controller, owners map[string]string) characterrepo.Repository {
	repo := charactermock.NewMockRepository(ctrl)
	repo.EXPECT().Get(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in characterrepo.GetInput) (*characterrepo.GetOutput, error) {
			owner, ok := owners[in.ID]
			if !ok {
				owner = "somebody-else"
			}
			return &characterrepo.GetOutput{
				Character: &entities.Character{Data: &tkcharacter.Data{ID: in.ID, PlayerID: owner}},
			}, nil
		},
	).AnyTimes()
	return repo
}

func seatedAs(mgr *sessionv1alpha1mock.MockManager, player, member string) {
	mgr.EXPECT().Roster(gomock.Any(), &sdk.RosterInput{Session: "sess-1", Member: member, Player: player}).
		Return(&sdk.RosterOutput{Members: []sdk.PublicMember{{ID: member, Kind: sdk.KindPlayer}}}, nil)
}

func TestRest_PartyRestHandsTheSDKEveryResterAndAnswersOnlyTheReports(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	seatedAs(mgr, "alice", "char-a")
	mgr.EXPECT().Rest(gomock.Any(), &sdk.RestInput{
		Session: "sess-1", Kind: sdk.RestShort,
		Resters: []sdk.Rester{{Member: "char-a", HitDice: 2}, {Member: "char-b", HitDice: 0}},
	}).Return(&sdk.RestOutput{
		Rested: []sdk.RestedMember{{Member: "char-a", HitPointsRestored: 9}},
		Saved:  sdk.SaveReport{Written: []string{"character:char-a"}},
	}, nil)

	h := &Handler{manager: mgr, characters: ownedBy(ctrl, map[string]string{"char-a": "alice"})}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.Rest(ctx, &sessionpb.RestRequest{
		Session: "sess-1", Kind: sessionpb.RestKind_REST_KIND_SHORT,
		Resters: []*sessionpb.RestingMember{{Member: "char-a", HitDice: 2}, {Member: "char-b"}},
	})
	require.NoError(t, err)
	require.Equal(t, []string{"character:char-a"}, resp.GetSaved().GetWritten())
}

func TestRest_TheSingularFieldsAreOneRester(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	seatedAs(mgr, "alice", "char-a")
	mgr.EXPECT().Rest(gomock.Any(), &sdk.RestInput{
		Session: "sess-1", Kind: sdk.RestShort,
		Resters: []sdk.Rester{{Member: "char-a", HitDice: 1}},
	}).Return(&sdk.RestOutput{}, nil)

	h := &Handler{manager: mgr, characters: ownedBy(ctrl, map[string]string{"char-a": "alice"})}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	//nolint:staticcheck // the deprecated singular fields are the case under test
	_, err := h.Rest(ctx, &sessionpb.RestRequest{
		Session: "sess-1", Kind: sessionpb.RestKind_REST_KIND_SHORT, Member: "char-a", HitDice: 1,
	})
	require.NoError(t, err)
}

// TestRest_ACallerControllingNoResterIsRefused: naming only other players'
// characters is refused before the SDK is reached (no Rest expectation).
func TestRest_ACallerControllingNoResterIsRefused(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	h := &Handler{manager: mgr, characters: ownedBy(ctrl, map[string]string{"char-a": "alice"})}
	ctx := auth.WithPlayerID(context.Background(), "mallory")
	_, err := h.Rest(ctx, &sessionpb.RestRequest{
		Session: "sess-1", Kind: sessionpb.RestKind_REST_KIND_SHORT,
		Resters: []*sessionpb.RestingMember{{Member: "char-a"}},
	})
	require.Error(t, err)
}

// TestRest_ACallerNotSeatedInTheSessionIsRefused: owning the rester is not
// enough; the caller must be seated in this session as that member.
func TestRest_ACallerNotSeatedInTheSessionIsRefused(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Roster(gomock.Any(), gomock.Any()).Return(nil, sdk.ErrNotSeated)
	h := &Handler{manager: mgr, characters: ownedBy(ctrl, map[string]string{"char-a": "alice"})}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.Rest(ctx, &sessionpb.RestRequest{
		Session: "sess-1", Kind: sessionpb.RestKind_REST_KIND_SHORT,
		Resters: []*sessionpb.RestingMember{{Member: "char-a"}},
	})
	requireCode(t, err, codes.PermissionDenied)
}

func TestRest_RequestShapeRefusals(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := &Handler{manager: sessionv1alpha1mock.NewMockManager(ctrl), characters: anyMemberOwnedBy(ctrl, "alice")}
	_, err := h.Rest(context.Background(), &sessionpb.RestRequest{})
	requireCode(t, err, codes.Unauthenticated)

	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err = h.Rest(ctx, &sessionpb.RestRequest{Session: "sess-1", Kind: sessionpb.RestKind_REST_KIND_SHORT})
	requireCode(t, err, codes.InvalidArgument)
	_, err = h.Rest(ctx, &sessionpb.RestRequest{
		Session: "sess-1", Resters: []*sessionpb.RestingMember{{Member: "char-a"}},
	})
	requireCode(t, err, codes.InvalidArgument)
}

func TestRest_TheSDKsRefusalIsTranslated(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code codes.Code
	}{
		{sdk.ErrInBubble, codes.FailedPrecondition},
		{sdk.ErrBadRest, codes.InvalidArgument},
		{sdk.ErrDuplicateMember, codes.InvalidArgument},
		{sdk.ErrNoCharacter, codes.NotFound},
	} {
		t.Run(tc.err.Error(), func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mgr := sessionv1alpha1mock.NewMockManager(ctrl)
			seatedAs(mgr, "alice", "char-a")
			mgr.EXPECT().Rest(gomock.Any(), gomock.Any()).Return(nil, tc.err)
			h := &Handler{manager: mgr, characters: ownedBy(ctrl, map[string]string{"char-a": "alice"})}
			ctx := auth.WithPlayerID(context.Background(), "alice")
			_, err := h.Rest(ctx, &sessionpb.RestRequest{
				Session: "sess-1", Kind: sessionpb.RestKind_REST_KIND_SHORT,
				Resters: []*sessionpb.RestingMember{{Member: "char-a"}},
			})
			requireCode(t, err, tc.code)
		})
	}
}

func TestTheEquipBeatReachesTheWire(t *testing.T) {
	for change, want := range map[sdk.EquipmentChange]sessionpb.EquipmentChange{
		sdk.EquipmentDrawn:  sessionpb.EquipmentChange_EQUIPMENT_CHANGE_DRAW,
		sdk.EquipmentStowed: sessionpb.EquipmentChange_EQUIPMENT_CHANGE_STOW,
	} {
		evt, err := eventToProto(sdk.Event{Seq: 4, Kind: sdk.EventEquipmentChanged, Body: sdk.EquipmentChangedBody{
			Member: "char-a", Slot: "main_hand", Item: "dnd5e:weapons:longsword", Change: change,
		}})
		require.NoError(t, err)
		require.Equal(t, sessionpb.EventKind_EVENT_KIND_EQUIPMENT_CHANGED, evt.GetKind())
		require.Equal(t, &sessionpb.EquipmentChanged{
			Member: "char-a", Slot: "main_hand", Item: "dnd5e:weapons:longsword", Change: want,
		}, evt.GetEquipmentChanged())
	}
	_, err := eventToProto(sdk.Event{Kind: sdk.EventEquipmentChanged, Body: sdk.EquipmentChangedBody{Change: "juggle"}})
	require.Error(t, err, "a change word this build cannot spell fails the beat")
}

func TestTheRestBeatReachesTheWire(t *testing.T) {
	evt, err := eventToProto(sdk.Event{Seq: 9, Kind: sdk.EventRested, Body: sdk.RestedBody{
		Member: "char-a", Kind: string(sdk.RestShort), HitPointsRestored: 7, HitPoints: 19,
		HitDiceSpent: 2, HitDiceRemaining: 1,
		ResourcesRefilled: []string{"dnd5e:features:second_wind"},
	}})
	require.NoError(t, err)
	require.Equal(t, sessionpb.EventKind_EVENT_KIND_RESTED, evt.GetKind())
	require.Equal(t, &sessionpb.Rested{
		Member: "char-a", Kind: sessionpb.RestKind_REST_KIND_SHORT, HitPointsRestored: 7, HitPoints: 19,
		HitDiceSpent: 2, HitDiceRemaining: 1, ResourcesRefilled: []string{"dnd5e:features:second_wind"},
	}, evt.GetRested())
	require.Nil(t, evt.GetRested().GetConcentrationEnded(), "nothing ended, nothing sent")
	require.Nil(t, evt.GetRested().GetEnded())
	_, err = eventToProto(sdk.Event{Kind: sdk.EventRested, Body: sdk.RestedBody{Kind: "nap"}})
	require.Error(t, err, "a rest kind this build cannot spell fails the beat")
}

// TestTheRestBeatCarriesWhatTheRestEnded: the rest's own beat is the one
// account of what it ended — each concentration the rester held and every
// condition the rest took off anyone. A bard's Bless held on an ally ends with
// the rest, and the ally's Blessed removal reaches `ended` with the ally as its
// target, after the bard's own removals and exactly once.
func TestTheRestBeatCarriesWhatTheRestEnded(t *testing.T) {
	body := sdk.RestedBody{Member: "bard", Kind: string(sdk.RestShort)}
	body.ConcentrationEnded = []sdk.RestConcentrationEnded{{
		ConcentrationEndedBody: sdk.ConcentrationEndedBody{
			Caster: "bard", Spell: sdk.SpellRef{Ref: "dnd5e:spells:bless", Name: "Bless"}, Reason: "rest",
		},
		Removed: []sdk.ConditionRemovedBody{
			{Target: "fighter", Ref: "dnd5e:conditions:blessed", Name: "Blessed", Reason: "rest", SourceID: "bard"},
		},
	}}
	body.Ended = []sdk.ConditionRemovedBody{
		{Target: "bard", Ref: "dnd5e:conditions:prone", Name: "Prone", Reason: "rest"},
	}
	evt, err := eventToProto(sdk.Event{Seq: 3, Kind: sdk.EventRested, Body: body})
	require.NoError(t, err)
	require.Equal(t, []*sessionpb.ConcentrationEnded{{
		Caster: "bard", Spell: &sessionpb.SpellRef{Ref: "dnd5e:spells:bless", Name: "Bless"}, Reason: "rest",
	}}, evt.GetRested().GetConcentrationEnded())
	require.Equal(t, []*sessionpb.ConditionRemoved{
		{Target: "bard", Ref: "dnd5e:conditions:prone", Name: "Prone", Reason: "rest"},
		{Target: "fighter", Ref: "dnd5e:conditions:blessed", Name: "Blessed", Reason: "rest", SourceId: "bard"},
	}, evt.GetRested().GetEnded(), "the rester's own removals, then the hold's, each once")
}
