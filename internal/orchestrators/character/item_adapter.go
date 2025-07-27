package character

import (
	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
	"github.com/KirkDiggler/rpg-toolkit/items"
)

// ItemAdapter adapts our InventoryItem to implement rpg-toolkit item interfaces
type ItemAdapter struct {
	inventoryItem *dnd5e.InventoryItem
	equipmentData *dnd5e.EquipmentData
}

// NewItemAdapter creates a new item adapter
func NewItemAdapter(inventoryItem *dnd5e.InventoryItem, equipmentData *dnd5e.EquipmentData) *ItemAdapter {
	return &ItemAdapter{
		inventoryItem: inventoryItem,
		equipmentData: equipmentData,
	}
}

// Ensure ItemAdapter implements the required interfaces
var (
	_ items.Item           = (*ItemAdapter)(nil)
	_ items.EquippableItem = (*ItemAdapter)(nil)
	_ items.WeaponItem     = (*ItemAdapter)(nil)
	_ items.ArmorItem      = (*ItemAdapter)(nil)
)

// Core Entity interface implementation

// GetType returns the type of this entity
func (a *ItemAdapter) GetType() string {
	if a.equipmentData == nil {
		return "item"
	}
	return a.equipmentData.Type
}

// Item interface implementation

// GetID returns the item's unique identifier
func (a *ItemAdapter) GetID() string {
	if a.inventoryItem == nil {
		return ""
	}
	return a.inventoryItem.ItemID
}

// GetName returns the item's display name
func (a *ItemAdapter) GetName() string {
	if a.equipmentData == nil {
		return a.inventoryItem.ItemID // Fallback to ID if no data
	}
	return a.equipmentData.Name
}

// GetWeight returns the item's weight in pounds
func (a *ItemAdapter) GetWeight() float64 {
	if a.equipmentData == nil {
		return 0.0
	}
	// Convert weight from tenths of pounds to pounds
	return float64(a.equipmentData.Weight) / 10.0
}

// GetValue returns the item's value in gold pieces
func (a *ItemAdapter) GetValue() int {
	if a.equipmentData == nil {
		return 0
	}
	// If we have gear data with cost in copper, convert to gold
	if a.equipmentData.GearData != nil && a.equipmentData.GearData.CostInCopper > 0 {
		// Convert copper to gold (100 cp = 1 gp)
		return int(a.equipmentData.GearData.CostInCopper / 100)
	}
	// Return 0 if no cost data is available
	return 0
}

// GetProperties returns the item's properties
func (a *ItemAdapter) GetProperties() []string {
	if a.equipmentData == nil || a.equipmentData.WeaponData == nil {
		return []string{}
	}
	// Return weapon properties directly from the Properties field
	return a.equipmentData.WeaponData.Properties
}

// IsStackable returns whether the item can stack
func (a *ItemAdapter) IsStackable() bool {
	if a.equipmentData == nil {
		return false
	}
	return a.equipmentData.Stackable
}

// GetMaxStack returns the maximum stack size
func (a *ItemAdapter) GetMaxStack() int {
	if !a.IsStackable() {
		return 1
	}
	// Default to large stack for stackable items
	return 999
}

// EquippableItem interface implementation

// IsEquippable returns whether the item can be equipped
func (a *ItemAdapter) IsEquippable() bool {
	if a.equipmentData == nil {
		return false
	}
	// Item is equippable if it has weapon or armor data
	return a.equipmentData.WeaponData != nil || a.equipmentData.ArmorData != nil
}

// GetValidSlots returns the slots where this item can be equipped
func (a *ItemAdapter) GetValidSlots() []string {
	if a.equipmentData == nil {
		return []string{}
	}

	if a.equipmentData.WeaponData != nil {
		// Weapons can go in main hand or off hand
		slots := []string{dnd5e.EquipmentSlotMainHand}
		if !a.IsTwoHanded() {
			slots = append(slots, dnd5e.EquipmentSlotOffHand)
		}
		return slots
	}

	if a.equipmentData.ArmorData != nil {
		// Armor goes in armor slot, shields in off hand
		if a.equipmentData.ArmorData.ArmorCategory == dnd5e.ArmorCategoryShield {
			return []string{dnd5e.EquipmentSlotOffHand}
		}
		return []string{dnd5e.EquipmentSlotArmor}
	}

	// For other equippable items, determine by type
	// This could be expanded based on item categories
	return []string{}
}

// GetRequiredSlots returns all slots this item occupies when equipped
func (a *ItemAdapter) GetRequiredSlots() []string {
	if a.equipmentData == nil {
		return []string{}
	}

	if a.equipmentData.WeaponData != nil && a.IsTwoHanded() {
		// Two-handed weapons require both hands
		return []string{dnd5e.EquipmentSlotMainHand, dnd5e.EquipmentSlotOffHand}
	}

	// Most items only require their primary slot
	return a.GetValidSlots()
}

// IsAttunable returns whether the item can be attuned to
func (a *ItemAdapter) IsAttunable() bool {
	if a.equipmentData == nil {
		return false
	}
	// Check if gear data indicates attunement requirement
	if a.equipmentData.GearData != nil {
		return a.equipmentData.GearData.RequiresAttunement
	}
	// For weapons and armor, check if they have magical properties
	// that might require attunement (this is a simplified check)
	if a.equipmentData.WeaponData != nil || a.equipmentData.ArmorData != nil {
		// Check if any property suggests magic item requiring attunement
		for _, prop := range a.equipmentData.Properties {
			if prop == dnd5e.WeaponPropertyRequiresAttunement || prop == dnd5e.WeaponPropertyMagic {
				return true
			}
		}
	}
	return false
}

