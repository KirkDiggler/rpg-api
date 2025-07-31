package external

import (
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