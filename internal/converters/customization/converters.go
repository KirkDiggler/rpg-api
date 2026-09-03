// Package customization converts appearance protos to and from toolkit customization data.
package customization

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"

	customizationpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/customization/v1alpha1"
	characterpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/customization"
)

// ProtoToToolkit converts the complete wire appearance to toolkit data.
// It only translates shape. Validation and provider interpretation belong to the toolkit.
func ProtoToToolkit(input *characterpb.Appearance) *customization.Appearance {
	if input == nil {
		return nil
	}

	return &customization.Appearance{
		Hair:   protoHairToToolkit(input.Hair),
		Outfit: protoOutfitToToolkit(input.Outfit),
	}
}

// ToolkitToProto converts the complete toolkit appearance to wire data.
// Deprecated wire string fields are intentionally left empty and inert.
func ToolkitToProto(input *customization.Appearance) *characterpb.Appearance {
	if input == nil {
		return nil
	}

	return &characterpb.Appearance{
		Hair:   toolkitHairToProto(input.Hair),
		Outfit: toolkitOutfitToProto(input.Outfit),
	}
}

func protoHairToToolkit(input *customizationpb.HairCustomization) *customization.HairCustomization {
	if input == nil {
		return nil
	}

	result := &customization.HairCustomization{
		Scalp:      protoSelectionToToolkit(input.Scalp),
		FacialHair: protoSelectionToToolkit(input.FacialHair),
	}
	if input.ColorSrgb != nil {
		result.ColorSRGB = proto.Uint32(*input.ColorSrgb)
	}
	if input.Roughness != nil {
		result.Roughness = proto.Float32(*input.Roughness)
	}
	return result
}

func toolkitHairToProto(input *customization.HairCustomization) *customizationpb.HairCustomization {
	if input == nil {
		return nil
	}

	result := &customizationpb.HairCustomization{
		Scalp:      toolkitSelectionToProto(input.Scalp),
		FacialHair: toolkitSelectionToProto(input.FacialHair),
	}
	if input.ColorSRGB != nil {
		result.ColorSrgb = proto.Uint32(*input.ColorSRGB)
	}
	if input.Roughness != nil {
		result.Roughness = proto.Float32(*input.Roughness)
	}
	return result
}

func protoOutfitToToolkit(input *customizationpb.OutfitCustomization) *customization.OutfitCustomization {
	if input == nil {
		return nil
	}

	result := &customization.OutfitCustomization{}
	if input.PrimaryColorSrgb != nil {
		result.PrimaryColorSRGB = proto.Uint32(*input.PrimaryColorSrgb)
	}
	if input.SecondaryColorSrgb != nil {
		result.SecondaryColorSRGB = proto.Uint32(*input.SecondaryColorSrgb)
	}
	return result
}

func toolkitOutfitToProto(input *customization.OutfitCustomization) *customizationpb.OutfitCustomization {
	if input == nil {
		return nil
	}

	result := &customizationpb.OutfitCustomization{}
	if input.PrimaryColorSRGB != nil {
		result.PrimaryColorSrgb = proto.Uint32(*input.PrimaryColorSRGB)
	}
	if input.SecondaryColorSRGB != nil {
		result.SecondaryColorSrgb = proto.Uint32(*input.SecondaryColorSRGB)
	}
	return result
}

func protoSelectionToToolkit(input *customizationpb.StyleSelection) *customization.StyleSelection {
	if input == nil {
		return nil
	}

	result := &customization.StyleSelection{}
	switch selection := input.Selection.(type) {
	case *customizationpb.StyleSelection_StyleRef:
		result.Kind = customization.StyleSelectionStyle
		result.StyleRef = selection.StyleRef
	case *customizationpb.StyleSelection_None:
		result.Kind = customization.StyleSelectionNone
	}
	return result
}

func toolkitSelectionToProto(input *customization.StyleSelection) *customizationpb.StyleSelection {
	if input == nil {
		return nil
	}

	result := &customizationpb.StyleSelection{}
	switch input.Kind {
	case customization.StyleSelectionStyle:
		result.Selection = &customizationpb.StyleSelection_StyleRef{StyleRef: input.StyleRef}
	case customization.StyleSelectionNone:
		result.Selection = &customizationpb.StyleSelection_None{None: &emptypb.Empty{}}
	}
	return result
}