// RequiresAttunement returns whether the item must be attuned to gain its benefits
func (a *ItemAdapter) RequiresAttunement() bool {
	return a.IsAttunable() // For now, if it's attunable, it requires attunement
}

// GetRequiredProficiency returns the proficiency needed to use this item effectively
func (a *ItemAdapter) GetRequiredProficiency() string {
	if a.IsWeapon() {
		return a.GetWeaponProficiency()
	}
	if a.IsArmor() {
		return a.GetArmorProficiency()
	}
	return ""
}

// WeaponItem interface implementation

// IsWeapon returns whether this is a weapon
func (a *ItemAdapter) IsWeapon() bool {
	return a.equipmentData != nil && a.equipmentData.WeaponData != nil
}

// GetDamage returns the weapon's damage dice
func (a *ItemAdapter) GetDamage() string {
	if !a.IsWeapon() {
		return ""
	}
	return a.equipmentData.WeaponData.DamageDice
}

// GetDamageType returns the weapon's damage type
func (a *ItemAdapter) GetDamageType() string {
	if !a.IsWeapon() {
		return ""
	}
	return a.equipmentData.WeaponData.DamageType
}

// GetRange returns the weapon's range in feet (0 for melee)
func (a *ItemAdapter) GetRange() int {
	if !a.IsWeapon() {
		return 0
	}
	// Return normal range for ranged weapons, 0 for melee
	return int(a.equipmentData.WeaponData.NormalRange)
}

// GetWeaponProficiency returns the proficiency required to use this weapon
func (a *ItemAdapter) GetWeaponProficiency() string {
	if !a.IsWeapon() {
		return ""
	}
	// Map weapon categories to proficiency types
	switch a.equipmentData.WeaponData.WeaponCategory {
	case dnd5e.WeaponCategorySimple:
		return dnd5e.ProficiencySimpleWeapons
	case dnd5e.WeaponCategoryMartial:
		return dnd5e.ProficiencyMartialWeapons
	default:
		// Specific weapon proficiency
		return a.GetID()
	}
}

// IsTwoHanded returns whether the weapon requires two hands
func (a *ItemAdapter) IsTwoHanded() bool {
	if !a.IsWeapon() {
		return false
	}
	// Check if "two-handed" is in the properties
	for _, prop := range a.equipmentData.WeaponData.Properties {
		if prop == dnd5e.WeaponPropertyTwoHanded {
			return true
		}
	}
	return false
}

// IsVersatile returns whether the weapon can be used with one or two hands
func (a *ItemAdapter) IsVersatile() bool {
	if !a.IsWeapon() {
		return false
	}
	// Check if "versatile" is in the properties
	for _, prop := range a.equipmentData.WeaponData.Properties {
		if prop == dnd5e.WeaponPropertyVersatile {
			return true
		}
	}
	return false
}

// IsFinesse returns whether the weapon can use Dexterity for attack and damage
func (a *ItemAdapter) IsFinesse() bool {
	if !a.IsWeapon() {
		return false
	}
	// Check if "finesse" is in the properties
	for _, prop := range a.equipmentData.WeaponData.Properties {
		if prop == dnd5e.WeaponPropertyFinesse {
			return true
		}
	}
	return false
}

// ArmorItem interface implementation

// IsArmor returns whether this is armor
func (a *ItemAdapter) IsArmor() bool {
	return a.equipmentData != nil && a.equipmentData.ArmorData != nil
}

// GetArmorClass returns the base armor class
func (a *ItemAdapter) GetArmorClass() int {
	if !a.IsArmor() {
		return 0
	}
	return int(a.equipmentData.ArmorData.BaseAC)
}

// GetMaxDexBonus returns the maximum Dexterity bonus allowed (-1 for no limit)
func (a *ItemAdapter) GetMaxDexBonus() int {
	if !a.IsArmor() {
		return -1
	}
	if !a.equipmentData.ArmorData.HasDexLimit {
		// No limit on dex bonus
		return -1
	}
	return int(a.equipmentData.ArmorData.MaxDexBonus)
}

// GetStrengthRequirement returns the minimum strength required to wear this armor
func (a *ItemAdapter) GetStrengthRequirement() int {
	if !a.IsArmor() {
		return 0
	}
	return int(a.equipmentData.ArmorData.StrMinimum)
}

// GetStealthDisadvantage returns whether the armor imposes disadvantage on Stealth checks
func (a *ItemAdapter) GetStealthDisadvantage() bool {
	return a.IsArmor() && a.equipmentData.ArmorData.StealthDisadvantage
}

// GetArmorProficiency returns the proficiency required to wear this armor
func (a *ItemAdapter) GetArmorProficiency() string {
	if !a.IsArmor() {
		return ""
	}
	// Map armor types to proficiency types
	switch a.equipmentData.ArmorData.ArmorCategory {
	case dnd5e.ArmorCategoryLight:
		return dnd5e.ProficiencyLightArmor
	case dnd5e.ArmorCategoryMedium:
		return dnd5e.ProficiencyMediumArmor
	case dnd5e.ArmorCategoryHeavy:
		return dnd5e.ProficiencyHeavyArmor
	case dnd5e.ArmorCategoryShield:
		return dnd5e.ProficiencyShields
	default:
		return a.equipmentData.ArmorData.ArmorCategory
	}
}
