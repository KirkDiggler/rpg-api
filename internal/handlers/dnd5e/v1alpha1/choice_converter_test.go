package v1alpha1

import (
	"testing"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/clients/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/entities/dnd5e"
	"github.com/stretchr/testify/suite"
)

type ChoiceConverterTestSuite struct {
	suite.Suite
}

func TestChoiceConverterSuite(t *testing.T) {
	suite.Run(t, new(ChoiceConverterTestSuite))
}

func (s *ChoiceConverterTestSuite) TestConvertEntityChoiceToProto() {
	tests := []struct {
		name     string
		input    *dnd5e.Choice
		choiceID string
		expected *dnd5ev1alpha1.Choice
	}{
		{
			name: "skill choice with explicit options",
			input: &dnd5e.Choice{
				Type:    "skills",
				Choose:  2,
				Options: []string{"Arcana", "History", "Investigation"},
			},
			choiceID: "wizard_skills",
			expected: &dnd5ev1alpha1.Choice{
				Id:          "wizard_skills",
				Description: "Choose 2 skills",
				ChooseCount: 2,
				ChoiceType:  dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_SKILL,
			},
		},
		{
			name: "language choice with category reference",
			input: &dnd5e.Choice{
				Type:   "language",
				Choose: 1,
				From:   "any language",
			},
			choiceID: "race_language_1",
			expected: &dnd5ev1alpha1.Choice{
				Id:          "race_language_1",
				Description: "Choose 1 language",
				ChooseCount: 1,
				ChoiceType:  dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_LANGUAGE,
			},
		},
		{
			name:     "nil choice",
			input:    nil,
			choiceID: "test",
			expected: nil,
		},
	}

	for _, tc := range tests {
		s.Run(tc.name, func() {
			result := convertEntityChoiceToProto(tc.input, tc.choiceID)
			
			if tc.expected == nil {
				s.Nil(result)
				return
			}
			
			s.Require().NotNil(result)
			s.Equal(tc.expected.Id, result.Id)
			s.Equal(tc.expected.Description, result.Description)
			s.Equal(tc.expected.ChooseCount, result.ChooseCount)
			s.Equal(tc.expected.ChoiceType, result.ChoiceType)
			
			// Check option set type
			if tc.input.From != "" && len(tc.input.Options) == 0 {
				s.IsType(&dnd5ev1alpha1.Choice_CategoryReference{}, result.OptionSet)
			} else {
				s.IsType(&dnd5ev1alpha1.Choice_ExplicitOptions{}, result.OptionSet)
			}
		})
	}
}

func (s *ChoiceConverterTestSuite) TestConvertEntityEquipmentChoiceToProto() {
	tests := []struct {
		name     string
		input    *dnd5e.EquipmentChoice
		choiceID string
		expected *dnd5ev1alpha1.Choice
	}{
		{
			name: "equipment with explicit options",
			input: &dnd5e.EquipmentChoice{
				Description: "(a) a mace or (b) a warhammer (if proficient)",
				Options:     []string{"mace", "warhammer"},
				ChooseCount: 1,
			},
			choiceID: "cleric_equipment_1",
			expected: &dnd5ev1alpha1.Choice{
				Id:          "cleric_equipment_1",
				Description: "(a) a mace or (b) a warhammer (if proficient)",
				ChooseCount: 1,
				ChoiceType:  dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_EQUIPMENT,
			},
		},
		{
			name: "equipment with category reference",
			input: &dnd5e.EquipmentChoice{
				Description: "any simple weapon",
				ChooseCount: 1,
			},
			choiceID: "fighter_equipment_2",
			expected: &dnd5ev1alpha1.Choice{
				Id:          "fighter_equipment_2",
				Description: "any simple weapon",
				ChooseCount: 1,
				ChoiceType:  dnd5ev1alpha1.ChoiceType_CHOICE_TYPE_EQUIPMENT,
			},
		},
		{
			name:     "nil equipment choice",
			input:    nil,
			choiceID: "test",
			expected: nil,
		},
	}

	for _, tc := range tests {
		s.Run(tc.name, func() {
			result := convertEntityEquipmentChoiceToProto(tc.input, tc.choiceID)
			
			if tc.expected == nil {
				s.Nil(result)
				return
			}
			
			s.Require().NotNil(result)
			s.Equal(tc.expected.Id, result.Id)
			s.Equal(tc.expected.Description, result.Description)
			s.Equal(tc.expected.ChooseCount, result.ChooseCount)
			s.Equal(tc.expected.ChoiceType, result.ChoiceType)
			
			// Check option set type
			if tc.input.Description == "any simple weapon" {
				s.IsType(&dnd5ev1alpha1.Choice_CategoryReference{}, result.OptionSet)
				catRef := result.OptionSet.(*dnd5ev1alpha1.Choice_CategoryReference)
				s.Equal("simple-weapons", catRef.CategoryReference.CategoryId)
			} else {
				s.IsType(&dnd5ev1alpha1.Choice_ExplicitOptions{}, result.OptionSet)
			}
		})
	}
}

