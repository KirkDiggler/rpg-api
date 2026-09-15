package sessionworld

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// armed_minds_test.go proves the shipped walk scene: two goblins in one room,
// armed differently, from one stat block (rpg-project#448).
//
// It compiles the REAL content/reference-minds.yaml rather than a fixture,
// because the thing under test is the file — the acceptance case is "place two
// goblin archers without touching Go", and a fixture would not be the file the
// walk plays.
type ArmedMindsSuite struct {
	suite.Suite

	minds *Dungeon
}

func TestArmedMindsSuite(t *testing.T) { suite.Run(t, new(ArmedMindsSuite)) }

var referenceMindsPath = filepath.Join("..", "..", "content", "reference-minds.yaml")

func (s *ArmedMindsSuite) SetupTest() {
	raw, err := os.ReadFile(referenceMindsPath)
	s.Require().NoError(err, "the shipped minds dungeon must exist at content/reference-minds.yaml")
	minds, err := Compile(raw)
	s.Require().NoError(err, "the shipped minds dungeon must compile")
	s.minds = minds
}

// goblins is every goblin the file places, in authored order.
func (s *ArmedMindsSuite) goblins() []Monster {
	s.T().Helper()
	out := make([]Monster, 0, 2)
	for _, m := range s.minds.Monsters {
		if m.Ref == "dnd5e:monsters:goblin" {
			out = append(out, m)
		}
	}
	return out
}

// TestTwoGoblinsOneStatBlockTwoLoadouts is the acceptance case.
func (s *ArmedMindsSuite) TestTwoGoblinsOneStatBlockTwoLoadouts() {
	goblins := s.goblins()
	s.Require().Len(goblins, 2, "the entrance holds two of them for walk 5")

	s.Equal(goblins[0].Ref, goblins[1].Ref,
		"the same stat block twice — arming a placement is not a new monster")
	s.NotEqual(goblins[0].MemberID, goblins[1].MemberID,
		"and they are still two members")

	s.Equal([]string{"dnd5e:weapons:scimitar", "dnd5e:weapons:shortbow"}, goblins[0].Actions,
		"the blade is listed FIRST, which is what makes this one swing when cornered")
	s.Equal([]string{"dnd5e:weapons:shortbow"}, goblins[1].Actions,
		"and this one has nothing to swing")
}

// TestAnUnarmedPlacementForwardsNothing is the control: the same file's other
// monsters name no actions, so the field above is about `actions:` rather
// than about compiling a monster at all.
func (s *ArmedMindsSuite) TestAnUnarmedPlacementForwardsNothing() {
	var checked int
	for _, m := range s.minds.Monsters {
		if m.Ref == "dnd5e:monsters:goblin" {
			continue
		}
		s.Nil(m.Actions, "%s names no actions and keeps its stat block's own arms", m.Ref)
		checked++
	}
	require.Positive(s.T(), checked, "the file must still place monsters the author armed with nothing")
}
