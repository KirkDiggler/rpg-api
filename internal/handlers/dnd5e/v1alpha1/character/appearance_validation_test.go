package character

import (
	"context"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"

	customizationpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/customization/v1alpha1"
	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	customizationconverter "github.com/KirkDiggler/rpg-api/internal/converters/customization"
	characterorchestrator "github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/orchestrators/character/mock"
	"github.com/KirkDiggler/rpg-toolkit/rpgerr"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/customization"
)

func TestUpdateAppearance_UnauthenticatedRefusesBeforeService(t *testing.T) {
	ctrl := gomock.NewController(t)
	service := charactermock.NewMockService(ctrl)
	handler, err := NewHandler(&HandlerConfig{CharacterService: service})
	require.NoError(t, err)

	response, err := handler.UpdateAppearance(context.Background(), &dnd5ev1alpha1.UpdateAppearanceRequest{
		DraftId:    "draft-appearance",
		Appearance: &dnd5ev1alpha1.Appearance{},
	})

	require.Nil(t, response)
	require.Equal(t, codes.Unauthenticated, status.Code(err))
	require.Equal(t, "player not authenticated", status.Convert(err).Message())
}

func TestUpdateAppearance_DelegatesCompleteAppearanceAndReturnsServiceDraft(t *testing.T) {
	ctrl := gomock.NewController(t)
	service := charactermock.NewMockService(ctrl)
	handler, err := NewHandler(&HandlerConfig{CharacterService: service})
	require.NoError(t, err)

	const draftID = "draft-appearance"
	requestAppearance := &dnd5ev1alpha1.Appearance{
		Hair: &customizationpb.HairCustomization{
			Scalp: &customizationpb.StyleSelection{
				Selection: &customizationpb.StyleSelection_StyleRef{StyleRef: "unknown:hair:ok"},
			},
			FacialHair: &customizationpb.StyleSelection{
				Selection: &customizationpb.StyleSelection_None{None: &emptypb.Empty{}},
			},
			ColorSrgb: proto.Uint32(0),
			Roughness: proto.Float32(0),
		},
		Outfit: &customizationpb.OutfitCustomization{
			PrimaryColorSrgb:   proto.Uint32(0x102030),
			SecondaryColorSrgb: proto.Uint32(0x405060),
		},
	}
	expectedAppearance := customizationconverter.ProtoToToolkit(requestAppearance)
	storedDraft := &toolkitchar.DraftData{ID: draftID, Appearance: expectedAppearance}

	service.EXPECT().SetAppearance(gomock.Any(), &characterorchestrator.SetAppearanceInput{
		DraftID:    draftID,
		PlayerID:   "player-1",
		Appearance: expectedAppearance,
	}).Return(&characterorchestrator.SetAppearanceOutput{Draft: storedDraft}, nil)

	response, err := handler.UpdateAppearance(auth.WithPlayerID(context.Background(), "player-1"), &dnd5ev1alpha1.UpdateAppearanceRequest{
		DraftId:    draftID,
		Appearance: requestAppearance,
	})
	require.NoError(t, err)
	require.NotNil(t, response.GetDraft())
	require.True(t, proto.Equal(requestAppearance, response.GetDraft().GetAppearance()))
}

func TestUpdateAppearance_DelegatesMalformedSemanticsToToolkit(t *testing.T) {
	ctrl := gomock.NewController(t)
	service := charactermock.NewMockService(ctrl)
	handler, err := NewHandler(&HandlerConfig{CharacterService: service})
	require.NoError(t, err)

	const draftID = "draft-malformed"
	requestAppearance := &dnd5ev1alpha1.Appearance{
		Hair: &customizationpb.HairCustomization{
			Scalp:     &customizationpb.StyleSelection{},
			ColorSrgb: proto.Uint32(0x1000000),
			Roughness: proto.Float32(float32(math.NaN())),
		},
		Outfit: &customizationpb.OutfitCustomization{
			PrimaryColorSrgb: proto.Uint32(0x1000000),
		},
	}

	service.EXPECT().SetAppearance(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, input *characterorchestrator.SetAppearanceInput) (*characterorchestrator.SetAppearanceOutput, error) {
			require.Equal(t, draftID, input.DraftID)
			require.Equal(t, "player-1", input.PlayerID)
			require.Equal(t, customization.StyleSelectionKind(""), input.Appearance.Hair.Scalp.Kind)
			require.Equal(t, uint32(0x1000000), *input.Appearance.Hair.ColorSRGB)
			require.True(t, math.IsNaN(float64(*input.Appearance.Hair.Roughness)))
			require.Equal(t, uint32(0x1000000), *input.Appearance.Outfit.PrimaryColorSRGB)
			return &characterorchestrator.SetAppearanceOutput{Draft: &toolkitchar.DraftData{
				ID:         draftID,
				Appearance: input.Appearance,
			}}, nil
		},
	)

	response, err := handler.UpdateAppearance(auth.WithPlayerID(context.Background(), "player-1"), &dnd5ev1alpha1.UpdateAppearanceRequest{
		DraftId:    draftID,
		Appearance: requestAppearance,
	})
	require.NoError(t, err)
	require.Equal(t, draftID, response.GetDraft().GetId())
	require.NotNil(t, response.GetDraft().GetAppearance().GetHair().GetScalp())
	require.Nil(t, response.GetDraft().GetAppearance().GetHair().GetScalp().GetSelection())
	require.Equal(t, uint32(0x1000000), response.GetDraft().GetAppearance().GetHair().GetColorSrgb())
	require.True(t, math.IsNaN(float64(response.GetDraft().GetAppearance().GetHair().GetRoughness())))
	require.Equal(t, uint32(0x1000000), response.GetDraft().GetAppearance().GetOutfit().GetPrimaryColorSrgb())
}

