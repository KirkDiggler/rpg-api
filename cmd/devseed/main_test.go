package main

// main_test.go — rpg-api#619: devseed fixtures declared an ArmorClass but
// several never equipped SlotArmor, so the toolkit's Character.EffectiveAC
// computed the unarmored branch (10 + DEX) instead of the intended armored
// value — every attack against these fixtures during the 2026-07-04
// playtest resolved against a lower AC than the fixture claimed.
//
// This proves the fix at the level that actually matters: not "is
// EquipmentSlots[SlotArmor] set" (a field-presence check that could still
// point at the wrong item or a mismatched AC), but "does the toolkit's own
// EffectiveAC(), computed from the equipped armor, equal the fixture's
// stated ArmorClass" — the exact quantity a live combat resolves attacks
// against.

import (
	"context"
	"testing"

	"github.com/KirkDiggler/rpg-toolkit/events"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
)

func TestDevseedFixtures_EffectiveACMatchesStatedArmorClass(t *testing.T) {
	// Scoped to the fixtures rpg-api#619 actually fixed (equipped armor items
	// matching the stated ArmorClass). buildBobBarbarianData and
	// buildCharliMonkData rely on a persisted UnarmoredDefenseCondition
	// instead of equipped armor for their stated AC — a from-LoadFromData
	// direct EffectiveAC() call shows THEM under-computing too (bob: 11 want
	// 14, charli: 13 want 15), but that's a different mechanism (condition
	// not contributing to the AC chain vs. armor never equipped) and is NOT
	// what #619 scoped or asked for. Flagging to the wave owner as a
	// separate finding rather than silently expanding this PR or silently
	// dropping it.
	tests := []struct {
		name  string
		build func() *character.Data
	}{
		{"finn (fighter, chain mail)", buildFinnFighterData},
		{"alice L2 (rogue, leather)", buildAliceRogueData},
		{"alice L1 (rogue, leather)", buildAliceRogueL1Data},
		{"wendy (wizard, unarmored)", buildWendyWizardData},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := tt.build()
			char, err := character.LoadFromData(context.Background(), data, events.NewEventBus())
			if err != nil {
				t.Fatalf("LoadFromData: %v", err)
			}

			got := char.EffectiveAC(context.Background())
			if got.Total != data.ArmorClass {
				t.Fatalf("EffectiveAC() = %d, want %d (the fixture's stated ArmorClass) — components: %+v",
					got.Total, data.ArmorClass, got.Components)
			}
		})
	}
}
