package encounter

import (
	"fmt"
	"math"

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
// Only GetEntityPosition and GetEntitiesInRange are meaningfully implemented;
// they are the only methods that Sneak Attack (and similar spatial conditions)
// call. All mutating methods (PlaceEntity, MoveEntity, RemoveEntity) return
// an error — mutation must go through the encounter SDK, not this view.
// Query-only methods that are not needed by current conditions (GetGrid,
// GetAllEntities, IsPositionOccupied, etc.) return zero/false/nil.
//
// The adapter is read-only and constructed fresh per attack from the encounter's
// *tkenc.Data snapshot. It does not hold a reference to the live Encounter
// object — only to the immutable data snapshot, so concurrent reads during
// the attack chain are safe.
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

// newEncounterHexRoom constructs a read-only spatial.Room view over the
// encounter's current entity positions. Called once per attack resolution
// to supply the gamectx Room for condition handlers.
func newEncounterHexRoom(data *tkenc.Data) spatial.Room {
	return &encounterHexRoom{data: data}
}

// hexToPos converts an encounter core.Hex to a spatial.Position.
// Q→X, R→Y following the encounter hex coordinate convention.
func hexToPos(h encountercore.Hex) spatial.Position {
	return spatial.Position{X: float64(h.Q), Y: float64(h.R)}
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
// are included. The radius is compared against the Euclidean distance between
// hexToPos(entity) and center, matching how spatial.BasicRoom implements it.
//
// The caller (SneakAttack) uses this to find allies adjacent to a target.
func (r *encounterHexRoom) GetEntitiesInRange(center spatial.Position, radius float64) []core.Entity {
	if r.data == nil {
		return nil
	}
	var result []core.Entity
	for _, pd := range r.data.Players {
		if pd.View == nil {
			continue
		}
		pos := hexToPos(pd.View.Position)
		if euclidean(pos, center) <= radius {
			result = append(result, &hexEntity{id: string(pd.EntityID), entityType: entityTypeCharacter})
		}
	}
	for _, md := range r.data.Monsters {
		pos := hexToPos(md.Position)
		if euclidean(pos, center) <= radius {
			result = append(result, &hexEntity{id: string(md.ID), entityType: entityTypeMonster})
		}
	}
	return result
}

// euclidean returns the Euclidean distance between two positions.
func euclidean(a, b spatial.Position) float64 {
	dx := a.X - b.X
	dy := a.Y - b.Y
	return math.Sqrt(dx*dx + dy*dy)
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

// GetGrid returns nil — the encounter hex room does not expose a grid
// system. Callers that need grid-based distance should use GetEntitiesInRange.
func (r *encounterHexRoom) GetGrid() spatial.Grid { return nil }

// PlaceEntity is unsupported on the read-only adapter. Mutation must go
// through the encounter SDK.
func (r *encounterHexRoom) PlaceEntity(_ core.Entity, _ spatial.Position) error {
	return fmt.Errorf("PlaceEntity unsupported on read-only encounter spatial adapter")
}

// MoveEntity is unsupported on the read-only adapter. Mutation must go
// through the encounter SDK.
func (r *encounterHexRoom) MoveEntity(_ string, _ spatial.Position) error {
	return fmt.Errorf("MoveEntity unsupported on read-only encounter spatial adapter")
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
