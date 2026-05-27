package encounter

import (
	"fmt"

	"github.com/KirkDiggler/rpg-toolkit/core"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	encountercore "github.com/KirkDiggler/rpg-toolkit/encounter/core"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

// encounterHexRoom implements spatial.Room backed by encounter Data hex positions.
//
// The encounter SDK uses core.Hex (Q, R, S) for entity positions. The dnd5e
// rulebook's gamectx.RequireRoom expects a spatial.Room for spatial queries
// (e.g. SneakAttack ally-adjacency check). This adapter bridges the two
// coordinate systems: Hex.Q maps to Position.X, Hex.R maps to Position.Y.
//
// GetEntityPosition, GetEntitiesInRange, GetAllEntities, GetEntitiesAt, and
// IsPositionOccupied are read-side queries used by condition handlers
// (SneakAttack, Protection, OpportunityAttack) for spatial gates.
//
// MoveEntity mutates the underlying encounter Data positions. Required by
// combat.MoveEntity (Wave 2.11e #539 — wired into the new Dnd5eMovementResolver
// path). Per-step mutation during MovementResolver iteration: the resolver
// calls combat.MoveEntity per step, which calls room.MoveEntity to advance
// the spatial state so the next step sees the updated position. The
// encounter SDK's iteration helper ALSO commits the final position via
// applyAndPublishMove/applyNPCMovementSteps after the loop, but that final
// commit matches the last room.MoveEntity update — no double-mutation harm.
//
// PlaceEntity and RemoveEntity remain unsupported — encounter membership
// must flow through tkenc.AddPlayer / AddMonster / etc.
//
// The adapter is constructed fresh per RPC from the encounter's *tkenc.Data
// pointer. It shares the Data state so mutations propagate back to the
// encounter snapshot saved at end-of-RPC.
type encounterHexRoom struct {
	data *tkenc.Data
}

// entityTypeCharacter is the core.EntityType used for player-controlled
// characters in the encounter room adapter. Sneak Attack checks for entities
// of type "character" to determine ally adjacency.
const entityTypeCharacter core.EntityType = "character"

// entityTypeMonster is the core.EntityType used for monster entities in the
// encounter room adapter.
const entityTypeMonster core.EntityType = "monster"

// hexEntity is a lightweight core.Entity implementation backed by an encounter
// entity ID and a resolved type. Used by encounterHexRoom.GetEntitiesInRange.
type hexEntity struct {
	id         string
	entityType core.EntityType
}

// GetID returns the entity's identifier.
func (h *hexEntity) GetID() string { return h.id }

// GetType returns the entity's type ("character" for players, "monster" for monsters).
func (h *hexEntity) GetType() core.EntityType { return h.entityType }

// newEncounterHexRoom constructs a spatial.Room view over the encounter's
// current entity positions. Called once per RPC to supply the gamectx Room
// for condition handlers (read side) and the MovementResolver's
// combat.MoveEntity calls (write side).
func newEncounterHexRoom(data *tkenc.Data) spatial.Room {
	return &encounterHexRoom{data: data}
}

// hexToPos converts an encounter core.Hex to a spatial.Position.
// Q→X, R→Y following the encounter hex coordinate convention.
func hexToPos(h encountercore.Hex) spatial.Position {
	return spatial.Position{X: float64(h.Q), Y: float64(h.R)}
}

// posToHex converts a spatial.Position back to an encounter core.Hex.
// X→Q, Y→R; the S coordinate is derived as -(Q+R) to keep the cube
// invariant Q+R+S=0 (matches how encounter.AddPlayer / AddMonster set
// the initial S from their input).
func posToHex(p spatial.Position) encountercore.Hex {
	q := int(p.X)
	r := int(p.Y)
	return encountercore.Hex{Q: q, R: r, S: -(q + r)}
}

// GetEntityPosition returns the spatial.Position of the given entity.
// Searches players (by EntityID) and monsters (by ID). Returns false if not found.
func (r *encounterHexRoom) GetEntityPosition(entityID string) (spatial.Position, bool) {
	if r.data == nil {
		return spatial.Position{}, false
	}
	// Search players by EntityID.
	for _, pd := range r.data.Players {
		if string(pd.EntityID) == entityID {
			if pd.View != nil {
				return hexToPos(pd.View.Position), true
			}
		}
	}
	// Search monsters by ID.
	if md, ok := r.data.Monsters[encountercore.EntityID(entityID)]; ok {
		return hexToPos(md.Position), true
	}
	return spatial.Position{}, false
}

// GetEntitiesInRange returns all encounter entities whose hex positions fall
// within the given Euclidean radius from center. Both players and monsters
// are included. The radius is compared using the axial hex cube-distance
// formula (|ΔQ|+|ΔR|+|ΔS|)/2 via AxialHexGrid, so all 6 hex neighbors
// are correctly treated as distance 1. The previous Euclidean check
// rejected 2 of the 6 neighbors (those with XY-Euclidean distance √2),
// suppressing Opportunity Attacks for entities at those positions (#549).
//
// The caller (SneakAttack) uses this to find allies adjacent to a target.
func (r *encounterHexRoom) GetEntitiesInRange(center spatial.Position, radius float64) []core.Entity {
	if r.data == nil {
		return nil
	}
	grid := r.GetGrid()
	var result []core.Entity
	for _, pd := range r.data.Players {
		if pd.View == nil {
			continue
		}
		pos := hexToPos(pd.View.Position)
		if grid.Distance(pos, center) <= radius {
			result = append(result, &hexEntity{id: string(pd.EntityID), entityType: entityTypeCharacter})
		}
	}
	for _, md := range r.data.Monsters {
		pos := hexToPos(md.Position)
		if grid.Distance(pos, center) <= radius {
			result = append(result, &hexEntity{id: string(md.ID), entityType: entityTypeMonster})
		}
	}
	return result
}

// GetAllEntities returns all entities in the room as a map of ID to Entity.
func (r *encounterHexRoom) GetAllEntities() map[string]core.Entity {
	if r.data == nil {
		return map[string]core.Entity{}
	}
	result := make(map[string]core.Entity, len(r.data.Players)+len(r.data.Monsters))
	for _, pd := range r.data.Players {
		result[string(pd.EntityID)] = &hexEntity{id: string(pd.EntityID), entityType: entityTypeCharacter}
	}
	for _, md := range r.data.Monsters {
		result[string(md.ID)] = &hexEntity{id: string(md.ID), entityType: entityTypeMonster}
	}
	return result
}

// GetID returns a stable identifier for this room view. Not serialized or
// stored; used only for interface compliance.
func (r *encounterHexRoom) GetID() string { return "encounter-hex-room" }

// GetType returns the entity type for the room itself. Not meaningful for
// read-only adapters; present for interface compliance.
func (r *encounterHexRoom) GetType() core.EntityType { return "encounter-hex-room" }

// GetGrid returns an AxialHexGrid for distance/adjacency calculations.
//
// Positions in this adapter are axial cube coordinates (Q→X, R→Y from
// encountercore.Hex). AxialHexGrid uses the cube-distance formula
// (|ΔQ|+|ΔR|+|ΔS|)/2 so all six adjacent hexes are distance 1, including
// the 2 neighbors whose XY-Euclidean distance is √2 — the positions the
// old SquareGrid/Euclidean path incorrectly excluded (bug #549).
//
// Dimensions are large enough that any encounter position falls within
// bounds; distance calculations are coordinate-driven, not bounded by grid
// size.
func (r *encounterHexRoom) GetGrid() spatial.Grid {
	return spatial.NewAxialHexGrid(spatial.AxialHexGridConfig{
		SpanWidth:  1000,
		SpanHeight: 1000,
	})
}

// PlaceEntity is unsupported on the read-only adapter. Mutation must go
// through the encounter SDK.
func (r *encounterHexRoom) PlaceEntity(_ core.Entity, _ spatial.Position) error {
	return fmt.Errorf("PlaceEntity unsupported on read-only encounter spatial adapter")
}

// MoveEntity mutates the underlying encounter Data position for the
// given entity. Wave 2.11e #539 — required by combat.MoveEntity which
// calls room.MoveEntity per step to advance spatial state. Searches
// Players (by EntityID) first, then Monsters (by ID). Returns an error
// if the entity isn't found in the encounter.
//
// Per-step mutation during MovementResolver iteration: the encounter
// SDK's iteration loop ALSO commits the final position via
// applyAndPublishMove (player) or applyNPCMovementSteps (NPC) after the
// loop, but the final commit matches the last room.MoveEntity update —
// no double-mutation harm.
func (r *encounterHexRoom) MoveEntity(entityID string, pos spatial.Position) error {
	if r.data == nil {
		return fmt.Errorf("encounter spatial adapter: data is nil")
	}
	hex := posToHex(pos)
	// Try players first.
	for _, pd := range r.data.Players {
		if string(pd.EntityID) == entityID {
			if pd.View == nil {
				return fmt.Errorf("encounter spatial adapter: player %q has nil view", entityID)
			}
			pd.View.Position = hex
			return nil
		}
	}
	// Try monsters.
	if md, ok := r.data.Monsters[encountercore.EntityID(entityID)]; ok {
		md.Position = hex
		return nil
	}
	return fmt.Errorf("encounter spatial adapter: entity %q not in encounter", entityID)
}

// RemoveEntity is unsupported on the read-only adapter. Mutation must go
// through the encounter SDK.
func (r *encounterHexRoom) RemoveEntity(_ string) error {
	return fmt.Errorf("RemoveEntity unsupported on read-only encounter spatial adapter")
}

// GetEntitiesAt returns all entities at a specific position.
func (r *encounterHexRoom) GetEntitiesAt(pos spatial.Position) []core.Entity {
	if r.data == nil {
		return nil
	}
	var result []core.Entity
	for _, pd := range r.data.Players {
		if pd.View == nil {
			continue
		}
		p := hexToPos(pd.View.Position)
		if p.Equals(pos) {
			result = append(result, &hexEntity{id: string(pd.EntityID), entityType: entityTypeCharacter})
		}
	}
	for _, md := range r.data.Monsters {
		p := hexToPos(md.Position)
		if p.Equals(pos) {
			result = append(result, &hexEntity{id: string(md.ID), entityType: entityTypeMonster})
		}
	}
	return result
}

// IsPositionOccupied returns true if any entity occupies the given position.
func (r *encounterHexRoom) IsPositionOccupied(pos spatial.Position) bool {
	return len(r.GetEntitiesAt(pos)) > 0
}

// CanPlaceEntity always returns false for the read-only adapter. Callers
// should not attempt PlaceEntity — it will return an error.
func (r *encounterHexRoom) CanPlaceEntity(_ core.Entity, _ spatial.Position) bool { return false }

// GetPositionsInRange returns an empty slice — position enumeration is not
// needed by current condition handlers.
func (r *encounterHexRoom) GetPositionsInRange(_ spatial.Position, _ float64) []spatial.Position {
	return nil
}

// GetLineOfSight returns an empty slice — line-of-sight enumeration is not
// needed by current condition handlers using this adapter.
func (r *encounterHexRoom) GetLineOfSight(_ spatial.Position, _ spatial.Position) []spatial.Position {
	return nil
}

// IsLineOfSightBlocked always returns false — the adapter does not model walls.
func (r *encounterHexRoom) IsLineOfSightBlocked(_ spatial.Position, _ spatial.Position) bool {
	return false
}
