package character

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	dicemock "github.com/KirkDiggler/rpg-api/internal/orchestrators/dice/mock"
	idgenmock "github.com/KirkDiggler/rpg-api/internal/pkg/idgen/mock"
	characterrepomock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	draftmock "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft/mock"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/conditions"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
)

// testCharacterRepositoryVersion is an opaque stored version for fixtures that
// read through the character repository.
const testCharacterRepositoryVersion = "version-1"

// EquipItemTestSuite proves the equip path is transport (rpg-project#542): the
// orchestrator hands the request to the session SDK's verb, passes its refusal
// through unchanged, and projects armour class from the record the verb saved.
//
// The equip tests set NO character repository expectation. gomock fails any
// call, so each is also the proof that the orchestrator neither reads nor
// writes a sheet around the verb. (The read tests in armor_class_test.go ride
// this suite's fixtures and set their own.)
type EquipItemTestSuite struct {
	suite.Suite
	ctrl              *gomock.Controller
	mockCharacterRepo *characterrepomock.MockRepository
	equipment         *scriptedEquipment
	orchestrator      *Orchestrator
	ctx               context.Context
	testCharacterID   string
}

func TestEquipItemSuite(t *testing.T) {
	suite.Run(t, new(EquipItemTestSuite))
}

func (s *EquipItemTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.equipment = &scriptedEquipment{t: s.T()}
	s.mockCharacterRepo = characterrepomock.NewMockRepository(s.ctrl)
	s.ctx = context.Background()
	s.testCharacterID = "char-fighter-1"

	var err error
	s.orchestrator, err = New(&Config{
		DraftRepo:        draftmock.NewMockRepository(s.ctrl),
		CharacterRepo:    s.mockCharacterRepo,
		DiceService:      dicemock.NewMockService(s.ctrl),
		IDGenerator:      idgenmock.NewMockGenerator(s.ctrl),
		DraftIDGenerator: idgenmock.NewMockGenerator(s.ctrl),
		Equipment:        s.equipment,
	})
	s.Require().NoError(err)
}

func (s *EquipItemTestSuite) TearDownTest() {
	s.ctrl.Finish()
	s.Empty(s.equipment.equips, "every scripted equip answer was asked for")
	s.Empty(s.equipment.unequips, "every scripted unequip answer was asked for")
}

// scriptedEquipment stands in for the SDK's equip verbs. Each call takes the
// next scripted answer and records the input it was handed; a call with no
// answer scripted fails the test, which is how a test says "the verb is never
// reached". (A generated mock would sit in a package that imports this one.)
type scriptedEquipment struct {
	t        *testing.T
	equips   []scriptedEquip
	unequips []scriptedEquip
	gotEquip []*sdk.EquipInput
	gotUneq  []*sdk.UnequipInput
}

type scriptedEquip struct {
	out *sdk.EquipOutput
	err error
}

func (f *scriptedEquipment) Equip(_ context.Context, in *sdk.EquipInput) (*sdk.EquipOutput, error) {
	f.gotEquip = append(f.gotEquip, in)
	if len(f.equips) == 0 {
		f.t.Fatalf("Equip called with no answer scripted: %+v", in)
	}
	next := f.equips[0]
	f.equips = f.equips[1:]
	return next.out, next.err
}

func (f *scriptedEquipment) Unequip(_ context.Context, in *sdk.UnequipInput) (*sdk.UnequipOutput, error) {
	f.gotUneq = append(f.gotUneq, in)
	if len(f.unequips) == 0 {
		f.t.Fatalf("Unequip called with no answer scripted: %+v", in)
	}
	next := f.unequips[0]
	f.unequips = f.unequips[1:]
	return next.out, next.err
}

func (f *scriptedEquipment) answerEquip(out *sdk.EquipOutput, err error) {
	f.equips = append(f.equips, scriptedEquip{out: out, err: err})
}

func (f *scriptedEquipment) answerUnequip(out *sdk.UnequipOutput, err error) {
	f.unequips = append(f.unequips, scriptedEquip{out: out, err: err})
}

