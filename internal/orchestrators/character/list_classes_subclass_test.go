package character_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/proficiencies"
)

type ListClassesSubclassTestSuite struct {
	suite.Suite
	ctrl         *gomock.Controller
	orchestrator *character.Orchestrator
}

func TestListClassesSubclassTestSuite(t *testing.T) {
	suite.Run(t, new(ListClassesSubclassTestSuite))
}

func (s *ListClassesSubclassTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.orchestrator = &character.Orchestrator{}
}

func (s *ListClassesSubclassTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *ListClassesSubclassTestSuite) TestListClasses_ClericHasSubclasses() {
	ctx := context.Background()

	output, err := s.orchestrator.ListClasses(ctx, &character.ListClassesInput{})
	s.Require().NoError(err)

	// Find the Cleric class
	var cleric *toolkitchar.StartingClass
	for _, class := range output.Classes {
		if class.ID == classes.Cleric {
			cleric = class
			break
		}
	}

	s.Require().NotNil(cleric, "Should have Cleric class")
	s.Require().NotEmpty(cleric.Subclass, "Cleric should have subclasses")
	s.GreaterOrEqual(len(cleric.Subclass), 6, "Cleric should have at least 6 domains")

	// Verify each subclass has proper data
	subclassMap := make(map[classes.Subclass]bool)
	for _, subclass := range cleric.Subclass {
		s.T().Logf("Found subclass: %s at level %d", subclass.ID, subclass.Level)

		// All Cleric subclasses are at level 1
		s.Equal(1, subclass.Level, "Cleric domains should be at level 1")

		// Each subclass should have grants (what it adds to base class)
		s.NotNil(subclass.Grants, "Subclass should have grants")

		// Track which subclasses we found
		subclassMap[subclass.ID] = true
	}

	// Verify specific domains exist
	s.True(subclassMap[classes.LifeDomain], "Should have Life Domain")
	s.True(subclassMap[classes.LightDomain], "Should have Light Domain")
	s.True(subclassMap[classes.KnowledgeDomain], "Should have Knowledge Domain")
	s.True(subclassMap[classes.NatureDomain], "Should have Nature Domain")
	s.True(subclassMap[classes.TempestDomain], "Should have Tempest Domain")
	s.True(subclassMap[classes.TrickeryDomain], "Should have Trickery Domain")
}

func (s *ListClassesSubclassTestSuite) TestListClasses_ClassesWithoutSubclasses() {
	ctx := context.Background()

	output, err := s.orchestrator.ListClasses(ctx, &character.ListClassesInput{})
	s.Require().NoError(err)

	// Classes that shouldn't have subclasses at level 1
	noSubclassClasses := []classes.Class{
		classes.Fighter,
		classes.Rogue,
		classes.Wizard,
		classes.Barbarian,
		classes.Bard,
		classes.Druid,
		classes.Monk,
		classes.Paladin,
		classes.Ranger,
	}

	for _, expectedClass := range noSubclassClasses {
		var found bool
		for _, class := range output.Classes {
			if class.ID == expectedClass {
				found = true
				s.Empty(class.Subclass, "%s should not have subclasses at level 1", expectedClass)
				break
			}
		}
		s.True(found, "Should have %s class", expectedClass)
	}
}

func (s *ListClassesSubclassTestSuite) TestListClasses_SubclassGrants() {
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

	s.Require().NotNil(cleric, "Should have Cleric class")

	// Check specific domain grants
	for _, subclass := range cleric.Subclass {
		switch subclass.ID {
		case classes.LifeDomain:
			// Life Domain should grant heavy armor proficiency
			s.Contains(subclass.Grants.ArmorProficiencies, proficiencies.ArmorHeavy,
				"Life Domain should grant heavy armor proficiency")

		case classes.KnowledgeDomain:
			// Knowledge Domain has unique requirements (extra languages)
			if subclass.Requirements != nil && subclass.Requirements.Languages != nil {
				s.Equal(2, subclass.Requirements.Languages.Count,
					"Knowledge Domain should grant 2 language choices")
			}

		case classes.NatureDomain:
			// Nature Domain might have unique cantrip requirement
			if subclass.Requirements != nil && subclass.Requirements.Cantrips != nil {
				s.T().Logf("Nature Domain cantrip requirement: %+v", subclass.Requirements.Cantrips)
			}
		}
	}
}
