// devseed populates Redis with a ready-to-play v2 encounter plus the
// canonical playtest characters and a goblin, so the rpg-dnd5e-web playtest
// harness can verify reaction flows end-to-end without going through
// CreateDraft/FinalizeDraft.
//
// What it writes
//
//	character:char-alice    — rogue with SneakAttack condition pre-applied
//	character:char-bob      — level-1 barbarian with Rage feature + RageCharges resource
//	character:char-wendy    — level-1 wizard with 1st-level spell slots (Shield-eligible, default fixture only)
//	enc:v2:<encounter-id>   — encounter referencing characters by EntityID,
//	                          plus a single goblin (monster.NewGoblin DataJSON)
//
// The character keys use the same schema as the production character repo
// (entities.Character{Data: *toolkitchar.Data}) so the rpg-api character
// handler can Get them by ID. The encounter key uses the v2 toolkit shape
// so handlers/dnd5e/v2/encounter can Load it without translation.
//
// Wave 2.11d wires conditions at load:
//   - applyReactionConditions Apply()s OpportunityAttack on EVERY character
//     unconditionally — the OA predicate gates whether it actually publishes
//     a trigger event. Shield is Apply()'d only when hasFirstLevelSpellSlot
//     returns true (wendy). Default readiness: OA on (seeded by the encounter
//     SDK's AddPlayer for combatants with DamageDice), Shield off.
//   - SneakAttack is persisted in alice's Conditions JSON so LoadFromData
//     re-Apply()s it on every rehydration (the once-per-turn gate persists
//     via SneakAttackData.UsedThisTurn → rpg-toolkit#655).
//   - Rage is persisted as a Feature so the API's Activate flow can call it;
//     bob is NOT raging at start.
//
// Usage
//
//	# Seed against local Redis with default encounter id and TURN_BASED mode
//	go run ./cmd/devseed
//
//	# Seed the wave-1-rogue fixture (L1 alice, L1 bob, goblin — no wendy)
//	go run ./cmd/devseed --fixture=wave-1-rogue
//
//	# Override encounter id (matches the harness URL ?encounterId=…)
//	go run ./cmd/devseed --encounter-id=dev-encounter
//
//	# Seed for free-roam (no initiative). Default is turn_based.
//	go run ./cmd/devseed --mode=free_roam
//
//	# Inject a goblin into an EXISTING encounter (e.g. one created via the
//	# lobby StartEncounter RPC) and flip it to TURN_BASED, WITHOUT rebuilding
//	# it or touching its existing players (rpg-api#634 — "a real fight on
//	# GameView"). --encounter-id must reference an already-persisted encounter.
//	go run ./cmd/devseed --inject-combat --encounter-id=<id>
//
//	# Custom Redis target
//	REDIS_ADDR=redis.example.com:6379 go run ./cmd/devseed
//
// Positions are aligned with the harness's SEEDED_FALLBACK in
// rpg-dnd5e-web/src/components/playtest/PlaytestHarness.tsx so the first
// MoveEntity dispatches against the right starting hexes.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/KirkDiggler/rpg-api/internal/pkg/devcombat"
	redisclient "github.com/KirkDiggler/rpg-api/internal/redis"
	encountersv2 "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/v2"
	coreResources "github.com/KirkDiggler/rpg-toolkit/core/resources"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	encountercore "github.com/KirkDiggler/rpg-toolkit/encounter/core"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/conditions"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	dnd5eResources "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/resources"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
)

const (
	// charKeyPrefix matches internal/repositories/character/redis.go.
	// Kept as a literal so devseed doesn't depend on the (currently
	// unexported) constant in the repository package.
	charKeyPrefix = "character:"

	// playerIndexPrefix mirrors the same constant in the character repo;
	// SADD'd here so ListByPlayerID returns seeded characters (per Copilot
	// review feedback on PR #543).
	playerIndexPrefix = "character:player:"

	// encKeyPrefix matches internal/repositories/encounters/v2/redis.go.
	encKeyPrefix = "enc:v2:"

	// encTTL mirrors the production wiring in cmd/server/server.go
	// (encountersv2.NewRedis(client, 24*time.Hour)). Characters get no TTL
	// to match the character repo.
	encTTL = 24 * time.Hour

	defaultEncounterID = "dev-encounter"
	defaultRedisAddr   = "localhost:6379"

	modeTurnBased = "turn_based"
	modeFreeRoam  = "free_roam"

	fixtureDefault        = ""
	fixtureWave1Rogue     = "wave-1-rogue"
	fixtureWave2Monk      = "wave-2-monk"
	fixtureWave2Beat2     = "wave-2-beat2"
	fixtureWave3Barbarian = "wave-3-barbarian"

	// beat2GoblinPassivePerception is deliberately set well below a typical
	// goblin's real SRD passive Perception (9, from WIS 8) so the Hide beat
	// of the wave-2-beat2 playtest script succeeds reliably rather than
	// probabilistically: charli's Stealth modifier is DEX +3 with no
	// proficiency, so even a worst-case roll of 1 (total 4) beats this.
	// Per the design doc's own R6 scope decision, only the Hide-SUCCESS path
	// is exercised live; failure is covered by a toolkit unit test instead.
	beat2GoblinPassivePerception = 5

	// wave3GoblinHP is bumped from the standard 7 so raging-bob's greataxe
	// (1d12 + STR3 + Rage2 = 6-17) doesn't one-shot the goblin. The playtest
	// needs to observe BOTH (a) raging-bob's attack AND (b) goblin's attack
	// on raging-bob (Rage resistance halves it).
	wave3GoblinHP = 30

	weaponShortsword    = "shortsword"
	weaponGreataxe      = "greataxe"
	weaponLongsword     = "longsword"
	inventoryTypeWeapon = "weapon"

	// armorLeather / armorChainMail match the toolkit's armor.Leather /
	// armor.ChainMail IDs ("leather" / "chain-mail") — see
	// rpg-toolkit/rulebooks/dnd5e/armor. Leather is base AC 11 + unlimited
	// DEX bonus (Alice: 11+3=14). Chain mail is base AC 16 with MaxDexBonus 0
	// (Finn: 16+0=16). Kept as local string literals (mirrors weaponLongsword
	// above) rather than importing the armor package, since devseed only
	// needs the item ID, not the toolkit's AC-calculation types.
	armorLeather       = "leather"
	armorChainMail     = "chain-mail"
	inventoryTypeArmor = "armor"

	damageTypePiercing    = "piercing"
	damageTypeSlashing    = "slashing"
	damageTypeFire        = "fire"
	damageTypeBludgeoning = "bludgeoning"

	monsterRefGoblin      = "dnd5e:monsters:goblin"
	goblinBaseDamageDice  = "1d6+2"
	barbarianGreataxeDice = "1d12+3"
)

