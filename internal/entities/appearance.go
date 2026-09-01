// Package entities defines the core data structures for the RPG API
package entities

// StyleSelectionKind identifies how a customization slot should be resolved.
type StyleSelectionKind string

const (
	StyleSelectionKindStyle StyleSelectionKind = "style"
	StyleSelectionKindNone  StyleSelectionKind = "none"
)

// StyleSelection distinguishes an exact provider-owned style from explicit none.
type StyleSelection struct {
	Kind     StyleSelectionKind `json:"kind"`
	StyleRef string             `json:"style_ref,omitempty"`
}

// HairCustomization stores provider-neutral hair rendering intent.
type HairCustomization struct {
	Scalp      *StyleSelection `json:"scalp,omitempty"`
	FacialHair *StyleSelection `json:"facial_hair,omitempty"`
	ColorSRGB  *uint32         `json:"color_srgb,omitempty"`
	Roughness  *float32        `json:"roughness,omitempty"`
}

// Appearance represents cosmetic character customization stored outside game data.
type Appearance struct {
	Hair *HairCustomization `json:"hair,omitempty"`
}
