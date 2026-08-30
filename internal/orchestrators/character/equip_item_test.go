package character

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"reflect"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	dicemock "github.com/KirkDiggler/rpg-api/internal/orchestrators/dice/mock"
	idgenmock "github.com/KirkDiggler/rpg-api/internal/pkg/idgen/mock"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	draftmock "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft/mock"
	coreResources "github.com/KirkDiggler/rpg-toolkit/core/resources"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/backgrounds"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/conditions"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/resources"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
)

// EquipItemTestSuite proves the rules-correct equip/unequip path (rpg-api#680):
// the orchestrator must route through the toolkit's rules engine — occupancy,
// slot-compatibility, swap-on-occupied — rather than writing EquipmentSlots
// directly. Kept as its own suite (not folded into OrchestratorTestSuite) so
// the fixtures stay equipment-focused and don't drag in draft/class/race
// setup this slice doesn't need.
const (
	testCharacterRepositoryVersion = "version-1"
	expectedMissingVersionMessage  = "character repository contract violation: missing character version"
)

type EquipItemTestSuite struct {
	suite.Suite
	ctrl              *gomock.Controller
	mockCharacterRepo *charactermock.MockRepository
	orchestrator      *Orchestrator
	ctx               context.Context

	testCharacterID string
}

func TestEquipItemSuite(t *testing.T) {
	suite.Run(t, new(EquipItemTestSuite))
}

func (s *EquipItemTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockCharacterRepo = charactermock.NewMockRepository(s.ctrl)
	s.ctx = context.Background()
	s.testCharacterID = "char-fighter-1"

	var err error
	s.orchestrator, err = New(&Config{
		DraftRepo:        draftmock.NewMockRepository(s.ctrl),
		CharacterRepo:    s.mockCharacterRepo,
		DiceService:      dicemock.NewMockService(s.ctrl),
		IDGenerator:      idgenmock.NewMockGenerator(s.ctrl),
		DraftIDGenerator: idgenmock.NewMockGenerator(s.ctrl),
	})
	s.Require().NoError(err)
}

