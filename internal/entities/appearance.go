// Package entities defines the core data structures for the RPG API
package entities

// Appearance represents the cosmetic customization of a character
// This is stored separately from character game data as it has no rules implications
type Appearance struct {
	SkinTone       string `json:"skin_tone"`       // Hex color e.g. "#D5A88C"
	PrimaryColor   string `json:"primary_color"`   // Armor/clothing color e.g. "#8B0000"
	SecondaryColor string `json:"secondary_color"` // Accent/trim color e.g. "#FFD700"
	EyeColor       string `json:"eye_color"`       // Eye iris color e.g. "#4A2511"
}