var (
	// Entity IDs are stable across runs so the harness can deep-link to a
	// specific player and the existing integration tests' EntityID constants
	// stay aligned.
	playerAlice  = encountercore.PlayerID("alice")
	playerBob    = encountercore.PlayerID("bob")
	playerWendy  = encountercore.PlayerID("wendy")
	playerCharli = encountercore.PlayerID("charli")
	playerFinn   = encountercore.PlayerID("finn")

	entityAlice  = encountercore.EntityID("char-alice")
	entityBob    = encountercore.EntityID("char-bob")
	entityWendy  = encountercore.EntityID("char-wendy")
	entityGoblin = encountercore.EntityID("goblin-1")
	entityCharli = encountercore.EntityID("char-charli")
	entityFinn   = encountercore.EntityID("char-finn")
)

func main() {
	if err := run(); err != nil {
		// Single top-level error sink so defers in run() always fire
		// (gocritic exitAfterDefer); log.Fatalf inside run() would skip
		// defer cancel() and the Redis client.Close defer.
		log.Fatalf("devseed: %v", err)
	}
}

func run() error {
	var (
		redisAddr   = flag.String("redis-addr", "", "Redis address (default: $REDIS_ADDR or "+defaultRedisAddr+")")
		encounterID = flag.String("encounter-id", defaultEncounterID, "Encounter ID to seed under enc:v2:<id>")
		mode        = flag.String("mode", modeTurnBased, "Encounter mode: turn_based or free_roam")
		fixture     = flag.String("fixture", fixtureDefault,
			"Fixture to seed: wave-1-rogue, wave-2-monk, wave-2-beat2, wave-3-barbarian. Default: alice L2 + bob + wendy.")
		injectCombat = flag.Bool("inject-combat", false,
			"Load the EXISTING encounter at --encounter-id, add a goblin, and flip it to TURN_BASED, "+
				"instead of seeding a fresh fixture. Requires an already-persisted encounter (e.g. one "+
				"created via the lobby StartEncounter RPC). Ignores --mode and --fixture.")
		injectCombatNPCFirst = flag.Bool("inject-combat-npc-first", false,
			"With --inject-combat, force the injected goblin to lead initiative (rpg-api#636 repro) "+
				"instead of leaving turn order to the real roll. No effect without --inject-combat.")
	)
	flag.Parse()

	addr := *redisAddr
	if addr == "" {
		addr = os.Getenv("REDIS_ADDR")
	}
	if addr == "" {
		addr = defaultRedisAddr
	}

	var encMode encountercore.EncounterMode
	if !*injectCombat {
		var modeErr error
		encMode, modeErr = parseMode(*mode)
		if modeErr != nil {
			return fmt.Errorf("invalid --mode: %w", modeErr)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Log the target before connecting so an error that swallows addr (per
	// the gosec G706 fix below) still leaves a clear breadcrumb.
	fmt.Fprintf(os.Stderr, "devseed: target redis=%s fixture=%q inject-combat=%v\n", addr, *fixture, *injectCombat)

	client, err := redisclient.NewClient(addr, nil)
	if err != nil {
		// gosec/G706: addr is operator-supplied via --redis-addr / REDIS_ADDR,
		// so we intentionally do NOT interpolate it into the error wrap.
		// The pre-connect breadcrumb above already emitted addr.
		return fmt.Errorf("connect redis: %w", err)
	}
	defer func() { _ = client.Close() }()

	if pingErr := client.Ping(ctx).Err(); pingErr != nil {
		// gosec/G706: same rationale as the NewClient error path.
		return fmt.Errorf("ping redis: %w", pingErr)
	}

	if *injectCombat {
		return runInjectCombat(ctx, client, *encounterID, *injectCombatNPCFirst)
	}

	var chars []*toolkitchar.Data
	var encData *tkenc.Data

	switch *fixture {
	case fixtureWave2Monk:
		chars = []*toolkitchar.Data{
			buildCharliMonkData(),
		}
		encData, err = buildWave2MonkEncounterData(*encounterID, encMode)
		if err != nil {
			return fmt.Errorf("build encounter: %w", err)
		}
	case fixtureWave2Beat2:
		chars = []*toolkitchar.Data{
			buildCharliMonkData(),
			buildFinnFighterData(),
		}
		encData, err = buildWave2Beat2EncounterData(*encounterID, encMode)
		if err != nil {
			return fmt.Errorf("build encounter: %w", err)
		}
	case fixtureWave3Barbarian:
		chars = []*toolkitchar.Data{
			buildBobBarbarianData(),
		}
		encData, err = buildWave3BarbarianEncounterData(*encounterID, encMode)
		if err != nil {
			return fmt.Errorf("build encounter: %w", err)
		}
	case fixtureWave1Rogue:
		chars = []*toolkitchar.Data{
			buildAliceRogueL1Data(),
			buildBobBarbarianData(),
		}
		encData, err = buildWave1RogueEncounterData(*encounterID, encMode)
		if err != nil {
			return fmt.Errorf("build encounter: %w", err)
		}
	case fixtureDefault:
		// Default: alice L2 + bob + wendy (original behavior).
		chars = []*toolkitchar.Data{
			buildAliceRogueData(),
			buildBobBarbarianData(),
			buildWendyWizardData(),
		}
		encData, err = buildEncounterData(*encounterID, encMode)
		if err != nil {
			return fmt.Errorf("build encounter: %w", err)
		}
	default:
		return fmt.Errorf("unknown fixture %q: supported values are %q, %q, %q, %q, and %q",
			*fixture, fixtureDefault, fixtureWave1Rogue, fixtureWave2Monk, fixtureWave2Beat2, fixtureWave3Barbarian)
	}

	// 1. Seed characters first so they exist when the harness or handler
	//    loads them via the character repo.
	for _, cd := range chars {
		if writeErr := writeCharacter(ctx, client, cd); writeErr != nil {
			return fmt.Errorf("write character %s: %w", cd.ID, writeErr)
		}
		fmt.Fprintf(os.Stderr, "seeded %s%s (%s level %d)\n", charKeyPrefix, cd.ID, cd.ClassID, cd.Level)
	}

	// 2. Seed encounter referencing the characters by EntityID.
	if writeErr := writeEncounter(ctx, client, encData); writeErr != nil {
		return fmt.Errorf("write encounter: %w", writeErr)
	}

	// encMode is a core.EncounterMode (int with a String() method); use %v
	// so the formatter resolves to .String() unambiguously (Copilot review
	// nit on PR #543 — %s also works via fmt.Stringer dispatch, but %v is
	// more idiomatic for non-string types).
	fmt.Fprintf(
		os.Stderr,
		"seeded %s%s: mode=%v players=%d monsters=%d ttl=%s\n",
		encKeyPrefix, encData.ID, encMode, len(encData.Players), len(encData.Monsters), encTTL,
	)
	return nil
}

// runInjectCombat implements --inject-combat: a thin CLI wrapper over
// devcombat.Inject (internal/pkg/devcombat), the shared code path
// cmd/devseed and the integration-test suite both call so the CLI mode and
// the test coverage exercise identical logic (package main cannot be
// imported by other packages, so the actual load/AddMonster/SetMode/save
// logic — including its known-gap documentation — lives in that package, not
// here). See devcombat.Inject's doc comment for what it does and why.
func runInjectCombat(ctx context.Context, client redisclient.Client, encounterID string, forceNPCFirst bool) error {
	repo := encountersv2.NewRedis(client, encTTL)

	out, err := devcombat.Inject(ctx, repo, devcombat.InjectInput{
		EncounterID:   encounterID,
		ForceNPCFirst: forceNPCFirst,
	})
	if err != nil {
		if errors.Is(err, encountersv2.ErrNotFound) {
			return fmt.Errorf(
				"encounter %q not found: --inject-combat requires an EXISTING encounter "+
					"(e.g. one created via the lobby StartEncounter RPC, or a prior devseed run)",
				encounterID,
			)
		}
		return err
	}

	if out.AlreadySetTurnBased {
		fmt.Fprintf(os.Stderr, "devseed --inject-combat: encounter %s was already TURN_BASED, re-rolled initiative to include the new monster\n", encounterID)
	}
	if forceNPCFirst {
		fmt.Fprintf(os.Stderr, "devseed --inject-combat-npc-first: goblin %s forced to lead initiative (rpg-api#636 repro)\n", out.GoblinID)
	}
	fmt.Fprintf(
		os.Stderr,
		"devseed --inject-combat: encounter=%s%s goblin=%s pos=(%d,%d,%d) mode=%v players=%d monsters=%d\n",
		encKeyPrefix, encounterID, out.GoblinID, out.Position.Q, out.Position.R, out.Position.S,
		out.Mode, out.PlayerCount, out.MonsterCount,
	)
	return nil
}

// parseMode normalizes the --mode flag against the SDK's EncounterMode enum.
// Returns an error rather than a default so typos surface loudly.
func parseMode(s string) (encountercore.EncounterMode, error) {
	switch strings.ToLower(s) {
	case modeTurnBased:
		return encountercore.ModeTurnBased, nil
	case modeFreeRoam:
		return encountercore.ModeFreeRoam, nil
	default:
		return 0, fmt.Errorf("unknown mode %q (expected %q or %q)", s, modeTurnBased, modeFreeRoam)
	}
}

// writeCharacter serializes the character via the same envelope shape the
// production character repo uses (entities.Character{Data: ...}) and SETs
// it under character:<id> with no TTL. Also SADDs the character ID into
// the player-index set so ListByPlayerID returns seeded characters — matches
// the production Create flow in internal/repositories/character/redis.go
// (Copilot review feedback on PR #543).
func writeCharacter(ctx context.Context, client redisclient.Client, data *toolkitchar.Data) error {
	if data == nil || data.ID == "" {
		return errors.New("character data nil or missing ID")
	}
	// Match entities.Character envelope: {"data": {...}}. Appearance is
	// omitted (the production character repo round-trips it as omitempty;
	// the harness does not require it for combat-only verification).
	envelope := map[string]any{"data": data}
	blob, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	// Atomic write: character blob + player-index SADD in a single
	// TxPipeline, matching the production repo's Create implementation
	// (internal/repositories/character/redis.go:99-114). Session index is
	// intentionally skipped — devseed never seeds session membership.
	pipe := client.TxPipeline()
	pipe.Set(ctx, charKeyPrefix+data.ID, blob, 0)
	if data.PlayerID != "" {
		pipe.SAdd(ctx, playerIndexPrefix+data.PlayerID, data.ID)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return fmt.Errorf("redis pipeline exec: %w", err)
	}
	return nil
}

// writeEncounter SETs the encounter under enc:v2:<id> with the same TTL the
// production v2 encounter repo uses (24h).
func writeEncounter(ctx context.Context, client redisclient.Client, data *tkenc.Data) error {
	if data == nil || data.ID == "" {
		return errors.New("encounter data nil or missing ID")
	}
	blob, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	if err := client.Set(ctx, encKeyPrefix+string(data.ID), blob, encTTL).Err(); err != nil {
		return fmt.Errorf("redis SET: %w", err)
	}
	return nil
}

// buildAliceRogueData mirrors the level-2 rogue fixture used by
// integration_sneak_attack_test.go (DEX 16 > STR 12, shortsword in main
// hand, SneakAttack condition persisted). LoadFromData rebuilds the live
// SneakAttack subscriber on every encounter rehydration; UsedThisTurn
// round-trips through the JSON via rpg-toolkit#655.
func buildAliceRogueData() *toolkitchar.Data {
	sneakCond := conditions.NewSneakAttackCondition(conditions.SneakAttackInput{
		CharacterID: string(entityAlice),
		Level:       2, // 1d6 sneak die at level 2
	})
	sneakJSON, err := sneakCond.ToJSON()
	if err != nil {
		// Constant inputs — a marshal failure here would be a toolkit bug.
		panic(fmt.Errorf("alice sneak attack ToJSON: %w", err))
	}

	now := time.Now().UTC()
	return &toolkitchar.Data{
		ID:               string(entityAlice),
		PlayerID:         string(playerAlice),
		Name:             "Alice the Rogue",
		Level:            2,
		ProficiencyBonus: 2,
		ClassID:          classes.Rogue,
		HitPoints:        16,
		MaxHitPoints:     16,
		ArmorClass:       14, // 11 (leather) + DEX(+3)
		AbilityScores: shared.AbilityScores{
			abilities.STR: 12, // +1
			abilities.DEX: 16, // +3 — finesse weapons use DEX
			abilities.CON: 14,
			abilities.INT: 12,
			abilities.WIS: 10,
			abilities.CHA: 10,
		},
		EquipmentSlots: toolkitchar.EquipmentSlots{
			toolkitchar.SlotMainHand: weaponShortsword,
			toolkitchar.SlotArmor:    armorLeather,
		},
		Inventory: []toolkitchar.InventoryItemData{
			{Type: inventoryTypeWeapon, ID: weaponShortsword, Quantity: 1},
			{Type: inventoryTypeArmor, ID: armorLeather, Quantity: 1},
		},
		Conditions: []json.RawMessage{sneakJSON},
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

// buildBobBarbarianData builds a level-1 barbarian with the Rage feature
// persisted (so the API's Activate flow can call it) and a RageCharges
// resource sized for level 1 (2 uses, recovers on long rest). Rage is NOT
// pre-applied — bob starts not-raging; the player activates Rage as a
// bonus action.
//
// applyReactionConditions (rpg-api side) Apply()s OpportunityAttack at
// LoadFromData time for every melee combatant; default readiness is ON
// (seeded by the encounter SDK's AddPlayer for combatants with DamageDice).
func buildBobBarbarianData() *toolkitchar.Data {
	now := time.Now().UTC()
	rageFeatureJSON := json.RawMessage(fmt.Sprintf(`{
		"ref": %q,
		"id": "rage",
		"name": "Rage",
		"level": 1
	}`, refs.Features.Rage()))

	return &toolkitchar.Data{
		ID:               string(entityBob),
		PlayerID:         string(playerBob),
		Name:             "Bob the Barbarian",
		Level:            1,
		ProficiencyBonus: 2,
		ClassID:          classes.Barbarian,
		HitPoints:        14,
		MaxHitPoints:     14,
		ArmorClass:       14, // Unarmored Defense: 10 + DEX(+1) + CON(+3)
		AbilityScores: shared.AbilityScores{
			abilities.STR: 16, // +3 — barbarian primary
			abilities.DEX: 13, // +1
			abilities.CON: 16, // +3
			abilities.INT: 8,
			abilities.WIS: 12,
			abilities.CHA: 10,
		},
		EquipmentSlots: toolkitchar.EquipmentSlots{
			toolkitchar.SlotMainHand: weaponGreataxe, // two-handed; off hand stays empty
		},
		Inventory: []toolkitchar.InventoryItemData{
			{Type: inventoryTypeWeapon, ID: weaponGreataxe, Quantity: 1},
		},
		Features: []json.RawMessage{rageFeatureJSON},
		Resources: map[coreResources.ResourceKey]toolkitchar.RecoverableResourceData{
			// Level 1 barbarian: 2 rage uses per long rest.
			dnd5eResources.RageCharges: {
				Current:   2,
				Maximum:   2,
				ResetType: coreResources.ResetLongRest,
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// buildWendyWizardData builds a level-1 wizard with 1st-level spell slots
// so applyReactionConditions.hasFirstLevelSpellSlot returns true and Shield
// gets Apply()'d at LoadFromData time. Mirrors the wendy fixture in
// integration_player_shield_test.go.
//
// AC 12 (mage armor not active) keeps Shield's reactive band easy to hit
// in MCP playtest scenarios: any goblin attack roll in [12, 16] would be
// blocked if Shield were ready.
//
// SpellSlots survives Get→LoadFromData→ToData thanks to rpg-toolkit#660
// (landed in v0.58.1). Pre-fix, this field would silently drop on
// rehydration and Shield never Apply()'d.
func buildWendyWizardData() *toolkitchar.Data {
	now := time.Now().UTC()
	return &toolkitchar.Data{
		ID:               string(entityWendy),
		PlayerID:         string(playerWendy),
		Name:             "Wendy the Wizard",
		Level:            1,
		ProficiencyBonus: 2,
		ClassID:          classes.Wizard,
		HitPoints:        8,
		MaxHitPoints:     8,
		ArmorClass:       12,
		AbilityScores: shared.AbilityScores{
			abilities.STR: 8,
			abilities.DEX: 14, // +2
			abilities.CON: 12,
			abilities.INT: 16, // +3 — wizard primary
			abilities.WIS: 10,
			abilities.CHA: 10,
		},
		SpellSlots: map[int]toolkitchar.SpellSlotData{
			1: {Max: 2, Used: 0},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// buildEncounterData seeds the encounter with all three players + the
// goblin, then flips it into the requested mode. Positions match the
// rpg-dnd5e-web PlaytestHarness SEEDED_FALLBACK so the harness renders
// the same scene whether or not its initial Subscribe has flushed.
//
// The encounter SDK is constructed with a fresh in-memory broker because
// devseed never publishes events; the broker is only used by encounter
// methods that emit (AddPlayer, SetMode). The persisted Data is what
// matters — when rpg-api LoadFromData()'s the encounter later, it builds
// its own broker.
func buildEncounterData(encounterID string, mode encountercore.EncounterMode) (*tkenc.Data, error) {
	transport := tkenc.NewInMemoryTransport()
	defer func() { _ = transport.Close() }()
	broker := tkenc.NewBroker(transport)
	defer func() { _ = broker.Close() }()

	enc := tkenc.New(context.Background(), encountercore.EncounterID(encounterID), broker)

	// alice (rogue) — finesse, DEX +3, shortsword 1d6+3. Adjacent to bob so
	// bob can be the sneak-eligibility ally check pivot.
	if err := enc.AddPlayer(tkenc.PlayerInput{
		PlayerID:    playerAlice,
		EntityID:    entityAlice,
		Position:    encountercore.Hex{Q: 0, R: 0, S: 0},
		SightRange:  10,
		HP:          16,
		MaxHP:       16,
		AC:          14,
		AttackBonus: 5,
		DamageDice:  "1d6+3",
		DamageType:  damageTypePiercing,
	}); err != nil {
		return nil, fmt.Errorf("add alice: %w", err)
	}

	// bob (barbarian) — greataxe 1d12+3. Placed adjacent to alice so the
	// sneak-attack ally adjacency check triggers when alice attacks an
	// enemy bob is adjacent to (via the encounter chain).
	if err := enc.AddPlayer(tkenc.PlayerInput{
		PlayerID:    playerBob,
		EntityID:    entityBob,
		Position:    encountercore.Hex{Q: 1, R: -1, S: 0},
		SightRange:  10,
		HP:          14,
		MaxHP:       14,
		AC:          14,
		AttackBonus: 5,
		DamageDice:  "1d12+3",
		DamageType:  damageTypeSlashing,
	}); err != nil {
		return nil, fmt.Errorf("add bob: %w", err)
	}

	// wendy (wizard) — placed back, fire bolt 1d10. DamageDice set so the
	// SDK qualifies her as a combatant for TakeAction; AttackBonus uses
	// INT mod (+3) + proficiency (+2).
	if err := enc.AddPlayer(tkenc.PlayerInput{
		PlayerID:    playerWendy,
		EntityID:    entityWendy,
		Position:    encountercore.Hex{Q: -1, R: 1, S: 0},
		SightRange:  10,
		HP:          8,
		MaxHP:       8,
		AC:          12,
		AttackBonus: 5,
		DamageDice:  "1d10",
		DamageType:  damageTypeFire,
	}); err != nil {
		return nil, fmt.Errorf("add wendy: %w", err)
	}

	// goblin: built via monster.NewGoblin so AbilityScores, actions, and
	// speed are SRD-canonical. DataJSON enables Dnd5eCombatResolver's
	// rehydration path (vs the AttackBonus/DamageDice snapshot fallback).
	goblin := monster.NewGoblin(string(entityGoblin))
	goblinData := goblin.ToData()
	goblinDataJSON, err := json.Marshal(goblinData)
	if err != nil {
		return nil, fmt.Errorf("marshal goblin data: %w", err)
	}

	if err := enc.AddMonster(tkenc.MonsterInput{
		ID:          entityGoblin,
		Position:    encountercore.Hex{Q: 3, R: -1, S: -2},
		HP:          goblinData.HitPoints,
		MaxHP:       goblinData.MaxHitPoints,
		AC:          goblinData.ArmorClass,
		Speed:       6, // 30ft / 5ft per hex
		MonsterRef:  monsterRefGoblin,
		AttackBonus: 4, // matches goblin's NewScimitarAction
		DamageDice:  goblinBaseDamageDice,
		DamageType:  damageTypeSlashing,
		DataJSON:    goblinDataJSON,
	}); err != nil {
		return nil, fmt.Errorf("add goblin: %w", err)
	}

	// SetMode is a no-op for FreeRoam-from-fresh-create (the SDK's NewData
	// already defaults to FreeRoam); only call it for TurnBased so the SDK
	// rolls initiative and seeds ActiveIdx/Round.
	if mode == encountercore.ModeTurnBased {
		if err := enc.SetMode(encountercore.ModeTurnBased); err != nil {
			return nil, fmt.Errorf("set mode turn_based: %w", err)
		}
	}

	return enc.ToData(), nil
}

// buildCharliMonkData returns a level-1 monk fixture for the wave-2-monk scenario.
// Charli: DEX 16 (+3) / WIS 14 (+2) / CON 14 (+2), HP 10, AC 15, ProficiencyBonus 2.
// No weapon equipped (unarmed strike via Martial Arts). No armor (Unarmored Defense active).
// Conditions: MartialArts + UnarmoredDefense(Monk) persisted so they rehydrate on LoadFromData.
func buildCharliMonkData() *toolkitchar.Data {
	martialArtsCond := conditions.NewMartialArtsCondition(conditions.MartialArtsInput{
		CharacterID: string(entityCharli),
		MonkLevel:   1,
	})
	martialArtsJSON, err := martialArtsCond.ToJSON()
	if err != nil {
		panic(fmt.Errorf("charli martial arts ToJSON: %w", err))
	}

	unarmoredDefenseCond := conditions.NewUnarmoredDefenseCondition(conditions.UnarmoredDefenseInput{
		CharacterID: string(entityCharli),
		Type:        conditions.UnarmoredDefenseMonk,
		Source:      "dnd5e:classes:monk",
	})
	unarmoredDefenseJSON, err := unarmoredDefenseCond.ToJSON()
	if err != nil {
		panic(fmt.Errorf("charli unarmored defense ToJSON: %w", err))
	}

	now := time.Now().UTC()
	return &toolkitchar.Data{
		ID:               string(entityCharli),
		PlayerID:         string(playerCharli),
		Name:             "Charli the Monk",
		Level:            1,
		ProficiencyBonus: 2,
		ClassID:          classes.Monk,
		HitPoints:        10,
		MaxHitPoints:     10,
		ArmorClass:       15, // 10 + DEX(+3) + WIS(+2) via Unarmored Defense
		AbilityScores: shared.AbilityScores{
			abilities.STR: 10, // +0 — low STR forces Martial Arts DEX swap visible in test
			abilities.DEX: 16, // +3
			abilities.CON: 14, // +2
			abilities.INT: 10,
			abilities.WIS: 14, // +2 — AC = 10 + 3 + 2 = 15
			abilities.CHA: 10,
		},
		// No weapon equipped: unarmed strike path triggers Martial Arts condition
		Conditions: []json.RawMessage{martialArtsJSON, unarmoredDefenseJSON},
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

// buildWave2MonkEncounterData seeds the wave-2-monk encounter: charli (monk, unarmed) adjacent
// to a standard goblin. No other players. Charli uses 1d4+DEX via Martial Arts.
func buildWave2MonkEncounterData(encounterID string, mode encountercore.EncounterMode) (*tkenc.Data, error) {
	transport := tkenc.NewInMemoryTransport()
	defer func() { _ = transport.Close() }()
	broker := tkenc.NewBroker(transport)
	defer func() { _ = broker.Close() }()

	enc := tkenc.New(context.Background(), encountercore.EncounterID(encounterID), broker)

	// charli (monk, unarmed) — placed at origin, adjacent to goblin at Q:1.
	// AttackBonus: DEX +3 + proficiency +2 = 5.
	if err := enc.AddPlayer(tkenc.PlayerInput{
		PlayerID:    playerCharli,
		EntityID:    entityCharli,
		Position:    encountercore.Hex{Q: 0, R: 0, S: 0},
		SightRange:  10,
		HP:          10,
		MaxHP:       10,
		AC:          15,
		AttackBonus: 5,
		DamageDice:  "1d4+3",
		DamageType:  damageTypeBludgeoning,
	}); err != nil {
		return nil, fmt.Errorf("add charli: %w", err)
	}

	goblin := monster.NewGoblin(string(entityGoblin))
	goblinData := goblin.ToData()
	goblinDataJSON, err := json.Marshal(goblinData)
	if err != nil {
		return nil, fmt.Errorf("marshal goblin data: %w", err)
	}

	// Goblin adjacent to charli (Q:1 R:0 — euclidean distance 1.0 ≤ 1.5).
	if err := enc.AddMonster(tkenc.MonsterInput{
		ID:          entityGoblin,
		Position:    encountercore.Hex{Q: 1, R: 0, S: -1},
		HP:          goblinData.HitPoints,
		MaxHP:       goblinData.MaxHitPoints,
		AC:          goblinData.ArmorClass,
		Speed:       6,
		MonsterRef:  monsterRefGoblin,
		AttackBonus: 4,
		DamageDice:  goblinBaseDamageDice,
		DamageType:  damageTypeSlashing,
		DataJSON:    goblinDataJSON,
	}); err != nil {
		return nil, fmt.Errorf("add goblin: %w", err)
	}

	if mode == encountercore.ModeTurnBased {
		if err := enc.SetMode(encountercore.ModeTurnBased); err != nil {
			return nil, fmt.Errorf("set mode turn_based: %w", err)
		}
	}

	return enc.ToData(), nil
}

// buildFinnFighterData returns a level-1 fighter fixture for the
// wave-2-beat2 scenario (design doc: "the other three brothers — Fighter
// recommended, simplest to read a 'gets hit' beat off"). Chain mail (no
// shield) + longsword, STR-based. HP is deliberately modest (12) so a
// couple of goblin hits plausibly drive it to 0 for the death-gate beat.
func buildFinnFighterData() *toolkitchar.Data {
	now := time.Now().UTC()
	return &toolkitchar.Data{
		ID:               string(entityFinn),
		PlayerID:         string(playerFinn),
		Name:             "Finn the Fighter",
		Level:            1,
		ProficiencyBonus: 2,
		ClassID:          classes.Fighter,
		HitPoints:        12,
		MaxHitPoints:     12,
		ArmorClass:       16, // chain mail (16), MaxDexBonus 0 — no shield
		AbilityScores: shared.AbilityScores{
			abilities.STR: 16, // +3 — fighter primary, longsword
			abilities.DEX: 12,
			abilities.CON: 14, // +2
			abilities.INT: 10,
			abilities.WIS: 10,
			abilities.CHA: 8,
		},
		EquipmentSlots: toolkitchar.EquipmentSlots{
			toolkitchar.SlotMainHand: weaponLongsword,
			toolkitchar.SlotArmor:    armorChainMail,
		},
		Inventory: []toolkitchar.InventoryItemData{
			{Type: inventoryTypeWeapon, ID: weaponLongsword, Quantity: 1},
			{Type: inventoryTypeArmor, ID: armorChainMail, Quantity: 1},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
}

// buildWave2Beat2EncounterData seeds the wave-2-beat2 encounter (Beat 2 —
// mechanical effects: Dodge/Help/Hide, rpg-project#75): charli (monk) +
// finn (fighter) + one goblin, per the design doc's devseed-fixture section.
//
// Positions: charli and finn start adjacent to each other; the goblin starts
// two hexes from finn — well within its 6-hex (30ft) move speed, satisfying
// "positioned so the goblin can reach the Fighter in one move" — and close
// enough to also observe charli for the Hide beat (perception.CanSeeAt is
// currently a range-only stub, so exact LOS geometry isn't required).
//
// The goblin's PassivePerception is overridden to beat2GoblinPassivePerception
// (well below its real SRD value) so the Hide beat's Stealth check succeeds
// reliably rather than probabilistically, per the design doc's R6 scope
// decision (only Hide-success is exercised live).
func buildWave2Beat2EncounterData(encounterID string, mode encountercore.EncounterMode) (*tkenc.Data, error) {
	transport := tkenc.NewInMemoryTransport()
	defer func() { _ = transport.Close() }()
	broker := tkenc.NewBroker(transport)
	defer func() { _ = broker.Close() }()

	enc := tkenc.New(context.Background(), encountercore.EncounterID(encounterID), broker)

	// charli (monk, unarmed) — same stat line as wave-2-monk.
	if err := enc.AddPlayer(tkenc.PlayerInput{
		PlayerID:    playerCharli,
		EntityID:    entityCharli,
		Position:    encountercore.Hex{Q: 0, R: 0, S: 0},
		SightRange:  10,
		HP:          10,
		MaxHP:       10,
		AC:          15,
		AttackBonus: 5,
		DamageDice:  "1d4+3",
		DamageType:  damageTypeBludgeoning,
	}); err != nil {
		return nil, fmt.Errorf("add charli: %w", err)
	}

	// finn (fighter, longsword) — adjacent to charli. AttackBonus: STR +3 +
	// proficiency +2 = 5.
	if err := enc.AddPlayer(tkenc.PlayerInput{
		PlayerID:    playerFinn,
		EntityID:    entityFinn,
		Position:    encountercore.Hex{Q: -1, R: 0, S: 1},
		SightRange:  10,
		HP:          12,
		MaxHP:       12,
		AC:          16,
		AttackBonus: 5,
		DamageDice:  "1d8+3",
		DamageType:  damageTypeSlashing,
	}); err != nil {
		return nil, fmt.Errorf("add finn: %w", err)
	}

	goblin := monster.NewGoblin(string(entityGoblin))
	goblinData := goblin.ToData()
	goblinData.Senses.PassivePerception = beat2GoblinPassivePerception
	goblinDataJSON, err := json.Marshal(goblinData)
	if err != nil {
		return nil, fmt.Errorf("marshal goblin data: %w", err)
	}

	// Two hexes from finn — within the goblin's 6-hex move speed ("reach the
	// Fighter in one move"), and close enough to observe charli for the Hide
	// beat.
	if err := enc.AddMonster(tkenc.MonsterInput{
		ID:          entityGoblin,
		Position:    encountercore.Hex{Q: 1, R: 0, S: -1},
		HP:          goblinData.HitPoints,
		MaxHP:       goblinData.MaxHitPoints,
		AC:          goblinData.ArmorClass,
		Speed:       6,
		MonsterRef:  monsterRefGoblin,
		AttackBonus: 4,
		DamageDice:  goblinBaseDamageDice,
		DamageType:  damageTypeSlashing,
		DataJSON:    goblinDataJSON,
	}); err != nil {
		return nil, fmt.Errorf("add goblin: %w", err)
	}

	if mode == encountercore.ModeTurnBased {
		if err := enc.SetMode(encountercore.ModeTurnBased); err != nil {
			return nil, fmt.Errorf("set mode turn_based: %w", err)
		}
	}

	return enc.ToData(), nil
}

// buildAliceRogueL1Data is the wave-1-rogue variant of buildAliceRogueData:
// level 1, HP 10. AbilityScores and equipment are identical to the L2 version;
// only Level, HitPoints/MaxHitPoints, and the SneakAttackInput.Level differ
// (L1 = 1d6, same as L2 per (level+1)/2).
func buildAliceRogueL1Data() *toolkitchar.Data {
	sneakCond := conditions.NewSneakAttackCondition(conditions.SneakAttackInput{
		CharacterID: string(entityAlice),
		Level:       1,
	})
	sneakJSON, err := sneakCond.ToJSON()
	if err != nil {
		panic(fmt.Errorf("alice L1 sneak attack ToJSON: %w", err))
	}

	now := time.Now().UTC()
	return &toolkitchar.Data{
		ID:               string(entityAlice),
		PlayerID:         string(playerAlice),
		Name:             "Alice the Rogue",
		Level:            1,
		ProficiencyBonus: 2,
		ClassID:          classes.Rogue,
		HitPoints:        10,
		MaxHitPoints:     10,
		ArmorClass:       14, // 11 (leather) + DEX(+3)
		AbilityScores: shared.AbilityScores{
			abilities.STR: 12,
			abilities.DEX: 16, // +3 — finesse weapons use DEX
			abilities.CON: 12,
			abilities.INT: 12,
			abilities.WIS: 10,
			abilities.CHA: 10,
		},
		EquipmentSlots: toolkitchar.EquipmentSlots{
			toolkitchar.SlotMainHand: weaponShortsword,
			toolkitchar.SlotArmor:    armorLeather,
		},
		Inventory: []toolkitchar.InventoryItemData{
			{Type: inventoryTypeWeapon, ID: weaponShortsword, Quantity: 1},
			{Type: inventoryTypeArmor, ID: armorLeather, Quantity: 1},
		},
		Conditions: []json.RawMessage{sneakJSON},
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

// buildWave1RogueEncounterData seeds the wave-1-rogue encounter: alice + bob +
// goblin only (no wendy). Positions match the default fixture so the harness
// SEEDED_FALLBACK still aligns.
func buildWave1RogueEncounterData(encounterID string, mode encountercore.EncounterMode) (*tkenc.Data, error) {
	transport := tkenc.NewInMemoryTransport()
	defer func() { _ = transport.Close() }()
	broker := tkenc.NewBroker(transport)
	defer func() { _ = broker.Close() }()

	enc := tkenc.New(context.Background(), encountercore.EncounterID(encounterID), broker)

	// alice L1 rogue — DEX +3, shortsword 1d6+3.
	if err := enc.AddPlayer(tkenc.PlayerInput{
		PlayerID:    playerAlice,
		EntityID:    entityAlice,
		Position:    encountercore.Hex{Q: 0, R: 0, S: 0},
		SightRange:  10,
		HP:          10,
		MaxHP:       10,
		AC:          14,
		AttackBonus: 5,
		DamageDice:  "1d6+3",
		DamageType:  damageTypePiercing,
	}); err != nil {
		return nil, fmt.Errorf("add alice: %w", err)
	}

	// bob (barbarian) — adjacent to goblin so ally-adjacency sneak attack triggers.
	if err := enc.AddPlayer(tkenc.PlayerInput{
		PlayerID:    playerBob,
		EntityID:    entityBob,
		Position:    encountercore.Hex{Q: 2, R: -1, S: -1},
		SightRange:  10,
		HP:          14,
		MaxHP:       14,
		AC:          14,
		AttackBonus: 5,
		DamageDice:  "1d12+3",
		DamageType:  damageTypeSlashing,
	}); err != nil {
		return nil, fmt.Errorf("add bob: %w", err)
	}

	goblin := monster.NewGoblin(string(entityGoblin))
	goblinData := goblin.ToData()
	goblinDataJSON, err := json.Marshal(goblinData)
	if err != nil {
		return nil, fmt.Errorf("marshal goblin data: %w", err)
	}

	if err := enc.AddMonster(tkenc.MonsterInput{
		ID:          entityGoblin,
		Position:    encountercore.Hex{Q: 3, R: -1, S: -2},
		HP:          goblinData.HitPoints,
		MaxHP:       goblinData.MaxHitPoints,
		AC:          goblinData.ArmorClass,
		Speed:       6,
		MonsterRef:  monsterRefGoblin,
		AttackBonus: 4,
		DamageDice:  goblinBaseDamageDice,
		DamageType:  damageTypeSlashing,
		DataJSON:    goblinDataJSON,
	}); err != nil {
		return nil, fmt.Errorf("add goblin: %w", err)
	}

	if mode == encountercore.ModeTurnBased {
		if err := enc.SetMode(encountercore.ModeTurnBased); err != nil {
			return nil, fmt.Errorf("set mode turn_based: %w", err)
		}
	}

	return enc.ToData(), nil
}

// buildWave3BarbarianEncounterData seeds the wave-3-barbarian encounter: bob (L1
// barbarian, not raging) adjacent to a goblin with bumped HP. HP is set to
// wave3GoblinHP (30) so raging-bob's greataxe doesn't one-shot it — the
// playtest needs to see BOTH bob's raging attack AND the goblin's attack on
// raging-bob (Rage resistance halves the incoming damage).
func buildWave3BarbarianEncounterData(encounterID string, mode encountercore.EncounterMode) (*tkenc.Data, error) {
	transport := tkenc.NewInMemoryTransport()
	defer func() { _ = transport.Close() }()
	broker := tkenc.NewBroker(transport)
	defer func() { _ = broker.Close() }()

	enc := tkenc.New(context.Background(), encountercore.EncounterID(encounterID), broker)

	// bob (barbarian) — greataxe 1d12+3. Adjacent to goblin at Q:1.
	// AttackBonus: STR +3 + proficiency +2 = 5.
	if err := enc.AddPlayer(tkenc.PlayerInput{
		PlayerID:    playerBob,
		EntityID:    entityBob,
		Position:    encountercore.Hex{Q: 0, R: 0, S: 0},
		SightRange:  10,
		HP:          14,
		MaxHP:       14,
		AC:          14,
		AttackBonus: 5,
		DamageDice:  barbarianGreataxeDice,
		DamageType:  damageTypeSlashing,
	}); err != nil {
		return nil, fmt.Errorf("add bob: %w", err)
	}

	goblin := monster.NewGoblin(string(entityGoblin))
	goblinData := goblin.ToData()
	goblinDataJSON, err := json.Marshal(goblinData)
	if err != nil {
		return nil, fmt.Errorf("marshal goblin data for wave-3-barbarian: %w", err)
	}

	// Goblin adjacent to bob (Q:1 R:0 — euclidean distance 1.0 ≤ 1.5).
	// HP bumped to wave3GoblinHP so raging-bob's greataxe doesn't one-shot it.
	if err := enc.AddMonster(tkenc.MonsterInput{
		ID:          entityGoblin,
		Position:    encountercore.Hex{Q: 1, R: 0, S: -1},
		HP:          wave3GoblinHP,
		MaxHP:       wave3GoblinHP,
		AC:          goblinData.ArmorClass,
		Speed:       6,
		MonsterRef:  monsterRefGoblin,
		AttackBonus: 4,
		DamageDice:  goblinBaseDamageDice,
		DamageType:  damageTypeSlashing,
		DataJSON:    goblinDataJSON,
	}); err != nil {
		return nil, fmt.Errorf("add goblin (wave-3-barbarian): %w", err)
	}

	if mode == encountercore.ModeTurnBased {
		if err := enc.SetMode(encountercore.ModeTurnBased); err != nil {
			return nil, fmt.Errorf("set mode turn_based (wave-3-barbarian): %w", err)
		}
	}

	return enc.ToData(), nil
}