func (s *EquipItemTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

// fighterWithLongswordAndShield is unarmed (nothing equipped) but carries a
// longsword and a shield — enough to exercise a plain equip and a
// slot-swap without needing the full class/race/background setup the draft
// tests build.
func (s *EquipItemTestSuite) fighterWithLongswordAndShield() *entities.Character {
	return &entities.Character{
		Data: &character.Data{
			ID:               s.testCharacterID,
			PlayerID:         "player-1",
			Name:             "Test Fighter",
			Level:            1,
			RaceID:           "human",
			ClassID:          "fighter",
			ProficiencyBonus: 2,
			HitPoints:        12,
			MaxHitPoints:     12,
			ArmorClass:       10,
			AbilityScores: shared.AbilityScores{
				abilities.STR: 16,
				abilities.DEX: 12,
				abilities.CON: 14,
				abilities.INT: 10,
				abilities.WIS: 10,
				abilities.CHA: 10,
			},
			Inventory: []character.InventoryItemData{
				{Type: "weapon", ID: "longsword", Quantity: 1},
				{Type: "armor", ID: "shield", Quantity: 1},
				{Type: "weapon", ID: "greatsword", Quantity: 1},
			},
			EquipmentSlots: character.EquipmentSlots{},
		},
	}
}

func (s *EquipItemTestSuite) appliedPatch(
	entity *entities.Character,
	input characterrepo.PatchEquipmentInput,
) *characterrepo.PatchEquipmentOutput {
	patchedData := *entity.Data
	patchedData.EquipmentSlots = maps.Clone(input.EquipmentSlots)
	patchedData.ArmorClass = input.ArmorClass
	return &characterrepo.PatchEquipmentOutput{
		Character: &entities.Character{Data: &patchedData, Appearance: entity.Appearance},
		Version:   "patched-version",
		Applied:   true,
	}
}

func (s *EquipItemTestSuite) TestEquipItem_MissingRepositoryVersionFailsBeforeMutation() {
	entity := s.fighterWithLongswordAndShield()
	entity.Data.EquipmentSlots = character.EquipmentSlots{character.SlotOffHand: "shield"}
	originalData := *entity.Data
	originalData.EquipmentSlots = maps.Clone(entity.Data.EquipmentSlots)
	originalSlotsIdentity := reflect.ValueOf(entity.Data.EquipmentSlots).Pointer()
	s.mockCharacterRepo.EXPECT().
		Get(s.ctx, characterrepo.GetInput{ID: s.testCharacterID}).
		Return(&characterrepo.GetOutput{Character: entity}, nil)
	// Deliberately no PatchEquipment expectation.

	projectCalls := 0
	s.orchestrator.projectLoaded = func(ctx context.Context, input *ProjectLoadedCharacterInput) (*ProjectLoadedCharacterOutput, error) {
		projectCalls++
		return projectLoadedCharacter(ctx, input)
	}

	out, err := s.orchestrator.EquipItem(s.ctx, &EquipItemInput{
		CharacterID: s.testCharacterID,
		ItemID:      "longsword",
		Slot:        character.SlotMainHand,
	})
	s.Require().Error(err)
	s.Nil(out)
	s.True(apierr.IsInternal(err), "expected Internal, got %v", err)
	var coded *apierr.Error
	s.Require().True(errors.As(err, &coded))
	s.Equal(expectedMissingVersionMessage, coded.Message)
	s.Zero(projectCalls, "missing version must fail before strict projection")
	s.Equal(originalData, *entity.Data)
	s.Equal(originalSlotsIdentity, reflect.ValueOf(entity.Data.EquipmentSlots).Pointer())
}

func (s *EquipItemTestSuite) TestUnequipItem_MissingRepositoryVersionFailsBeforeMutation() {
	entity := s.fighterWithLongswordAndShield()
	entity.Data.EquipmentSlots = character.EquipmentSlots{character.SlotMainHand: "longsword"}
	originalData := *entity.Data
	originalData.EquipmentSlots = maps.Clone(entity.Data.EquipmentSlots)
	originalSlotsIdentity := reflect.ValueOf(entity.Data.EquipmentSlots).Pointer()
	s.mockCharacterRepo.EXPECT().
		Get(s.ctx, characterrepo.GetInput{ID: s.testCharacterID}).
		Return(&characterrepo.GetOutput{Character: entity}, nil)
	// Deliberately no PatchEquipment expectation.

	projectCalls := 0
	s.orchestrator.projectLoaded = func(ctx context.Context, input *ProjectLoadedCharacterInput) (*ProjectLoadedCharacterOutput, error) {
		projectCalls++
		return projectLoadedCharacter(ctx, input)
	}

	out, err := s.orchestrator.UnequipItem(s.ctx, &UnequipItemInput{
		CharacterID: s.testCharacterID,
		Slot:        character.SlotMainHand,
	})
	s.Require().Error(err)
	s.Nil(out)
	s.True(apierr.IsInternal(err), "expected Internal, got %v", err)
	var coded *apierr.Error
	s.Require().True(errors.As(err, &coded))
	s.Equal(expectedMissingVersionMessage, coded.Message)
	s.Zero(projectCalls, "missing version must fail before strict projection")
	s.Equal(originalData, *entity.Data)
	s.Equal(originalSlotsIdentity, reflect.ValueOf(entity.Data.EquipmentSlots).Pointer())
}

func (s *EquipItemTestSuite) TestEquipItem_Success_NoPreviousOccupant() {
	charEntity := s.fighterWithLongswordAndShield()

	s.mockCharacterRepo.EXPECT().
		Get(s.ctx, characterrepo.GetInput{ID: s.testCharacterID}).
		Return(&characterrepo.GetOutput{Character: charEntity, Version: testCharacterRepositoryVersion}, nil)

	s.mockCharacterRepo.EXPECT().
		PatchEquipment(s.ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, input characterrepo.PatchEquipmentInput) (*characterrepo.PatchEquipmentOutput, error) {
			s.Assert().Equal("longsword", input.EquipmentSlots.Get(character.SlotMainHand))
			return s.appliedPatch(charEntity, input), nil
		})

	out, err := s.orchestrator.EquipItem(s.ctx, &EquipItemInput{
		CharacterID: s.testCharacterID,
		ItemID:      "longsword",
		Slot:        character.SlotMainHand,
	})
	s.Require().NoError(err)
	s.Assert().Empty(out.PreviousItemID)
}

