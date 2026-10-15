package sessionv1alpha1

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	sessionv1alpha1mock "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1/mock"
)

func TestGetView_Unauthenticated_Errors(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := &Handler{characters: anyMemberOwnedBy(ctrl, "alice")}
	_, err := h.GetView(context.Background(), &sessionpb.GetViewRequest{})
	requireCode(t, err, codes.Unauthenticated)
}

func TestGetView_HappyPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	viewInput := &sdk.ViewInput{Session: "sess-1", Member: "char-1"}
	mgr.EXPECT().View(gomock.Any(), viewInput).Return(
		&sdk.ViewOutput{Sightings: []sdk.Sighting{{Subject: "goblin-1", Passage: sdk.PassageStandable}},
			Areas: []sdk.SightArea{{ID: "area-1", Name: "Fog", Ref: "spell:fog", RadiusFeet: 15}}}, nil,
	)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	resp, err := h.GetView(ctx, &sessionpb.GetViewRequest{Session: "sess-1", Member: "char-1"})
	require.NoError(t, err)
	require.Len(t, resp.GetSightings(), 1)
	require.Equal(t, sessionpb.Passage_PASSAGE_STANDABLE, resp.GetSightings()[0].GetPassage())
	require.Len(t, resp.GetAreas(), 1)
	require.Equal(t, "area-1", resp.GetAreas()[0].GetId())
	require.Equal(t, "spell:fog", resp.GetAreas()[0].GetSourceRef())
}

func TestGetView_ManagerError_TranslatesViaErrorTable(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().View(gomock.Any(), gomock.Any()).Return(nil, sdk.ErrNoMember)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	_, err := h.GetView(ctx, &sessionpb.GetViewRequest{Session: "sess-1", Member: "bogus"})
	requireCode(t, err, codes.NotFound)
}

// TestGetView_CarriesVisibleCellsInOrder: seen-now is the session's answer
// copied cell for cell, in the order given (rpg-project#586 R1).
func TestGetView_CarriesVisibleCellsInOrder(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	cells := []spatial.Position{{X: 2, Y: -1}, {X: 0, Y: 0}, {X: 1, Y: 0}}
	mgr.EXPECT().View(gomock.Any(), &sdk.ViewInput{Session: "sess-1", Member: "char-1"}).Return(&sdk.ViewOutput{VisibleCells: cells}, nil)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")}
	resp, err := h.GetView(auth.WithPlayerID(context.Background(), "alice"), &sessionpb.GetViewRequest{Session: "sess-1", Member: "char-1"})
	require.NoError(t, err)
	require.Len(t, resp.GetVisibleCells(), 3)
	for i, c := range cells {
		require.Equal(t, c.X, resp.GetVisibleCells()[i].GetX(), "cell %d", i)
		require.Equal(t, c.Y, resp.GetVisibleCells()[i].GetY(), "cell %d", i)
	}
}

// TestGetView_TwoSeatsReadTheirOwnCells: each request reaches the SDK for the
// named seat alone, and each response carries that seat's answer -- never a
// union of the two.
func TestGetView_TwoSeatsReadTheirOwnCells(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().View(gomock.Any(), &sdk.ViewInput{Session: "sess-1", Member: "alice"}).Return(
		&sdk.ViewOutput{VisibleCells: []spatial.Position{{X: 0, Y: 0}}}, nil)
	mgr.EXPECT().View(gomock.Any(), &sdk.ViewInput{Session: "sess-1", Member: "bob"}).Return(
		&sdk.ViewOutput{VisibleCells: []spatial.Position{{X: 3, Y: 0}, {X: 4, Y: 0}}}, nil)

	h := &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "player")}
	ctx := auth.WithPlayerID(context.Background(), "player")
	alice, err := h.GetView(ctx, &sessionpb.GetViewRequest{Session: "sess-1", Member: "alice"})
	require.NoError(t, err)
	bob, err := h.GetView(ctx, &sessionpb.GetViewRequest{Session: "sess-1", Member: "bob"})
	require.NoError(t, err)
	require.Len(t, alice.GetVisibleCells(), 1)
	require.Equal(t, 0.0, alice.GetVisibleCells()[0].GetX())
	require.Len(t, bob.GetVisibleCells(), 2)
	require.Equal(t, 3.0, bob.GetVisibleCells()[0].GetX())
}

// TestGetView_RefusesAnotherMembersSeat: a caller naming a member it does not
// control is refused before the SDK is asked, so no cells can leak.
func TestGetView_RefusesAnotherMembersSeat(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)

	h := &Handler{manager: mgr, characters: ownedCharacterRepo(ctrl, "bob", "player-bob")}
	resp, err := h.GetView(auth.WithPlayerID(context.Background(), "player-alice"), &sessionpb.GetViewRequest{Session: "sess-1", Member: "bob"})
	requireCode(t, err, codes.PermissionDenied)
	require.Nil(t, resp)
}
