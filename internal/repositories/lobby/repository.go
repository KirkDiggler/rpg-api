// Package lobby is the party-assembly store: join refs, membership, ready
// flags, and lifecycle for a v1alpha1 LobbyService lobby.
//
// This is pure API state (rpg-api#629 / rpg-project#81 lobby-surface design).
// The toolkit has zero lobby concept and this package must keep it that way —
// Data and Member are rpg-api-owned entities, not a toolkit mirror.
package lobby

import (
	"context"
	"errors"
)

// ErrNotFound indicates the lobby id (or join ref) has no stored data.
var ErrNotFound = errors.New("lobby not found")

// Status is a lobby's lifecycle state. WAITING -> STARTED is terminal — no
// lobby ever transitions back to WAITING (lobby-surface.md "Lifecycle").
type Status int

const (
	// StatusUnspecified is the zero value; never persisted.
	StatusUnspecified Status = iota
	// StatusWaiting is a lobby accepting joins/ready-ups, pre-StartEncounter.
	StatusWaiting
	// StatusStarted is a lobby whose StartEncounter has fired. Terminal.
	StatusStarted
)

// Member is one seat in a lobby.
type Member struct {
	PlayerID string `json:"player_id"`
	// CharacterID is the character this player has bound. JoinLobby is
	// idempotent and rebinds this field on repeat calls (lobby-surface.md
	// "Contract edge cases") rather than erroring.
	CharacterID string `json:"character_id"`
	// CharacterName is server-enriched from the character store at
	// bind/rebind time so clients render a name without a second fetch.
	CharacterName string `json:"character_name"`
	IsHost        bool   `json:"is_host"`
	IsReady       bool   `json:"is_ready"`
	// IsConnected tracks StreamLobby subscribe/disconnect, NOT membership.
	// A dropped stream flips this false but keeps the seat — only explicit
	// LeaveLobby removes a member (lobby-surface.md "Presence").
	IsConnected bool `json:"is_connected"`
}

// Data is a lobby's persisted state.
type Data struct {
	ID           string `json:"id"`
	JoinRef      string `json:"join_ref"`
	CampaignID   string `json:"campaign_id"`
	HostPlayerID string `json:"host_player_id"`
	Status       Status `json:"status"`
	// Members is keyed by PlayerID for O(1) membership / ownership checks.
	Members map[string]*Member `json:"members"`
	// MemberOrder preserves join order (oldest first). Go map iteration order
	// is random, so host migration ("oldest remaining member",
	// lobby-surface.md "Host leaves") needs this explicit ordering. Every
	// mutating method keeps it in lockstep with Members.
	MemberOrder []string `json:"member_order"`
	// EncounterID is set once StartEncounter transitions Status to Started.
	EncounterID string `json:"encounter_id,omitempty"`
}

// Repository persists lobby Data. Implementations MUST support lookup by
// ID (every RPC except JoinLobby), JoinRef (JoinLobby's only membership key,
// per the CreateLobby -> {lobby_id, join_ref} split), and PlayerID (the
// resume-after-refresh lookup, GetMyActiveLobby — rpg-dnd5e-web#444).
type Repository interface {
	// Get returns ErrNotFound if id has no stored data.
	Get(ctx context.Context, id string) (*Data, error)

	// GetByJoinRef returns ErrNotFound if joinRef has no stored data.
	GetByJoinRef(ctx context.Context, joinRef string) (*Data, error)

	// GetByPlayerID returns the lobby playerID is currently indexed against,
	// or ErrNotFound if playerID has no active index entry. One active lobby
	// per player: Save re-indexes every member present in data.Members on
	// every call, so a player who is (or becomes) a member of a second lobby
	// has their entry silently overwritten to point at whichever lobby was
	// Saved most recently — last write wins, no error, no dual-membership
	// tracking. Callers needing liveness (is the STARTED lobby's encounter
	// still running) must check separately; this method returns raw stored
	// Data with no cross-check.
	GetByPlayerID(ctx context.Context, playerID string) (*Data, error)

	// Save replaces the stored data for data.ID, indexed by ID, JoinRef, and
	// PlayerID for every member currently in data.Members. Save only ever
	// ADDS/refreshes player index entries for members present in data — it
	// never removes an entry for a player who is no longer a member (a
	// removed member's stale entry would otherwise keep resolving to this
	// lobby). Callers that remove a member (LeaveLobby) MUST pair their Save
	// with an explicit ClearPlayerIndex for that player.
	Save(ctx context.Context, data *Data) error

	// ClearPlayerIndex removes playerID's active-lobby index entry. No-op
	// (nil error) if playerID has no entry. LeaveLobby calls this for the
	// departing player; nothing else needs to.
	ClearPlayerIndex(ctx context.Context, playerID string) error
}