// TestEquipItem_TwoHanded_ClearsOffHand proves the toolkit's occupancy rule
// (rpg-toolkit#812) actually runs through this orchestrator method, not just
// in the toolkit's own tests: equipping a two-handed weapon into main_hand
// must clear off_hand as a side effect of the SAME EquipItem call.
func (s *EquipItemTestSuite) TestEquipItem_TwoHanded_ClearsOffHand() {
	charEntity := s.fighterWithLongswordAndShield()
	charEntity.Data.EquipmentSlots = character.EquipmentSlots{
		character.SlotOffHand: "shield",
	}

	s.mockCharacterRepo.EXPECT().
		Get(s.ctx, characterrepo.GetInput{ID: s.testCharacterID}).
		Return(&characterrepo.GetOutput{Character: charEntity, Version: testCharacterRepositoryVersion}, nil)

	s.mockCharacterRepo.EXPECT().
		PatchEquipment(s.ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, input characterrepo.PatchEquipmentInput) (*characterrepo.PatchEquipmentOutput, error) {
			s.Assert().Equal("greatsword", input.EquipmentSlots.Get(character.SlotMainHand))
			s.Assert().Empty(input.EquipmentSlots.Get(character.SlotOffHand),
				"equipping a two-handed weapon must clear off_hand")
			return s.appliedPatch(charEntity, input), nil
		})

	out, err := s.orchestrator.EquipItem(s.ctx, &EquipItemInput{
		CharacterID: s.testCharacterID,
		ItemID:      "greatsword",
		Slot:        character.SlotMainHand,
	})
	s.Require().NoError(err)
	s.Assert().Empty(out.PreviousItemID, "main_hand had nothing equipped before this call")
}

func (s *EquipItemTestSuite) TestEquipItem_Swap_ReturnsPreviousOccupant() {
	charEntity := s.fighterWithLongswordAndShield()
	charEntity.Data.Inventory = append(charEntity.Data.Inventory,
		character.InventoryItemData{Type: "weapon", ID: "handaxe", Quantity: 1})
	charEntity.Data.EquipmentSlots = character.EquipmentSlots{
		character.SlotMainHand: "handaxe",
	}

	s.mockCharacterRepo.EXPECT().
		Get(s.ctx, characterrepo.GetInput{ID: s.testCharacterID}).
		Return(&characterrepo.GetOutput{Character: charEntity, Version: testCharacterRepositoryVersion}, nil)

	s.mockCharacterRepo.EXPECT().
		PatchEquipment(s.ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, input characterrepo.PatchEquipmentInput) (*characterrepo.PatchEquipmentOutput, error) {
			return s.appliedPatch(charEntity, input), nil
		})

	out, err := s.orchestrator.EquipItem(s.ctx, &EquipItemInput{
		CharacterID: s.testCharacterID,
		ItemID:      "longsword",
		Slot:        character.SlotMainHand,
	})
	s.Require().NoError(err)
	s.Assert().Equal("handaxe", out.PreviousItemID)
}

func (s *EquipItemTestSuite) TestEquipItem_ItemNotInInventory_ReturnsNotFound() {
	charEntity := s.fighterWithLongswordAndShield()

	s.mockCharacterRepo.EXPECT().
		Get(s.ctx, characterrepo.GetInput{ID: s.testCharacterID}).
		Return(&characterrepo.GetOutput{Character: charEntity, Version: testCharacterRepositoryVersion}, nil)

	_, err := s.orchestrator.EquipItem(s.ctx, &EquipItemInput{
		CharacterID: s.testCharacterID,
		ItemID:      "not-owned-item",
		Slot:        character.SlotMainHand,
	})
	s.Require().Error(err)
	s.Assert().True(apierr.IsNotFound(err), "expected NotFound, got %v", err)
}

// TestEquipItem_IncompatibleSlot_ReturnsInvalidArgument proves the toolkit's
// new slot-compatibility validation (rpg-toolkit#812 — equipping into an
// incompatible slot used to silently succeed) surfaces as an
// InvalidArgument, not a silent no-op or an opaque Internal error.
func (s *EquipItemTestSuite) TestEquipItem_IncompatibleSlot_ReturnsInvalidArgument() {
	charEntity := s.fighterWithLongswordAndShield()

	s.mockCharacterRepo.EXPECT().
		Get(s.ctx, characterrepo.GetInput{ID: s.testCharacterID}).
		Return(&characterrepo.GetOutput{Character: charEntity, Version: testCharacterRepositoryVersion}, nil)

	_, err := s.orchestrator.EquipItem(s.ctx, &EquipItemInput{
		CharacterID: s.testCharacterID,
		ItemID:      "shield",
		Slot:        character.SlotMainHand,
	})
	s.Require().Error(err)
	s.Assert().True(apierr.IsInvalidArgument(err), "expected InvalidArgument, got %v", err)
}

