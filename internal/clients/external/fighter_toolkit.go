package external

import (
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/class/fighter"
)

// getFighterFromToolkit returns the Fighter class data from the toolkit
func (c *client) getFighterFromToolkit() (*ClassDataOutput, error) {
	fighterData := fighter.Get()
	
	// Create UI data for Fighter
	uiData := &ClassUIData{
		Description:                 "A master of martial combat, skilled with a variety of weapons and armor. Fighters are versatile warriors who excel in physical combat.",
		PrimaryAbilitiesDescription: "Strength or Dexterity, and Constitution",
	}
	
	return &ClassDataOutput{
		ClassData: fighterData,
		UIData:    uiData,
	}, nil
}