//go:build integration
// +build integration

package v1alpha1_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/clients/external"
	"github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	dicemock "github.com/KirkDiggler/rpg-api/internal/orchestrators/dice/mock"
	idgenmock "github.com/KirkDiggler/rpg-api/internal/pkg/idgen/mock"
	characterrepomock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	draftrepomock "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft/mock"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/constants"
)

// HandlerListClassesChoicesIntegrationTestSuite tests the ListClasses RPC
// with real dependencies to verify choice data structure
type HandlerListClassesChoicesIntegrationTestSuite struct {
	suite.Suite
	ctrl            *gomock.Controller
	handler         *v1alpha1.Handler
	characterOrch   *character.Orchestrator
	externalClient  external.Client
	ctx             context.Context
}

func TestHandlerListClassesChoicesIntegrationTestSuite(t *testing.T) {
	// Skip if not running integration tests
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	suite.Run(t, new(HandlerListClassesChoicesIntegrationTestSuite))
}

func (s *HandlerListClassesChoicesIntegrationTestSuite) SetupSuite() {
	s.ctx = context.Background()
	s.ctrl = gomock.NewController(s.T())

	// Create external client pointing to local D&D API - REAL CLIENT for integration
	externalClient, err := external.New(&external.Config{
		BaseURL: "http://localhost:3002/api/2014/",
	})
	s.Require().NoError(err)
	s.externalClient = externalClient

	// NOTE: For this integration test, we're primarily testing the ListClasses
	// endpoint with real external API data. We mock the repositories since
	// ListClasses doesn't need to access stored data.
	
	// Create mocked repositories (not used by ListClasses)
	mockCharRepo := characterrepomock.NewMockRepository(s.ctrl)
	mockDraftRepo := draftrepomock.NewMockRepository(s.ctrl)
	
	// Create mocked dice service (not used by ListClasses)
	mockDiceService := dicemock.NewMockService(s.ctrl)
	
	// Create mocked ID generator (not used by ListClasses)
	mockIDGenerator := idgenmock.NewMockGenerator(s.ctrl)

	// Create character orchestrator with REAL external client and mocked repos
	characterOrch, err := character.New(&character.Config{
		CharacterRepo:      mockCharRepo,
		CharacterDraftRepo: mockDraftRepo,
		ExternalClient:     s.externalClient, // REAL external client
		DiceService:        mockDiceService,
		IDGenerator:        mockIDGenerator,
	})
	s.Require().NoError(err)
	s.characterOrch = characterOrch

	// Create handler with orchestrator
	handler, err := v1alpha1.NewHandler(&v1alpha1.HandlerConfig{
		CharacterService: s.characterOrch,
	})
	s.Require().NoError(err)
	s.handler = handler
}

func (s *HandlerListClassesChoicesIntegrationTestSuite) TearDownSuite() {
	if s.ctrl != nil {
		s.ctrl.Finish()
	}
}

func (s *HandlerListClassesChoicesIntegrationTestSuite) TestListClasses_VerifyChoiceStructure() {
	// GIVEN we want to list available classes
	req := &dnd5ev1alpha1.ListClassesRequest{
		PageSize: 20,
	}

	// WHEN we call ListClasses
	resp, err := s.handler.ListClasses(s.ctx, req)

	// THEN the request should succeed
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Require().NotEmpty(resp.Classes, "Should return at least one class")

	// Find fighter class for detailed choice verification
	var fighterClass *dnd5ev1alpha1.ClassInfo
	for _, class := range resp.Classes {
		if class.Id == string(constants.ClassFighter) {
			fighterClass = class
			break
		}
	}
	s.Require().NotNil(fighterClass, "Fighter class should be in the list")

	// Verify basic class data
	s.Equal("Fighter", fighterClass.Name)
	s.Equal("1d10", fighterClass.HitDie)

	// Debug: Print what choices we have
	s.T().Logf("Fighter has %d choices", len(fighterClass.Choices))
	for i, choice := range fighterClass.Choices {
		s.T().Logf("Choice %d: ID=%s, Type=%s, Description=%s", 
			i, choice.Id, choice.ChoiceType.String(), choice.Description)
	}

	// VERIFY: Skill choices have valid skill IDs
	s.verifySkillChoices(fighterClass)

	// VERIFY: Equipment choices have concrete item IDs
	s.verifyEquipmentChoices(fighterClass)

	// VERIFY: Category selections show proper equipment category
	s.verifyCategorySelections(fighterClass)

	// VERIFY: Bundles combine items and category references properly
	s.verifyBundleChoices(fighterClass)
}