func (s *EquipItemTestSuite) TestUnequipItem_Success() {
	charEntity := s.fighterWithLongswordAndShield()
	charEntity.Data.EquipmentSlots = character.EquipmentSlots{
		character.SlotMainHand: "longsword",
	}

	s.mockCharacterRepo.EXPECT().
		Get(s.ctx, characterrepo.GetInput{ID: s.testCharacterID}).
		Return(&characterrepo.GetOutput{Character: charEntity, Version: testCharacterRepositoryVersion}, nil)

	s.mockCharacterRepo.EXPECT().
		PatchEquipment(s.ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, input characterrepo.PatchEquipmentInput) (*characterrepo.PatchEquipmentOutput, error) {
			s.Assert().Empty(input.EquipmentSlots.Get(character.SlotMainHand))
			return s.appliedPatch(charEntity, input), nil
		})

	out, err := s.orchestrator.UnequipItem(s.ctx, &UnequipItemInput{
		CharacterID: s.testCharacterID,
		Slot:        character.SlotMainHand,
	})
	s.Require().NoError(err)
	s.Assert().Equal("longsword", out.UnequippedItemID)
	s.Require().NotNil(out.Character)
	projected, projectErr := ProjectView(s.ctx, &ProjectViewInput{Data: out.Character.Data})
	s.Require().NoError(projectErr)
	s.Equal(projected.View, out.View, "unequip persisted entity and detached View must be the same post-state")
}

// TestEquipItem_PreservesNonEquipmentFields is the gate-finding-2 regression:
// persistence must merge only the equipment fields instead of writing the
// toolkit sheet's lossy ToData result. Strict loading now rejects unknown
// inventory entries rather than preserving a blob it cannot project, but all
// valid non-equipment data and API-owned appearance still remain byte-for-byte
// unchanged.
func (s *EquipItemTestSuite) TestEquipItem_PreservesNonEquipmentFields() {
	charEntity := s.fighterWithLongswordAndShield()
	fixedCreatedAt := time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)
	charEntity.Data.BackgroundID = backgrounds.Soldier
	charEntity.Data.CreatedAt = fixedCreatedAt
	charEntity.Data.SpellSlots = map[int]character.SpellSlotData{1: {Max: 2, Used: 1}}
	charEntity.Data.ClassResources = map[shared.ClassResourceType]character.ResourceData{
		shared.ClassResourceType(99): {Name: "legacy", Current: 1, Max: 2},
	}
	charEntity.Appearance = &entities.Appearance{
		SkinTone: "#D5A88C", PrimaryColor: "#8B0000", SecondaryColor: "#FFD700", EyeColor: "#4A2511",
	}

	s.mockCharacterRepo.EXPECT().
		Get(s.ctx, characterrepo.GetInput{ID: s.testCharacterID}).
		Return(&characterrepo.GetOutput{Character: charEntity, Version: testCharacterRepositoryVersion}, nil)

	var persisted *entities.Character
	s.mockCharacterRepo.EXPECT().
		PatchEquipment(s.ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, input characterrepo.PatchEquipmentInput) (*characterrepo.PatchEquipmentOutput, error) {
			output := s.appliedPatch(charEntity, input)
			persisted = output.Character
			return output, nil
		})

	_, err := s.orchestrator.EquipItem(s.ctx, &EquipItemInput{
		CharacterID: s.testCharacterID,
		ItemID:      "longsword",
		Slot:        character.SlotMainHand,
	})
	s.Require().NoError(err)
	s.Require().NotNil(persisted)

	s.Assert().Equal(backgrounds.Soldier, persisted.Data.BackgroundID, "BackgroundID must survive an equip call")
	s.Assert().True(fixedCreatedAt.Equal(persisted.Data.CreatedAt), "CreatedAt must survive an equip call")
	s.Assert().Equal(charEntity.Data.Inventory, persisted.Data.Inventory)
	s.Assert().Equal(charEntity.Data.SpellSlots, persisted.Data.SpellSlots)
	s.Assert().Equal(charEntity.Data.ClassResources, persisted.Data.ClassResources)
	s.Assert().Equal(charEntity.Appearance, persisted.Appearance)

	// The actual equip must still have applied — this isn't a no-op merge.
	s.Assert().Equal("longsword", persisted.Data.EquipmentSlots.Get(character.SlotMainHand))
}