// fighter carries a longsword, a shield, a greatsword and chain mail with
// nothing equipped; tests set the slots the verb's saved record would hold.
func (s *EquipItemTestSuite) fighter() *character.Data {
	return &character.Data{
		ID:               s.testCharacterID,
		PlayerID:         "player-1",
		Name:             "Test Fighter",
		Level:            1,
		RaceID:           "human",
		ClassID:          "fighter",
		ProficiencyBonus: 2,
		HitPoints:        12,
		MaxHitPoints:     12,
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
			{Type: "armor", ID: "chain-mail", Quantity: 1},
		},
		EquipmentSlots: character.EquipmentSlots{},
	}
}

// monkSheet carries Unarmored Defense (monk) with DEX 16 and WIS 14, so
// its folded AC is 15 = 10 + DEX 3 + WIS 2. Unarmored Defense reads WIS through
// the cast resolution installs; a fold on a host-attached sheet answered 13
// before that door existed (rpg-toolkit#1276, #1965).
func (s *EquipItemTestSuite) monkSheet() *character.Data {
	data := s.fighter()
	data.Name = "Test Monk"
	data.ClassID = "monk"
	data.AbilityScores = shared.AbilityScores{
		abilities.STR: 10,
		abilities.DEX: 16,
		abilities.CON: 12,
		abilities.INT: 10,
		abilities.WIS: 14,
		abilities.CHA: 8,
	}
	data.Inventory = []character.InventoryItemData{{Type: "weapon", ID: "quarterstaff", Quantity: 1}}
	unarmoredDefense, err := (&conditions.UnarmoredDefenseCondition{
		MemberID: s.testCharacterID,
		Type:     conditions.UnarmoredDefenseMonk,
	}).ToJSON()
	s.Require().NoError(err)
	data.Conditions = []json.RawMessage{unarmoredDefense}
	return data
}

// fighterWithLongswordAndShield and unarmouredMonk are the stored-entity
// forms the read tests in armor_class_test.go ride.
func (s *EquipItemTestSuite) fighterWithLongswordAndShield() *entities.Character {
	return &entities.Character{Data: s.fighter()}
}

func (s *EquipItemTestSuite) unarmouredMonk() *entities.Character {
	return &entities.Character{Data: s.monkSheet()}
}

func (s *EquipItemTestSuite) TestEquipItem_HandsTheVerbTheRequestAndProjectsItsSavedRecord() {
	saved := s.fighter()
	saved.EquipmentSlots = character.EquipmentSlots{character.SlotArmor: "chain-mail"}

	s.equipment.answerEquip(&sdk.EquipOutput{
		Character: saved,
		Drawn:     []sdk.EquipmentMove{{Slot: "armor", Item: "dnd5e:armor:chain-mail"}},
	}, nil)

	out, err := s.orchestrator.EquipItem(s.ctx, &EquipItemInput{
		CharacterID: s.testCharacterID,
		ItemID:      "chain-mail",
		Slot:        character.SlotArmor,
	})
	s.Require().NoError(err)
	s.Equal([]*sdk.EquipInput{{Character: s.testCharacterID, Slot: "armor", Item: "chain-mail"}}, s.equipment.gotEquip)
	s.Same(saved, out.Character.Data, "the response carries the record the verb saved, not a re-read")
	s.Require().NotNil(out.ArmorClass)
	// chain mail is fixed AC 16 with no DEX bonus: hand-computable.
	s.Equal(16, out.ArmorClass.Total, "armour class is folded from the saved record")
	s.Require().NotNil(out.View)
	s.Require().NotNil(out.View.Equipment)
	s.Equal(16, out.View.Equipment.ACTotal, "the view agrees with the response's fold")
	s.Empty(out.PreviousItemID, "nothing was put away")
}

