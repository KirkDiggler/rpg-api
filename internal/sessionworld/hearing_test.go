package sessionworld

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/suite"

	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

// hearing_test.go pins the hearing room (rpg-project#569, wave B): the
// shipped file the B-walk is played on. Each assertion is one thing the walk
// needs to be true before anybody opens the door.

// referenceHearingPath is the hearing room's shipped file.
var referenceHearingPath = filepath.Join("..", "..", "content", "reference-hearing.yaml")

type HearingSuite struct {
	suite.Suite

	hearing *Dungeon
}

func TestHearingSuite(t *testing.T) { suite.Run(t, new(HearingSuite)) }

func (s *HearingSuite) SetupTest() {
	raw, err := os.ReadFile(referenceHearingPath)
	s.Require().NoError(err, "the hearing room must exist at content/reference-hearing.yaml")
	compiled, err := Compile(raw)
	s.Require().NoError(err, "the hearing room must compile")
	s.hearing = compiled
}

func (s *HearingSuite) placement(id string) Monster {
	s.T().Helper()
	for _, m := range monstersOf(s.hearing) {
		if m.PlacementID == id {
			return m
		}
	}
	s.Require().Failf("no such placement", "the hearing room places nothing with id %q", id)
	return Monster{}
}

// TestTheDoorBetweenStartsClosed: the closed door is the experiment. Shut,
// it is one more hop on the sound's way to the guard; the far door is open.
func (s *HearingSuite) TestTheDoorBetweenStartsClosed() {
	states := map[tkencounter.DoorID]tkencounter.DoorStateKind{}
	for _, d := range s.hearing.Spec.Field.Doors {
		states[d.ID] = d.State.Kind()
	}
	// A door's compiled id is scoped by the dungeon's key.
	s.Equal(tkencounter.DoorClosed, states["reference-hearing/guardroom-door"], "the guardroom door starts shut")
	s.Equal(tkencounter.DoorOpen, states["reference-hearing/back-room-door"], "the back room door stands open")
}

// TestTheGuardWalksASquareAndListensForAClang is the guard's table, row by
// row: the square, the clang, the shout and the strike.
func (s *HearingSuite) TestTheGuardWalksASquareAndListensForAClang() {
	guard := s.placement("guard")
	s.Equal("watch", guard.Faction)
	rows := guard.Table[tkencounter.AnswerTime]
	s.Require().Len(rows, 4, "the use case's gate-patrol, row for row")

	var patrol, listen, shout, strike *tkencounter.Answer
	for i := range rows {
		row := &rows[i]
		s.Require().Lenf(row.When, 1, "row %d names one condition", i)
		switch {
		case row.When[0].Heard != nil:
			listen = row
		case row.When[0].Enemy == tkencounter.EnemyNone:
			patrol = row
		case row.When[0].Enemy == tkencounter.EnemySeen:
			shout = row
		case row.When[0].Enemy == tkencounter.EnemyReach:
			strike = row
		}
	}

	s.Require().NotNil(patrol, "nobody in sight: walk the square")
	s.Require().NotNil(patrol.Toward)
	orientation := s.hearing.Spec.Field.Canvas.Orientation
	corners := make([]spatial.Position, 0, 4)
	for _, c := range [][2]float64{{2, 1}, {4, 1}, {4, 6}, {2, 6}} {
		corners = append(corners, cellOf(orientation, spatial.Position{X: c[0], Y: c[1]}))
	}
	s.Equal(corners, patrol.Toward.Path, "the four authored corners, in order, in the field's axial frame")

	s.Require().NotNil(listen, "a clang heard: walk toward it")
	s.Equal(&tkencounter.HeardWhen{Kind: tkencounter.SoundClang, Within: 2}, listen.When[0].Heard)
	s.Require().NotNil(listen.Toward)
	s.Equal(tkencounter.SelectorSound, listen.Toward.Word, "toward where the sound was made, not toward a member")

	s.Require().NotNil(shout, "an enemy in sight: shout")
	s.Equal("To arms!", shout.Say)
	s.Equal(tkencounter.FactID("alarm-raised"), shout.Fact, "the shout teaches the alarm")

	s.Require().NotNil(strike, "an enemy in reach: attack it")
	s.NotNil(strike.Attack)
}

// TestTheReserveWaitsForTheAlarmAndWalksToTheGuardroom: off the map until
// `alarm-raised` is known, then a standing order toward the guardroom.
func (s *HearingSuite) TestTheReserveWaitsForTheAlarmAndWalksToTheGuardroom() {
	reserve := s.placement("reserve")
	s.Equal("thugs", reserve.Faction)
	s.Equal(tkencounter.TriggerFact{Fact: "alarm-raised"}, reserve.Arrives)

	var walk *tkencounter.Answer
	rows := reserve.Table[tkencounter.AnswerTime]
	for i := range rows {
		for _, when := range rows[i].When {
			if when.Fact == "alarm-raised" {
				walk = &rows[i]
			}
		}
	}
	s.Require().NotNil(walk, "a row reads the alarm")
	s.Require().NotNil(walk.Toward)
	s.Require().NotNil(walk.Toward.At)
	s.Equal(cellOf(s.hearing.Spec.Field.Canvas.Orientation, spatial.Position{X: 3, Y: 3}), *walk.Toward.At,
		"the reserve walks onto the offset [3, 3] the file names, compiled to the field's axial frame")
	s.Equal("guardroom", s.regionOf(*walk.Toward.At), "the cell it walks to is in the guardroom")

	s.Nil(s.placement("thug").Arrives, "the thug behind the door stands there from the first frame")
}

// TestTheWatchTurnsOnTheAlarm: civil to the party until the alarm, and
// hostile to the thugs from frame one — the line that gives the guard's
// `enemy: seen` row somebody to see.
func (s *HearingSuite) TestTheWatchTurnsOnTheAlarm() {
	type pair = [2]tkencounter.FactionID
	stances := map[pair]tkencounter.DispositionInput{}
	for _, d := range s.hearing.Spec.Field.Dispositions {
		stances[d.Between] = d
	}

	watchParty, ok := stances[pair{"watch", "party"}]
	s.Require().True(ok, "the watch has a stance toward the party")
	s.Equal(tkencounter.StanceNeutral, watchParty.Stance)
	s.Equal(tkencounter.TriggerFact{Fact: "alarm-raised"}, watchParty.Until)

	s.Equal(tkencounter.StanceHostile, stances[pair{"thugs", "party"}].Stance)
	s.Equal(tkencounter.StanceHostile, stances[pair{"watch", "thugs"}].Stance)
}

// regionOf names the region that owns a cell of the compiled field. Both
// sides are compiled cells, so they compare in the field's own frame.
func (s *HearingSuite) regionOf(cell spatial.Position) string {
	s.T().Helper()
	for _, r := range s.hearing.Spec.Field.Regions {
		for _, c := range r.Cells {
			if c == cell {
				return r.ID
			}
		}
	}
	return ""
}