// TestEquipItem_RejectsUnprojectableDataWithoutWriting proves every strict
// private-state failure happens before repository PatchEquipment.
func (s *EquipItemTestSuite) TestEquipItem_RejectsUnprojectableDataWithoutWriting() {
	tests := []struct {
		name   string
		mutate func(*entities.Character)
	}{
		{
			name: "missing player identity",
			mutate: func(entity *entities.Character) {
				entity.Data.PlayerID = ""
			},
		},
		{
			name: "missing class identity",
			mutate: func(entity *entities.Character) {
				entity.Data.ClassID = ""
			},
		},
		{
			name: "missing race identity",
			mutate: func(entity *entities.Character) {
				entity.Data.RaceID = ""
			},
		},
		{
			name: "malformed condition",
			mutate: func(entity *entities.Character) {
				entity.Data.Conditions = []json.RawMessage{json.RawMessage(`{"ref":{"module":"dnd5e","type":"conditions","id":"unknown"}}`)}
			},
		},
		{
			name: "malformed feature",
			mutate: func(entity *entities.Character) {
				entity.Data.Features = []json.RawMessage{json.RawMessage(`{"ref":`)}
			},
		},
		{
			name: "unknown item",
			mutate: func(entity *entities.Character) {
				entity.Data.Inventory = append(entity.Data.Inventory,
					character.InventoryItemData{Type: "item", ID: "vorpal-spork", Quantity: 1})
			},
		},
		{
			name: "malformed resource",
			mutate: func(entity *entities.Character) {
				entity.Data.Resources = map[coreResources.ResourceKey]character.RecoverableResourceData{
					resources.HitDice: {Current: 2, Maximum: 1},
				}
			},
		},
		{
			name: "unknown status descriptor",
			mutate: func(entity *entities.Character) {
				entity.Data.Conditions = []json.RawMessage{mustJSON(s.T(), conditions.ShieldSpellConditionData{
					Ref: refs.Spells.Shield(), MemberID: entity.Data.ID,
				})}
			},
		},
	}

	for _, tc := range tests {
		s.Run(tc.name, func() {
			entity := s.fighterWithLongswordAndShield()
			tc.mutate(entity)
			s.mockCharacterRepo.EXPECT().
				Get(s.ctx, characterrepo.GetInput{ID: s.testCharacterID}).
				Return(&characterrepo.GetOutput{Character: entity, Version: testCharacterRepositoryVersion}, nil)
			// Deliberately no PatchEquipment expectation: gomock fails if malformed
			// private state reaches persistence.

			out, err := s.orchestrator.EquipItem(s.ctx, &EquipItemInput{
				CharacterID: s.testCharacterID,
				ItemID:      "longsword",
				Slot:        character.SlotMainHand,
			})
			s.Require().Error(err)
			s.Nil(out)
		})
	}
}

func (s *EquipItemTestSuite) TestUnequipItem_MissingOwnerIdentityWritesNothing() {
	entity := s.fighterWithLongswordAndShield()
	entity.Data.PlayerID = ""
	entity.Data.EquipmentSlots = character.EquipmentSlots{character.SlotMainHand: "longsword"}
	s.mockCharacterRepo.EXPECT().
		Get(s.ctx, characterrepo.GetInput{ID: s.testCharacterID}).
		Return(&characterrepo.GetOutput{Character: entity, Version: testCharacterRepositoryVersion}, nil)
	// Deliberately no PatchEquipment expectation.

	out, err := s.orchestrator.UnequipItem(s.ctx, &UnequipItemInput{
		CharacterID: s.testCharacterID,
		Slot:        character.SlotMainHand,
	})
	s.Require().Error(err)
	s.Nil(out)
}

func (s *EquipItemTestSuite) TestUnequipItem_MalformedConditionWritesNothing() {
	entity := s.fighterWithLongswordAndShield()
	entity.Data.EquipmentSlots = character.EquipmentSlots{character.SlotMainHand: "longsword"}
	entity.Data.Conditions = []json.RawMessage{json.RawMessage(`{"ref":{"module":"dnd5e","type":"conditions","id":"unknown"}}`)}
	s.mockCharacterRepo.EXPECT().
		Get(s.ctx, characterrepo.GetInput{ID: s.testCharacterID}).
		Return(&characterrepo.GetOutput{Character: entity, Version: testCharacterRepositoryVersion}, nil)
	// Deliberately no PatchEquipment expectation.

	out, err := s.orchestrator.UnequipItem(s.ctx, &UnequipItemInput{
		CharacterID: s.testCharacterID,
		Slot:        character.SlotMainHand,
	})
	s.Require().Error(err)
	s.Nil(out)
}