func (s *EquipItemTestSuite) TestEquipItem_PreviousItemIsTheFirstItemTheVerbPutAway() {
	saved := s.fighter()
	saved.EquipmentSlots = character.EquipmentSlots{character.SlotMainHand: "greatsword"}

	s.equipment.answerEquip(&sdk.EquipOutput{
		Character: saved,
		Stowed: []sdk.EquipmentMove{
			{Slot: "main_hand", Item: "dnd5e:weapons:longsword"},
			{Slot: "main_hand", Item: "dnd5e:armor:shield"},
		},
		Drawn: []sdk.EquipmentMove{{Slot: "main_hand", Item: "dnd5e:weapons:greatsword"}},
	}, nil)

	out, err := s.orchestrator.EquipItem(s.ctx, &EquipItemInput{
		CharacterID: s.testCharacterID,
		ItemID:      "greatsword",
		Slot:        character.SlotMainHand,
	})
	s.Require().NoError(err)
	s.Equal("longsword", out.PreviousItemID, "the bare id of the first stow, as the v1alpha1 wire keys inventory")
}

func (s *EquipItemTestSuite) TestUnequipItem_HandsTheVerbTheSlotAndProjectsItsSavedRecord() {
	saved := s.fighter()

	s.equipment.answerUnequip(&sdk.UnequipOutput{
		Character: saved,
		Stowed:    []sdk.EquipmentMove{{Slot: "armor", Item: "dnd5e:armor:chain-mail"}},
	}, nil)

	out, err := s.orchestrator.UnequipItem(s.ctx, &UnequipItemInput{
		CharacterID: s.testCharacterID,
		Slot:        character.SlotArmor,
	})
	s.Require().NoError(err)
	s.Equal([]*sdk.UnequipInput{{Character: s.testCharacterID, Slot: "armor"}}, s.equipment.gotUneq)
	s.Same(saved, out.Character.Data)
	s.Require().NotNil(out.ArmorClass)
	s.Equal(11, out.ArmorClass.Total, "unarmoured: 10 + DEX 1")
	s.Equal("chain-mail", out.UnequippedItemID)
}

// TestEquipItem_AMonkReturnsTheWisdomInclusiveAC is the rpg-toolkit#1965
// tier-1 #2 regression on the new path: the fold over the verb's saved record
// answers 10 + DEX + WIS.
func (s *EquipItemTestSuite) TestEquipItem_AMonkReturnsTheWisdomInclusiveAC() {
	saved := s.monkSheet()
	saved.EquipmentSlots = character.EquipmentSlots{character.SlotMainHand: "quarterstaff"}
	s.equipment.answerEquip(&sdk.EquipOutput{Character: saved}, nil)

	out, err := s.orchestrator.EquipItem(s.ctx, &EquipItemInput{
		CharacterID: s.testCharacterID,
		ItemID:      "quarterstaff",
		Slot:        character.SlotMainHand,
	})
	s.Require().NoError(err)
	s.Require().NotNil(out.ArmorClass)
	s.Equal(15, out.ArmorClass.Total, "10 base + 3 DEX + 2 WIS")
}

func (s *EquipItemTestSuite) TestUnequipItem_AMonkReturnsTheWisdomInclusiveAC() {
	s.equipment.answerUnequip(&sdk.UnequipOutput{Character: s.monkSheet()}, nil)

	out, err := s.orchestrator.UnequipItem(s.ctx, &UnequipItemInput{
		CharacterID: s.testCharacterID,
		Slot:        character.SlotMainHand,
	})
	s.Require().NoError(err)
	s.Require().NotNil(out.ArmorClass)
	s.Equal(15, out.ArmorClass.Total, "10 base + 3 DEX + 2 WIS")
}

