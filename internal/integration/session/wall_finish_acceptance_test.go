package session_test

import (
	"context"
	"os"
	"strings"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
)

func (s *GroundAcceptanceSuite) TestNativeWallFinishSurvivesColdHostAfterSourceEdit() {
	const brick = "dnd5e:material:fantasy-kingdom:wall_brick_01"
	const stucco = "dnd5e:material:fantasy-kingdom:wall_stucco_01"
	h := newAcceptanceHarness(s.T())
	ctx := auth.WithPlayerID(context.Background(), "player-alice")
	_, err := h.charRepo.Create(ctx, characterrepo.CreateInput{Character: &entities.Character{Data: armedFighter("alice", "player-alice")}})
	s.Require().NoError(err)
	raw, err := os.ReadFile("testdata/wall-finishes.yaml")
	s.Require().NoError(err)
	compiled, err := sessionworld.Compile(raw)
	s.Require().NoError(err)
	h.launchAtStart(s.T(), "wall-finish-run", compiled.Spec, "alice")
	request := &sessionpb.GetKnowledgeRequest{Session: "wall-finish-run", Member: "alice"}
	before, err := h.handler.GetKnowledge(ctx, request)
	s.Require().NoError(err)
	s.Require().Len(before.Atlas.StructuralWalls, 1)
	faces := before.Atlas.StructuralWalls[0].FaceFinishes
	s.Require().Len(faces, 2)
	s.Equal("positive-z", faces[0].Side)
	s.Equal(brick, faces[0].MaterialRef)
	s.Equal("negative-z", faces[1].Side)
	s.Equal(stucco, faces[1].MaterialRef)
	later, err := sessionworld.Compile([]byte(strings.ReplaceAll(string(raw), brick, stucco)))
	s.Require().NoError(err)
	s.Equal(stucco, later.Spec.Field.StructuralWalls[0].FaceFinishes[0].MaterialRef)
	compiled.Spec.Field.StructuralWalls[0].FaceFinishes[0].MaterialRef = stucco
	coldSites, err := sessionorch.NewSites(nil)
	s.Require().NoError(err)
	s.Zero(coldSites.Compiles())
	fresh := freshSessionHandler(s.T(), h, coldSites)
	after, err := fresh.GetKnowledge(ctx, request)
	s.Require().NoError(err)
	s.Equal(1, coldSites.Compiles(), "read the captured site from Redis, not a warm provider or current source")
	s.Equal(before.Atlas.StructuralWalls, after.Atlas.StructuralWalls)
	s.Equal(before.Atlas.Cells, after.Atlas.Cells)
}