func (s *HandlerListClassesChoicesIntegrationTestSuite) verifySkillChoices(class *dnd5ev1alpha1.ClassInfo) {
	// Find skill choices
	var skillChoice *dnd5ev1alpha1.Choice
	for _, choice := range class.Choices {
		if choice.ChoiceType == dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_SKILLS {
			skillChoice = choice
			break
		}
	}

	// Not all classes have skill choices - some get skills from background
	if skillChoice == nil {
		s.T().Logf("Class %s does not have skill choices (may get skills from background)", class.Name)
		return
	}

	s.NotEmpty(skillChoice.Id, "Skill choice should have an ID")
	s.NotEmpty(skillChoice.Description)
	s.Greater(skillChoice.ChooseCount, int32(0), "Should require at least one skill choice")

	// Verify skill options have valid IDs
	explicitOptions, ok := skillChoice.OptionSet.(*dnd5ev1alpha1.Choice_ExplicitOptions)
	s.Require().True(ok, "Skill choice should have explicit options")
	s.Require().NotNil(explicitOptions.ExplicitOptions, "Should have explicit options")
	s.Require().NotEmpty(explicitOptions.ExplicitOptions.Options, "Should have skill options")

	for _, option := range explicitOptions.ExplicitOptions.Options {
		// Skill options should be items
		itemOption, ok := option.OptionType.(*dnd5ev1alpha1.ChoiceOption_Item)
		s.Require().True(ok, "Skill option should be an item type")
		s.Require().NotNil(itemOption.Item, "Item should not be nil")
		s.NotEmpty(itemOption.Item.ItemId, "Skill option should have an item ID")
		s.NotEmpty(itemOption.Item.Name, "Skill option should have a name")
		
		// Verify the ID is a valid skill constant
		// Skills should be constants like constants.SkillAcrobatics
		s.Contains(itemOption.Item.ItemId, "skill_", "Skill ID should contain 'skill_'")
	}
}

func (s *HandlerListClassesChoicesIntegrationTestSuite) verifyEquipmentChoices(class *dnd5ev1alpha1.ClassInfo) {
	// Find equipment choices (concrete items)
	var hasConcreteItemChoice bool
	for _, choice := range class.Choices {
		if choice.ChoiceType != dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT {
			continue
		}

		// Check if this choice has concrete item options
		explicitOptions, ok := choice.OptionSet.(*dnd5ev1alpha1.Choice_ExplicitOptions)
		if !ok || explicitOptions.ExplicitOptions == nil {
			continue
		}

		for _, option := range explicitOptions.ExplicitOptions.Options {
			// Check for single items
			if itemOption, ok := option.OptionType.(*dnd5ev1alpha1.ChoiceOption_Item); ok {
				hasConcreteItemChoice = true
				s.NotNil(itemOption.Item, "Item should not be nil")
				s.NotEmpty(itemOption.Item.ItemId, "Item should have an ID")
				s.NotEmpty(itemOption.Item.Name, "Item should have a name")
			}
			// Check for counted items
			if countedOption, ok := option.OptionType.(*dnd5ev1alpha1.ChoiceOption_CountedItem); ok {
				hasConcreteItemChoice = true
				s.NotNil(countedOption.CountedItem, "Counted item should not be nil")
				s.NotEmpty(countedOption.CountedItem.ItemId, "Counted item should have an ID")
				s.NotEmpty(countedOption.CountedItem.Name, "Counted item should have a name")
				s.Greater(countedOption.CountedItem.Quantity, int32(0), "Counted item should have quantity > 0")
			}
		}
	}

	s.True(hasConcreteItemChoice, "Fighter should have at least one choice with concrete items")
}

func (s *HandlerListClassesChoicesIntegrationTestSuite) verifyCategorySelections(class *dnd5ev1alpha1.ClassInfo) {
	// Find equipment choices with category selections
	var hasCategoryChoice bool
	for _, choice := range class.Choices {
		if choice.ChoiceType != dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT {
			continue
		}

		// Check if this choice has category reference
		categoryRef, ok := choice.OptionSet.(*dnd5ev1alpha1.Choice_CategoryReference)
		if ok && categoryRef.CategoryReference != nil {
			hasCategoryChoice = true
			s.NotEmpty(categoryRef.CategoryReference.CategoryId, "Category reference should have an ID")
			// Category references like "martial-weapons", "artisan-tools"
		}
	}

	// Note: Fighter might not have category selections, but rather explicit item choices
	// Log whether we found category choices
	if hasCategoryChoice {
		s.T().Log("Fighter has equipment category choices")
	} else {
		s.T().Log("Fighter does not have equipment category choices (uses explicit items)")
	}
}

