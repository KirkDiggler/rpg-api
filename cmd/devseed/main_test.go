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
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"

	encountersv2 "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/v2"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	encountercore "github.com/KirkDiggler/rpg-toolkit/encounter/core"
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

// TestRunInjectCombat_DelegatesToDevcombat is a smoke test for the --inject-
// combat CLI wrapper: it proves runInjectCombat actually calls through to
// devcombat.Inject (internal/pkg/devcombat) against a real Redis-backed
// repository, and that a missing encounter surfaces a devseed-flavored,
// actionable error message. devcombat's own test suite
// (internal/pkg/devcombat/inject_test.go) covers the injection logic
// itself (goblin seeding, redundant-SetMode handling, existing players
// left untouched) — this test only needs to prove the wrapper wiring.
func TestRunInjectCombat_DelegatesToDevcombat(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()
	repo := encountersv2.NewRedis(client, time.Hour)

	const encID = "lobby-enc-1"
	enc := tkenc.New(ctx, encountercore.EncounterID(encID), tkenc.NewBroker(tkenc.NewInMemoryTransport()))
	if err := enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "alice", EntityID: "char-alice",
		Position: encountercore.Hex{Q: 0, R: 0, S: 0}, SightRange: 10,
		HP: 12, MaxHP: 12, AC: 14,
	}); err != nil {
		t.Fatalf("AddPlayer alice: %v", err)
	}
	if err := repo.Save(ctx, enc.ToData()); err != nil {
		t.Fatalf("Save seeded encounter: %v", err)
	}

	if err := runInjectCombat(ctx, client, encID, false); err != nil {
		t.Fatalf("runInjectCombat: %v", err)
	}
	data, err := repo.Get(ctx, encID)
	if err != nil {
		t.Fatalf("Get after inject: %v", err)
	}
	if data.Mode != encountercore.ModeTurnBased || len(data.Monsters) != 1 {
		t.Fatalf("runInjectCombat did not apply devcombat.Inject's effect: mode=%v monsters=%d", data.Mode, len(data.Monsters))
	}

	err = runInjectCombat(ctx, client, "no-such-encounter", false)
	if err == nil {
		t.Fatal("expected an error for a missing encounter")
	}
	if !strings.Contains(err.Error(), "StartEncounter RPC") {
		t.Fatalf("expected an actionable devseed error message, got: %v", err)
	}
}

// TestRunInjectCombat_ForceNPCFirst_DelegatesToDevcombat proves the
// --inject-combat-npc-first wiring: runInjectCombat forwards forceNPCFirst to
// devcombat.Inject's ForceNPCFirst input, which is where the deterministic
// reorder actually happens (see devcombat's own TestInject_ForceNPCFirst_
// GoblinLeadsInitiative for that behavior's coverage) — this test only proves
// the CLI wrapper passes the flag through.
func TestRunInjectCombat_ForceNPCFirst_DelegatesToDevcombat(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	defer func() { _ = client.Close() }()
	repo := encountersv2.NewRedis(client, time.Hour)

	const encID = "lobby-enc-npc-first"
	enc := tkenc.New(ctx, encountercore.EncounterID(encID), tkenc.NewBroker(tkenc.NewInMemoryTransport()))
	if err := enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "alice", EntityID: "char-alice",
		Position: encountercore.Hex{Q: 0, R: 0, S: 0}, SightRange: 10,
		HP: 12, MaxHP: 12, AC: 14,
	}); err != nil {
		t.Fatalf("AddPlayer alice: %v", err)
	}
	if err := repo.Save(ctx, enc.ToData()); err != nil {
		t.Fatalf("Save seeded encounter: %v", err)
	}

	if err := runInjectCombat(ctx, client, encID, true); err != nil {
		t.Fatalf("runInjectCombat: %v", err)
	}
	data, err := repo.Get(ctx, encID)
	if err != nil {
		t.Fatalf("Get after inject: %v", err)
	}
	if data.ActiveIdx != 0 || len(data.Initiative) == 0 || data.Initiative[0] != "goblin-1" {
		t.Fatalf("runInjectCombat did not forward ForceNPCFirst: active_idx=%d initiative=%v", data.ActiveIdx, data.Initiative)
	}
}
