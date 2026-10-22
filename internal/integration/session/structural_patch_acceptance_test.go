package session_test

import (
	"context"
	_ "embed"
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	storypb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	sessionhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
)

// The wall crosses between axial cells (1,0) and (2,0). Only the door is
// concealed; none of the five floor cells is a secret. Ordinary room discovery
// still withholds the opposite side of the closed geometric divider.
//
//go:embed testdata/structural_patch.yaml
var structuralPatchSource string

type StructuralPatchAcceptanceSuite struct{ suite.Suite }

// structuralAsAct carries a v1 snapshot message into its act-stream
// counterpart by field name, so a reveal on the act stream can be compared
// with the v1 GetKnowledge snapshot it must agree with. The act stream's
// messages are a superset by name; a v1 field the counterpart lacks fails.
func structuralAsAct[T proto.Message](t *testing.T, v1 proto.Message, into T) T {
	t.Helper()
	raw, err := protojson.Marshal(v1)
	require.NoError(t, err)
	require.NoError(t, protojson.Unmarshal(raw, into))
	return into
}

func TestStructuralPatchAcceptanceSuite(t *testing.T) {
	suite.Run(t, new(StructuralPatchAcceptanceSuite))
}

func (s *StructuralPatchAcceptanceSuite) TestCompiledDoorDiscoveryReplacesOpeningsOnTheWire() {
	h := newAcceptanceHarness(s.T())
	automatic, err := sessionorch.New(sessionorch.Config{
		Redis: h.redis, Characters: h.charRepo, TTL: time.Hour, Dice: testDice{},
		PresentationIDs: idgen.NewSequential("patch-presentation"),
	})
	s.Require().NoError(err)
	h.manager = automatic
	h.handler, err = sessionhandler.New(&sessionhandler.HandlerConfig{
		Manager: automatic.Manager, Characters: h.charRepo,
	})
	s.Require().NoError(err)
	h.story = newStoryHandler(s.T(), automatic, h.charRepo)
	ctx := auth.WithPlayerID(context.Background(), "player-alice")
	_, err = h.charRepo.Create(ctx, characterrepo.CreateInput{
		Character: &entities.Character{Data: armedFighter("alice", "player-alice")},
	})
	s.Require().NoError(err)
	dungeon, err := sessionworld.Compile([]byte(structuralPatchSource))
	s.Require().NoError(err)
	h.launch(s.T(), "structural-patch-run", dungeon.Spec, seatAt("alice", 4, 0))
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
	story := storyOf(ctx, s.T(), h.story, "structural-patch-run", "alice")
	var reveal *storypb.SecretRevealed
	for _, act := range story {
		if body := act.GetPerception().GetRevealed(); body != nil {
			s.Nil(reveal, "exactly one concealment reveal")
			s.Equal(storypb.Sense_SENSE_REVEAL, act.GetPerception().GetSense())
			s.Equal("alice", act.GetPerception().GetObserver().GetId())
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
	// The snapshot is v1's; the patch is the act stream's. Both are carried
	// into the act stream's geometry to compare them field for field.
	patched := structuralAsAct(s.T(), before.Atlas.StructuralWalls[0], &storypb.AtlasStructuralWall{})
	patched.Openings = patch.Openings
	s.True(proto.Equal(patched, structuralAsAct(s.T(), after.Atlas.StructuralWalls[0], &storypb.AtlasStructuralWall{})))
	s.True(proto.Equal(reveal.StructuralDoors[0], structuralAsAct(s.T(), after.Atlas.StructuralDoors[0], &storypb.AtlasStructuralDoor{})))
	s.Require().Len(after.Atlas.Cells, len(before.Atlas.Cells))
	for i, cell := range before.Atlas.Cells {
		s.True(proto.Equal(cell, after.Atlas.Cells[i]), "finding the closed door reveals no new floor")
	}
	replay := storyOf(ctx, s.T(), h.story, "structural-patch-run", "alice")
	s.Require().Len(replay, len(story))
	for i := range story {
		s.True(proto.Equal(story[i], replay[i]), "repository replay must retain the original wire payload")
	}

	s.Run("new playthrough resets discovery without resetting the first encounter", func() {
		// Same authored key and character, with no profile deletion/workaround.
		// A character holds one seat, so the first run is left before the next;
		// the old run is no longer readable by that member, so only the new
		// run's own discovery is asserted.
		_, exitErr := h.handler.Exit(ctx, &sessionpb.ExitRequest{Session: "structural-patch-run", Member: "alice"})
		s.Require().NoError(exitErr)
		h.launch(s.T(), "structural-second-run", dungeon.Spec, seatAt("alice", 4, 0))
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
	})
}
