package character

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/orchestrators/character/mock"
)

type ListSpellsTestSuite struct {
	suite.Suite
	ctrl        *gomock.Controller
	mockService *charactermock.MockService
	handler     *Handler
}

func (s *ListSpellsTestSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
	s.mockService = charactermock.NewMockService(s.ctrl)
	s.handler = &Handler{
		characterService: s.mockService,
	}
}

func (s *ListSpellsTestSuite) TearDownTest() {
	s.ctrl.Finish()
}

func (s *ListSpellsTestSuite) TestListSpellsByLevel_Success() {
	ctx := context.Background()

	// Mock orchestrator response
	mockResult := &character.ListSpellsByLevelOutput{
		Spells: []character.SpellInfo{
			{
				ID:          "fire-bolt",
				Name:        "Fire Bolt",
				Description: "Hurl a mote of fire at a creature or object (1d10 fire damage)",
				Level:       0,
			},
			{
				ID:          "ray-of-frost",
				Name:        "Ray of Frost",
				Description: "A frigid beam that deals 1d8 cold damage and reduces speed by 10 feet",
				Level:       0,
			},
		},
		Total: 2,
	}

	s.mockService.EXPECT().
		ListSpellsByLevel(gomock.Any(), &character.ListSpellsByLevelInput{
			Level:    0,
			PageSize: 0,
		}).
		Return(mockResult, nil)

	// Make request
	req := &dnd5ev1alpha1.ListSpellsByLevelRequest{
		Level:    0,
		PageSize: 0,
	}

	resp, err := s.handler.ListSpellsByLevel(ctx, req)

	// Assertions
	s.Require().NoError(err)
	s.Require().NotNil(resp)
	s.Assert().Len(resp.Spells, 2)
	s.Assert().Equal(int32(2), resp.TotalSize)

	// Check first spell
	s.Assert().Equal(dnd5ev1alpha1.Spell_SPELL_FIRE_BOLT, resp.Spells[0].SpellId)
	s.Assert().Equal("Fire Bolt", resp.Spells[0].Name)
	s.Assert().Equal("Hurl a mote of fire at a creature or object (1d10 fire damage)", resp.Spells[0].Description)
	s.Assert().Equal(int32(0), resp.Spells[0].Level)

	// Check second spell
	s.Assert().Equal(dnd5ev1alpha1.Spell_SPELL_RAY_OF_FROST, resp.Spells[1].SpellId)
	s.Assert().Equal("Ray of Frost", resp.Spells[1].Name)
	s.Assert().Equal("A frigid beam that deals 1d8 cold damage and reduces speed by 10 feet", resp.Spells[1].Description)
	s.Assert().Equal(int32(0), resp.Spells[1].Level)
}

func TestListSpellsTestSuite(t *testing.T) {
	suite.Run(t, new(ListSpellsTestSuite))
}
