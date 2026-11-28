package session_test

import (
	"context"
	"os"
	"strings"

	materialpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/presentation/material/v1alpha1"
	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
)

func (s *GroundAcceptanceSuite) TestNativeMaterialChannelsReachTwoClientsAndColdHost() {
	const run = "material-channels-run"
	h := newAcceptanceHarness(s.T())
	for _, member := range []string{"alice", "bob"} {
		ctx := auth.WithPlayerID(context.Background(), "player-"+member)
		_, err := h.charRepo.Create(ctx, characterrepo.CreateInput{Character: &entities.Character{Data: armedFighter(member, "player-"+member)}})
		s.Require().NoError(err)
	}
	raw, err := os.ReadFile("testdata/material-channels.yaml")
	s.Require().NoError(err)
	compiled, err := sessionworld.Compile(raw)
	s.Require().NoError(err)
	h.launchAtStart(s.T(), run, compiled.Spec, "alice", "bob")
	snapshots := map[string]*sessionpb.GetKnowledgeResponse{}
	rosters := map[string][]*materialpb.MaterialChannelAdjustment{}
	for _, member := range []string{"alice", "bob"} {
		ctx := auth.WithPlayerID(context.Background(), "player-"+member)
		snapshot, readErr := h.handler.GetKnowledge(ctx, &sessionpb.GetKnowledgeRequest{Session: run, Member: member})
		s.Require().NoError(readErr)
		s.Require().Len(snapshot.Atlas.StructuralWalls, 1)
		wall := snapshot.Atlas.StructuralWalls[0]
		s.Require().Len(wall.MaterialChannels, 1)
		s.Require().IsType(&materialpb.ChannelAdjustment_Multiplier{}, wall.MaterialChannels[0].Normal.Value)
		s.Equal(.5, wall.MaterialChannels[0].Normal.GetMultiplier())
		s.Nil(wall.MaterialChannels[0].Emissive)
		s.Require().Len(snapshot.Atlas.StructuralDoors, 1)
		frame := snapshot.Atlas.StructuralDoors[0].FramePresentation
		s.Require().NotNil(frame)
		s.Equal(wall.MaterialChannels, frame.MaterialChannels)
		found := false
		for _, prop := range snapshot.Atlas.PropPresentations {
			if prop.Id != "channel-lantern" {
				continue
			}
			found = true
			s.Require().Len(prop.MaterialChannels, 1)
			s.NotNil(prop.MaterialChannels[0].Emissive.GetOff())
			s.Nil(prop.MaterialChannels[0].Normal)
		}
		s.True(found, "the actual native prop reaches captured presentation")
		roster, rosterErr := h.handler.GetRoster(ctx, &sessionpb.GetRosterRequest{Session: run, Member: member})
		s.Require().NoError(rosterErr)
		for _, npc := range roster.Members {
			if npc.MonsterRef != "dnd5e:monsters:skeleton" {
				continue
			}
			s.Require().Len(npc.MaterialChannels, 1)
			s.Require().IsType(&materialpb.ChannelAdjustment_Multiplier{}, npc.MaterialChannels[0].Normal.Value)
			s.Equal(float64(1), npc.MaterialChannels[0].Normal.GetMultiplier())
			rosters[member] = npc.MaterialChannels
		}
		s.Require().Contains(rosters, member)
		snapshots[member] = snapshot
	}
	later, err := sessionworld.Compile([]byte(strings.ReplaceAll(string(raw), "multiplier: 0.5", "multiplier: 2")))
	s.Require().NoError(err)
	s.Equal(float64(2), later.Spec.Field.StructuralWalls[0].MaterialChannels[0].Normal.Multiplier)
	compiled.Spec.Field.StructuralWalls[0].MaterialChannels[0].Normal.Multiplier = 2
	compiled.Spec.Monsters[0].MaterialChannels[0].Normal.Multiplier = 2
	coldSites, err := sessionorch.NewSites(nil)
	s.Require().NoError(err)
	fresh := freshSessionHandler(s.T(), h, coldSites)
	for _, member := range []string{"alice", "bob"} {
		ctx := auth.WithPlayerID(context.Background(), "player-"+member)
		after, err := fresh.GetKnowledge(ctx, &sessionpb.GetKnowledgeRequest{Session: run, Member: member})
		s.Require().NoError(err)
		s.Equal(snapshots[member].Atlas.StructuralWalls, after.Atlas.StructuralWalls)
		s.Equal(snapshots[member].Atlas.StructuralDoors, after.Atlas.StructuralDoors)
		s.Equal(snapshots[member].Atlas.PropPresentations, after.Atlas.PropPresentations)
		s.Equal(snapshots[member].Atlas.Cells, after.Atlas.Cells)
		roster, err := fresh.GetRoster(ctx, &sessionpb.GetRosterRequest{Session: run, Member: member})
		s.Require().NoError(err)
		foundNPC := false
		for _, npc := range roster.Members {
			if npc.MonsterRef == "dnd5e:monsters:skeleton" {
				foundNPC = true
				s.Equal(rosters[member], npc.MaterialChannels)
			}
		}
		s.True(foundNPC, "the captured NPC survives the cold-host roster read")
	}
	s.Equal(1, coldSites.Compiles(), "reload immutable persisted site data, not later source edits")
}

func (s *GroundAcceptanceSuite) TestMaterialChannelFrameCanBeDisclosedWithoutItsWall() {
	h := newAcceptanceHarness(s.T())
	ctx := auth.WithPlayerID(context.Background(), "player-alice")
	_, err := h.charRepo.Create(ctx, characterrepo.CreateInput{Character: &entities.Character{Data: armedFighter("alice", "player-alice")}})
	s.Require().NoError(err)
	raw, err := os.ReadFile("testdata/material-channels.yaml")
	s.Require().NoError(err)
	compiled, err := sessionworld.Compile(raw)
	s.Require().NoError(err)
	compiled.Spec.Field.Concealments = []encounter.ConcealmentInput{{ID: "hidden-wall", Checks: []encounter.CheckApproach{{Ability: "perception", DC: 15}}, Props: []encounter.PropID{compiled.Spec.Field.StructuralWalls[0].ID}}}
	h.launchAtStart(s.T(), "channels-hidden-wall", compiled.Spec, "alice")
	snapshot, err := h.handler.GetKnowledge(ctx, &sessionpb.GetKnowledgeRequest{Session: "channels-hidden-wall", Member: "alice"})
	s.Require().NoError(err)
	s.Empty(snapshot.Atlas.StructuralWalls)
	s.Require().Len(snapshot.Atlas.StructuralDoors, 1)
	frame := snapshot.Atlas.StructuralDoors[0].FramePresentation
	s.Require().NotNil(frame)
	s.Require().Len(frame.MaterialChannels, 1)
	s.Equal(.5, frame.MaterialChannels[0].Normal.GetMultiplier())
}
