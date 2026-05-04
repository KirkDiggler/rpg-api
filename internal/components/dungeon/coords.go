// Package dungeon provides procedural dungeon generation with themed rooms and encounters.
//
// This file defines the canonical coordinate types for the dungeon component.
// LocalPosition and AbsolutePosition are intentionally distinct types so the compiler
// catches accidental substitution between room-relative and dungeon-relative
// coordinates — see rpg-project/ideas/coordinate-types/design.md and rpg-api issue #471
// for context. Module is the only public bridge between the two spaces; ad-hoc
// element-wise math at call sites is not allowed.
//
// Both types use cube coordinates with the invariant X + Y + Z = 0. The constructors
// in this file enforce that invariant by deriving Y from X and Z. This is the same
// shape as rpg-toolkit's spatial.CubeCoordinate, but we keep these types independent
// here: aliasing or embedding would not strengthen the local-vs-absolute distinction
// (Add/Distance methods would still operate on the embedded primitive), and keeping
// them local preserves graduation flexibility when the dungeon component eventually
// moves to rpg-toolkit (see issue #479).
package dungeon

import "fmt"

// LocalPosition is a room-relative cube coordinate. It has no meaning without an
// associated room — the X+Y+Z=0 invariant is enforced by the constructor.
//
// LocalPosition values must not be substituted for AbsolutePosition values without
// going through Module.LocalToAbsolute; the type system enforces this.
type LocalPosition struct {
	X int
	Y int
	Z int
}

// AbsolutePosition is a dungeon-relative cube coordinate. The X+Y+Z=0 invariant
// is enforced by the constructor and preserved by Module.LocalToAbsolute, since
// element-wise addition of two valid cube coordinates is itself a valid cube
// coordinate.
type AbsolutePosition struct {
	X int
	Y int
	Z int
}

// NewLocalPosition constructs a LocalPosition from X and Z, deriving Y so that
// X+Y+Z=0. Use this whenever building a room-local position to make the cube
// invariant explicit and avoid the 2D-vestige bug class.
func NewLocalPosition(x, z int) LocalPosition {
	return LocalPosition{X: x, Y: -x - z, Z: z}
}

// NewAbsolutePosition constructs an AbsolutePosition from X and Z, deriving Y so
// that X+Y+Z=0.
func NewAbsolutePosition(x, z int) AbsolutePosition {
	return AbsolutePosition{X: x, Y: -x - z, Z: z}
}

// IsValid reports whether the local position satisfies X+Y+Z=0. Constructors
// produce valid positions; this is for defensive checks on data crossing
// boundaries (e.g., loaded from persistence, decoded from proto).
func (p LocalPosition) IsValid() bool {
	return p.X+p.Y+p.Z == 0
}

// String returns a human-readable form of the local position, useful in logs
// and error messages.
func (p LocalPosition) String() string {
	return fmt.Sprintf("local(%d, %d, %d)", p.X, p.Y, p.Z)
}

// IsValid reports whether the absolute position satisfies X+Y+Z=0.
func (p AbsolutePosition) IsValid() bool {
	return p.X+p.Y+p.Z == 0
}

// String returns a human-readable form of the absolute position.
func (p AbsolutePosition) String() string {
	return fmt.Sprintf("abs(%d, %d, %d)", p.X, p.Y, p.Z)
}
