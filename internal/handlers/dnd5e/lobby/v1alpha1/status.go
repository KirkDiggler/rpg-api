package lobby

import (
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
	"github.com/KirkDiggler/rpg-api/internal/worldcontext"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

// lobbyStatusError maps the lobby orchestrator's sentinel errors onto gRPC
// status codes. Shared by every RPC handler in this package — each RPC only
// ever returns a subset of these sentinels, so one exhaustive switch covers
// all of them rather than duplicating the mapping per verb.
//
//   - ErrLobbyNotFound → NotFound
//   - ErrPlayerNotInLobby → PermissionDenied
//   - ErrNotHost → PermissionDenied
//   - ErrCharacterOwnershipMismatch → PermissionDenied
//   - ErrCharacterNotFound → NotFound
//   - ErrDungeonNotFound → NotFound
//   - dungeons.ErrNoCaller → FailedPrecondition
//   - ErrLobbyAlreadyStarted / ErrLobbyFull / ErrNotAllReady / ErrLobbyNotStarted /
//     ErrEncounterAlreadyEnded / ErrCharacterSeatedElsewhere → FailedPrecondition
//   - sdk.ErrInvalidWorld (a launch refusing the dungeon) → FailedPrecondition
//   - sdk.ErrWindowOpen (abandoning while a roll waits) → FailedPrecondition
//   - sdk.ErrSharingNotBuilt (a sharing stop not built yet) → FailedPrecondition
//   - unclassified → Internal
func lobbyStatusError(err error) error {
	switch {
	case errors.Is(err, lobbyorch.ErrLobbyNotFound):
		return status.Error(codes.NotFound, "lobby not found")
	case errors.Is(err, lobbyorch.ErrCharacterNotFound):
		return status.Error(codes.NotFound, "character not found")
	case errors.Is(err, lobbyorch.ErrDungeonNotFound):
		return status.Error(codes.NotFound, "dungeon not found")
	case errors.Is(err, dungeons.ErrNoCaller):
		return status.Error(codes.FailedPrecondition, worldcontext.RequiredMessage)
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
	case errors.Is(err, lobbyorch.ErrCharacterSeatedElsewhere):
		// The wrapped text carries the character and the session that holds
		// it; that sentence is the whole of what the host needs.
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, sdk.ErrInvalidWorld):
		// THE LAUNCH REFUSED THE DUNGEON (rpg-project#923): the session judged
		// the stored content unlaunchable — a party bigger than its seats, an
		// interaction whose approach is beyond Speak reach. The host's request
		// is well formed; the dungeon is what must change before a retry, so
		// FAILED_PRECONDITION, and the SDK's sentence (which names the
		// interaction or the seat count) is the message the author reads.
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, sdk.ErrSharingNotBuilt):
		// THE PARTY'S SHARING DIAL AT A STOP NOT BUILT (rpg-project#935,
		// rpg-project#937): the dungeon is valid -- it compiled, previewed and
		// saved -- and its author can change the setting, so this is a
		// precondition, not a broken world. The engine's sentence names the
		// stop and why.
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, sdk.ErrWindowOpen):
		// ABANDONING WHILE A ROLL WAITS (rpg-project#924): the session refuses
		// End while any window is open, a waiting Speak check included. The
		// host's request becomes legal once the roll is answered, so
		// FAILED_PRECONDITION, and the SDK's sentence names who the table
		// waits on.
		return status.Error(codes.FailedPrecondition, err.Error())
	}
	return status.Errorf(codes.Internal, "lobby: %v", err)
}
