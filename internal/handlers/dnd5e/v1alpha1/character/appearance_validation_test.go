package character

import (
	"context"
	"math"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"

	customizationpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/customization/v1alpha1"
	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterorchestrator "github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/orchestrators/character/mock"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
)

func TestUpdateAppearance_ValidFullRequestMutatesAndReturnsStoredDraft(t *testing.T) {
	ctrl := gomock.NewController(t)
	service := charactermock.NewMockService(ctrl)
	handler, err := NewHandler(&HandlerConfig{CharacterService: service})
	require.NoError(t, err)

	const draftID = "draft-hair"
	request := validUpdateAppearanceRequest(draftID)
	appearance := &entities.Appearance{
		Hair: &entities.HairCustomization{
			Scalp: &entities.StyleSelection{
				Kind:     entities.StyleSelectionKindStyle,
				StyleRef: "modular-fantasy-hero:hair:38",
			},
			FacialHair: &entities.StyleSelection{Kind: entities.StyleSelectionKindNone},
			ColorSRGB:  proto.Uint32(0x5A3825),
			Roughness:  proto.Float32(0.72),
		},
	}

	gomock.InOrder(
		service.EXPECT().
			SetAppearance(gomock.Any(), &characterorchestrator.SetAppearanceInput{
				DraftID:    draftID,
				Appearance: appearance,
			}).
			Return(&characterorchestrator.SetAppearanceOutput{Appearance: appearance}, nil),
		service.EXPECT().
			GetDraft(gomock.Any(), &characterorchestrator.GetDraftInput{DraftID: draftID}).
			Return(&characterorchestrator.GetDraftOutput{
				Draft: &entities.CharacterDraft{
					Data:       &toolkitchar.DraftData{ID: draftID},
					Appearance: appearance,
				},
			}, nil),
	)

	response, err := handler.UpdateAppearance(context.Background(), request)
	require.NoError(t, err)
	require.Equal(t, draftID, response.GetDraft().GetId())
	require.Equal(t, "modular-fantasy-hero:hair:38", response.GetDraft().GetAppearance().GetHair().GetScalp().GetStyleRef())
	require.NotNil(t, response.GetDraft().GetAppearance().GetHair().GetFacialHair().GetNone())
	require.Equal(t, uint32(0x5A3825), response.GetDraft().GetAppearance().GetHair().GetColorSrgb())
	require.InDelta(t, 0.72, response.GetDraft().GetAppearance().GetHair().GetRoughness(), 0.000001)
}

func TestUpdateAppearance_InvalidRequestsRefuseBeforeMutation(t *testing.T) {
	malformedSelection := validUpdateAppearanceRequest("draft-hair")
	malformedSelection.Appearance.Hair.Scalp = &customizationpb.StyleSelection{}

	emptyStyleRef := validUpdateAppearanceRequest("draft-hair")
	emptyStyleRef.Appearance.Hair.Scalp = &customizationpb.StyleSelection{
		Selection: &customizationpb.StyleSelection_StyleRef{},
	}

	oversizedStyleRef := validUpdateAppearanceRequest("draft-hair")
	oversizedStyleRef.Appearance.Hair.Scalp = &customizationpb.StyleSelection{
		Selection: &customizationpb.StyleSelection_StyleRef{StyleRef: strings.Repeat("é", 129)},
	}

	invalidColor := validUpdateAppearanceRequest("draft-hair")
	invalidColor.Appearance.Hair.ColorSrgb = proto.Uint32(0x1000000)

	negativeRoughness := validUpdateAppearanceRequest("draft-hair")
	negativeRoughness.Appearance.Hair.Roughness = proto.Float32(-0.0001)

	overOneRoughness := validUpdateAppearanceRequest("draft-hair")
	overOneRoughness.Appearance.Hair.Roughness = proto.Float32(1.0001)

	nanRoughness := validUpdateAppearanceRequest("draft-hair")
	nanRoughness.Appearance.Hair.Roughness = proto.Float32(float32(math.NaN()))

	positiveInfinityRoughness := validUpdateAppearanceRequest("draft-hair")
	positiveInfinityRoughness.Appearance.Hair.Roughness = proto.Float32(float32(math.Inf(1)))

	negativeInfinityRoughness := validUpdateAppearanceRequest("draft-hair")
	negativeInfinityRoughness.Appearance.Hair.Roughness = proto.Float32(float32(math.Inf(-1)))

	tests := []struct {
		name    string
		request *dnd5ev1alpha1.UpdateAppearanceRequest
		message string
	}{
		{name: "nil request", request: nil, message: "request is required"},
		{name: "empty draft id", request: validUpdateAppearanceRequest(""), message: "draft_id is required"},
		{name: "nil appearance", request: &dnd5ev1alpha1.UpdateAppearanceRequest{DraftId: "draft-hair"}, message: "appearance is required"},
		{name: "present selection without oneof", request: malformedSelection, message: "appearance.hair.scalp.selection is required"},
		{name: "empty style ref", request: emptyStyleRef, message: "appearance.hair.scalp.style_ref is required"},
		{name: "style ref over 256 UTF-8 bytes", request: oversizedStyleRef, message: "appearance.hair.scalp.style_ref must be at most 256 bytes"},
		{name: "color outside RGB24", request: invalidColor, message: "appearance.hair.color_srgb must be between 0 and 0xFFFFFF"},
		{name: "negative roughness", request: negativeRoughness, message: "appearance.hair.roughness must be finite and between 0 and 1"},
		{name: "roughness over one", request: overOneRoughness, message: "appearance.hair.roughness must be finite and between 0 and 1"},
		{name: "NaN roughness", request: nanRoughness, message: "appearance.hair.roughness must be finite and between 0 and 1"},
		{name: "positive infinity roughness", request: positiveInfinityRoughness, message: "appearance.hair.roughness must be finite and between 0 and 1"},
		{name: "negative infinity roughness", request: negativeInfinityRoughness, message: "appearance.hair.roughness must be finite and between 0 and 1"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			service := charactermock.NewMockService(ctrl)
			handler, err := NewHandler(&HandlerConfig{CharacterService: service})
			require.NoError(t, err)

			response, err := handler.UpdateAppearance(context.Background(), test.request)
			require.Nil(t, response)
			require.Error(t, err)
			require.Equal(t, codes.InvalidArgument, status.Code(err))
			require.Equal(t, test.message, status.Convert(err).Message())
		})
	}
}

func validUpdateAppearanceRequest(draftID string) *dnd5ev1alpha1.UpdateAppearanceRequest {
	return &dnd5ev1alpha1.UpdateAppearanceRequest{
		DraftId: draftID,
		Appearance: &dnd5ev1alpha1.Appearance{
			Hair: &customizationpb.HairCustomization{
				Scalp: &customizationpb.StyleSelection{
					Selection: &customizationpb.StyleSelection_StyleRef{
						StyleRef: "modular-fantasy-hero:hair:38",
					},
				},
				FacialHair: &customizationpb.StyleSelection{
					Selection: &customizationpb.StyleSelection_None{None: &emptypb.Empty{}},
				},
				ColorSrgb: proto.Uint32(0x5A3825),
				Roughness: proto.Float32(0.72),
			},
		},
	}
}
