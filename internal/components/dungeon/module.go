package dungeon

import (
	"errors"
	"fmt"
)

// ErrRoomNotFound is returned when LocalToAbsolute or AbsoluteToLocal is called
// for a roomID that the Module does not know about.
var ErrRoomNotFound = errors.New("room not found in module")

// Module carries dungeon state needed to translate between room-local and
// dungeon-absolute coordinates. It is the only legal bridge between LocalPosition
// and AbsolutePosition; ad-hoc element-wise math at call sites is not allowed.
//
// Following the rpg-toolkit pattern, Module loads from data and serializes back
// to data via LoadFromData / ToData. The data orchestrator owns persistence;
// Module owns the coordinate translation logic.
//
// See rpg-project/ideas/coordinate-types/design.md for context.
type Module struct {
	// roomOrigins is the per-room origin in dungeon-absolute coordinates,
	// keyed by roomID.
	roomOrigins map[string]AbsolutePosition
}

// Data is the serializable shape of a Module. The data orchestrator persists
// this and reconstructs the Module via LoadFromData on load. Keeping Data as
// a concrete struct (rather than interface{}) lets the type system carry
// meaning across the persistence boundary.
type Data struct {
	// RoomOrigins maps roomID to the room's absolute origin in dungeon-space.
	RoomOrigins map[string]AbsolutePosition `json:"room_origins"`
}

// LoadFromData reconstructs a Module from its serialized Data shape. Returns
// an error if the data is structurally invalid (e.g., a room origin that
// violates the cube invariant).
func LoadFromData(d *Data) (*Module, error) {
	if d == nil {
		return nil, errors.New("dungeon: LoadFromData input is nil")
	}

	origins := make(map[string]AbsolutePosition, len(d.RoomOrigins))
	for roomID, origin := range d.RoomOrigins {
		if !origin.IsValid() {
			return nil, fmt.Errorf(
				"dungeon: room %q has invalid cube origin %s (X+Y+Z must equal 0)",
				roomID, origin,
			)
		}
		origins[roomID] = origin
	}

	return &Module{roomOrigins: origins}, nil
}

// ToData returns the serializable Data shape of the Module. The returned Data
// is a defensive copy: mutations to the result do not affect the Module.
func (m *Module) ToData() *Data {
	if m == nil {
		return &Data{RoomOrigins: map[string]AbsolutePosition{}}
	}

	origins := make(map[string]AbsolutePosition, len(m.roomOrigins))
	for roomID, origin := range m.roomOrigins {
		origins[roomID] = origin
	}
	return &Data{RoomOrigins: origins}
}

// LocalToAbsolute converts a room-local cube position to dungeon-absolute by
// element-wise addition with the room's origin. This is the only legal way
// to cross the local→absolute boundary; the X+Y+Z=0 invariant is preserved
// because both inputs are valid cube coordinates and addition of two valid
// cube coordinates is itself a valid cube coordinate.
//
// Returns ErrRoomNotFound (wrapped) if roomID is not known to the Module.
func (m *Module) LocalToAbsolute(roomID string, p LocalPosition) (AbsolutePosition, error) {
	if m == nil {
		return AbsolutePosition{}, errors.New("dungeon: LocalToAbsolute called on nil Module")
	}

	origin, ok := m.roomOrigins[roomID]
	if !ok {
		return AbsolutePosition{}, fmt.Errorf("dungeon: %w: %q", ErrRoomNotFound, roomID)
	}

	return AbsolutePosition{
		X: p.X + origin.X,
		Y: p.Y + origin.Y,
		Z: p.Z + origin.Z,
	}, nil
}

// AbsoluteToLocal converts a dungeon-absolute cube position back into the
// room-local space of the named room. Returns ErrRoomNotFound (wrapped) if
// roomID is not known.
func (m *Module) AbsoluteToLocal(roomID string, p AbsolutePosition) (LocalPosition, error) {
	if m == nil {
		return LocalPosition{}, errors.New("dungeon: AbsoluteToLocal called on nil Module")
	}

	origin, ok := m.roomOrigins[roomID]
	if !ok {
		return LocalPosition{}, fmt.Errorf("dungeon: %w: %q", ErrRoomNotFound, roomID)
	}

	return LocalPosition{
		X: p.X - origin.X,
		Y: p.Y - origin.Y,
		Z: p.Z - origin.Z,
	}, nil
}

// RoomOrigin returns the absolute origin of the named room. Returns
// ErrRoomNotFound (wrapped) if roomID is not known.
func (m *Module) RoomOrigin(roomID string) (AbsolutePosition, error) {
	if m == nil {
		return AbsolutePosition{}, errors.New("dungeon: RoomOrigin called on nil Module")
	}

	origin, ok := m.roomOrigins[roomID]
	if !ok {
		return AbsolutePosition{}, fmt.Errorf("dungeon: %w: %q", ErrRoomNotFound, roomID)
	}
	return origin, nil
}
