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

// TestAfford_HappyPath_ProjectsDeclarations pins the turn-clock shape,
// Affordable=false and its Shortfall carried verbatim -- the currency-naming
// text is the SDK's own wording (ADR-0042), and this layer must not paraphrase
// it away.
func TestAfford_HappyPath_ProjectsDeclarations(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Afford(gomock.Any(), &sdk.AffordInput{Session: "sess-1", Member: "char-1"}).Return(&sdk.AffordOutput{
		Clock: sdk.ClockTurn,
		Declarations: []sdk.Declaration{
			{Verb: sdk.VerbAttack, Slot: sdk.SlotAction, Affordable: false, Shortfall: "action: 1 needed, 0 left"},
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
	require.False(t, decl.GetAffordable())
	require.Equal(t, "action: 1 needed, 0 left", decl.GetShortfall())
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

// TestAfford_PerTargetAttackDeclarations pins rpg-project#249 §6 (Kirk): one
// ATTACK declaration per candidate target in reach, each carrying its own
// Target -- the client's "enemies in reach" highlight is read straight off
// this list, never computed client-side.
func TestAfford_PerTargetAttackDeclarations(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	goblin, skeleton := "goblin-1", "skeleton-1"
	mgr.EXPECT().Afford(gomock.Any(), &sdk.AffordInput{Session: "sess-1", Member: "char-1"}).Return(&sdk.AffordOutput{
		Clock: sdk.ClockTurn,
		Declarations: []sdk.Declaration{
			{Verb: sdk.VerbAttack, Slot: sdk.SlotAction, Affordable: true, Target: &goblin},
			{Verb: sdk.VerbAttack, Slot: sdk.SlotAction, Affordable: true, Target: &skeleton},
		},
	}, nil)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.Afford(ctx, &sessionpb.AffordRequest{Session: "sess-1", Member: "char-1"})
	require.NoError(t, err)
	require.Len(t, resp.GetDeclarations(), 2)
	require.Equal(t, "goblin-1", resp.GetDeclarations()[0].GetTarget())
	require.Equal(t, "skeleton-1", resp.GetDeclarations()[1].GetTarget())
	for _, d := range resp.GetDeclarations() {
		require.True(t, d.GetAffordable())
		require.Nil(t, d.Why, "affordable true carries no why")
	}
}

// TestAfford_NoTargetInReach_SingleDeclarationNoTarget pins the other half
// of §6: when no candidate is in reach, Afford still answers once -- a
// single ATTACK declaration, unaffordable, Why.Reason NO_TARGET_IN_REACH,
// and no target at all (never a target whose id happens to be empty).
func TestAfford_NoTargetInReach_SingleDeclarationNoTarget(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Afford(gomock.Any(), &sdk.AffordInput{Session: "sess-1", Member: "char-1"}).Return(&sdk.AffordOutput{
		Clock: sdk.ClockTurn,
		Declarations: []sdk.Declaration{
			{
				Verb: sdk.VerbAttack, Affordable: false,
				Why: &sdk.Shortfall{Reason: sdk.ShortfallNoTargetInReach, Text: "no target in reach"},
			},
		},
	}, nil)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.Afford(ctx, &sessionpb.AffordRequest{Session: "sess-1", Member: "char-1"})
	require.NoError(t, err)
	require.Len(t, resp.GetDeclarations(), 1)
	decl := resp.GetDeclarations()[0]
	require.False(t, decl.GetAffordable())
	require.Nil(t, decl.Target)
	require.NotNil(t, decl.Why)
	require.Equal(t, sessionpb.ShortfallReason_SHORTFALL_REASON_NO_TARGET_IN_REACH, decl.Why.GetReason())
}

// TestAfford_MoveDeclaration pins VerbMove's own shape beside the ATTACK
// ones above: no Target (Move never carries one), correctly distinguished
// from VerbAttack on the wire. Remaining is deliberately NOT asserted here
// -- it lands with the separate, still-WIP Move-on-clock wave
// (rpg-toolkit#1169, branch feat/session-move-clock), out of scope for this
// PR so the two waves stay on their own branches.
func TestAfford_MoveDeclaration(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Afford(gomock.Any(), &sdk.AffordInput{Session: "sess-1", Member: "char-1"}).Return(&sdk.AffordOutput{
		Clock: sdk.ClockTurn,
		Declarations: []sdk.Declaration{
			{Verb: sdk.VerbMove, Affordable: true},
		},
	}, nil)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.Afford(ctx, &sessionpb.AffordRequest{Session: "sess-1", Member: "char-1"})
	require.NoError(t, err)
	require.Len(t, resp.GetDeclarations(), 1)
	decl := resp.GetDeclarations()[0]
	require.Equal(t, sessionpb.Verb_VERB_MOVE, decl.GetVerb())
	require.True(t, decl.GetAffordable())
	require.Nil(t, decl.Target, "Move never carries a target")
}
