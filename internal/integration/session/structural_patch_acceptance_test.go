package session_test

import (
	"context"
	_ "embed"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/proto"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	sessionhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

// The wall crosses between axial cells (1,0) and (2,0). Only the door is
// concealed; none of the five floor cells is a secret. Ordinary room discovery
// still withholds the opposite side of the closed geometric divider.
//
//go:embed testdata/structural_patch.yaml
var structuralPatchSource string

type StructuralPatchAcceptanceSuite struct{ suite.Suite }

func TestStructuralPatchAcceptanceSuite(t *testing.T) {
	suite.Run(t, new(StructuralPatchAcceptanceSuite))
}

func (s *StructuralPatchAcceptanceSuite) TestCompiledDoorDiscoveryReplacesOpeningsOnTheWire() {
	h := newAcceptanceHarness(s.T())
	// The shared older fixtures retain legacy Search. This case adopts the
	// production automatic-discovery capability over the same real repositories.
	automatic, err := sessionorch.New(sessionorch.Config{
		Redis: h.redis, Characters: h.charRepo, TTL: time.Hour, Dice: testDice{},
		PresentationIDs: idgen.NewSequential("patch-presentation"), AutomaticDiscovery: true,
	})
	s.Require().NoError(err)
	h.manager = automatic
	h.handler, err = sessionhandler.New(&sessionhandler.HandlerConfig{
		Manager: automatic.Manager, Broker: automatic.Broker, Characters: h.charRepo,
	})
	s.Require().NoError(err)
	ctx := auth.WithPlayerID(context.Background(), "player-alice")
	_, err = h.charRepo.Create(ctx, characterrepo.CreateInput{
		Character: &entities.Character{Data: armedFighter("alice", "player-alice")},
	})
	s.Require().NoError(err)
	dungeon, err := sessionworld.Compile([]byte(structuralPatchSource))
	s.Require().NoError(err)
	_, err = h.manager.Manager.StartSession(ctx, &sdk.StartSessionInput{
		Session: "structural-patch-run", Encounter: "structural-world", World: dungeon.World,
	})
	s.Require().NoError(err)
	_, err = h.handler.Join(ctx, &sessionpb.JoinRequest{Session: "structural-patch-run", Member: "alice", Position: pbAt(4, 0)})
	s.Require().NoError(err)
	request := &sessionpb.GetKnowledgeRequest{Session: "structural-patch-run", Member: "alice"}
	before, err := h.handler.GetKnowledge(ctx, request)
	s.Require().NoError(err)
	s.Require().Len(before.Atlas.StructuralWalls, 1)
	wall := before.Atlas.StructuralWalls[0]
	s.Equal("wall", wall.Id)
	s.Equal("dnd5e:env:dark-fortress:45_wall_01", wall.Ref)
	feetPerSceneUnit := 5 / math.Sqrt(3)
	s.Require().NotNil(wall.From)
	s.Require().NotNil(wall.To)
	s.InDelta(7.5, wall.From.X, 1e-12)
	s.InDelta(-3*feetPerSceneUnit, wall.From.Y, 1e-12)
	s.InDelta(7.5, wall.To.X, 1e-12)
	s.InDelta(3*feetPerSceneUnit, wall.To.Y, 1e-12)
	s.InDelta(2*feetPerSceneUnit, wall.Height, 1e-12)
	s.InDelta(0.25*feetPerSceneUnit, wall.Thickness, 1e-12)
	s.Zero(wall.Elevation)
	s.Empty(before.Atlas.StructuralWalls[0].Openings)
	s.Empty(before.Atlas.StructuralDoors)
	s.Require().Len(before.Atlas.Cells, 3, "the known side keeps its floor; the opposite room is still undiscovered")
	for i, q := range []int{2, 3, 4} {
		s.True(proto.Equal(before.Atlas.Cells[i], pbAt(q, 0)))
	}

	moved, err := h.handler.Move(ctx, &sessionpb.MoveRequest{Session: "structural-patch-run", Member: "alice", Path: []*sessionpb.Position{pbAt(3, 0)}})
	s.Require().NoError(err)
	s.Require().Len(moved.Steps, 1, "the proximity-triggering step must actually complete")
	story, err := h.handler.GetStory(ctx, &sessionpb.GetStoryRequest{Session: "structural-patch-run", Member: "alice"})
	s.Require().NoError(err)
	var reveal *sessionpb.ConcealmentRevealed
	for _, entry := range story.Entries {
		if body := entry.GetConcealmentRevealed(); body != nil {
			s.Nil(reveal, "exactly one concealment reveal")
			reveal = body
		}
	}
	s.Require().NotNil(reveal, "real automatic discovery must produce the existing reveal event")
	s.Empty(reveal.StructuralWalls, "a known wall is not reintroduced whole")
	s.Empty(reveal.Cells, "discovery adds no unselected floor")
	s.Require().Len(reveal.StructuralWallOpeningsReplacements, 1)
	s.Require().Len(reveal.StructuralDoors, 1)
	patch := reveal.StructuralWallOpeningsReplacements[0]
	s.Equal("wall", patch.WallId)
	s.Require().Len(patch.Openings, 1)
	s.Equal("gap", patch.Openings[0].Id)
	s.InDelta(3*feetPerSceneUnit, patch.Openings[0].Position, 1e-12)
	s.InDelta(2*feetPerSceneUnit, patch.Openings[0].Width, 1e-12)
	door := reveal.StructuralDoors[0]
	s.Equal("structural-patch-acceptance/gate", door.Id)
	s.Equal("dnd5e:env:dark-fortress:wall_door_double_01", door.Ref)
	s.Require().NotNil(door.From)
	s.Require().NotNil(door.To)
	s.InDelta(7.5, door.From.X, 1e-12)
	s.InDelta(-feetPerSceneUnit, door.From.Y, 1e-12)
	s.InDelta(7.5, door.To.X, 1e-12)
	s.InDelta(feetPerSceneUnit, door.To.Y, 1e-12)
	s.InDelta(2*feetPerSceneUnit, door.Height, 1e-12)
	s.InDelta(0.25*feetPerSceneUnit, door.Thickness, 1e-12)
	s.Zero(door.Elevation)
	after, err := h.handler.GetKnowledge(ctx, request)
	s.Require().NoError(err)
	patched := proto.Clone(before.Atlas.StructuralWalls[0]).(*sessionpb.AtlasStructuralWall)
	patched.Openings = patch.Openings
	s.True(proto.Equal(patched, after.Atlas.StructuralWalls[0]))
	s.True(proto.Equal(reveal.StructuralDoors[0], after.Atlas.StructuralDoors[0]))
	s.Require().Len(after.Atlas.Cells, len(before.Atlas.Cells))
	for i, cell := range before.Atlas.Cells {
		s.True(proto.Equal(cell, after.Atlas.Cells[i]), "finding the closed door reveals no new floor")
	}
	replay, err := h.handler.GetStory(ctx, &sessionpb.GetStoryRequest{Session: "structural-patch-run", Member: "alice"})
	s.Require().NoError(err)
	s.True(proto.Equal(story, replay), "repository replay must retain the original wire payload")

	s.Run("new playthrough resets discovery without resetting the first encounter", func() {
		// Same authored key and character, with no profile deletion/workaround.
		_, startErr := h.manager.Manager.StartSession(ctx, &sdk.StartSessionInput{
			Session: "structural-second-run", Encounter: "structural-second-world", World: dungeon.World,
		})
		s.Require().NoError(startErr)
		_, joinErr := h.handler.Join(ctx, &sessionpb.JoinRequest{Session: "structural-second-run", Member: "alice", Position: pbAt(4, 0)})
		s.Require().NoError(joinErr)
		freshRequest := &sessionpb.GetKnowledgeRequest{Session: "structural-second-run", Member: "alice"}
		fresh, readErr := h.handler.GetKnowledge(ctx, freshRequest)
		s.Require().NoError(readErr)
		s.Require().Len(fresh.Atlas.StructuralWalls, 1)
		s.Empty(fresh.Atlas.StructuralWalls[0].Openings, "new encounter starts undiscovered")
		s.Empty(fresh.Atlas.StructuralDoors)
		_, moveErr := h.handler.Move(ctx, &sessionpb.MoveRequest{Session: "structural-second-run", Member: "alice", Path: []*sessionpb.Position{pbAt(3, 0)}})
		s.Require().NoError(moveErr)
		freshAfter, afterErr := h.handler.GetKnowledge(ctx, freshRequest)
		s.Require().NoError(afterErr)
		s.Require().Len(freshAfter.Atlas.StructuralDoors, 1, "the new encounter has its own discovery attempt")
		firstStillKnown, firstErr := h.handler.GetKnowledge(ctx, request)
		s.Require().NoError(firstErr)
		s.True(proto.Equal(after.Atlas, firstStillKnown.Atlas), "starting a new run does not reset the old encounter")
	})
}
