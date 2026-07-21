package encounter_test

// integration_equipment_test.go — rpg-api#680 sign-off: equip through the
// toolkit's rules engine, real AC, composed stat_line, and two-handed
// occupancy, all observable on the SAME v1alpha2 CharacterData wire shape
// the encounter snapshot serves. This is the "equip→snapshot flow" proof
// the slice's design doc names as its honesty bar.
//
// Flow proven per case: EquipItem/UnequipItem via the v1alpha2 character
// service (handlers/dnd5e/v2/character) — the same rules-correct
// orchestrator method the v1alpha1 handler shares — then GetEncounter via
// the v1alpha2 encounter service, asserting the snapshot's CharacterData
// reflects the change: armor_class_detail.total matches the toolkit's own
// EffectiveAC() (not a stored int), Entity.armor_class stays in sync with
// it, stat_line/name/kind/slot_keys are toolkit-composed pass-through, and
// equipping a two-handed weapon clears off_hand as a side effect of one
// EquipItem call (rpg-toolkit#812's occupancy rule, not reimplemented here).

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	characterpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/character"
	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterhandlerv2 "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v2/character"
	v2encounter "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v2/encounter"
	orchcharacter "github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	dicemock "github.com/KirkDiggler/rpg-api/internal/orchestrators/dice/mock"
	idgenmock "github.com/KirkDiggler/rpg-api/internal/pkg/idgen/mock"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	draftmock "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft/mock"
	encountersv2 "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/v2"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	encountercore "github.com/KirkDiggler/rpg-toolkit/encounter/core"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
)

// EquipmentIntegrationSuite wires the v1alpha2 character service and the
// v1alpha2 encounter service against the SAME mock character repo (backed
// by inMemoryCharStore, defined once in integration_sneak_attack_test.go
// and reused across this package's integration suites), so an equip
// written through one surface is observable through the other — exactly
// the cross-surface flow #680 exists to prove.
type EquipmentIntegrationSuite struct {
	suite.Suite
	ctrl             *gomock.Controller
	mockCharRepo     *charactermock.MockRepository
	charStore        *inMemoryCharStore
	broker           *tkenc.Broker
	repo             encountersv2.Repository
	encHandler       *v2encounter.Handler
	characterHandler *characterhandlerv2.Handler
}

func TestEquipmentIntegrationSuite(t *testing.T) {
	suite.Run(t, new(EquipmentIntegrationSuite))
}

func (s *EquipmentIntegrationSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockCharRepo = charactermock.NewMockRepository(s.ctrl)
	s.charStore = newInMemoryCharStore()
	s.broker = tkenc.NewBroker(tkenc.NewInMemoryTransport())
	s.repo = encountersv2.NewInMemory()

	encHandler, err := v2encounter.New(&v2encounter.HandlerConfig{
		Broker: s.broker,
		Repo:   s.repo,
		CombatResolverConfig: &v2encounter.Dnd5eCombatResolverConfig{
			CharacterRepo: s.mockCharRepo,
		},
	})
	s.Require().NoError(err)
	s.encHandler = encHandler

	// The character orchestrator's Config requires non-nil draft/dice/idgen
	// deps even though EquipItem/UnequipItem/GetCharacter never call them —
	// mocked-but-unused, matching the orchestrator suite's own SetupTest.
	characterService, err := orchcharacter.New(&orchcharacter.Config{
		DraftRepo:        draftmock.NewMockRepository(s.ctrl),
		CharacterRepo:    s.mockCharRepo,
		DiceService:      dicemock.NewMockService(s.ctrl),
		IDGenerator:      idgenmock.NewMockGenerator(s.ctrl),
		DraftIDGenerator: idgenmock.NewMockGenerator(s.ctrl),
	})
	s.Require().NoError(err)

	characterHandler, err := characterhandlerv2.New(&characterhandlerv2.HandlerConfig{
		CharacterService: characterService,
	})
	s.Require().NoError(err)
	s.characterHandler = characterHandler
}

func (s *EquipmentIntegrationSuite) TearDownTest() {
	s.ctrl.Finish()
}

// seedCharRepoMock wires Get/Update for characterID against the shared
// charStore (same DoAndReturn pattern as setupCharRepoMock in
// integration_sneak_attack_test.go / integration_barbarian_rage_test.go):
// Get always reads the current store state, so it sees write-backs from
// prior EquipItem/UnequipItem calls in the same test.
func (s *EquipmentIntegrationSuite) seedCharRepoMock(data *character.Data) {
	s.charStore.set(data)
	id := data.ID

	s.mockCharRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: id}).
		DoAndReturn(func(_ context.Context, _ characterrepo.GetInput) (*characterrepo.GetOutput, error) {
			d := s.charStore.get(id)
			if d == nil {
				return nil, apierr.NotFound("character not found in charStore")
			}
			return &characterrepo.GetOutput{Character: &entities.Character{Data: d}}, nil
		}).AnyTimes()

	s.mockCharRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, input characterrepo.UpdateInput) (*characterrepo.UpdateOutput, error) {
			if input.Character != nil && input.Character.Data != nil {
				s.charStore.update(input.Character.Data)
			}
			return &characterrepo.UpdateOutput{Character: input.Character}, nil
		}).AnyTimes()
}

