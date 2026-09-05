package entities

import "time"

// CompositionDefinition is the stable identity and current head of a guild-owned composition.
type CompositionDefinition struct {
	ID                string    `json:"id"`
	GuildID           string    `json:"guild_id"`
	CreatedByPlayerID string    `json:"created_by_player_id"`
	CreatedAt         time.Time `json:"created_at"`
	HeadRevisionID    string    `json:"head_revision_id"`
}

// CompositionRevision is an immutable snapshot of composition source.
type CompositionRevision struct {
	ID                string            `json:"id"`
	DefinitionID      string            `json:"definition_id"`
	GuildID           string            `json:"guild_id"`
	CreatedByPlayerID string            `json:"created_by_player_id"`
	CreatedAt         time.Time         `json:"created_at"`
	Source            CompositionSource `json:"source"`
}

// CompositionSource is editable semantic source. AssetRef values are logical
// base-catalog references; preserving them does not version the underlying asset bytes.
type CompositionSource struct {
	Version uint32             `json:"version"`
	Name    string             `json:"name"`
	Items   []CompositionItem  `json:"items"`
	Groups  []CompositionGroup `json:"groups"`
}

// CompositionItem is one visual prop in root-frame coordinates.
type CompositionItem struct {
	ID        string               `json:"id"`
	Kind      string               `json:"kind"`
	AssetRef  string               `json:"asset_ref"`
	Label     string               `json:"label"`
	Transform CompositionTransform `json:"transform"`
	ParentID  string               `json:"parent_id,omitempty"`
	SupportID string               `json:"support_id,omitempty"`
}

// CompositionGroup is an authoring relationship node, not a gameplay object.
type CompositionGroup struct {
	ID        string               `json:"id"`
	Kind      string               `json:"kind"`
	Label     string               `json:"label"`
	Transform CompositionTransform `json:"transform"`
	ParentID  string               `json:"parent_id,omitempty"`
}

// CompositionTransform is a root-frame position and Y-axis yaw in radians.
type CompositionTransform struct {
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	Z         float64 `json:"z"`
	RotationY float64 `json:"rotation_y"`
}