func (s *EquipItemTestSuite) TestEquipItem_PostProjectionFailureLeavesRepositoryEntityUnchanged() {
	entity := s.fighterWithLongswordAndShield()
	entity.Data.EquipmentSlots = character.EquipmentSlots{character.SlotOffHand: "shield"}
	originalSlots := entity.Data.EquipmentSlots
	originalSlotsIdentity := reflect.ValueOf(originalSlots).Pointer()
	s.mockCharacterRepo.EXPECT().
		Get(s.ctx, characterrepo.GetInput{ID: s.testCharacterID}).
		Return(&characterrepo.GetOutput{Character: entity, Version: testCharacterRepositoryVersion}, nil)

	calls := 0
	var workingSlots character.EquipmentSlots
	s.orchestrator.projectLoaded = func(ctx context.Context, input *ProjectLoadedCharacterInput) (*ProjectLoadedCharacterOutput, error) {
		calls++
		if calls == 2 {
			workingSlots = input.Character.ToData().EquipmentSlots
			return nil, errors.New("post-state descriptor failed")
		}
		return projectLoadedCharacter(ctx, input)
	}
	// Deliberately no PatchEquipment expectation: the complete post-view is required
	// before persistence.

	out, err := s.orchestrator.EquipItem(s.ctx, &EquipItemInput{
		CharacterID: s.testCharacterID,
		ItemID:      "longsword",
		Slot:        character.SlotMainHand,
	})
	s.Require().Error(err)
	s.Nil(out)
	s.Equal(2, calls)
	s.Equal(character.EquipmentSlots{character.SlotOffHand: "shield"}, entity.Data.EquipmentSlots)
	s.Equal(originalSlotsIdentity, reflect.ValueOf(entity.Data.EquipmentSlots).Pointer(),
		"repository-returned entity must retain its original map")
	s.NotEqual(originalSlotsIdentity, reflect.ValueOf(workingSlots).Pointer(),
		"the mutated working map must be isolated from repository-returned data")
}

func (s *EquipItemTestSuite) TestUnequipItem_PostProjectionFailureLeavesRepositoryEntityUnchanged() {
	entity := s.fighterWithLongswordAndShield()
	entity.Data.EquipmentSlots = character.EquipmentSlots{
		character.SlotMainHand: "longsword",
		character.SlotOffHand:  "shield",
	}
	originalSlots := entity.Data.EquipmentSlots
	originalSlotsIdentity := reflect.ValueOf(originalSlots).Pointer()
	s.mockCharacterRepo.EXPECT().
		Get(s.ctx, characterrepo.GetInput{ID: s.testCharacterID}).
		Return(&characterrepo.GetOutput{Character: entity, Version: testCharacterRepositoryVersion}, nil)

	calls := 0
	var workingSlots character.EquipmentSlots
	s.orchestrator.projectLoaded = func(ctx context.Context, input *ProjectLoadedCharacterInput) (*ProjectLoadedCharacterOutput, error) {
		calls++
		if calls == 2 {
			workingSlots = input.Character.ToData().EquipmentSlots
			return nil, errors.New("post-state descriptor failed")
		}
		return projectLoadedCharacter(ctx, input)
	}
	// Deliberately no PatchEquipment expectation.

	out, err := s.orchestrator.UnequipItem(s.ctx, &UnequipItemInput{
		CharacterID: s.testCharacterID,
		Slot:        character.SlotMainHand,
	})
	s.Require().Error(err)
	s.Nil(out)
	s.Equal(2, calls)
	s.Equal(character.EquipmentSlots{
		character.SlotMainHand: "longsword",
		character.SlotOffHand:  "shield",
	}, entity.Data.EquipmentSlots)
	s.Equal(originalSlotsIdentity, reflect.ValueOf(entity.Data.EquipmentSlots).Pointer(),
		"repository-returned entity must retain its original map")
	s.NotEqual(originalSlotsIdentity, reflect.ValueOf(workingSlots).Pointer(),
		"the mutated working map must be isolated from repository-returned data")
}

func (s *EquipItemTestSuite) TestEquipItem_PatchFailureLeavesRepositoryEntityUnchanged() {
	entity := s.fighterWithLongswordAndShield()
	entity.Data.EquipmentSlots = character.EquipmentSlots{character.SlotOffHand: "shield"}
	originalSlots := entity.Data.EquipmentSlots
	originalSlotsIdentity := reflect.ValueOf(originalSlots).Pointer()
	s.mockCharacterRepo.EXPECT().
		Get(s.ctx, characterrepo.GetInput{ID: s.testCharacterID}).
		Return(&characterrepo.GetOutput{Character: entity, Version: testCharacterRepositoryVersion}, nil)
	s.mockCharacterRepo.EXPECT().
		PatchEquipment(s.ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, input characterrepo.PatchEquipmentInput) (*characterrepo.PatchEquipmentOutput, error) {
			s.Equal(character.EquipmentSlots{
				character.SlotMainHand: "longsword",
				character.SlotOffHand:  "shield",
			}, input.EquipmentSlots)
			s.Equal(character.EquipmentSlots{character.SlotOffHand: "shield"}, input.ExpectedEquipmentSlots)
			s.Equal(character.EquipmentSlots{character.SlotOffHand: "shield"}, entity.Data.EquipmentSlots)
			s.NotEqual(originalSlotsIdentity, reflect.ValueOf(input.EquipmentSlots).Pointer(),
				"PatchEquipment must receive the isolated working map")
			return nil, errors.New("patch failed")
		})

	out, err := s.orchestrator.EquipItem(s.ctx, &EquipItemInput{
		CharacterID: s.testCharacterID,
		ItemID:      "longsword",
		Slot:        character.SlotMainHand,
	})
	s.Require().Error(err)
	s.Nil(out)
	s.Equal(character.EquipmentSlots{character.SlotOffHand: "shield"}, entity.Data.EquipmentSlots)
	s.Equal(originalSlotsIdentity, reflect.ValueOf(entity.Data.EquipmentSlots).Pointer())
}

