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
	sessionv1alpha1mock "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1/mock"
)

func TestAfford_Unauthenticated_Errors(t *testing.T) {
	h := &Handler{}
	_, err := h.Afford(context.Background(), &sessionpb.AffordRequest{})
	requireCode(t, err, codes.Unauthenticated)
}

// TestAfford_HappyPath_ProjectsNestedDeclaration pins the manager response as
// the sole source of declaration identity, attack identity, target shape, and
// independent target availability. The API does not flatten or re-rule it.
func TestAfford_HappyPath_ProjectsNestedDeclaration(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Afford(gomock.Any(), &sdk.AffordInput{Session: "sess-1", Member: "char-1"}).Return(&sdk.AffordOutput{
		Clock: sdk.ClockTurn,
		Declarations: []sdk.Declaration{
			{
				Verb: sdk.VerbAttack, Slot: sdk.SlotAction, Available: false,
				Why:        &sdk.Shortfall{Reason: sdk.ShortfallNoBudget, Currency: sdk.CurrencyAction, Needed: 1, Text: "action: 1 needed, 0 left"},
				ID:         "decl-attack-1",
				Attack:     &sdk.AttackRef{Ref: "dnd5e:weapons:longsword", Name: "Longsword", DamageType: sdk.DamageSlashing},
				TargetKind: sdk.TargetMember,
				Candidates: []sdk.TargetCandidate{
					{Member: "goblin-1", Available: true},
					{Member: "skeleton-1", Available: false, Why: &sdk.Shortfall{Reason: sdk.ShortfallTargetOutOfReach, Text: "target out of reach"}},
				},
			},
		},
	}, nil)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.Afford(ctx, &sessionpb.AffordRequest{Session: "sess-1", Member: "char-1"})
	require.NoError(t, err)
	require.Equal(t, sessionpb.ClockKind_CLOCK_KIND_TURN, resp.GetClock())
	require.Len(t, resp.GetDeclarations(), 1)
	decl := resp.GetDeclarations()[0]
	require.Equal(t, sessionpb.Verb_VERB_ATTACK, decl.GetVerb())
	require.Equal(t, sessionpb.Slot_SLOT_ACTION, decl.GetSlot())
	require.False(t, decl.GetAvailable())
	require.Equal(t, "action: 1 needed, 0 left", decl.GetWhy().GetText())
	require.Equal(t, "decl-attack-1", decl.GetId())
	require.Equal(t, "dnd5e:weapons:longsword", decl.GetAttack().GetRef())
	require.Equal(t, sessionpb.TargetKind_TARGET_KIND_MEMBER, decl.GetTargetKind())
	require.Len(t, decl.GetCandidates(), 2)
	require.True(t, decl.GetCandidates()[0].GetAvailable())
	require.Nil(t, decl.GetCandidates()[0].GetWhy())
	require.False(t, decl.GetCandidates()[1].GetAvailable())
	require.Equal(t, sessionpb.ShortfallReason_SHORTFALL_REASON_TARGET_OUT_OF_REACH, decl.GetCandidates()[1].GetWhy().GetReason())
}

// TestAfford_WorldClock_DeclarationsEmpty pins the other half of ADR-0042:
// on the world clock the economy does not apply, and that arrives as an
// EMPTY repeated field, never a null one -- a client reading "declarations":
// [] must not mistake it for "unknown" or "ask again".
func TestAfford_WorldClock_DeclarationsEmpty(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Afford(gomock.Any(), &sdk.AffordInput{Session: "sess-1", Member: "char-1"}).Return(&sdk.AffordOutput{
		Clock:        sdk.ClockWorld,
		Declarations: []sdk.Declaration{},
	}, nil)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.Afford(ctx, &sessionpb.AffordRequest{Session: "sess-1", Member: "char-1"})
	require.NoError(t, err)
	require.Equal(t, sessionpb.ClockKind_CLOCK_KIND_WORLD, resp.GetClock())
	require.NotNil(t, resp.GetDeclarations())
	require.Empty(t, resp.GetDeclarations())
}

