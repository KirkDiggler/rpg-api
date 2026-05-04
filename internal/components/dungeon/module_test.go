package dungeon_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
)

type ModuleTestSuite struct {
	suite.Suite
}

func TestModuleTestSuite(t *testing.T) {
	suite.Run(t, new(ModuleTestSuite))
}

// twoRoomData returns canonical Module data with two rooms — origin and one
// shifted to (10, -17, 7) (a valid cube origin) — used across translation tests.
func twoRoomData() *dungeon.Data {
	return &dungeon.Data{
		RoomOrigins: map[string]dungeon.AbsolutePosition{
			"room-start": dungeon.NewAbsolutePosition(0, 0),
			"room-east":  dungeon.NewAbsolutePosition(10, 7),
		},
	}
}

func (s *ModuleTestSuite) TestLoadFromData_NilInput_ReturnsError() {
	m, err := dungeon.LoadFromData(nil)
	s.Require().Error(err)
	s.Nil(m)
}

func (s *ModuleTestSuite) TestLoadFromData_EmptyData_ReturnsModule() {
	m, err := dungeon.LoadFromData(&dungeon.Data{})
	s.Require().NoError(err)
	s.Require().NotNil(m)

	// Translating any room should now report not-found.
	_, err = m.LocalToAbsolute("any", dungeon.NewLocalPosition(0, 0))
	s.Require().Error(err)
	s.True(errors.Is(err, dungeon.ErrRoomNotFound))
}

func (s *ModuleTestSuite) TestLoadFromData_RejectsInvalidCubeOrigin() {
	bad := &dungeon.Data{
		RoomOrigins: map[string]dungeon.AbsolutePosition{
			// Skips the constructor; X+Y+Z=2 is not a valid cube coord.
			"broken": {X: 1, Y: 1, Z: 0},
		},
	}
	m, err := dungeon.LoadFromData(bad)
	s.Require().Error(err)
	s.Nil(m)
	s.Contains(err.Error(), "broken")
}

func (s *ModuleTestSuite) TestRoundTrip_LoadThenToData_PreservesOrigins() {
	original := twoRoomData()
	m, err := dungeon.LoadFromData(original)
	s.Require().NoError(err)

	round := m.ToData()
	s.Require().NotNil(round)
	s.Equal(original.RoomOrigins, round.RoomOrigins)
}

func (s *ModuleTestSuite) TestToData_ReturnsDefensiveCopy() {
	m, err := dungeon.LoadFromData(twoRoomData())
	s.Require().NoError(err)

	first := m.ToData()
	first.RoomOrigins["room-start"] = dungeon.NewAbsolutePosition(99, 99)

	second := m.ToData()
	s.Equal(
		dungeon.NewAbsolutePosition(0, 0),
		second.RoomOrigins["room-start"],
		"mutating returned Data must not affect Module state",
	)
}

func (s *ModuleTestSuite) TestLocalToAbsolute_StartRoom_IsIdentity() {
	m, err := dungeon.LoadFromData(twoRoomData())
	s.Require().NoError(err)

	abs, err := m.LocalToAbsolute("room-start", dungeon.NewLocalPosition(3, -2))
	s.Require().NoError(err)
	s.Equal(dungeon.NewAbsolutePosition(3, -2), abs)
}

func (s *ModuleTestSuite) TestLocalToAbsolute_ShiftedRoom_AppliesOriginOffset() {
	m, err := dungeon.LoadFromData(twoRoomData())
	s.Require().NoError(err)

	// room-east origin is (10, -17, 7); local (3, -1, -2) should land at (13, -18, 5).
	abs, err := m.LocalToAbsolute("room-east", dungeon.NewLocalPosition(3, -2))
	s.Require().NoError(err)
	s.Equal(dungeon.AbsolutePosition{X: 13, Y: -18, Z: 5}, abs)
	s.True(abs.IsValid(), "translation must preserve cube invariant")
}

func (s *ModuleTestSuite) TestLocalToAbsolute_UnknownRoom_ReturnsErrRoomNotFound() {
	m, err := dungeon.LoadFromData(twoRoomData())
	s.Require().NoError(err)

	_, err = m.LocalToAbsolute("ghost-room", dungeon.NewLocalPosition(0, 0))
	s.Require().Error(err)
	s.True(errors.Is(err, dungeon.ErrRoomNotFound))
	s.Contains(err.Error(), "ghost-room")
}

func (s *ModuleTestSuite) TestAbsoluteToLocal_RoundTripsThroughLocalToAbsolute() {
	m, err := dungeon.LoadFromData(twoRoomData())
	s.Require().NoError(err)

	original := dungeon.NewLocalPosition(4, -3)
	abs, err := m.LocalToAbsolute("room-east", original)
	s.Require().NoError(err)

	back, err := m.AbsoluteToLocal("room-east", abs)
	s.Require().NoError(err)
	s.Equal(original, back)
}

func (s *ModuleTestSuite) TestAbsoluteToLocal_UnknownRoom_ReturnsErrRoomNotFound() {
	m, err := dungeon.LoadFromData(twoRoomData())
	s.Require().NoError(err)

	_, err = m.AbsoluteToLocal("ghost-room", dungeon.NewAbsolutePosition(0, 0))
	s.Require().Error(err)
	s.True(errors.Is(err, dungeon.ErrRoomNotFound))
}

func (s *ModuleTestSuite) TestRoomOrigin_KnownRoom_ReturnsOrigin() {
	m, err := dungeon.LoadFromData(twoRoomData())
	s.Require().NoError(err)

	origin, err := m.RoomOrigin("room-east")
	s.Require().NoError(err)
	s.Equal(dungeon.NewAbsolutePosition(10, 7), origin)
}

func (s *ModuleTestSuite) TestRoomOrigin_UnknownRoom_ReturnsErrRoomNotFound() {
	m, err := dungeon.LoadFromData(twoRoomData())
	s.Require().NoError(err)

	_, err = m.RoomOrigin("ghost-room")
	s.Require().Error(err)
	s.True(errors.Is(err, dungeon.ErrRoomNotFound))
}

func (s *ModuleTestSuite) TestNilModule_AllMethodsReturnError() {
	var m *dungeon.Module

	_, err := m.LocalToAbsolute("any", dungeon.NewLocalPosition(0, 0))
	s.Require().Error(err)

	_, err = m.AbsoluteToLocal("any", dungeon.NewAbsolutePosition(0, 0))
	s.Require().Error(err)

	_, err = m.RoomOrigin("any")
	s.Require().Error(err)

	// ToData on nil receiver is benign — returns an empty Data.
	d := m.ToData()
	s.Require().NotNil(d)
	s.Empty(d.RoomOrigins)
}
