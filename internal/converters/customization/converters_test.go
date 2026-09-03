package customization

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"

	customizationpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/customization/v1alpha1"
	characterpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/customization"
)

func TestProtoToToolkitAppearance_PreservesNilAndEmpty(t *testing.T) {
	require.Nil(t, ProtoToToolkit(nil))

	got := ProtoToToolkit(&characterpb.Appearance{})
	require.Equal(t, &customization.Appearance{}, got)
}

func TestToolkitToProtoAppearance_PreservesNilAndEmpty(t *testing.T) {
	require.Nil(t, ToolkitToProto(nil))

	got := ToolkitToProto(&customization.Appearance{})
	require.True(t, proto.Equal(&characterpb.Appearance{}, got))
}

func TestAppearanceConverters_PreservePresentEmptyNestedMessages(t *testing.T) {
	wire := &characterpb.Appearance{
		Hair:   &customizationpb.HairCustomization{},
		Outfit: &customizationpb.OutfitCustomization{},
	}

	got := ProtoToToolkit(wire)
	require.NotNil(t, got.Hair)
	require.NotNil(t, got.Outfit)

	converted := ToolkitToProto(got)
	require.NotNil(t, converted.Hair)
	require.NotNil(t, converted.Outfit)
	require.True(t, proto.Equal(wire, converted))
}

func TestAppearanceConverters_PreserveCompleteAppearance(t *testing.T) {
	wire := &characterpb.Appearance{
		// Deprecated fields are deliberately populated to prove they stay inert.
		SkinTone:       "legacy-skin",
		PrimaryColor:   "legacy-primary",
		SecondaryColor: "legacy-secondary",
		EyeColor:       "legacy-eyes",
		Hair: &customizationpb.HairCustomization{
			Scalp: &customizationpb.StyleSelection{
				Selection: &customizationpb.StyleSelection_StyleRef{StyleRef: "unknown:hair:ok"},
			},
			FacialHair: &customizationpb.StyleSelection{}, // malformed shape reaches toolkit
			ColorSrgb:  proto.Uint32(0),
			Roughness:  proto.Float32(0),
		},
		Outfit: &customizationpb.OutfitCustomization{
			PrimaryColorSrgb:   proto.Uint32(0),
			SecondaryColorSrgb: proto.Uint32(0xFFFFFF),
		},
	}

	got := ProtoToToolkit(wire)
	require.NotNil(t, got)
	require.Equal(t, customization.StyleSelectionKind(""), got.Hair.FacialHair.Kind)
	require.NotNil(t, got.Outfit.PrimaryColorSRGB)
	require.Zero(t, *got.Outfit.PrimaryColorSRGB)
	require.NotNil(t, got.Outfit.SecondaryColorSRGB)
	require.Equal(t, uint32(0xFFFFFF), *got.Outfit.SecondaryColorSRGB)

	converted := ToolkitToProto(got)
	require.True(t, proto.Equal(&characterpb.Appearance{
		Hair:   wire.Hair,
		Outfit: wire.Outfit,
	}, converted), "deprecated wire fields must remain inert")
}

func TestAppearanceConverters_PreserveOutfitChannelPresence(t *testing.T) {
	tests := []struct {
		name   string
		outfit *customizationpb.OutfitCustomization
	}{
		{name: "primary only", outfit: &customizationpb.OutfitCustomization{PrimaryColorSrgb: proto.Uint32(0x102030)}},
		{name: "secondary only", outfit: &customizationpb.OutfitCustomization{SecondaryColorSrgb: proto.Uint32(0x405060)}},
		{name: "both", outfit: &customizationpb.OutfitCustomization{
			PrimaryColorSrgb:   proto.Uint32(0x102030),
			SecondaryColorSrgb: proto.Uint32(0x405060),
		}},
		{name: "explicit black", outfit: &customizationpb.OutfitCustomization{
			PrimaryColorSrgb: proto.Uint32(0),
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wire := &characterpb.Appearance{Outfit: tt.outfit}
			got := ProtoToToolkit(wire)
			require.Equal(t, wire.Outfit.PrimaryColorSrgb != nil, got.Outfit.PrimaryColorSRGB != nil)
			require.Equal(t, wire.Outfit.SecondaryColorSrgb != nil, got.Outfit.SecondaryColorSRGB != nil)
			require.True(t, proto.Equal(wire, ToolkitToProto(got)))
		})
	}
}

func TestAppearanceConverters_PreserveStyleSelections(t *testing.T) {
	tests := []struct {
		name      string
		selection *customizationpb.StyleSelection
	}{
		{
			name: "style",
			selection: &customizationpb.StyleSelection{
				Selection: &customizationpb.StyleSelection_StyleRef{StyleRef: "provider:hair:38"},
			},
		},
		{
			name: "none",
			selection: &customizationpb.StyleSelection{
				Selection: &customizationpb.StyleSelection_None{None: &emptypb.Empty{}},
			},
		},
		{name: "malformed no oneof", selection: &customizationpb.StyleSelection{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wire := &characterpb.Appearance{Hair: &customizationpb.HairCustomization{
				Scalp: tt.selection,
			}}

			got := ProtoToToolkit(wire)
			require.NotNil(t, got.Hair.Scalp)
			switch tt.name {
			case "style":
				require.Equal(t, customization.StyleSelectionStyle, got.Hair.Scalp.Kind)
				require.Equal(t, "provider:hair:38", got.Hair.Scalp.StyleRef)
			case "none":
				require.Equal(t, customization.StyleSelectionNone, got.Hair.Scalp.Kind)
				require.Empty(t, got.Hair.Scalp.StyleRef)
			case "malformed no oneof":
				require.Equal(t, customization.StyleSelectionKind(""), got.Hair.Scalp.Kind)
			}
			require.True(t, proto.Equal(wire, ToolkitToProto(got)))
		})
	}
}

func TestAppearanceConverters_DoNotValidateOrInterpretValues(t *testing.T) {
	wire := &characterpb.Appearance{Hair: &customizationpb.HairCustomization{
		ColorSrgb: proto.Uint32(0x1000000),
		Roughness: proto.Float32(float32(math.NaN())),
	}, Outfit: &customizationpb.OutfitCustomization{
		PrimaryColorSrgb: proto.Uint32(0x1000000),
	}}

	got := ProtoToToolkit(wire)
	require.Equal(t, uint32(0x1000000), *got.Hair.ColorSRGB)
	require.True(t, math.IsNaN(float64(*got.Hair.Roughness)))
	require.Equal(t, uint32(0x1000000), *got.Outfit.PrimaryColorSRGB)
	require.True(t, proto.Equal(wire, ToolkitToProto(got)))
}