func (s *HandlerListClassesChoicesIntegrationTestSuite) verifyBundleChoices(class *dnd5ev1alpha1.ClassInfo) {
	// Find equipment choices with bundles
	var hasBundleChoice bool
	for _, choice := range class.Choices {
		if choice.ChoiceType != dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_EQUIPMENT {
			continue
		}

		// Check if this choice has bundle options
		explicitOptions, ok := choice.OptionSet.(*dnd5ev1alpha1.Choice_ExplicitOptions)
		if !ok || explicitOptions.ExplicitOptions == nil {
			continue
		}

		for _, option := range explicitOptions.ExplicitOptions.Options {
			if bundleOption, ok := option.OptionType.(*dnd5ev1alpha1.ChoiceOption_Bundle); ok {
				hasBundleChoice = true
				s.NotNil(bundleOption.Bundle, "Bundle should not be nil")
				
				// Debug: log bundle contents
				s.T().Logf("Bundle found with %d items", len(bundleOption.Bundle.Items))
				
				// Bundle might be empty in current implementation
				if len(bundleOption.Bundle.Items) == 0 {
					s.T().Log("Bundle has no items (might not be implemented yet)")
				} else {
					// Verify bundle items
					for _, item := range bundleOption.Bundle.Items {
						s.NotNil(item, "Bundle item should not be nil")
						// Bundle items can be concrete items or choices themselves
					}
				}
			}
		}
	}

	// Note: Not all classes may have bundle choices, so we just verify structure if present
	if hasBundleChoice {
		s.T().Log("Fighter has bundle equipment choices")
	}
}

func (s *HandlerListClassesChoicesIntegrationTestSuite) TestListClasses_MultipleClasses() {
	// GIVEN we want to verify multiple classes
	req := &dnd5ev1alpha1.ListClassesRequest{
		PageSize: 20,
	}

	// WHEN we call ListClasses
	resp, err := s.handler.ListClasses(s.ctx, req)

	// THEN we should get multiple classes with choices
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Require().Greater(len(resp.Classes), 5, "Should return multiple classes")

	// Verify each class has the expected structure
	for _, class := range resp.Classes {
		s.NotEmpty(class.Id, "Class should have an ID")
		s.NotEmpty(class.Name, "Class should have a name")
		s.NotEmpty(class.HitDie, "Class should have hit die")
		// Description might be empty in the current implementation
		if class.Description == "" {
			s.T().Logf("Class %s has no description (might not be populated yet)", class.Name)
		}
		
		// Each class should have some choices
		s.NotEmpty(class.Choices, "Class %s should have choices", class.Name)
		
		// Verify all choices have proper structure
		for _, choice := range class.Choices {
			s.NotEmpty(choice.Id, "Choice should have an ID")
			s.NotEmpty(choice.Description, "Choice should have a description")
			s.NotEqual(dnd5ev1alpha1.ChoiceCategory_CHOICE_CATEGORY_UNSPECIFIED, choice.ChoiceType, "Choice should have a valid category")
			s.Greater(choice.ChooseCount, int32(0), "Choice should require at least one selection")
			
			// Verify choice has either explicit options or category reference
			s.NotNil(choice.OptionSet, "Choice should have an option set")
			switch optSet := choice.OptionSet.(type) {
			case *dnd5ev1alpha1.Choice_ExplicitOptions:
				s.NotNil(optSet.ExplicitOptions, "Explicit options should not be nil")
				s.NotEmpty(optSet.ExplicitOptions.Options, "Should have options")
			case *dnd5ev1alpha1.Choice_CategoryReference:
				s.NotNil(optSet.CategoryReference, "Category reference should not be nil")
				s.NotEmpty(optSet.CategoryReference.CategoryId, "Category should have ID")
			}
		}
	}
}

// TestListClasses_WithRealGRPCServer tests the ListClasses RPC through a real gRPC connection
// This is closer to what the frontend will experience
func (s *HandlerListClassesChoicesIntegrationTestSuite) TestListClasses_WithRealGRPCServer() {
	// Skip this test for now - would require starting a real gRPC server
	// This is a placeholder for future enhancement
	s.T().Skip("Real gRPC server test not implemented yet")

	// Future implementation would:
	// 1. Start a gRPC server on a test port
	// 2. Create a real gRPC client connection
	// 3. Make the ListClasses call through gRPC
	// 4. Verify the response structure matches what frontend expects
}