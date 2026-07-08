package lobby

import (
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
)

// lobbyStatusError maps the lobby orchestrator's sentinel errors onto gRPC
// status codes. Shared by every RPC handler in this package — each RPC only
// ever returns a subset of these sentinels, so one exhaustive switch covers
// all six rather than duplicating the mapping per verb.
//
//   - ErrLobbyNotFound → NotFound
//   - ErrPlayerNotInLobby → PermissionDenied
//   - ErrNotHost → PermissionDenied
//   - ErrCharacterOwnershipMismatch → PermissionDenied
//   - ErrCharacterNotFound → NotFound
//   - ErrLobbyAlreadyStarted / ErrLobbyFull / ErrNotAllReady → FailedPrecondition
//   - unclassified → Internal
func lobbyStatusError(err error) error {
	switch {
	case errors.Is(err, lobbyorch.ErrLobbyNotFound):
		return status.Error(codes.NotFound, "lobby not found")
	case errors.Is(err, lobbyorch.ErrCharacterNotFound):
		return status.Error(codes.NotFound, "character not found")
	case errors.Is(err, lobbyorch.ErrPlayerNotInLobby):
		return status.Error(codes.PermissionDenied, "player is not a member of this lobby")
	case errors.Is(err, lobbyorch.ErrNotHost):
		return status.Error(codes.PermissionDenied, "only the host can start the encounter")
	case errors.Is(err, lobbyorch.ErrCharacterOwnershipMismatch):
		return status.Error(codes.PermissionDenied, "character_id does not belong to the authenticated player")
	case errors.Is(err, lobbyorch.ErrLobbyAlreadyStarted):
		return status.Error(codes.FailedPrecondition, "lobby has already started")
	case errors.Is(err, lobbyorch.ErrLobbyFull):
		return status.Error(codes.FailedPrecondition, "lobby is full")
	case errors.Is(err, lobbyorch.ErrNotAllReady):
		return status.Error(codes.FailedPrecondition, "not all members are ready")
	}
	return status.Errorf(codes.Internal, "lobby: %v", err)
}
