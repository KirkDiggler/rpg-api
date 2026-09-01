// Package customization converts shared customization protos to API entities and back.
package customization

import (
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/emptypb"

	customizationpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/customization/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/entities"
)

// ProtoToEntity converts hair customization from its wire representation.
func ProtoToEntity(input *customizationpb.HairCustomization) *entities.HairCustomization {
	if input == nil {
		return nil
	}

	result := &entities.HairCustomization{
		Scalp:      protoSelectionToEntity(input.Scalp),
		FacialHair: protoSelectionToEntity(input.FacialHair),
	}
	if input.ColorSrgb != nil {
		result.ColorSRGB = proto.Uint32(*input.ColorSrgb)
	}
	if input.Roughness != nil {
		result.Roughness = proto.Float32(*input.Roughness)
	}
	return result
}

// EntityToProto converts hair customization to its wire representation.
func EntityToProto(input *entities.HairCustomization) *customizationpb.HairCustomization {
	if input == nil {
		return nil
	}

	result := &customizationpb.HairCustomization{
		Scalp:      entitySelectionToProto(input.Scalp),
		FacialHair: entitySelectionToProto(input.FacialHair),
	}
	if input.ColorSRGB != nil {
		result.ColorSrgb = proto.Uint32(*input.ColorSRGB)
	}
	if input.Roughness != nil {
		result.Roughness = proto.Float32(*input.Roughness)
	}
	return result
}

func protoSelectionToEntity(input *customizationpb.StyleSelection) *entities.StyleSelection {
	if input == nil {
		return nil
	}

	result := &entities.StyleSelection{}
	switch selection := input.Selection.(type) {
	case *customizationpb.StyleSelection_StyleRef:
		result.Kind = entities.StyleSelectionKindStyle
		result.StyleRef = selection.StyleRef
	case *customizationpb.StyleSelection_None:
		result.Kind = entities.StyleSelectionKindNone
	}
	return result
}

func entitySelectionToProto(input *entities.StyleSelection) *customizationpb.StyleSelection {
	if input == nil {
		return nil
	}

	result := &customizationpb.StyleSelection{}
	switch input.Kind {
	case entities.StyleSelectionKindStyle:
		result.Selection = &customizationpb.StyleSelection_StyleRef{StyleRef: input.StyleRef}
	case entities.StyleSelectionKindNone:
		result.Selection = &customizationpb.StyleSelection_None{None: &emptypb.Empty{}}
	}
	return result
}