func (s *ChoiceConverterTestSuite) TestDetectEquipmentType() {
	tests := []struct {
		description string
		expected    dnd5ev1alpha1.EquipmentType
	}{
		{
			description: "any simple weapon",
			expected:    dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_SIMPLE_MELEE_WEAPON,
		},
		{
			description: "Choose any martial weapon",
			expected:    dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_MARTIAL_MELEE_WEAPON,
		},
		{
			description: "one set of artisan's tools",
			expected:    dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_ARTISAN_TOOLS,
		},
		{
			description: "(a) a shortsword or (b) any simple weapon",
			expected:    dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_SIMPLE_MELEE_WEAPON,
		},
		{
			description: "a mace and a shield",
			expected:    dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_SHIELD,
		},
		{
			description: "regular equipment with no type",
			expected:    dnd5ev1alpha1.EquipmentType_EQUIPMENT_TYPE_UNSPECIFIED,
		},
	}

	for _, tc := range tests {
		s.Run(tc.description, func() {
			result := detectEquipmentType(tc.description)
			s.Equal(tc.expected, result)
		})
	}
}

func (s *ChoiceConverterTestSuite) TestParseEquipmentOption() {
	tests := []struct {
		name     string
		input    string
		validate func(*dnd5ev1alpha1.ChoiceOption)
	}{
		{
			name:  "simple item",
			input: "shortsword",
			validate: func(opt *dnd5ev1alpha1.ChoiceOption) {
				s.Require().NotNil(opt)
				s.IsType(&dnd5ev1alpha1.ChoiceOption_Item{}, opt.OptionType)
				item := opt.OptionType.(*dnd5ev1alpha1.ChoiceOption_Item)
				s.Equal("shortsword", item.Item.Name)
				s.Equal("shortsword", item.Item.ItemId)
			},
		},
		{
			name:  "item with quantity",
			input: "2 handaxes",
			validate: func(opt *dnd5ev1alpha1.ChoiceOption) {
				s.Require().NotNil(opt)
				s.IsType(&dnd5ev1alpha1.ChoiceOption_CountedItem{}, opt.OptionType)
				counted := opt.OptionType.(*dnd5ev1alpha1.ChoiceOption_CountedItem)
				s.Equal("handaxes", counted.CountedItem.Name)
				s.Equal("handaxes", counted.CountedItem.ItemId)
				s.Equal(int32(2), counted.CountedItem.Quantity)
			},
		},
		{
			name:  "item with prefix",
			input: "(a) a shortsword",
			validate: func(opt *dnd5ev1alpha1.ChoiceOption) {
				s.Require().NotNil(opt)
				s.IsType(&dnd5ev1alpha1.ChoiceOption_Item{}, opt.OptionType)
				item := opt.OptionType.(*dnd5ev1alpha1.ChoiceOption_Item)
				s.Equal("a shortsword", item.Item.Name)
				s.Equal("a-shortsword", item.Item.ItemId)
			},
		},
		{
			name:  "quantity with arrows",
			input: "20 arrows",
			validate: func(opt *dnd5ev1alpha1.ChoiceOption) {
				s.Require().NotNil(opt)
				s.IsType(&dnd5ev1alpha1.ChoiceOption_CountedItem{}, opt.OptionType)
				counted := opt.OptionType.(*dnd5ev1alpha1.ChoiceOption_CountedItem)
				s.Equal("arrows", counted.CountedItem.Name)
				s.Equal(int32(20), counted.CountedItem.Quantity)
			},
		},
		{
			name:  "nested choice with specific items",
			input: "(a) a mace or (b) a warhammer",
			validate: func(opt *dnd5ev1alpha1.ChoiceOption) {
				s.Require().NotNil(opt)
				s.IsType(&dnd5ev1alpha1.ChoiceOption_NestedChoice{}, opt.OptionType)
				nested := opt.OptionType.(*dnd5ev1alpha1.ChoiceOption_NestedChoice)
				s.Require().NotNil(nested.NestedChoice.Choice)
				s.Equal("(a) a mace or (b) a warhammer", nested.NestedChoice.Choice.Description)
				s.Equal(int32(1), nested.NestedChoice.Choice.ChooseCount)
				
				// Check the nested options
				s.IsType(&dnd5ev1alpha1.Choice_ExplicitOptions{}, nested.NestedChoice.Choice.OptionSet)
				explicitOpts := nested.NestedChoice.Choice.OptionSet.(*dnd5ev1alpha1.Choice_ExplicitOptions)
				s.Len(explicitOpts.ExplicitOptions.Options, 2)
				
				// First option should be "a mace"
				opt1 := explicitOpts.ExplicitOptions.Options[0]
				s.IsType(&dnd5ev1alpha1.ChoiceOption_Item{}, opt1.OptionType)
				item1 := opt1.OptionType.(*dnd5ev1alpha1.ChoiceOption_Item)
				s.Equal("a mace", item1.Item.Name)
				
				// Second option should be "a warhammer"
				opt2 := explicitOpts.ExplicitOptions.Options[1]
				s.IsType(&dnd5ev1alpha1.ChoiceOption_Item{}, opt2.OptionType)
				item2 := opt2.OptionType.(*dnd5ev1alpha1.ChoiceOption_Item)
				s.Equal("a warhammer", item2.Item.Name)
			},
		},
		{
			name:  "nested choice with category reference",
			input: "(a) a shortsword or (b) any simple weapon",
			validate: func(opt *dnd5ev1alpha1.ChoiceOption) {
				s.Require().NotNil(opt)
				s.IsType(&dnd5ev1alpha1.ChoiceOption_NestedChoice{}, opt.OptionType)
				nested := opt.OptionType.(*dnd5ev1alpha1.ChoiceOption_NestedChoice)
				s.Require().NotNil(nested.NestedChoice.Choice)
				
				// Check the nested options
				s.IsType(&dnd5ev1alpha1.Choice_ExplicitOptions{}, nested.NestedChoice.Choice.OptionSet)
				explicitOpts := nested.NestedChoice.Choice.OptionSet.(*dnd5ev1alpha1.Choice_ExplicitOptions)
				s.Len(explicitOpts.ExplicitOptions.Options, 2)
				
				// First option should be "a shortsword"
				opt1 := explicitOpts.ExplicitOptions.Options[0]
				s.IsType(&dnd5ev1alpha1.ChoiceOption_Item{}, opt1.OptionType)
				item1 := opt1.OptionType.(*dnd5ev1alpha1.ChoiceOption_Item)
				s.Equal("a shortsword", item1.Item.Name)
				s.Equal("a-shortsword", item1.Item.ItemId)
				
				// Second option should be "any simple weapon" with category ID
				opt2 := explicitOpts.ExplicitOptions.Options[1]
				s.IsType(&dnd5ev1alpha1.ChoiceOption_Item{}, opt2.OptionType)
				item2 := opt2.OptionType.(*dnd5ev1alpha1.ChoiceOption_Item)
				s.Equal("any simple weapon", item2.Item.Name)
				s.Equal("simple-weapons", item2.Item.ItemId) // Should be the category ID
			},
		},
	}

	for _, tc := range tests {
		s.Run(tc.name, func() {
			result := parseEquipmentOption(tc.input)
			tc.validate(result)
		})
	}
}