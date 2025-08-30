package character_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
)

type ListClassesTestSuite struct {
	suite.Suite
	ctrl         *gomock.Controller
	orchestrator *character.Orchestrator
}

func TestListClassesTestSuite(t *testing.T) {
	suite.Run(t, new(ListClassesTestSuite))
}

func (s *ListClassesTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	
	// Create orchestrator with minimal dependencies
	// We're testing the actual toolkit integration, not mocking it
	s.orchestrator = &character.Orchestrator{}
}

func (s *ListClassesTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *ListClassesTestSuite) TestListClasses_ReturnsAllClasses() {
	ctx := context.Background()
	
	output, err := s.orchestrator.ListClasses(ctx, &character.ListClassesInput{})
	s.Require().NoError(err)
	s.Require().NotNil(output)
	
	// Should have 12 base classes (Fighter, Barbarian, Bard, Cleric, Druid, Monk, Paladin, Ranger, Rogue, Sorcerer, Warlock, Wizard)
	s.Equal(12, len(output.Classes), "Should have exactly 12 base classes")
	
	// Verify we have some expected classes
	classMap := make(map[string]bool)
	for _, class := range output.Classes {
		classMap[string(class.ID)] = true
	}
	
	// Check base classes that don't have level 1 subclasses
	s.True(classMap["fighter"], "Should have Fighter")
	s.True(classMap["rogue"], "Should have Rogue")
	s.True(classMap["wizard"], "Should have Wizard")
	
	// Check classes that have level 1 subclasses
	s.True(classMap["cleric"], "Should have Cleric")
	s.True(classMap["sorcerer"], "Should have Sorcerer")
	s.True(classMap["warlock"], "Should have Warlock")
	
	// Verify Cleric has subclasses nested within it
	var cleric *toolkitchar.StartingClass
	for _, class := range output.Classes {
		if class.ID == classes.Cleric {
			cleric = class
			break
		}
	}
	s.Require().NotNil(cleric, "Cleric should be in the list")
	s.Greater(len(cleric.Subclass), 0, "Cleric should have subclasses")
	
	// Check that specific subclasses exist within Cleric
	hasLifeDomain := false
	hasLightDomain := false
	for _, subclass := range cleric.Subclass {
		if subclass.ID == classes.LifeDomain {
			hasLifeDomain = true
		}
		if subclass.ID == classes.LightDomain {
			hasLightDomain = true
		}
	}
	s.True(hasLifeDomain, "Cleric should have Life Domain subclass")
	s.True(hasLightDomain, "Cleric should have Light Domain subclass")
}

func (s *ListClassesTestSuite) TestListClasses_RogueHasExpertise() {
	ctx := context.Background()
	
	output, err := s.orchestrator.ListClasses(ctx, &character.ListClassesInput{})
	s.Require().NoError(err)
	
	// Find Rogue
	var rogue *toolkitchar.StartingClass
	for _, class := range output.Classes {
		if class.ID == "rogue" {
			rogue = class
			break
		}
	}
	
	s.Require().NotNil(rogue, "Rogue class should exist")
	s.Require().NotNil(rogue.Requirements, "Rogue should have requirements")
	
	// Check for expertise requirement
	// Expertise is typically a separate requirement from skills
	// Let's examine what requirements Rogue actually has
	s.T().Logf("Rogue requirements:")
	if rogue.Requirements.Skills != nil {
		s.T().Logf("  Skills: Count=%d, Label=%s, Options=%v", 
			rogue.Requirements.Skills.Count,
			rogue.Requirements.Skills.Label,
			rogue.Requirements.Skills.Options)
	}
	
	// Check for expertise requirement
	s.Require().NotNil(rogue.Requirements.Expertise, "Rogue should have expertise requirements")
	s.Equal(2, rogue.Requirements.Expertise.Count, "Rogue should choose 2 skills/tools for expertise")
	s.NotEmpty(rogue.Requirements.Expertise.Label, "Expertise should have a descriptive label")
	s.T().Logf("  Expertise: Count=%d, Label=%s", 
		rogue.Requirements.Expertise.Count,
		rogue.Requirements.Expertise.Label)
	
	// Also verify Rogue has 4 skill choices
	s.Require().NotNil(rogue.Requirements.Skills, "Rogue should have skill requirements")
	s.Equal(4, rogue.Requirements.Skills.Count, "Rogue should choose 4 skills")
}