// seedSoloEncounter creates and saves a single-player encounter with no
// room — playerEntity's viewer-always-sees-self branch (project.go)
// doesn't need Space/walls, so this test only needs a player seat to exist.
func (s *EquipmentIntegrationSuite) seedSoloEncounter(encID, playerID, entityID string) {
	enc := tkenc.New(context.Background(), encountercore.EncounterID(encID), s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID:   encountercore.PlayerID(playerID),
		EntityID:   encountercore.EntityID(entityID),
		Position:   encountercore.Hex{Q: 0, R: 0, S: 0},
		SightRange: 10,
		HP:         10,
		MaxHP:      10,
		AC:         10,
	}))
	s.Require().NoError(s.repo.Save(context.Background(), enc.ToData()))
}

// viewerCharacterData fetches the encounter snapshot for playerID and
// returns their own entity's CharacterData — the exact wire shape
// EquipItem/UnequipItem also return, proving both surfaces agree.
func (s *EquipmentIntegrationSuite) viewerCharacterData(
	ctx context.Context, encID, entityID string,
) *encounterv2pb.CharacterData {
	resp, err := s.encHandler.GetEncounter(ctx, &encounterv2pb.GetEncounterRequest{EncounterId: encID})
	s.Require().NoError(err)

	for _, e := range resp.GetEncounter().GetSpace().GetEntities() {
		if e.GetId() != entityID {
			continue
		}
		s.Require().NotNil(e.GetArmorClass(), "Entity.armor_class must be set once equipment is projected")
		cd := e.GetCharacter()
		s.Require().NotNil(cd)
		s.Require().NotNil(cd.GetArmorClassDetail(),
			"armor_class_detail must stay in sync with Entity.armor_class (rpg-api#680 gate)")
		s.Assert().Equal(e.GetArmorClass(), cd.GetArmorClassDetail().GetTotal(),
			"Entity.armor_class and armor_class_detail.total must be the same value")
		return cd
	}
	s.Require().Fail("viewer entity not found in snapshot")
	return nil
}

// abilityScoresWithDex builds a fixed ability-score block with only DEX
// varying — the AC cases below need distinct DEX values (chain mail caps
// its DEX bonus at 0, so DEX is irrelevant there; leather has an unlimited
// DEX bonus, so DEX drives the total) without dragging every other score
// into the table.
func abilityScoresWithDex(dex int) shared.AbilityScores {
	return shared.AbilityScores{
		abilities.STR: 14,
		abilities.DEX: dex,
		abilities.CON: 14,
		abilities.INT: 10,
		abilities.WIS: 10,
		abilities.CHA: 8,
	}
}

