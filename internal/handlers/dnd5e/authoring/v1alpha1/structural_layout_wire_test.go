package authoringv1alpha1_test

import (
	"context"
	_ "embed"
	"math"
	"testing"

	"github.com/stretchr/testify/suite"

	authoringpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/authoring/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	authoringhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/authoring/v1alpha1"
	authoringorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/authoring"
)

const structuralLayoutKey = "structural-layout-wire"

//go:embed testdata/structural_layout.yaml
var structuralLayoutSource string

type StructuralLayoutWireSuite struct{ suite.Suite }

func TestStructuralLayoutWireSuite(t *testing.T) { suite.Run(t, new(StructuralLayoutWireSuite)) }

func (s *StructuralLayoutWireSuite) TestRealRegistryAndSDKCarryAuthoredLayoutToTheWire() {
	ctx := auth.WithPlayerID(context.Background(), "structural-author")
	registry, _ := dungeonstest.Scratch(s.T())
	orch, err := authoringorch.New(&authoringorch.Config{Dungeons: registry})
	s.Require().NoError(err)
	handler, err := authoringhandler.New(&authoringhandler.HandlerConfig{Orchestrator: orch})
	s.Require().NoError(err)
	k := 5 / math.Sqrt(3)
	for _, validateOnly := range []bool{true, false} {
		response, putErr := handler.PutDungeon(ctx, &authoringpb.PutDungeonRequest{
			Key: structuralLayoutKey, Yaml: structuralLayoutSource, ValidateOnly: validateOnly,
		})
		s.Require().NoError(putErr)
		s.Require().Empty(response.Errors)
		s.Require().NotNil(response.Atlas)
		s.Require().Len(response.Atlas.StructuralWalls, 1)
		wall := response.Atlas.StructuralWalls[0]
		s.Equal("wall", wall.Id)
		s.Equal("dnd5e:env:dark-fortress:45_wall_01", wall.Ref)
		s.InDelta(0, wall.From.X, 1e-12)
		s.InDelta(10*k, wall.To.X, 1e-12)
		s.InDelta(3*k, wall.From.Y, 1e-12)
		s.InDelta(2*k, wall.Height, 1e-12)
		s.InDelta(0.3*k, wall.Thickness, 1e-12, "not the independent blocker depth")
		s.Require().Len(wall.Openings, 1)
		s.InDelta(7*k, wall.Openings[0].Position, 1e-12)
		s.InDelta(2*k, wall.Openings[0].Width, 1e-12)
		s.Require().Len(response.Atlas.StructuralDoors, 1)
		door := response.Atlas.StructuralDoors[0]
		s.Equal(structuralLayoutKey+"/gate", door.Id)
		s.InDelta(6*k, door.From.X, 1e-12)
		s.InDelta(8*k, door.To.X, 1e-12)
		s.InDelta(3*k, door.From.Y, 1e-12, "visual pose does not inherit the blocker offset")
		s.Empty(response.Atlas.RoomSceneJson)
	}
	saved, err := handler.GetDungeon(ctx, &authoringpb.GetDungeonRequest{Key: structuralLayoutKey})
	s.Require().NoError(err)
	s.Equal(structuralLayoutSource, saved.Yaml, "publish stores source bytes, not the projection")
}
