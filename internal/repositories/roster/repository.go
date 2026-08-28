// Package roster is the launch-written record of who is in a session — the
// server-side source for the session wire's GetRoster (rpg-project#264,
// ideas/characters/presentation).
//
// PURE API STATE, same law as the lobby store: the toolkit's session module
// deliberately has no roster enumeration and never grows one — presentation
// is not rules data. The lobby orchestrator writes one row at launch (it is
// the only moment that knows every member and every authored spawn at once),
// and the session handler reads it back to assemble PublicMemberInfo.
//
// The row stores IDENTITY FACTS ONLY: member ids, kinds, and the authored
// monster ref/name the spawn reported. It stores nothing that lives
// elsewhere — a player's name and class/race refs are read FRESH from the
// character record at GetRoster time (so an edit between encounters is never
// served stale), and nothing private ever passes through here at all.
package roster

import (
	"context"
	"errors"
)

// ErrNotFound indicates the encounter id has no stored roster.
var ErrNotFound = errors.New("roster not found")

// Kind says which side of the public split a member's identity resolves
// through: a player's identity lives in their character record, a monster's
// in its authored ref.
type Kind int

const (
	// KindUnspecified is the zero value; never persisted.
	KindUnspecified Kind = iota
	// KindPlayer is a character-backed member. ID doubles as the character
	// id (the launch flow joins members by character id).
	KindPlayer
	// KindMonster is an authored spawn.
	KindMonster
)

// Member is one row of the roster: the identity facts known at launch.
type Member struct {
	// ID is the session member id — the id Sighting.subject and Member.id
	// speak on the wire. For players this is also the character id.
	ID string `json:"id"`
	// Kind is player or monster.
	Kind Kind `json:"kind"`
	// Ref is the authored monster ref ("dnd5e:monsters:skeleton"), exactly
	// as the spawn reported it. Empty for players — their identity is read
	// from the character record, not stored here.
	Ref string `json:"ref,omitempty"`
	// Name is the monster's authored display name, exactly as the spawn
	// reported it (SpawnOutput.NPC.Name). Empty for players — read fresh.
	Name string `json:"name,omitempty"`
}

// Data is one session's roster.
type Data struct {
	// EncounterID keys the row — the same id the session verbs speak.
	EncounterID string `json:"encounter_id"`
	// Members in launch order: party first (join order), then spawns
	// (compiler order).
	Members []Member `json:"members"`
}

// Repository persists roster Data, keyed by encounter id.
type Repository interface {
	// Get returns ErrNotFound if encounterID has no stored roster.
	Get(ctx context.Context, encounterID string) (*Data, error)

	// Save replaces the stored roster for data.EncounterID.
	Save(ctx context.Context, data *Data) error
}