// TestIntegration_EquipItem_RealAC is gate-finding-3's fix: the fighter and
// rogue cases equip ARMOR (not a weapon — a weapon equip never changes AC
// at all, which was the flaw in the prior version of this test) and assert
// against a HAND-COMPUTED expected AC from the D&D armor tables
// (armor.go's ChainMail/Leather base AC + DEX-bonus rule), never by calling
// EffectiveAC a second time — that would just be EffectiveAC-vs-EffectiveAC,
// proving nothing. Both rows seed a deliberately stale ArmorClass: 10 and
// confirm the wire never serves it, and the two rows land on genuinely
// different totals (16 vs 14), not two reskins of the same case.
func (s *EquipmentIntegrationSuite) TestIntegration_EquipItem_RealAC() {
	tests := []struct {
		name        string
		characterID string
		playerID    string
		className   string
		dex         int
		itemID      string
		wantKind    string
		wantAC      int32 // hand-computed from the D&D armor table, independent of EffectiveAC
		statLineHas string
	}{
		{
			// Chain mail: base AC 16, MaxDexBonus 0 (armor.go) — DEX is
			// irrelevant here, so the fixed 16 is the whole expectation.
			name:        "fighter equips chain mail (heavy, no DEX bonus) -> fixed AC 16",
			characterID: "char-equip-fighter",
			playerID:    "player-equip-fighter",
			className:   "fighter",
			dex:         12,
			itemID:      "chain-mail",
			wantKind:    "armor",
			wantAC:      16,
			statLineHas: "AC 16",
		},
		{
			// Leather: base AC 11, unlimited DEX bonus (armor.go) -> 11 + 3 (DEX 16 = +3 mod) = 14.
			name:        "rogue equips leather (light, unlimited DEX bonus) -> AC 11+DEX",
			characterID: "char-equip-rogue",
			playerID:    "player-equip-rogue",
			className:   "rogue",
			dex:         16,
			itemID:      "leather",
			wantKind:    "armor",
			wantAC:      14,
			statLineHas: "AC 11",
		},
	}

	for _, tc := range tests {
		s.Run(tc.name, func() {
			data := &character.Data{
				ID:               tc.characterID,
				Name:             tc.name,
				Level:            1,
				ClassID:          tc.className,
				ProficiencyBonus: 2,
				HitPoints:        10,
				MaxHitPoints:     10,
				ArmorClass:       10, // stale stored int — the whole point is the wire must NOT serve this.
				AbilityScores:    abilityScoresWithDex(tc.dex),
				Inventory: []character.InventoryItemData{
					{Type: "armor", ID: tc.itemID, Quantity: 1},
				},
				EquipmentSlots: character.EquipmentSlots{},
			}
			s.seedCharRepoMock(data)

			encID := "enc-equip-" + tc.characterID
			s.seedSoloEncounter(encID, tc.playerID, tc.characterID)
			ctx := auth.WithPlayerID(context.Background(), tc.playerID)

			equipResp, err := s.characterHandler.EquipItem(ctx, &characterpb.EquipItemRequest{
				CharacterId: tc.characterID,
				Item:        &encounterv2pb.Ref{Module: "dnd5e", Type: "item", Id: tc.itemID},
				SlotKey:     "armor",
			})
			s.Require().NoError(err)

			// Assertion pasted to 'main': AC on the wire matches the hand-computed
			// D&D armor-table value, not a stored int (converters.go's #680 sin,
			// now killed) — and not just EffectiveAC compared against itself.
			s.Assert().Equal(tc.wantAC, equipResp.GetCharacter().GetArmorClassDetail().GetTotal(),
				"EquipItemResponse AC must equal the hand-computed armor-table total, not the stale stored ArmorClass")
			s.Assert().NotEqual(int32(10), equipResp.GetCharacter().GetArmorClassDetail().GetTotal(),
				"AC must have moved off the stale seeded stored int (10) once equipment is real")

			// Same assertion again against the encounter snapshot: one composition,
			// two callers, never drifting (rpg-api#680's shared BuildEquipmentCharacterData).
			snapCD := s.viewerCharacterData(ctx, encID, tc.characterID)
			s.Assert().Equal(tc.wantAC, snapCD.GetArmorClassDetail().GetTotal())

			s.Require().Contains(snapCD.GetEquipped(), "armor")
			s.Assert().Equal(tc.itemID, snapCD.GetEquipped()["armor"].GetId())

			var found *encounterv2pb.Item
			for _, item := range snapCD.GetInventory() {
				if item.GetRef().GetId() == tc.itemID {
					found = item
				}
			}
			s.Require().NotNil(found, "equipped item must still be listed in inventory (contract: inventory includes equipped)")
			s.Assert().Equal(tc.wantKind, found.GetKind())
			s.Assert().NotEmpty(found.GetName())
			s.Assert().Contains(found.GetStatLine(), tc.statLineHas)
			s.Assert().NotEmpty(found.GetSlotKeys())
		})
	}
}

