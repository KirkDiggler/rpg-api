package client

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/spf13/cobra"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
)

var (
	equipmentType string
	pageSize      int32
)

var listEquipmentCmd = &cobra.Command{
	Use:   "list-equipment",
	Short: "List equipment by type",
	Long: `List D&D 5e equipment filtered by type such as:
- simple-melee-weapons
- martial-melee-weapons
- simple-ranged-weapons
- martial-ranged-weapons
- light-armor
- medium-armor
- heavy-armor
- shields
- adventuring-gear`,
	RunE: runListEquipment,
}

func init() {
	listEquipmentCmd.Flags().StringVar(&equipmentType, "type", "", "Equipment type to filter by (required)")
	listEquipmentCmd.Flags().Int32Var(&pageSize, "page-size", 20, "Number of items per page")
	_ = listEquipmentCmd.MarkFlagRequired("type")
}

func runListEquipment(_ *cobra.Command, _ []string) error {
	client, cleanup, err := createCharacterClient()
	if err != nil {
		return err
	}
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	log.Printf("Requesting equipment of type '%s' from %s...", equipmentType, serverAddr)

	// Map string type to enum
	var equipmentTypeEnum dnd5ev1alpha1.EquipmentType
	switch strings.ToLower(equipmentType) {
	case "simple-melee-weapon", "simple-melee-weapons":
		equipmentTypeEnum = dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_SIMPLE_MELEE_WEAPON
	case "martial-melee-weapon", "martial-melee-weapons":
		equipmentTypeEnum = dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_MARTIAL_MELEE_WEAPON
	case "simple-ranged-weapon", "simple-ranged-weapons":
		equipmentTypeEnum = dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_SIMPLE_RANGED_WEAPON
	case "martial-ranged-weapon", "martial-ranged-weapons":
		equipmentTypeEnum = dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_MARTIAL_RANGED_WEAPON
	case "light-armor":
		equipmentTypeEnum = dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_LIGHT_ARMOR
	case "medium-armor":
		equipmentTypeEnum = dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_MEDIUM_ARMOR
	case "heavy-armor":
		equipmentTypeEnum = dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_HEAVY_ARMOR
	case "shield", "shields":
		equipmentTypeEnum = dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_SHIELD
	case "adventuring-gear":
		equipmentTypeEnum = dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_ADVENTURING_GEAR
	default:
		return fmt.Errorf("unknown equipment type: %s", equipmentType)
	}

	req := &dnd5ev1alpha1.ListEquipmentByTypeRequest{
		EquipmentType: equipmentTypeEnum,
		PageSize:      pageSize,
	}

	resp, err := client.ListEquipmentByType(ctx, req)
	if err != nil {
		return fmt.Errorf("failed to list equipment: %w", err)
	}

	fmt.Printf("Found %d equipment items of type '%s':\n\n", resp.TotalSize, equipmentType)

	for _, equipment := range resp.Equipment {
		fmt.Printf("⚔️  %s (ID: %s)\n", equipment.Name, equipment.Id)

		if equipment.Description != "" {
			fmt.Printf("   Description: %s\n", equipment.Description)
		}

		fmt.Printf("   Category: %s\n", equipment.Category)

		if equipment.Cost != nil {
			fmt.Printf("   Cost: %d %s\n", equipment.Cost.Quantity, equipment.Cost.Unit)
		}

		if equipment.Weight != nil {
			fmt.Printf("   Weight: %d %s\n", equipment.Weight.Quantity, equipment.Weight.Unit)
		}

		// Show weapon-specific data
		if weaponData := equipment.GetWeaponData(); weaponData != nil {
			fmt.Printf("   🗡️  Weapon Properties:\n")
			fmt.Printf("     - Category: %s\n", weaponData.WeaponCategory)
			fmt.Printf("     - Range: %s\n", weaponData.Range)
			if weaponData.DamageDice != "" {
				fmt.Printf("     - Damage: %s %s\n", weaponData.DamageDice, weaponData.DamageType)
			}
			if len(weaponData.Properties) > 0 {
				fmt.Printf("     - Properties: %s\n", strings.Join(weaponData.Properties, ", "))
			}
			if weaponData.NormalRange > 0 {
				fmt.Printf("     - Normal Range: %d ft\n", weaponData.NormalRange)
			}
			if weaponData.LongRange > 0 {
				fmt.Printf("     - Long Range: %d ft\n", weaponData.LongRange)
			}
		}

		// Show armor-specific data
		if armorData := equipment.GetArmorData(); armorData != nil {
			fmt.Printf("   🛡️  Armor Properties:\n")
			fmt.Printf("     - Category: %s\n", armorData.ArmorCategory)
			fmt.Printf("     - Base AC: %d\n", armorData.BaseAc)
			if armorData.DexBonus {
				fmt.Printf("     - Dex Bonus: Yes")
				if armorData.HasDexLimit {
					fmt.Printf(" (max +%d)", armorData.MaxDexBonus)
				}
				fmt.Println()
			}
			if armorData.StrMinimum > 0 {
				fmt.Printf("     - Str Minimum: %d\n", armorData.StrMinimum)
			}
			if armorData.StealthDisadvantage {
				fmt.Printf("     - Stealth: Disadvantage\n")
			}
		}

		// Show gear-specific data
		if gearData := equipment.GetGearData(); gearData != nil {
			fmt.Printf("   🎒 Gear Properties:\n")
			fmt.Printf("     - Category: %s\n", gearData.GearCategory)
			if len(gearData.Properties) > 0 {
				fmt.Printf("     - Properties: %s\n", strings.Join(gearData.Properties, ", "))
			}
		}

		fmt.Println() // Empty line between equipment
	}

	if resp.NextPageToken != "" {
		fmt.Printf("More results available. Next page token: %s\n", resp.NextPageToken)
	}

	return nil
}
