package dungeon_test

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-api/internal/components/dungeon"
)

type CoordsTestSuite struct {
	suite.Suite
}

func TestCoordsTestSuite(t *testing.T) {
	suite.Run(t, new(CoordsTestSuite))
}

func (s *CoordsTestSuite) TestNewLocalPosition_DerivesYToSatisfyCubeInvariant() {
	p := dungeon.NewLocalPosition(3, -1)

	s.Equal(3, p.X)
	s.Equal(-2, p.Y, "Y must equal -X-Z to satisfy cube invariant")
	s.Equal(-1, p.Z)
	s.True(p.IsValid())
}

func (s *CoordsTestSuite) TestNewLocalPosition_OriginIsValid() {
	p := dungeon.NewLocalPosition(0, 0)
	s.Equal(dungeon.LocalPosition{X: 0, Y: 0, Z: 0}, p)
	s.True(p.IsValid())
}

func (s *CoordsTestSuite) TestNewLocalPosition_NegativeCoords() {
	p := dungeon.NewLocalPosition(-5, -5)
	s.Equal(-5, p.X)
	s.Equal(10, p.Y)
	s.Equal(-5, p.Z)
	s.True(p.IsValid())
}

func (s *CoordsTestSuite) TestNewAbsolutePosition_DerivesYToSatisfyCubeInvariant() {
	p := dungeon.NewAbsolutePosition(7, 4)

	s.Equal(7, p.X)
	s.Equal(-11, p.Y)
	s.Equal(4, p.Z)
	s.True(p.IsValid())
}

func (s *CoordsTestSuite) TestNewAbsolutePosition_OriginIsValid() {
	p := dungeon.NewAbsolutePosition(0, 0)
	s.True(p.IsValid())
}

func (s *CoordsTestSuite) TestLocalPosition_IsValid_RejectsBrokenInvariant() {
	// Hand-rolled position that violates X+Y+Z=0 — caller skipped the constructor.
	bad := dungeon.LocalPosition{X: 1, Y: 1, Z: 1}
	s.False(bad.IsValid(), "X+Y+Z=3 should not be a valid cube coordinate")
}

func (s *CoordsTestSuite) TestAbsolutePosition_IsValid_RejectsBrokenInvariant() {
	bad := dungeon.AbsolutePosition{X: 5, Y: 0, Z: 0}
	s.False(bad.IsValid(), "X+Y+Z=5 should not be a valid cube coordinate")
}

func (s *CoordsTestSuite) TestLocalPosition_String_HasLocalPrefix() {
	p := dungeon.NewLocalPosition(2, 3)
	// Format: local(X, Y, Z) — distinguishes from absolute in logs/errors.
	s.Equal("local(2, -5, 3)", p.String())
}

func (s *CoordsTestSuite) TestAbsolutePosition_String_HasAbsPrefix() {
	p := dungeon.NewAbsolutePosition(2, 3)
	s.Equal("abs(2, -5, 3)", p.String())
}

func (s *CoordsTestSuite) TestLocalAndAbsolute_AreDistinctTypes() {
	// This test exists to document the central invariant: LocalPosition and
	// AbsolutePosition cannot be substituted for each other. The compile-time
	// check (dungeon.LocalPosition assigned to AbsolutePosition variable) is
	// the actual enforcement; this test just makes the intent explicit.
	local := dungeon.NewLocalPosition(1, 2)
	abs := dungeon.NewAbsolutePosition(1, 2)

	// Same numeric values, different types.
	s.Equal(local.X, abs.X)
	s.Equal(local.Y, abs.Y)
	s.Equal(local.Z, abs.Z)
}