// TestIntegration_EquipItem_TwoHanded_ClearsOffHandOnSnapshot covers the
// barbarian cast: a shield is already equipped in off_hand; equipping a
// two-handed greataxe into main_hand must clear off_hand as a side effect
// of the SAME EquipItem call (rpg-toolkit#812's occupancy rule) — and that
// side effect must be visible on the encounter snapshot, not just the
// EquipItemResponse.
func (s *EquipmentIntegrationSuite) TestIntegration_EquipItem_TwoHanded_ClearsOffHandOnSnapshot() {
	const (
		characterID = "char-equip-barbarian"
		playerID    = "player-equip-barbarian"
		encID       = "enc-equip-barbarian"
	)

	data := &character.Data{
		ID:               characterID,
		Name:             "Test Barbarian",
		Level:            1,
		ClassID:          "barbarian",
		ProficiencyBonus: 2,
		HitPoints:        14,
		MaxHitPoints:     14,
		ArmorClass:       10,
		AbilityScores: shared.AbilityScores{
			abilities.STR: 16,
			abilities.DEX: 13,
			abilities.CON: 16,
			abilities.INT: 8,
			abilities.WIS: 12,
			abilities.CHA: 10,
		},
		Inventory: []character.InventoryItemData{
			{Type: "weapon", ID: "greataxe", Quantity: 1},
			{Type: "armor", ID: "shield", Quantity: 1},
		},
		EquipmentSlots: character.EquipmentSlots{
			character.SlotOffHand: "shield",
		},
	}
	s.seedCharRepoMock(data)
	s.seedSoloEncounter(encID, playerID, characterID)
	ctx := auth.WithPlayerID(context.Background(), playerID)

	// Precondition, asserted through the real wire: off_hand starts occupied.
	preSnap := s.viewerCharacterData(ctx, encID, characterID)
	s.Require().Contains(preSnap.GetEquipped(), "off_hand")

	equipResp, err := s.characterHandler.EquipItem(ctx, &characterpb.EquipItemRequest{
		CharacterId: characterID,
		Item:        &encounterv2pb.Ref{Module: "dnd5e", Type: "item", Id: "greataxe"},
		SlotKey:     "main_hand",
	})
	s.Require().NoError(err)

	s.Assert().Equal("greataxe", equipResp.GetCharacter().GetEquipped()["main_hand"].GetId())
	s.Assert().NotContains(equipResp.GetCharacter().GetEquipped(), "off_hand",
		"equipping a two-handed weapon must clear off_hand on the EquipItemResponse")

	// The occupancy side effect must also be visible on the encounter
	// snapshot — the flow this test exists to prove, not just the RPC's
	// own echo.
	snapCD := s.viewerCharacterData(ctx, encID, characterID)
	s.Assert().Equal("greataxe", snapCD.GetEquipped()["main_hand"].GetId())
	s.Assert().NotContains(snapCD.GetEquipped(), "off_hand")

	// Shield stays in inventory, just unequipped — never dropped (toolkit
	// swap-on-occupied semantics, rpg-toolkit#812).
	var shieldStillOwned bool
	var greataxeItem *encounterv2pb.Item
	for _, item := range snapCD.GetInventory() {
		if item.GetRef().GetId() == "shield" {
			shieldStillOwned = true
		}
		if item.GetRef().GetId() == "greataxe" {
			greataxeItem = item
		}
	}
	s.Assert().True(shieldStillOwned, "unequipped shield must remain in inventory, not be dropped")

	// Weapon display-field coverage (stat_line/kind/slot_keys), the
	// counterpart to the armor-focused AC test above.
	s.Require().NotNil(greataxeItem)
	s.Assert().Equal("weapon", greataxeItem.GetKind())
	s.Assert().Contains(greataxeItem.GetStatLine(), "slashing")
	s.Assert().Contains(greataxeItem.GetSlotKeys(), "main_hand")
	s.Assert().NotContains(greataxeItem.GetSlotKeys(), "off_hand",
		"a two-handed weapon's SlotKeys must not list off_hand as independently valid")
}

// TestIntegration_UnequipItem_RemovesFromSnapshot proves UnequipItem's
// effect is visible the same way EquipItem's is: the occupant returns to
// inventory (unequipped, not deleted) and disappears from `equipped`.
func (s *EquipmentIntegrationSuite) TestIntegration_UnequipItem_RemovesFromSnapshot() {
	const (
		characterID = "char-equip-unequip-rogue"
		playerID    = "player-equip-unequip-rogue"
		encID       = "enc-equip-unequip-rogue"
	)

	data := &character.Data{
		ID:               characterID,
		Name:             "Test Rogue",
		Level:            1,
		ClassID:          "rogue",
		ProficiencyBonus: 2,
		HitPoints:        9,
		MaxHitPoints:     9,
		ArmorClass:       10,
		AbilityScores: shared.AbilityScores{
			abilities.STR: 10,
			abilities.DEX: 16,
			abilities.CON: 12,
			abilities.INT: 10,
			abilities.WIS: 12,
			abilities.CHA: 14,
		},
		Inventory: []character.InventoryItemData{
			{Type: "weapon", ID: "dagger", Quantity: 1},
		},
		EquipmentSlots: character.EquipmentSlots{
			character.SlotMainHand: "dagger",
		},
	}
	s.seedCharRepoMock(data)
	s.seedSoloEncounter(encID, playerID, characterID)
	ctx := auth.WithPlayerID(context.Background(), playerID)

	unequipResp, err := s.characterHandler.UnequipItem(ctx, &characterpb.UnequipItemRequest{
		CharacterId: characterID,
		SlotKey:     "main_hand",
	})
	s.Require().NoError(err)
	s.Assert().NotContains(unequipResp.GetCharacter().GetEquipped(), "main_hand")

	snapCD := s.viewerCharacterData(ctx, encID, characterID)
	s.Assert().NotContains(snapCD.GetEquipped(), "main_hand")

	var daggerStillOwned bool
	for _, item := range snapCD.GetInventory() {
		if item.GetRef().GetId() == "dagger" {
			daggerStillOwned = true
		}
	}
	s.Assert().True(daggerStillOwned, "unequipped dagger must remain in inventory")
}
