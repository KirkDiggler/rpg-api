package character

import (
	"math"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	customizationpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/customization/v1alpha1"
	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
)

const (
	maxStyleRefBytes = 256
	maxColorSRGB     = 0xFFFFFF

	appearanceRequiredMessage = "appearance is required"
	hairScalpField            = "appearance.hair.scalp"
	hairFacialHairField       = "appearance.hair.facial_hair"
	hairColorSRGBField        = "appearance.hair.color_srgb"
	hairRoughnessField        = "appearance.hair.roughness"
)

func validateAppearance(appearance *dnd5ev1alpha1.Appearance) error {
	if appearance == nil {
		return status.Error(codes.InvalidArgument, appearanceRequiredMessage)
	}
	return validateHairCustomization(appearance.Hair)
}

func validateHairCustomization(hair *customizationpb.HairCustomization) error {
	if hair == nil {
		return nil
	}
	if err := validateStyleSelection(hairScalpField, hair.Scalp); err != nil {
		return err
	}
	if err := validateStyleSelection(hairFacialHairField, hair.FacialHair); err != nil {
		return err
	}
	if hair.ColorSrgb != nil && *hair.ColorSrgb > maxColorSRGB {
		return status.Errorf(codes.InvalidArgument, "%s must be between 0 and 0xFFFFFF", hairColorSRGBField)
	}
	if hair.Roughness != nil {
		value := float64(*hair.Roughness)
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
			return status.Errorf(codes.InvalidArgument, "%s must be finite and between 0 and 1", hairRoughnessField)
		}
	}
	return nil
}

func validateStyleSelection(field string, selection *customizationpb.StyleSelection) error {
	if selection == nil {
		return nil
	}

	switch selected := selection.Selection.(type) {
	case *customizationpb.StyleSelection_StyleRef:
		if selected.StyleRef == "" {
			return status.Errorf(codes.InvalidArgument, "%s.style_ref is required", field)
		}
		if len(selected.StyleRef) > maxStyleRefBytes {
			return status.Errorf(codes.InvalidArgument, "%s.style_ref must be at most %d bytes", field, maxStyleRefBytes)
		}
	case *customizationpb.StyleSelection_None:
		return nil
	default:
		return status.Errorf(codes.InvalidArgument, "%s.selection is required", field)
	}
	return nil
}
