package lobby

import (
	"context"
	"log"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	lobbyv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/lobby/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
)

// disconnectPresenceTimeout bounds the best-effort presence flip on stream
// teardown so a stalled repository call (e.g. a Redis network stall) cannot
// hang the teardown goroutine indefinitely.
const disconnectPresenceTimeout = 5 * time.Second

// StreamLobby opens a server-streaming session for the authenticated player.
// Snapshot-first, mirroring StreamEncounter
// (internal/handlers/dnd5e/v2/encounter/handler.go): subscribe to the broker
// BEFORE loading the snapshot so no event fired in between is missed, send
// the roster snapshot, then forward broker events until the client
// disconnects.
//
// Presence is a side effect of this method's lifecycle (lobby-surface.md
// "Presence"): subscribing flips is_connected true and broadcasts
// MemberConnectionChanged; the deferred call on stream teardown flips it
// back false. A dropped connection is NOT a LeaveLobby — the seat stays
// reserved.
func (h *Handler) StreamLobby(
	req *lobbyv1alpha1.StreamLobbyRequest,
	stream lobbyv1alpha1.LobbyService_StreamLobbyServer,
) error {
	ctx := stream.Context()
	playerID := auth.GetPlayerID(ctx)
	if playerID == "" {
		return status.Error(codes.Unauthenticated, "no player id in context")
	}
	lobbyID := req.GetLobbyId()
	if lobbyID == "" {
		return status.Error(codes.InvalidArgument, "lobby_id is required")
	}

	// Subscribe FIRST so the broker holds events in its buffered channel
	// while SetConnected + the snapshot send run.
	sub, err := h.broker.Subscribe(lobbyID)
	if err != nil {
		return status.Errorf(codes.Internal, "subscribe %q: %v", lobbyID, err)
	}
	defer func() { _ = sub.Close() }()

	connectOut, err := h.orch.SetConnected(ctx, &lobbyorch.SetConnectedInput{
		PlayerID: playerID, LobbyID: lobbyID, Connected: true,
	})
	if err != nil {
		return lobbyStatusError(err)
	}

	// Flip presence back off on disconnect, best-effort on a detached but
	// bounded context (stream.Context() is already canceled by the time this
	// defer fires; the bound keeps a stalled repository call — e.g. a Redis
	// network stall — from hanging this teardown goroutine indefinitely). A
	// failure here is not surfaced — the RPC's real work is already done;
	// the seat simply keeps a stale is_connected until the next SetConnected
	// success (e.g. a fresh reconnect).
	defer func() {
		disconnectCtx, cancel := context.WithTimeout(context.Background(), disconnectPresenceTimeout)
		defer cancel()
		if _, connErr := h.orch.SetConnected(disconnectCtx, &lobbyorch.SetConnectedInput{
			PlayerID: playerID, LobbyID: lobbyID, Connected: false,
		}); connErr != nil {
			log.Printf("lobby/v1alpha1 StreamLobby: disconnect presence flip for %q/%q: %v", lobbyID, playerID, connErr)
		}
	}()

	if err := stream.Send(translateSnapshot(connectOut.Members)); err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case evt, ok := <-sub.Events():
			if !ok {
				return nil
			}
			out, translateErr := translateEvent(evt)
			if translateErr != nil {
				// No wire mapping for this event kind — log so the gap is
				// visible rather than silently dropped, matching the v2
				// encounter translator's ErrUnknownEventType handling.
				log.Printf("lobby/v1alpha1 translator gap: lobby=%q: %v", lobbyID, translateErr)
				continue
			}
			if err := stream.Send(out); err != nil {
				return err
			}
		}
	}
}
