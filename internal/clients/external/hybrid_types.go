package external

import (
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/class"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/race"
)

// RaceDataOutput contains both toolkit data and UI presentation data
type RaceDataOutput struct {
	// Core mechanics data from toolkit
	RaceData *race.Data
	// UI/presentation data
	UIData *RaceUIData
}

// RaceUIData contains presentation/flavor text for UI
type RaceUIData struct {
	SizeDescription      string
	AgeDescription       string
	AlignmentDescription string
}

// ClassDataOutput contains both toolkit data and UI presentation data
type ClassDataOutput struct {
	// Core mechanics data from toolkit
	ClassData *class.Data
	// UI/presentation data
	UIData *ClassUIData
}

// ClassUIData contains presentation/flavor text for UI
type ClassUIData struct {
	// Flavor text about the class
	Description string
	// Primary abilities description
	PrimaryAbilitiesDescription string
}