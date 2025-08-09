package equipment

// EquipmentSlot represents the type of equipment slot
type EquipmentSlot string

// Define all available equipment slots
const (
	SlotMainHand EquipmentSlot = "main_hand"
	SlotOffHand  EquipmentSlot = "off_hand"
	SlotArmor    EquipmentSlot = "armor"
	SlotHelmet   EquipmentSlot = "helmet"
	SlotGloves   EquipmentSlot = "gloves"
	SlotBoots    EquipmentSlot = "boots"
	SlotCloak    EquipmentSlot = "cloak"
	SlotAmulet   EquipmentSlot = "amulet"
	SlotRing1    EquipmentSlot = "ring1"
	SlotRing2    EquipmentSlot = "ring2"
	SlotBelt     EquipmentSlot = "belt"
	SlotShield   EquipmentSlot = "shield"
)

// String returns the string representation of the equipment slot
func (s EquipmentSlot) String() string {
	return string(s)
}

// IsValid checks if the equipment slot is valid
func (s EquipmentSlot) IsValid() bool {
	switch s {
	case SlotMainHand, SlotOffHand, SlotArmor, SlotHelmet, SlotGloves,
		SlotBoots, SlotCloak, SlotAmulet, SlotRing1, SlotRing2, SlotBelt, SlotShield:
		return true
	default:
		return false
	}
}

// AllEquipmentSlots returns a slice of all valid equipment slots
func AllEquipmentSlots() []EquipmentSlot {
	return []EquipmentSlot{
		SlotMainHand,
		SlotOffHand,
		SlotArmor,
		SlotHelmet,
		SlotGloves,
		SlotBoots,
		SlotCloak,
		SlotAmulet,
		SlotRing1,
		SlotRing2,
		SlotBelt,
		SlotShield,
	}
}

// EquipmentSlotFromString converts a string to an EquipmentSlot
// Returns the slot and true if valid, empty slot and false if invalid
func EquipmentSlotFromString(s string) (EquipmentSlot, bool) {
	slot := EquipmentSlot(s)
	if slot.IsValid() {
		return slot, true
	}
	return "", false
}

// EquipmentSlotFromProtoString converts a proto enum string to an EquipmentSlot
// Proto enums come in format "EQUIPMENT_SLOT_ARMOR", we convert to "armor"
func EquipmentSlotFromProtoString(protoSlot string) (EquipmentSlot, bool) {
	switch protoSlot {
	case "EQUIPMENT_SLOT_MAIN_HAND":
		return SlotMainHand, true
	case "EQUIPMENT_SLOT_OFF_HAND":
		return SlotOffHand, true
	case "EQUIPMENT_SLOT_ARMOR":
		return SlotArmor, true
	case "EQUIPMENT_SLOT_HELMET":
		return SlotHelmet, true
	case "EQUIPMENT_SLOT_GLOVES":
		return SlotGloves, true
	case "EQUIPMENT_SLOT_BOOTS":
		return SlotBoots, true
	case "EQUIPMENT_SLOT_CLOAK":
		return SlotCloak, true
	case "EQUIPMENT_SLOT_AMULET":
		return SlotAmulet, true
	case "EQUIPMENT_SLOT_RING_1":
		return SlotRing1, true
	case "EQUIPMENT_SLOT_RING_2":
		return SlotRing2, true
	case "EQUIPMENT_SLOT_BELT":
		return SlotBelt, true
	case "EQUIPMENT_SLOT_SHIELD":
		return SlotShield, true
	default:
		return "", false
	}
}

// Equipment represents a piece of equipment
type Equipment struct {
	ID          string
	Name        string
	Description string
	Type        string
	ValidSlots  []EquipmentSlot // Which slots this equipment can be equipped to
	Quantity    int32
	Weight      float32
	Properties  []string
}

// CanEquipToSlot checks if this equipment can be equipped to the given slot
func (e *Equipment) CanEquipToSlot(slot EquipmentSlot) bool {
	for _, validSlot := range e.ValidSlots {
		if validSlot == slot {
			return true
		}
	}
	return false
}
