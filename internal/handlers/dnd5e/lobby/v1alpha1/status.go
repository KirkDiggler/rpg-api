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
// all eight rather than duplicating the mapping per verb.
//
//   - ErrLobbyNotFound → NotFound
//   - ErrPlayerNotInLobby → PermissionDenied
//   - ErrNotHost → PermissionDenied
//   - ErrCharacterOwnershipMismatch → PermissionDenied
//   - ErrCharacterNotFound → NotFound
//   - ErrLobbyAlreadyStarted / ErrLobbyFull / ErrNotAllReady / ErrLobbyNotStarted /
//     ErrEncounterAlreadyEnded → FailedPrecondition
//   - ErrUnknownDungeonKey → NotFound (Task E3 — previously fell through to
//     Internal; unreachable via any real proto surface until a caller can
//     supply a DungeonKey, but honest regardless)
//   - *DisabledDungeonKeyError → InvalidArgument, carrying the stored
//     validation CAUSE (Task E3 — Unwrap()'s Cause, never Error()'s own
//     "lobby orchestrator: dungeon key ... is disabled:" wrapping prefix,
//     which is internal error-taxonomy language, not something a client
//     authoring content should ever see). Falls back to Error() if Cause
//     is nil — DisabledDungeonKeyError is exported, so a future
//     construction site could build one without a Cause; a handler
//     panicking on that malformed error would be worse than a message
//     that's merely less specific.
//   - unclassified → Internal
func lobbyStatusError(err error) error {
	var disabledErr *lobbyorch.DisabledDungeonKeyError
	switch {
	case errors.Is(err, lobbyorch.ErrLobbyNotFound):
		return status.Error(codes.NotFound, "lobby not found")
	case errors.Is(err, lobbyorch.ErrCharacterNotFound):
		return status.Error(codes.NotFound, "character not found")
	case errors.Is(err, lobbyorch.ErrUnknownDungeonKey):
		return status.Error(codes.NotFound, "unknown dungeon key")
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
	case errors.Is(err, lobbyorch.ErrLobbyNotStarted):
		return status.Error(codes.FailedPrecondition, "lobby has not started an encounter")
	case errors.Is(err, lobbyorch.ErrEncounterAlreadyEnded):
		return status.Error(codes.FailedPrecondition, "encounter has already ended")
	case errors.As(err, &disabledErr):
		if disabledErr.Cause == nil {
			return status.Error(codes.InvalidArgument, disabledErr.Error())
		}
		return status.Error(codes.InvalidArgument, disabledErr.Cause.Error())
	}
	return status.Errorf(codes.Internal, "lobby: %v", err)
}