func (s *ListClassesTestSuite) TestListClasses_FighterHasEquipmentChoices() {
	ctx := context.Background()
	
	output, err := s.orchestrator.ListClasses(ctx, &character.ListClassesInput{})
	s.Require().NoError(err)
	
	// Find Fighter
	var fighter *toolkitchar.StartingClass
	for _, class := range output.Classes {
		if class.ID == "fighter" {
			fighter = class
			break
		}
	}
	
	s.Require().NotNil(fighter, "Fighter class should exist")
	s.Require().NotNil(fighter.Requirements, "Fighter should have requirements")
	
	// Fighter should have equipment choices
	s.Require().NotEmpty(fighter.Requirements.Equipment, "Fighter should have equipment choices")
	
	// Log equipment choices for debugging
	for i, eq := range fighter.Requirements.Equipment {
		s.T().Logf("Fighter equipment choice %d: %s", i+1, eq.Label)
		for j, opt := range eq.Options {
			s.T().Logf("  Option %d: %s", j+1, opt.Label)
		}
	}
}

func (s *ListClassesTestSuite) TestListClasses_ClericHasCantripsButNoSpells() {
	ctx := context.Background()
	
	output, err := s.orchestrator.ListClasses(ctx, &character.ListClassesInput{})
	s.Require().NoError(err)
	
	// Find Cleric
	var cleric *toolkitchar.StartingClass
	for _, class := range output.Classes {
		if class.ID == classes.Cleric {
			cleric = class
			break
		}
	}
	
	s.Require().NotNil(cleric, "Cleric should exist")
	s.Require().NotNil(cleric.Requirements, "Cleric should have requirements")
	
	// Check for cantrips
	s.Require().NotNil(cleric.Requirements.Cantrips, "Cleric should have cantrip requirements")
	s.Equal(3, cleric.Requirements.Cantrips.Count, "Clerics choose 3 cantrips at level 1")
	s.Equal(0, cleric.Requirements.Cantrips.Level, "Cantrips are level 0")
	
	// Clerics PREPARE spells, they don't LEARN them during character creation
	s.Nil(cleric.Requirements.Spells, "Cleric should NOT have spell learning requirements (they prepare spells)")
	
	// Verify Life Domain exists as a subclass
	hasLifeDomain := false
	for _, subclass := range cleric.Subclass {
		if subclass.ID == classes.LifeDomain {
			hasLifeDomain = true
			// Life Domain grants heavy armor proficiency
			s.Require().NotNil(subclass.Grants, "Life Domain should have grants")
			break
		}
	}
	s.True(hasLifeDomain, "Cleric should have Life Domain as a subclass")
}

func (s *ListClassesTestSuite) TestListClasses_WizardHasSpellbook() {
	ctx := context.Background()
	
	output, err := s.orchestrator.ListClasses(ctx, &character.ListClassesInput{})
	s.Require().NoError(err)
	
	// Find Wizard
	var wizard *toolkitchar.StartingClass
	for _, class := range output.Classes {
		if class.ID == classes.Wizard {
			wizard = class
			break
		}
	}
	
	s.Require().NotNil(wizard, "Wizard class should exist")
	s.Require().NotNil(wizard.Requirements, "Wizard should have requirements")
	
	// Wizard should have cantrips
	s.Require().NotNil(wizard.Requirements.Cantrips, "Wizard should have cantrip requirements")
	s.Equal(3, wizard.Requirements.Cantrips.Count, "Wizard chooses 3 cantrips at level 1")
	
	// Wizard should have spells (6 in spellbook at level 1)
	s.Require().NotNil(wizard.Requirements.Spells, "Wizard should have spell requirements")
	s.Equal(6, wizard.Requirements.Spells.Count, "Wizard starts with 6 spells in spellbook")
}