func TestUpdateAppearance_UsesReturnedDraftWithoutRefetch(t *testing.T) {
	ctrl := gomock.NewController(t)
	service := charactermock.NewMockService(ctrl)
	handler, err := NewHandler(&HandlerConfig{CharacterService: service})
	require.NoError(t, err)

	const draftID = "draft-no-refetch"
	request := &dnd5ev1alpha1.UpdateAppearanceRequest{
		DraftId:    draftID,
		Appearance: &dnd5ev1alpha1.Appearance{},
	}
	service.EXPECT().SetAppearance(gomock.Any(), gomock.Any()).Return(&characterorchestrator.SetAppearanceOutput{
		Draft: &toolkitchar.DraftData{ID: draftID, Name: "stored-name", Appearance: &customization.Appearance{}},
	}, nil)

	response, err := handler.UpdateAppearance(auth.WithPlayerID(context.Background(), "player-1"), request)
	require.NoError(t, err)
	require.Equal(t, "stored-name", response.GetDraft().GetName())
}

func TestUpdateAppearance_RejectsOnlyTransportEnvelopeFailures(t *testing.T) {
	tests := []struct {
		name string
		req  *dnd5ev1alpha1.UpdateAppearanceRequest
		msg  string
	}{
		{name: "nil request", req: nil, msg: "request is required"},
		{name: "missing draft", req: &dnd5ev1alpha1.UpdateAppearanceRequest{Appearance: &dnd5ev1alpha1.Appearance{}}, msg: "draft_id is required"},
		{name: "missing appearance", req: &dnd5ev1alpha1.UpdateAppearanceRequest{DraftId: "draft-1"}, msg: "appearance is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			service := charactermock.NewMockService(ctrl)
			handler, err := NewHandler(&HandlerConfig{CharacterService: service})
			require.NoError(t, err)

			response, err := handler.UpdateAppearance(auth.WithPlayerID(context.Background(), "player-1"), tt.req)
			require.Nil(t, response)
			require.Equal(t, codes.InvalidArgument, status.Code(err))
			require.Equal(t, tt.msg, status.Convert(err).Message())
		})
	}
}

func TestUpdateAppearance_NotFoundUsesLegacyMessage(t *testing.T) {
	ctrl := gomock.NewController(t)
	service := charactermock.NewMockService(ctrl)
	handler, err := NewHandler(&HandlerConfig{CharacterService: service})
	require.NoError(t, err)

	service.EXPECT().SetAppearance(gomock.Any(), gomock.Any()).Return(nil,
		apierr.NotFound("draft storage record missing"))

	response, err := handler.UpdateAppearance(auth.WithPlayerID(context.Background(), "player-1"), &dnd5ev1alpha1.UpdateAppearanceRequest{
		DraftId:    "draft-missing",
		Appearance: &dnd5ev1alpha1.Appearance{},
	})
	require.Nil(t, response)
	require.Equal(t, codes.NotFound, status.Code(err))
	require.Equal(t, "draft not found", status.Convert(err).Message())
}

func TestUpdateAppearance_TranslatesToolkitErrors(t *testing.T) {
	ctrl := gomock.NewController(t)
	service := charactermock.NewMockService(ctrl)
	handler, err := NewHandler(&HandlerConfig{CharacterService: service})
	require.NoError(t, err)

	service.EXPECT().SetAppearance(gomock.Any(), gomock.Any()).Return(nil,
		rpgerr.New(rpgerr.CodeInvalidArgument, "appearance.hair.scalp.selection is required"))

	response, err := handler.UpdateAppearance(auth.WithPlayerID(context.Background(), "player-1"), &dnd5ev1alpha1.UpdateAppearanceRequest{
		DraftId:    "draft-error",
		Appearance: &dnd5ev1alpha1.Appearance{},
	})
	require.Nil(t, response)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	require.Equal(t, "appearance.hair.scalp.selection is required", status.Convert(err).Message())
}
