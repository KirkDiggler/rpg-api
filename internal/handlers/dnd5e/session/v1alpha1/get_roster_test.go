package sessionv1alpha1

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	customizationpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/customization/v1alpha1"
	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	sessionv1alpha1mock "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1/mock"
)

func TestGetRoster_DelegatesOnceAndProjectsSDKOutput(t *testing.T) {
	ctrl := gomock.NewController(t)
	manager := sessionv1alpha1mock.NewMockManager(ctrl)
	color := uint32(0)
	roughness := float32(0)
	manager.EXPECT().Roster(gomock.Any(), &sdk.RosterInput{
		Session: "sess-1",
		Player:  "player-1",
	}).Return(&sdk.RosterOutput{Members: []sdk.PublicMember{
		{
			ID: "char-1", Kind: sdk.KindPlayer, Name: "Alice",
			ClassRef: "fighter", RaceRef: "human",
			Customization: sdk.Customization{
				Hair: &sdk.HairCustomization{
					Scalp:      &sdk.StyleSelection{Kind: sdk.StyleSelectionStyle, StyleRef: "provider:hair:38"},
					FacialHair: &sdk.StyleSelection{Kind: sdk.StyleSelectionNone},
					ColorSRGB:  &color,
					Roughness:  &roughness,
				},
				Outfit: &sdk.OutfitCustomization{
					PrimaryColorSRGB:   &color,
					SecondaryColorSRGB: &color,
				},
			},
		},
		{ID: "char-2", Kind: sdk.KindPlayer, Name: "Bob", Customization: sdk.Customization{}},
		{ID: "skeleton-1", Kind: sdk.KindMonster, Name: "Skeleton", MonsterRef: "dnd5e:monsters:skeleton", Customization: sdk.Customization{}},
	}}, nil)
	h := &Handler{manager: manager}

	got, err := h.GetRoster(auth.WithPlayerID(context.Background(), "player-1"), &sessionpb.GetRosterRequest{Session: "sess-1"})

	require.NoError(t, err)
	want := &sessionpb.GetRosterResponse{Members: []*sessionpb.PublicMemberInfo{
		{
			Id: "char-1", Kind: sessionpb.MemberKind_MEMBER_KIND_PLAYER, Name: "Alice",
			ClassRef: "fighter", RaceRef: "human", Customization: &sessionpb.Customization{
				Hair: &customizationpb.HairCustomization{
					Scalp:      styleRef("provider:hair:38"),
					FacialHair: noneStyle(),
					ColorSrgb:  proto.Uint32(0),
					Roughness:  proto.Float32(0),
				},
				Outfit: &customizationpb.OutfitCustomization{
					PrimaryColorSrgb:   proto.Uint32(0),
					SecondaryColorSrgb: proto.Uint32(0),
				},
			},
		},
		{Id: "char-2", Kind: sessionpb.MemberKind_MEMBER_KIND_PLAYER, Name: "Bob", Customization: &sessionpb.Customization{}},
		{Id: "skeleton-1", Kind: sessionpb.MemberKind_MEMBER_KIND_MONSTER, Name: "Skeleton", MonsterRef: "dnd5e:monsters:skeleton", Customization: &sessionpb.Customization{}},
	}}
	require.True(t, proto.Equal(want, got), "got %s", got)
}

func TestGetRoster_RefusesUnauthenticatedAndMissingSessionBeforeSDK(t *testing.T) {
	for _, tc := range []struct {
		name string
		ctx  context.Context
		req  *sessionpb.GetRosterRequest
		code codes.Code
	}{
		{name: "unauthenticated", ctx: context.Background(), req: &sessionpb.GetRosterRequest{Session: "sess-1"}, code: codes.Unauthenticated},
		{name: "missing session", ctx: auth.WithPlayerID(context.Background(), "player-1"), req: &sessionpb.GetRosterRequest{}, code: codes.InvalidArgument},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			manager := sessionv1alpha1mock.NewMockManager(ctrl)
			h := &Handler{manager: manager}

			_, err := h.GetRoster(tc.ctx, tc.req)

			requireCode(t, err, tc.code)
		})
	}
}

func TestGetRoster_TranslatesSDKErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		code codes.Code
	}{
		{name: "no session", err: sdk.ErrNoSession, code: codes.NotFound},
		{name: "no encounter", err: sdk.ErrNoEncounter, code: codes.NotFound},
		{name: "no character", err: sdk.ErrNoCharacter, code: codes.NotFound},
		{name: "bad character", err: sdk.ErrBadCharacter, code: codes.Internal},
		{name: "not seated", err: sdk.ErrNotSeated, code: codes.PermissionDenied},
		{name: "repository failure", err: errors.New("redis unavailable"), code: codes.Internal},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			manager := sessionv1alpha1mock.NewMockManager(ctrl)
			manager.EXPECT().Roster(gomock.Any(), &sdk.RosterInput{Session: "sess-1", Player: "player-1"}).Return(nil, tc.err)
			h := &Handler{manager: manager}

			_, err := h.GetRoster(auth.WithPlayerID(context.Background(), "player-1"), &sessionpb.GetRosterRequest{Session: "sess-1"})

			requireCode(t, err, tc.code)
		})
	}
}

func styleRef(ref string) *customizationpb.StyleSelection {
	return &customizationpb.StyleSelection{Selection: &customizationpb.StyleSelection_StyleRef{StyleRef: ref}}
}

func noneStyle() *customizationpb.StyleSelection {
	return &customizationpb.StyleSelection{Selection: &customizationpb.StyleSelection_None{None: &emptypb.Empty{}}}
}