func (s *ListClassesTestSuite) TestListClasses_BarbarianHasMinimalChoices() {
	ctx := context.Background()
	
	output, err := s.orchestrator.ListClasses(ctx, &character.ListClassesInput{})
	s.Require().NoError(err)
	
	// Find Barbarian
	var barbarian *toolkitchar.StartingClass
	for _, class := range output.Classes {
		if class.ID == classes.Barbarian {
			barbarian = class
			break
		}
	}
	
	s.Require().NotNil(barbarian, "Barbarian class should exist")
	s.Require().NotNil(barbarian.Requirements, "Barbarian should have requirements")
	
	// Barbarian should have skill choices
	s.Require().NotNil(barbarian.Requirements.Skills, "Barbarian should have skill requirements")
	s.Equal(2, barbarian.Requirements.Skills.Count, "Barbarian chooses 2 skills")
	
	// Barbarian should NOT have spell choices
	s.Nil(barbarian.Requirements.Cantrips, "Barbarian should not have cantrips")
	s.Nil(barbarian.Requirements.Spells, "Barbarian should not have spells")
}

func (s *ListClassesTestSuite) TestListClasses_SubclassStructure() {
	ctx := context.Background()
	
	output, err := s.orchestrator.ListClasses(ctx, &character.ListClassesInput{})
	s.Require().NoError(err)
	
	// Find specific classes to check their subclass structure
	var cleric, sorcerer, warlock, fighter *toolkitchar.StartingClass
	for _, class := range output.Classes {
		switch class.ID {
		case classes.Cleric:
			cleric = class
		case classes.Sorcerer:
			sorcerer = class
		case classes.Warlock:
			warlock = class
		case classes.Fighter:
			fighter = class
		}
	}
	
	// Cleric should have subclasses (domains at level 1)
	s.Require().NotNil(cleric, "Cleric should be in the list")
	s.Greater(len(cleric.Subclass), 0, "Cleric should have subclasses")
	s.T().Logf("Cleric has %d subclasses", len(cleric.Subclass))
	
	// Sorcerer should have subclasses (origins at level 1)
	s.Require().NotNil(sorcerer, "Sorcerer should be in the list")
	s.Greater(len(sorcerer.Subclass), 0, "Sorcerer should have subclasses")
	s.T().Logf("Sorcerer has %d subclasses", len(sorcerer.Subclass))
	
	// Warlock should have subclasses (patrons at level 1)
	s.Require().NotNil(warlock, "Warlock should be in the list")
	s.Greater(len(warlock.Subclass), 0, "Warlock should have subclasses")
	s.T().Logf("Warlock has %d subclasses", len(warlock.Subclass))
	
	// Fighter should NOT have subclasses (gets them at level 3)
	s.Require().NotNil(fighter, "Fighter should be in the list")
	s.Empty(fighter.Subclass, "Fighter should not have level 1 subclasses")
	s.T().Logf("Fighter has %d subclasses", len(fighter.Subclass))
}

func (s *ListClassesTestSuite) TestListClasses_AllClassesHaveRequiredFields() {
	ctx := context.Background()
	
	output, err := s.orchestrator.ListClasses(ctx, &character.ListClassesInput{})
	s.Require().NoError(err)
	
	for _, class := range output.Classes {
		// Every class should have basic fields
		s.NotEmpty(class.ID, "Class should have ID")
		
		// Every class should have grants
		s.Require().NotNil(class.Grants, "Class %s should have Grants", class.ID)
		s.Greater(class.Grants.HitDice, 0, "Class %s should have HitDice", class.ID)
		s.Len(class.Grants.SavingThrows, 2, "Class %s should have 2 saving throw proficiencies", class.ID)
		
		// Every class should have requirements (even if some fields are nil)
		s.NotNil(class.Requirements, "Class %s should have Requirements", class.ID)
	}
}