// TestTheVerbsRefusalPassesThroughUnchanged: the SDK's sentinel survives the
// orchestrator so the handler's one translation table (sdkerr) can name it.
// Each refusal is one the verb owns — the orchestrator decides none of them.
func (s *EquipItemTestSuite) TestTheVerbsRefusalPassesThroughUnchanged() {
	for _, refusal := range []error{
		sdk.ErrNotYourTurn, sdk.ErrDowned, sdk.ErrCannotAfford, sdk.ErrArmorInFight,
		sdk.ErrBadEquip, sdk.ErrNoCharacter, sdk.ErrSaveFailed,
	} {
		s.Run(refusal.Error(), func() {
			s.equipment.answerEquip(nil, fmt.Errorf("equip: %w", refusal))
			out, err := s.orchestrator.EquipItem(s.ctx, &EquipItemInput{
				CharacterID: s.testCharacterID, ItemID: "longsword", Slot: character.SlotMainHand,
			})
			s.Nil(out)
			s.ErrorIs(err, refusal)

			s.equipment.answerUnequip(nil, fmt.Errorf("unequip: %w", refusal))
			unequipped, err := s.orchestrator.UnequipItem(s.ctx, &UnequipItemInput{
				CharacterID: s.testCharacterID, Slot: character.SlotMainHand,
			})
			s.Nil(unequipped)
			s.ErrorIs(err, refusal)
		})
	}
}

// TestAnUnprojectableSavedRecordIsCharacterDataUnavailable: an Inspired die
// with no granting bard parses but will not Apply, so the strict fold refuses
// the saved record. The change is already durable — the verb saved it — and
// the call answers the projection failure rather than a guessed armour class.
func (s *EquipItemTestSuite) TestAnUnprojectableSavedRecordIsCharacterDataUnavailable() {
	saved := s.monkSheet()
	inspired, err := (&conditions.InspiredCondition{MemberID: s.testCharacterID}).ToJSON()
	s.Require().NoError(err)
	saved.Conditions = append(saved.Conditions, inspired)
	s.equipment.answerEquip(&sdk.EquipOutput{Character: saved}, nil)

	out, err := s.orchestrator.EquipItem(s.ctx, &EquipItemInput{
		CharacterID: s.testCharacterID, ItemID: "quarterstaff", Slot: character.SlotMainHand,
	})
	s.Require().Error(err)
	s.Nil(out)
	s.True(apierr.IsInternal(err), "%v", err)
	s.Contains(err.Error(), refs.Conditions.Inspired().String())
}

func (s *EquipItemTestSuite) TestAVerbThatSavedNoRecordIsRefused() {
	s.equipment.answerEquip(&sdk.EquipOutput{}, nil)
	out, err := s.orchestrator.EquipItem(s.ctx, &EquipItemInput{
		CharacterID: s.testCharacterID, ItemID: "longsword", Slot: character.SlotMainHand,
	})
	s.Nil(out)
	s.True(apierr.IsInternal(err), "%v", err)
}

// TestARequestMissingAFieldNeverReachesTheVerb: no answer is scripted, so the
// fake fails the test if the verb is called.
func (s *EquipItemTestSuite) TestARequestMissingAFieldNeverReachesTheVerb() {
	for name, in := range map[string]*EquipItemInput{
		"nil":          nil,
		"no character": {ItemID: "longsword", Slot: character.SlotMainHand},
		"no item":      {CharacterID: s.testCharacterID, Slot: character.SlotMainHand},
		"no slot":      {CharacterID: s.testCharacterID, ItemID: "longsword"},
	} {
		s.Run(name, func() {
			_, err := s.orchestrator.EquipItem(s.ctx, in)
			s.True(apierr.IsInvalidArgument(err), "%v", err)
		})
	}
	for name, in := range map[string]*UnequipItemInput{
		"nil":          nil,
		"no character": {Slot: character.SlotMainHand},
		"no slot":      {CharacterID: s.testCharacterID},
	} {
		s.Run("unequip "+name, func() {
			_, err := s.orchestrator.UnequipItem(s.ctx, in)
			s.True(apierr.IsInvalidArgument(err), "%v", err)
		})
	}
}

func (s *EquipItemTestSuite) TestNewRefusesAConfigWithoutEquipment() {
	_, err := New(&Config{
		DraftRepo:        draftmock.NewMockRepository(s.ctrl),
		CharacterRepo:    characterrepomock.NewMockRepository(s.ctrl),
		DiceService:      dicemock.NewMockService(s.ctrl),
		IDGenerator:      idgenmock.NewMockGenerator(s.ctrl),
		DraftIDGenerator: idgenmock.NewMockGenerator(s.ctrl),
	})
	s.True(apierr.IsInvalidArgument(err), "%v", err)
}
