package lobby

import (
	"fmt"

	lobbyv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/lobby/v1alpha1"
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
)

// translateLobbyStatus converts the entity-layer lobby Status to its proto
// mirror. lobbyrepo.StatusUnspecified (the zero value, used by
// GetMyActiveLobbyOutput's "no active lobby" empty response) maps to
// LOBBY_STATUS_UNSPECIFIED, proto3's own zero value — no explicit case
// needed, the switch's default covers it.
func translateLobbyStatus(s lobbyrepo.Status) lobbyv1alpha1.LobbyStatus {
	switch s {
	case lobbyrepo.StatusWaiting:
		return lobbyv1alpha1.LobbyStatus_LOBBY_STATUS_WAITING
	case lobbyrepo.StatusStarted:
		return lobbyv1alpha1.LobbyStatus_LOBBY_STATUS_STARTED
	default:
		return lobbyv1alpha1.LobbyStatus_LOBBY_STATUS_UNSPECIFIED
	}
}

// translateMember converts one entity-layer Member to its proto mirror.
func translateMember(m *lobbyrepo.Member) *lobbyv1alpha1.LobbyMember {
	if m == nil {
		return nil
	}
	return &lobbyv1alpha1.LobbyMember{
		PlayerId:      m.PlayerID,
		CharacterId:   m.CharacterID,
		CharacterName: m.CharacterName,
		IsHost:        m.IsHost,
		IsReady:       m.IsReady,
		IsConnected:   m.IsConnected,
	}
}

// translateMembers converts an ordered Member slice to its proto mirror.
func translateMembers(ms []*lobbyrepo.Member) []*lobbyv1alpha1.LobbyMember {
	out := make([]*lobbyv1alpha1.LobbyMember, 0, len(ms))
	for _, m := range ms {
		out = append(out, translateMember(m))
	}
	return out
}

// translateSnapshot builds the Snapshot LobbyEvent — the first event
// StreamLobby sends, mirroring StreamEncounter's SnapshotDelivered.
func translateSnapshot(members []*lobbyrepo.Member) *lobbyv1alpha1.LobbyEvent {
	return &lobbyv1alpha1.LobbyEvent{
		Event: &lobbyv1alpha1.LobbyEvent_Snapshot{
			Snapshot: &lobbyv1alpha1.LobbySnapshot{Members: translateMembers(members)},
		},
	}
}

// translateEvent converts one entity-layer Event into its proto
// LobbyEvent oneof branch. Returns an error for a Kind this translator has
// no mapping for — the caller logs and skips rather than sending a malformed
// wire event (mirrors the v2 encounter translator's ErrUnknownEventType gap
// handling).
func translateEvent(evt *lobbyorch.Event) (*lobbyv1alpha1.LobbyEvent, error) {
	switch evt.Kind {
	case lobbyorch.EventKindSnapshot:
		return translateSnapshot(evt.Snapshot.Members), nil
	case lobbyorch.EventKindMemberJoined:
		return &lobbyv1alpha1.LobbyEvent{
			Event: &lobbyv1alpha1.LobbyEvent_MemberJoined{
				MemberJoined: &lobbyv1alpha1.MemberJoined{Member: translateMember(evt.MemberJoined)},
			},
		}, nil
	case lobbyorch.EventKindMemberLeft:
		return &lobbyv1alpha1.LobbyEvent{
			Event: &lobbyv1alpha1.LobbyEvent_MemberLeft{
				MemberLeft: &lobbyv1alpha1.MemberLeft{PlayerId: evt.MemberLeft.PlayerID},
			},
		}, nil
	case lobbyorch.EventKindMemberReady:
		return &lobbyv1alpha1.LobbyEvent{
			Event: &lobbyv1alpha1.LobbyEvent_MemberReady{
				MemberReady: &lobbyv1alpha1.MemberReady{
					PlayerId: evt.MemberReady.PlayerID, Ready: evt.MemberReady.Ready,
				},
			},
		}, nil
	case lobbyorch.EventKindMemberConnectionChanged:
		return &lobbyv1alpha1.LobbyEvent{
			Event: &lobbyv1alpha1.LobbyEvent_MemberConnectionChanged{
				MemberConnectionChanged: &lobbyv1alpha1.MemberConnectionChanged{
					PlayerId: evt.MemberConnectionChanged.PlayerID, Connected: evt.MemberConnectionChanged.Connected,
				},
			},
		}, nil
	case lobbyorch.EventKindHostChanged:
		return &lobbyv1alpha1.LobbyEvent{
			Event: &lobbyv1alpha1.LobbyEvent_HostChanged{
				HostChanged: &lobbyv1alpha1.HostChanged{PlayerId: evt.HostChanged.PlayerID},
			},
		}, nil
	case lobbyorch.EventKindEncounterStarted:
		return &lobbyv1alpha1.LobbyEvent{
			Event: &lobbyv1alpha1.LobbyEvent_EncounterStarted{
				EncounterStarted: &lobbyv1alpha1.EncounterStarted{EncounterId: evt.EncounterStarted.EncounterID},
			},
		}, nil
	default:
		return nil, fmt.Errorf("lobby/v1alpha1 translator: unknown event kind %d", evt.Kind)
	}
}
