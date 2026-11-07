package session_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	sessionhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
)

type GroundAcceptanceSuite struct{ suite.Suite }

func TestGroundAcceptanceSuite(t *testing.T) { suite.Run(t, new(GroundAcceptanceSuite)) }

func (s *GroundAcceptanceSuite) TestNativeGroundIsCapturedAndSurvivesFreshHostReadsAfterSourceEdits() {
	const grass = "dnd5e:ground:fantasy-kingdom:grass_01"
	const dirt = "dnd5e:ground:fantasy-kingdom:dirt_01"
	const stone = "dnd5e:ground:dungeon:stone_01"
	h := newAcceptanceHarness(s.T())
	ctx := auth.WithPlayerID(context.Background(), "player-alice")
	_, err := h.charRepo.Create(ctx, characterrepo.CreateInput{Character: &entities.Character{Data: armedFighter("alice", "player-alice")}})
	s.Require().NoError(err)
	raw, err := os.ReadFile("testdata/courtyard-ground.yaml")
	s.Require().NoError(err)
	source := filepath.Join(s.T().TempDir(), "courtyard.yaml")
	s.Require().NoError(os.WriteFile(source, raw, 0o600))
	compiled, err := sessionworld.Compile(raw)
	s.Require().NoError(err)
	h.launchAtStart(s.T(), "ground-run", compiled.Spec, "alice")
	request := &sessionpb.GetKnowledgeRequest{Session: "ground-run", Member: "alice"}
	before, err := h.handler.GetKnowledge(ctx, request)
	s.Require().NoError(err)
	groundOf := func(atlas *sessionpb.GetAtlasResponse) *sessionpb.RegionGround {
		for _, region := range atlas.Regions {
			if region.Id == "courtyard" {
				return region.Ground
			}
		}
		return nil
	}
	beforeGround := groundOf(before.Atlas)
	s.Require().NotNil(beforeGround)
	s.Equal(grass, beforeGround.BaseRef)
	s.Require().Len(beforeGround.Paint, 1)
	s.Equal(dirt, beforeGround.Paint[0].Ref)
	s.Len(beforeGround.Paint[0].Cells, 3)

	// Changing the valid authored source and its compiled input cannot repaint
	// a started run. The new host below only has the persisted run to read.
	changed := []byte(strings.ReplaceAll(string(raw), grass, stone))
	s.Require().NoError(os.WriteFile(source, changed, 0o600))
	reread, err := os.ReadFile(source)
	s.Require().NoError(err)
	later, err := sessionworld.Compile(reread)
	s.Require().NoError(err)
	s.Equal(stone, later.Spec.Field.Regions[0].Ground.BaseRef)
	compiled.Spec.Field.Regions[0].Ground.BaseRef = stone
	orch, err := sessionorch.New(sessionorch.Config{Redis: h.redis, Characters: h.charRepo, TTL: 24 * time.Hour,
		Dice: testDice{}, PresentationIDs: idgen.NewSequential("ground-reload")})
	s.Require().NoError(err)
	fresh, err := sessionhandler.New(&sessionhandler.HandlerConfig{Manager: orch.Manager, Characters: h.charRepo, Preferences: orch.Preferences})
	s.Require().NoError(err)
	after, err := fresh.GetKnowledge(ctx, request)
	s.Require().NoError(err)
	afterGround := groundOf(after.Atlas)
	s.Require().NotNil(afterGround)
	s.Equal(grass, afterGround.BaseRef)
	s.Equal(beforeGround.Paint, afterGround.Paint)
	s.Equal(before.Atlas.Cells, after.Atlas.Cells, "appearance does not change floor membership")
}
