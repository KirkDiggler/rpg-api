package character

import (
	"encoding/json"
	"os"
	"path/filepath"

	"google.golang.org/protobuf/proto"

	"github.com/KirkDiggler/rpg-toolkit/core"
	coreResources "github.com/KirkDiggler/rpg-toolkit/core/resources"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/magicitems"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"

	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
)

// A CUSTOM ITEM ON THE SHEET (rpg-project#850): a character who owns an item
// with its own record gets a row for it, never looked up in the rulebook's
// catalog, and a slot that holds it points at that row.

const (
	cloakID   = "lieutenants-cloak"
	cloakSlot = "cloak"

	// capturedFighterData is the wire bytes the fighter with no custom items
	// got on origin/dev (8c2e3719) before this change. Set CAPTURE_GOLDEN=1
	// to rewrite it from the current tree.
	capturedFighterData = "testdata/fighter_character_data.pb"
)

// lieutenantsCloak is the cloak the item tests share: a worn-slot item with
// one pool of one charge paying for Second Wind.
func (s *HandlerTestSuite) lieutenantsCloak() magicitems.Data {
	item, err := magicitems.New(cloakID, magicitems.Description{
		Ref:   core.Ref{Module: "site", Type: "items", ID: cloakID},
		Name:  "Lieutenant's cloak",
		Slots: []string{cloakSlot},
		Pools: map[string]magicitems.Pool{
			"charges": {Maximum: 1, RestoreOn: []coreResources.ResetType{coreResources.ResetShortRest, coreResources.ResetLongRest}},
		},
		Grants: map[string]magicitems.Grant{
			"second-wind": {
				Ability:    *refs.Features.SecondWind(),
				Parameters: json.RawMessage(`{"level":1}`),
				Pool:       "charges",
				Uses:       1,
			},
		},
	})
	s.Require().NoError(err)
	return item
}

// fighterOwningTheCloak is the handler suite's fighter with the cloak added,
// worn or not.
func (s *HandlerTestSuite) fighterOwningTheCloak(worn bool) *character.Data {
	data := s.fighterCharacterEntity().Data
	s.Require().NoError(character.AddItem(data, s.lieutenantsCloak()))
	if worn {
		data.EquipmentSlots[character.InventorySlot(cloakSlot)] = cloakID
	}
	return data
}

func (s *HandlerTestSuite) TestBuildCharacterData_ACloakNotWornIsAnInventoryRowWithNoCatalogueLookup() {
	plain, err := BuildCharacterData(s.project(s.fighterCharacterEntity().Data))
	s.Require().NoError(err)

	cd, err := BuildCharacterData(s.project(s.fighterOwningTheCloak(false)))
	s.Require().NoError(err, "a custom row is not looked up in the catalog")

	s.Require().Len(cd.GetInventory(), len(plain.GetInventory())+1)
	for i, row := range plain.GetInventory() {
		s.True(proto.Equal(row, cd.GetInventory()[i]), "the catalog rows are exactly as before: %s", row.GetRef().GetId())
	}

	row := cd.GetInventory()[len(cd.GetInventory())-1]
	s.Equal(&encounterv2pb.Ref{Module: "site", Type: "items", Id: cloakID}, row.GetRef())
	s.Equal("Lieutenant's cloak", row.GetName())
	s.Equal("Second Wind · 1 of 1 charges", row.GetStatLine())
	s.Equal(cloakSlot, row.GetKind())
	s.Equal([]string{cloakSlot}, row.GetSlotKeys())
	s.Equal(int32(1), row.GetQuantity())
	s.Nil(row.GetPrice(), "no price: the rulebook has none for it")
	s.Empty(row.GetEquipmentType())

	keys := make([]string, 0, len(cd.GetSlots()))
	for _, slot := range cd.GetSlots() {
		keys = append(keys, slot.GetKey())
	}
	s.Contains(keys, cloakSlot)
	s.NotContains(cd.GetEquipped(), cloakSlot)
}

func (s *HandlerTestSuite) TestBuildCharacterData_AWornCloakIsEquippedAtItsOwnRow() {
	plain, err := BuildCharacterData(s.project(s.fighterCharacterEntity().Data))
	s.Require().NoError(err)

	cd, err := BuildCharacterData(s.project(s.fighterOwningTheCloak(true)))
	s.Require().NoError(err)

	var row *encounterv2pb.Item
	for _, item := range cd.GetInventory() {
		if item.GetRef().GetId() == cloakID {
			row = item
		}
	}
	s.Require().NotNil(row)
	s.Require().Contains(cd.GetEquipped(), cloakSlot)
	s.True(proto.Equal(row.GetRef(), cd.GetEquipped()[cloakSlot]), "the slot points at the row's own ref: %s", cd.GetEquipped()[cloakSlot])

	s.True(proto.Equal(plain.GetEquipped()["main_hand"], cd.GetEquipped()["main_hand"]), "the main hand's ref is what it was")
	s.True(proto.Equal(plain.GetEquipped()["off_hand"], cd.GetEquipped()["off_hand"]), "the off hand's ref is what it was")

	var found *encounterv2pb.ResourceView
	for _, resource := range cd.GetResources() {
		if resource.GetName() == "Second Wind · Lieutenant's cloak" {
			found = resource
		}
	}
	s.Require().NotNil(found, "the worn cloak's count is on the sheet")
	s.Equal(int32(1), found.GetCurrent())
	s.Equal(int32(1), found.GetMaximum())
}

func (s *HandlerTestSuite) TestBuildCharacterData_ACharacterWithNoCustomItemsIsByteForByteWhatItWas() {
	cd, err := BuildCharacterData(s.project(s.fighterCharacterEntity().Data))
	s.Require().NoError(err)
	got, err := proto.MarshalOptions{Deterministic: true}.Marshal(cd)
	s.Require().NoError(err)

	if os.Getenv("CAPTURE_GOLDEN") != "" {
		s.Require().NoError(os.MkdirAll(filepath.Dir(capturedFighterData), 0o755))
		s.Require().NoError(os.WriteFile(capturedFighterData, got, 0o600))
	}
	want, err := os.ReadFile(capturedFighterData)
	s.Require().NoError(err)
	s.Equal(want, got)
}