func (s *EquipItemTestSuite) TestUnequipItem_PatchFailureLeavesRepositoryEntityUnchanged() {
	entity := s.fighterWithLongswordAndShield()
	entity.Data.EquipmentSlots = character.EquipmentSlots{
		character.SlotMainHand: "longsword",
		character.SlotOffHand:  "shield",
	}
	originalSlots := entity.Data.EquipmentSlots
	originalSlotsIdentity := reflect.ValueOf(originalSlots).Pointer()
	s.mockCharacterRepo.EXPECT().
		Get(s.ctx, characterrepo.GetInput{ID: s.testCharacterID}).
		Return(&characterrepo.GetOutput{Character: entity, Version: testCharacterRepositoryVersion}, nil)
	s.mockCharacterRepo.EXPECT().
		PatchEquipment(s.ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, input characterrepo.PatchEquipmentInput) (*characterrepo.PatchEquipmentOutput, error) {
			s.Equal(character.EquipmentSlots{character.SlotOffHand: "shield"}, input.EquipmentSlots)
			s.Equal(character.EquipmentSlots{
				character.SlotMainHand: "longsword",
				character.SlotOffHand:  "shield",
			}, input.ExpectedEquipmentSlots)
			s.Equal(character.EquipmentSlots{
				character.SlotMainHand: "longsword",
				character.SlotOffHand:  "shield",
			}, entity.Data.EquipmentSlots)
			s.NotEqual(originalSlotsIdentity, reflect.ValueOf(input.EquipmentSlots).Pointer(),
				"PatchEquipment must receive the isolated working map")
			return nil, errors.New("patch failed")
		})

	out, err := s.orchestrator.UnequipItem(s.ctx, &UnequipItemInput{
		CharacterID: s.testCharacterID,
		Slot:        character.SlotMainHand,
	})
	s.Require().Error(err)
	s.Nil(out)
	s.Equal(character.EquipmentSlots{
		character.SlotMainHand: "longsword",
		character.SlotOffHand:  "shield",
	}, entity.Data.EquipmentSlots)
	s.Equal(originalSlotsIdentity, reflect.ValueOf(entity.Data.EquipmentSlots).Pointer())
}

func (s *EquipItemTestSuite) TestEquipItem_RetriesConcurrentNonEquipmentRevisionAndReturnsOnePostState() {
	entity := s.fighterWithLongswordAndShield()
	s.mockCharacterRepo.EXPECT().
		Get(s.ctx, characterrepo.GetInput{ID: s.testCharacterID}).
		Return(&characterrepo.GetOutput{Character: entity, Version: "version-before-combat"}, nil)

	latestData := *entity.Data
	latestData.HitPoints = 7
	latestData.EquipmentSlots = maps.Clone(entity.Data.EquipmentSlots)
	latest := &entities.Character{Data: &latestData, Appearance: entity.Appearance}

	gomock.InOrder(
		s.mockCharacterRepo.EXPECT().
			PatchEquipment(s.ctx, gomock.Any()).
			DoAndReturn(func(_ context.Context, input characterrepo.PatchEquipmentInput) (*characterrepo.PatchEquipmentOutput, error) {
				s.Equal("version-before-combat", input.ExpectedVersion)
				s.Equal(entity.Data.EquipmentSlots, input.ExpectedEquipmentSlots)
				s.Equal("longsword", input.EquipmentSlots.Get(character.SlotMainHand))
				return &characterrepo.PatchEquipmentOutput{
					Character: latest,
					Version:   "version-after-combat",
					Applied:   false,
				}, nil
			}),
		s.mockCharacterRepo.EXPECT().
			PatchEquipment(s.ctx, gomock.Any()).
			DoAndReturn(func(_ context.Context, input characterrepo.PatchEquipmentInput) (*characterrepo.PatchEquipmentOutput, error) {
				s.Equal("version-after-combat", input.ExpectedVersion)
				s.Equal(latest.Data.EquipmentSlots, input.ExpectedEquipmentSlots)
				s.Equal("longsword", input.EquipmentSlots.Get(character.SlotMainHand))
				s.NotZero(input.ArmorClass)

				patchedData := *latest.Data
				patchedData.EquipmentSlots = maps.Clone(input.EquipmentSlots)
				patchedData.ArmorClass = input.ArmorClass
				return &characterrepo.PatchEquipmentOutput{
					Character: &entities.Character{Data: &patchedData, Appearance: latest.Appearance},
					Version:   "version-patched",
					Applied:   true,
				}, nil
			}),
	)

	out, err := s.orchestrator.EquipItem(s.ctx, &EquipItemInput{
		CharacterID: s.testCharacterID,
		ItemID:      "longsword",
		Slot:        character.SlotMainHand,
	})
	s.Require().NoError(err)
	s.Require().NotNil(out.Character)
	s.Equal(7, out.Character.Data.HitPoints, "the concurrent combat state is the response post-state")
	s.Equal(7, out.View.Status.HitPoints.Current, "the detached View and returned persisted entity must agree")
	s.Equal(out.Character.Data.PlayerID, out.View.Identity.PlayerID)
	s.Equal(out.Character.Data.ClassID, out.View.Identity.ClassID)
	s.Equal(out.Character.Data.RaceID, out.View.Identity.RaceID)
}

