package lobby

import lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"

// EventKind identifies which field of Event is populated. One entity-layer
// mirror per LobbyEvent proto oneof branch (lobby-surface.md's Proposed
// Surface) — the handler's translate.go maps these onto proto, so this
// package never imports proto (matches the v2 encounter orchestrator's
// boundary discipline).
type EventKind int

// Event kinds, one per LobbyEvent proto oneof branch.
const (
	EventKindUnspecified EventKind = iota
	EventKindSnapshot
	EventKindMemberJoined
	EventKindMemberLeft
	EventKindMemberReady
	EventKindMemberConnectionChanged
	EventKindHostChanged
	EventKindEncounterStarted
)

// Event is a single lobby-stream event. Kind selects which field is
// populated; exactly one is non-nil per Kind.
type Event struct {
	Kind EventKind

	Snapshot                *SnapshotPayload
	MemberJoined            *lobbyrepo.Member
	MemberLeft              *MemberLeftPayload
	MemberReady             *MemberReadyPayload
	MemberConnectionChanged *MemberConnectionChangedPayload
	HostChanged             *HostChangedPayload
	EncounterStarted        *EncounterStartedPayload
}

// SnapshotPayload is the first event StreamLobby sends: the full roster at
// subscribe time (mirrors StreamEncounter's SnapshotDelivered).
type SnapshotPayload struct {
	Members []*lobbyrepo.Member
}

// MemberLeftPayload names the player who explicitly left via LeaveLobby.
type MemberLeftPayload struct {
	PlayerID string
}

// MemberReadyPayload carries a SetReady toggle.
type MemberReadyPayload struct {
	PlayerID string
	Ready    bool
}

// MemberConnectionChangedPayload carries a StreamLobby subscribe/disconnect
// presence flip. NOT a membership change — the seat stays reserved.
type MemberConnectionChangedPayload struct {
	PlayerID  string
	Connected bool
}

// HostChangedPayload names the new host after a LeaveLobby-triggered
// migration.
type HostChangedPayload struct {
	PlayerID string
}

// EncounterStartedPayload carries the freshly constructed encounter's ID.
// Publish only happens AFTER the encounter is persisted — persist-then-emit
// is load-bearing (lobby-surface.md): a client reacting to this event must
// find the encounter already in the encounter repo.
type EncounterStartedPayload struct {
	EncounterID string
}
