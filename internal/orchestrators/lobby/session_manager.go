package lobby

import (
	"context"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

//go:generate mockgen -destination=mock/mock_session_manager.go -package=lobbymock github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby SessionManager

// SessionManager is the session SDK surface consumed by lobby orchestration:
// the six rulebooks/dnd5e/session Manager methods non-test lobby code calls —
// StartEncounter builds onto StartSession/Spawn/Join/PlaceNPC, and
// GetMyActiveLobby/AbandonEncounter query/close through Status/End.
//
// Defined here, at the point of use, rather than depended on as the SDK's own
// (concrete, unmockable) *session.Manager type — this is what lets lobby tests
// script SDK outcomes without constructing a playable session. The real
// *sdk.Manager satisfies this interface structurally; no adapter is needed and
// production wiring keeps passing the concrete manager.
type SessionManager interface {
	StartSession(context.Context, *sdk.StartSessionInput) (*sdk.StartSessionOutput, error)
	Spawn(context.Context, *sdk.SpawnInput) (*sdk.SpawnOutput, error)
	Join(context.Context, *sdk.JoinInput) (*sdk.JoinOutput, error)
	PlaceNPC(context.Context, *sdk.PlaceNPCInput) (*sdk.PlaceNPCOutput, error)
	Status(context.Context, *sdk.StatusInput) (*sdk.Status, error)
	End(context.Context, *sdk.EndInput) (*sdk.EndOutput, error)
}

var _ SessionManager = (*sdk.Manager)(nil)
