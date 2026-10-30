package sessionworld

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"
)

// NpcPacingSiteSuite pins the authored setup; actual visual ordering is walked
// through the real browser/act stream, not inferred from successful decoding.
type NpcPacingSiteSuite struct {
	suite.Suite
}

func TestNpcPacingSiteSuite(t *testing.T) { suite.Run(t, new(NpcPacingSiteSuite)) }

func (s *NpcPacingSiteSuite) TestSiteHasTwoVisibleWalkersAndAResultIndependentStartSignal() {
	raw, err := os.ReadFile(filepath.Join("..", "..", "content", "reference-npc-pacing.yaml"))
	s.Require().NoError(err)
	dungeon, err := Compile(raw)
	s.Require().NoError(err)
	s.Equal("reference-npc-pacing", dungeon.Key)
	s.Equal("NPC Pacing Yard", dungeon.Name)
	s.Require().NotEmpty(dungeon.PartySeats)
	s.Equal(0.0, dungeon.PartySeats[0].X)
	s.Equal(0.0, dungeon.PartySeats[0].Y)
	s.Require().Len(dungeon.Spec.Monsters, 2)
	leader := dungeon.Spec.Monsters[0]
	other := dungeon.Spec.Monsters[1]
	s.Equal("walker-a", leader.ID)
	s.Equal("walker-b", other.ID)
	leaderCell := cellOf(dungeon.Spec.Field.Canvas.Orientation, leader.At)
	otherCell := cellOf(dungeon.Spec.Field.Canvas.Orientation, other.At)
	s.Equal(5.0, leaderCell.X)
	s.Equal(0.0, leaderCell.Y)
	s.Equal(0.0, otherCell.X)
	s.Equal(5.0, otherCell.Y)
	s.Equal("dnd5e:npcs:goblin:knight-01", leader.AppearanceRef)
	s.Equal("dnd5e:npcs:goblin:prisoner-02", other.AppearanceRef)
	s.Empty(leader.DefaultActivityRef, "the knight uses the provider default")
	s.Equal("dnd5e:activities:thoughtful", other.DefaultActivityRef)
	s.Equal("drill", leader.Faction)
	s.Equal("drill", other.Faction)
	s.Require().Len(leader.Options, 1)
	start := leader.Options[0]
	s.Equal("begin-drill", start.ID)
	s.Require().Len(start.Success, 1)
	s.Require().Len(start.Failure, 1)
	s.NotEmpty(start.Success[0].Fact)
	s.Equal(start.Success[0].Fact, start.Failure[0].Fact, "neither die result may prevent the verification sequence")
	s.Contains(string(raw), "until: {fact: drill-started}")
	s.Contains(string(raw), "actions: [dnd5e:weapons:unarmed-strike]")
}