func TestAfford_ManagerError_TranslatesViaErrorTable(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want codes.Code
	}{
		{name: "ErrNoMember", err: sdk.ErrNoMember, want: codes.NotFound},
		{name: "ErrNoMemberID", err: sdk.ErrNoMemberID, want: codes.InvalidArgument},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			mgr := sessionv1alpha1mock.NewMockManager(ctrl)
			mgr.EXPECT().Afford(gomock.Any(), gomock.Any()).Return(nil, tt.err)

			h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
			ctx := auth.WithPlayerID(context.Background(), "alice")
			_, err := h.Afford(ctx, &sessionpb.AffordRequest{Session: "sess-1", Member: "char-1"})
			requireCode(t, err, tt.want)
		})
	}
}

// TestAfford_NoTargetInReach_KeepsCandidateRows pins the nested contract:
// the declaration-level NO_TARGET_IN_REACH answer does not discard ruled
// candidates, and each unavailable target keeps its own TARGET_OUT_OF_REACH.
func TestAfford_NoTargetInReach_KeepsCandidateRows(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Afford(gomock.Any(), &sdk.AffordInput{Session: "sess-1", Member: "char-1"}).Return(&sdk.AffordOutput{
		Clock: sdk.ClockTurn,
		Declarations: []sdk.Declaration{
			{
				Verb: sdk.VerbAttack, Available: false, ID: "decl-attack-1", TargetKind: sdk.TargetMember,
				Attack: &sdk.AttackRef{Ref: "dnd5e:weapons:longsword", Name: "Longsword", DamageType: sdk.DamageSlashing},
				Why:    &sdk.Shortfall{Reason: sdk.ShortfallNoTargetInReach, Text: "no target in reach"},
				Candidates: []sdk.TargetCandidate{
					{Member: "skeleton-1", Available: false, Why: &sdk.Shortfall{Reason: sdk.ShortfallTargetOutOfReach, Text: "target out of reach"}},
				},
			},
		},
	}, nil)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.Afford(ctx, &sessionpb.AffordRequest{Session: "sess-1", Member: "char-1"})
	require.NoError(t, err)
	require.Len(t, resp.GetDeclarations(), 1)
	decl := resp.GetDeclarations()[0]
	require.False(t, decl.GetAvailable())
	require.Len(t, decl.GetCandidates(), 1, "NO_TARGET_IN_REACH does not remove ruled candidates")
	require.Equal(t, "skeleton-1", decl.GetCandidates()[0].GetMember())
	require.Equal(t, sessionpb.ShortfallReason_SHORTFALL_REASON_TARGET_OUT_OF_REACH, decl.GetCandidates()[0].GetWhy().GetReason())
	require.NotNil(t, decl.Why)
	require.Equal(t, sessionpb.ShortfallReason_SHORTFALL_REASON_NO_TARGET_IN_REACH, decl.Why.GetReason())
}

func TestAfford_MoveAndEndTurnDeclarations(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	remaining := 15
	mgr.EXPECT().Afford(gomock.Any(), &sdk.AffordInput{Session: "sess-1", Member: "char-1"}).Return(&sdk.AffordOutput{
		Clock: sdk.ClockTurn,
		Declarations: []sdk.Declaration{
			{Verb: sdk.VerbMove, Slot: sdk.SlotNone, Available: true, Remaining: &remaining, ID: "decl-move-1", TargetKind: sdk.TargetPath, Candidates: []sdk.TargetCandidate{}},
			{Verb: sdk.VerbEndTurn, Slot: sdk.SlotNone, Available: true, ID: "decl-end-1", TargetKind: sdk.TargetNone, Candidates: []sdk.TargetCandidate{}},
		},
	}, nil)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.Afford(ctx, &sessionpb.AffordRequest{Session: "sess-1", Member: "char-1"})
	require.NoError(t, err)
	require.Len(t, resp.GetDeclarations(), 2)
	move := resp.GetDeclarations()[0]
	require.Equal(t, sessionpb.Verb_VERB_MOVE, move.GetVerb())
	require.NotNil(t, move.Remaining)
	require.Equal(t, int32(15), move.GetRemaining())
	require.Equal(t, sessionpb.TargetKind_TARGET_KIND_PATH, move.GetTargetKind())
	require.NotNil(t, move.Candidates)
	require.Empty(t, move.Candidates)
	end := resp.GetDeclarations()[1]
	require.Equal(t, sessionpb.Verb_VERB_END_TURN, end.GetVerb())
	require.Nil(t, end.Remaining)
	require.Equal(t, sessionpb.TargetKind_TARGET_KIND_NONE, end.GetTargetKind())
}
