package customization

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"

	customizationpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/customization/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/entities"
)

func TestCustomizationProtoToEntity_NilAndDefaults(t *testing.T) {
	require.Nil(t, ProtoToEntity(nil))

	got := ProtoToEntity(&customizationpb.HairCustomization{})
	require.NotNil(t, got)
	require.Nil(t, got.Scalp)
	require.Nil(t, got.FacialHair)
	require.Nil(t, got.ColorSRGB)
	require.Nil(t, got.Roughness)
}

func TestCustomizationEntityToProto_NilAndDefaults(t *testing.T) {
	require.Nil(t, EntityToProto(nil))

	got := EntityToProto(&entities.HairCustomization{})
	require.NotNil(t, got)
	require.Nil(t, got.Scalp)
	require.Nil(t, got.FacialHair)
	require.Nil(t, got.ColorSrgb)
	require.Nil(t, got.Roughness)
}

func TestCustomizationConverters_PreserveSelectionsAndCopyPointers(t *testing.T) {
	style := &customizationpb.StyleSelection{
		Selection: &customizationpb.StyleSelection_StyleRef{
			StyleRef: "modular-fantasy-hero:hair:38",
		},
	}
	none := &customizationpb.StyleSelection{
		Selection: &customizationpb.StyleSelection_None{None: &emptypb.Empty{}},
	}
	hair := &customizationpb.HairCustomization{
		Scalp:      style,
		FacialHair: none,
		ColorSrgb:  proto.Uint32(0x5A3825),
		Roughness:  proto.Float32(0.72),
	}

	entity := ProtoToEntity(hair)
	require.NotNil(t, entity)
	require.Equal(t, entities.StyleSelectionKindStyle, entity.Scalp.Kind)
	require.Equal(t, "modular-fantasy-hero:hair:38", entity.Scalp.StyleRef)
	require.Equal(t, entities.StyleSelectionKindNone, entity.FacialHair.Kind)
	require.Empty(t, entity.FacialHair.StyleRef)
	require.NotNil(t, entity.ColorSRGB)
	require.Equal(t, uint32(0x5A3825), *entity.ColorSRGB)
	require.NotNil(t, entity.Roughness)
	require.InDelta(t, 0.72, *entity.Roughness, 0.000001)
	require.NotSame(t, hair.ColorSrgb, entity.ColorSRGB)
	require.NotSame(t, hair.Roughness, entity.Roughness)

	converted := EntityToProto(entity)
	require.NotNil(t, converted)
	require.Equal(t, "modular-fantasy-hero:hair:38", converted.GetScalp().GetStyleRef())
	require.NotNil(t, converted.GetFacialHair().GetNone())
	require.NotNil(t, converted.ColorSrgb)
	require.Equal(t, uint32(0x5A3825), converted.GetColorSrgb())
	require.NotNil(t, converted.Roughness)
	require.InDelta(t, 0.72, converted.GetRoughness(), 0.000001)
	require.NotSame(t, entity.ColorSRGB, converted.ColorSrgb)
	require.NotSame(t, entity.Roughness, converted.Roughness)

	style.Selection.(*customizationpb.StyleSelection_StyleRef).StyleRef = "changed-input"
	require.Equal(t, "modular-fantasy-hero:hair:38", entity.Scalp.StyleRef)
	entity.Scalp.StyleRef = "changed-entity"
	require.Equal(t, "modular-fantasy-hero:hair:38", converted.GetScalp().GetStyleRef())
}

func TestCustomizationConverters_PreservePresentZeroOptionals(t *testing.T) {
	input := &customizationpb.HairCustomization{
		ColorSrgb: proto.Uint32(0),
		Roughness: proto.Float32(0),
	}

	entity := ProtoToEntity(input)
	require.NotNil(t, entity.ColorSRGB)
	require.Zero(t, *entity.ColorSRGB)
	require.NotNil(t, entity.Roughness)
	require.Zero(t, *entity.Roughness)
	require.NotSame(t, input.ColorSrgb, entity.ColorSRGB)
	require.NotSame(t, input.Roughness, entity.Roughness)

	converted := EntityToProto(entity)
	require.NotNil(t, converted.ColorSrgb)
	require.Zero(t, converted.GetColorSrgb())
	require.NotNil(t, converted.Roughness)
	require.Zero(t, converted.GetRoughness())
	require.NotSame(t, entity.ColorSRGB, converted.ColorSrgb)
	require.NotSame(t, entity.Roughness, converted.Roughness)
}
