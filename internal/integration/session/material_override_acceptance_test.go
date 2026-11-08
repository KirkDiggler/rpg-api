package session_test

import (
	"context"
	"os"
	"strings"
	"time"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	sessionhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
)

func (s *GroundAcceptanceSuite) TestNativeMaterialChoicesSurviveSourceEditsAndFreshHostReads() {
	const selected = "dnd5e:material:fantasy-kingdom:atlas_04_c"
	const changed = "dnd5e:material:fantasy-kingdom:atlas_01_a"
	h := newAcceptanceHarness(s.T())
	ctx := auth.WithPlayerID(context.Background(), "player-alice")
	_, err := h.charRepo.Create(ctx, characterrepo.CreateInput{Character: &entities.Character{Data: armedFighter("alice", "player-alice")}})
	s.Require().NoError(err)
	raw, err := os.ReadFile("testdata/material-overrides.yaml")
	s.Require().NoError(err)
	compiled, err := sessionworld.Compile(raw)
	s.Require().NoError(err)
	h.launchAtStart(s.T(), "material-run", compiled.Spec, "alice")
	request := &sessionpb.GetKnowledgeRequest{Session: "material-run", Member: "alice"}
	before, err := h.handler.GetKnowledge(ctx, request)
	s.Require().NoError(err)
	assertChoices := func(atlas *sessionpb.GetAtlasResponse) {
		s.Require().Len(atlas.StructuralWalls, 1)
		s.Require().Len(atlas.PropPresentations, 1)
		for _, values := range [][]*sessionpb.MaterialOverride{atlas.StructuralWalls[0].MaterialOverrides, atlas.PropPresentations[0].MaterialOverrides} {
			s.Require().Len(values, 1)
			s.Equal("atlas", values[0].SurfaceId)
			s.Equal(selected, values[0].MaterialRef)
		}
	}
	assertChoices(before.Atlas)
	later, err := sessionworld.Compile([]byte(strings.ReplaceAll(string(raw), selected, changed)))
	s.Require().NoError(err)
	s.Equal(changed, later.Spec.Field.StructuralWalls[0].MaterialOverrides[0].MaterialRef)
	s.Equal(changed, later.Spec.Field.PropPresentations[0].MaterialOverrides[0].MaterialRef)
	compiled.Spec.Field.StructuralWalls[0].MaterialOverrides[0].MaterialRef = changed
	compiled.Spec.Field.PropPresentations[0].MaterialOverrides[0].MaterialRef = changed
	orch, err := sessionorch.New(sessionorch.Config{Redis: h.redis, Characters: h.charRepo, TTL: 24 * time.Hour, Dice: testDice{}, PresentationIDs: idgen.NewSequential("material-reload")})
	s.Require().NoError(err)
	fresh, err := sessionhandler.New(&sessionhandler.HandlerConfig{Manager: orch.Manager, Characters: h.charRepo, Preferences: orch.Preferences})
	s.Require().NoError(err)
	after, err := fresh.GetKnowledge(ctx, request)
	s.Require().NoError(err)
	assertChoices(after.Atlas)
	s.Equal(before.Atlas.Cells, after.Atlas.Cells)
	s.Equal(before.Atlas.StructuralWalls, after.Atlas.StructuralWalls)
	s.Equal(before.Atlas.PropPresentations, after.Atlas.PropPresentations)
}