func (s *EquipItemTestSuite) TestEquipItem_OutputViewEqualsCapturedPersistedPostState() {
	entity := s.fighterWithLongswordAndShield()
	s.mockCharacterRepo.EXPECT().
		Get(s.ctx, characterrepo.GetInput{ID: s.testCharacterID}).
		Return(&characterrepo.GetOutput{Character: entity, Version: testCharacterRepositoryVersion}, nil)

	var persisted *character.Data
	s.mockCharacterRepo.EXPECT().
		PatchEquipment(s.ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, input characterrepo.PatchEquipmentInput) (*characterrepo.PatchEquipmentOutput, error) {
			output := s.appliedPatch(entity, input)
			persisted = output.Character.Data
			return output, nil
		})

	out, err := s.orchestrator.EquipItem(s.ctx, &EquipItemInput{
		CharacterID: s.testCharacterID,
		ItemID:      "longsword",
		Slot:        character.SlotMainHand,
	})
	s.Require().NoError(err)
	s.Require().NotNil(out.View)
	s.Require().NotNil(persisted)

	projected, err := ProjectView(s.ctx, &ProjectViewInput{Data: persisted})
	s.Require().NoError(err)
	s.Equal(projected.View, out.View)
}

// TestEquipItem_SyncsStoredArmorClass is the gate-finding-1 regression: the
// stored ArmorClass int must be refreshed to the toolkit's real EffectiveAC
// total on every equip.
func (s *EquipItemTestSuite) TestEquipItem_SyncsStoredArmorClass() {
	charEntity := s.fighterWithLongswordAndShield()
	charEntity.Data.ArmorClass = 10 // deliberately stale/wrong stored value
	charEntity.Data.Inventory = append(charEntity.Data.Inventory,
		character.InventoryItemData{Type: "armor", ID: "chain-mail", Quantity: 1})

	s.mockCharacterRepo.EXPECT().
		Get(s.ctx, characterrepo.GetInput{ID: s.testCharacterID}).
		Return(&characterrepo.GetOutput{Character: charEntity, Version: testCharacterRepositoryVersion}, nil)

	var persisted *character.Data
	s.mockCharacterRepo.EXPECT().
		PatchEquipment(s.ctx, gomock.Any()).
		DoAndReturn(func(_ context.Context, input characterrepo.PatchEquipmentInput) (*characterrepo.PatchEquipmentOutput, error) {
			output := s.appliedPatch(charEntity, input)
			persisted = output.Character.Data
			return output, nil
		})

	// chain-mail is a fixed-AC-16 heavy armor (no DEX bonus) — a
	// hand-computable, non-tautological expectation.
	_, err := s.orchestrator.EquipItem(s.ctx, &EquipItemInput{
		CharacterID: s.testCharacterID,
		ItemID:      "chain-mail",
		Slot:        character.SlotArmor,
	})
	s.Require().NoError(err)
	s.Require().NotNil(persisted)
	s.Assert().Equal(16, persisted.ArmorClass, "stored ArmorClass must be refreshed to the real EffectiveAC total")
}
