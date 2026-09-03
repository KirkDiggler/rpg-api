package sessionv1alpha1

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	sessionaccess "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/sessionaccess"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	rosterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/roster"
)

//go:generate mockgen -destination=mock/mock_manager.go -package=sessionv1alpha1mock github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1 Manager

// Manager is the subset of the toolkit session.Manager this service calls.
// Defined here, at the point of use, rather than depended on as the SDK's own
// (concrete, unmockable) *session.Manager type -- this is what lets handler
// tests fake verb outcomes without a real Manager/Redis/dice roller behind
// them. *session.Manager satisfies this interface structurally; no adapter
// is needed at construction.
type Manager interface {
	Join(ctx context.Context, in *sdk.JoinInput) (*sdk.JoinOutput, error)
	Exit(ctx context.Context, in *sdk.ExitInput) (*sdk.ExitOutput, error)
	Move(ctx context.Context, in *sdk.MoveInput) (*sdk.MoveOutput, error)
	Attack(ctx context.Context, in *sdk.AttackInput) (*sdk.AttackOutput, error)
	DeathSave(ctx context.Context, in *sdk.DeathSaveInput) (*sdk.DeathSaveOutput, error)
	Afford(ctx context.Context, in *sdk.AffordInput) (*sdk.AffordOutput, error)
	Turn(ctx context.Context, in *sdk.TurnInput) (*sdk.TurnOutput, error)
	EndTurn(ctx context.Context, in *sdk.EndTurnInput) (*sdk.EndTurnOutput, error)
	Activate(ctx context.Context, in *sdk.ActivateInput) (*sdk.ActivateOutput, error)
	Dissolve(ctx context.Context, in *sdk.DissolveInput) (*sdk.DissolveOutput, error)
	End(ctx context.Context, in *sdk.EndInput) (*sdk.EndOutput, error)
	Status(ctx context.Context, in *sdk.StatusInput) (*sdk.Status, error)
	Story(ctx context.Context, in *sdk.StoryInput) ([]sdk.Event, error)
	View(ctx context.Context, in *sdk.ViewInput) ([]sdk.Sighting, error)
	Atlas(ctx context.Context, in *sdk.AtlasInput) (*sdk.Atlas, error)
	Where(ctx context.Context, in *sdk.WhereInput) (*sdk.WhereOutput, error)
	Doors(ctx context.Context, in *sdk.DoorsInput) (*sdk.DoorsOutput, error)
	OpenDoor(ctx context.Context, in *sdk.OpenDoorInput) (*sdk.OpenDoorOutput, error)
	Unlock(ctx context.Context, in *sdk.UnlockInput) (*sdk.UnlockOutput, error)
	Search(ctx context.Context, in *sdk.SearchInput) (*sdk.SearchOutput, error)
	Interact(ctx context.Context, in *sdk.InteractInput) (*sdk.InteractOutput, error)
}

// Handler is the wire form of the toolkit's rulebooks/dnd5e/session SDK.
// Every method is proto <-> SDK translation only (design rule 8): extract
// the caller, build an SDK input, call exactly one Manager verb, translate
// the output back. No rule lives here.
type Handler struct {
	sessionpb.UnimplementedSessionServiceServer

	manager Manager
	// broker is the concrete session orchestrator Broker rather than a local
	// interface: StreamEvents already needs sessionorch.Subscription (its
	// Subscribe return type) imported regardless, so a hand-rolled interface
	// would buy no additional test isolation over using the real broker,
	// which is cheap to construct in tests (see broker_test.go's own suite).
	broker *sessionorch.Broker
	// characters backs the entitlement check every member-taking verb runs
	// (callerActingAs below). The SDK's sentinels answer whether a member is
	// IN the session; they cannot answer whether this caller is allowed to be
	// that member, which is the question this repository exists here to settle.
	characters characterrepo.Repository
	// roster is the launch-written roster store GetRoster serves from
	// (rpg-project#264, ideas/characters/presentation).
	roster rosterrepo.Repository
	// access centralizes the member/seat entitlement checks shared across the
	// session presentation handlers.
	access *sessionaccess.Access
}

// HandlerConfig carries what New needs to build a Handler. Every field is
// required.
type HandlerConfig struct {
	Manager    Manager
	Broker     *sessionorch.Broker
	Characters characterrepo.Repository
	// Roster is the launch-written roster store GetRoster serves from
	// (rpg-project#264). Required.
	Roster rosterrepo.Repository
	// Access is the shared caller/member/session gate. Optional for legacy
	// tests that construct HandlerConfig directly; production and the
	// integration harness pass one shared instance to SessionService and
	// SessionPresentationService.
	Access *sessionaccess.Access
}

// New constructs a Handler. Returns an error on any missing dependency.
func New(cfg *HandlerConfig) (*Handler, error) {
	if cfg == nil {
		return nil, errors.New("session handler: HandlerConfig is required")
	}
	if cfg.Manager == nil {
		return nil, errors.New("session handler: HandlerConfig.Manager is required")
	}
	if cfg.Broker == nil {
		return nil, errors.New("session handler: HandlerConfig.Broker is required")
	}
	if cfg.Characters == nil {
		return nil, errors.New("session handler: HandlerConfig.Characters is required")
	}
	if cfg.Roster == nil {
		return nil, errors.New("session handler: HandlerConfig.Roster is required")
	}
	access := cfg.Access
	if access == nil {
		var err error
		access, err = sessionaccess.New(cfg.Characters, cfg.Roster)
		if err != nil {
			return nil, err
		}
	}
	return &Handler{manager: cfg.Manager, broker: cfg.Broker, characters: cfg.Characters, roster: cfg.Roster, access: access}, nil
}

// authenticatedPlayerID extracts the caller's player ID from ctx, or a
// codes.Unauthenticated status if there isn't one. Every handler starts here.
func authenticatedPlayerID(ctx context.Context) (string, error) {
	playerID := auth.GetPlayerID(ctx)
	if playerID == "" {
		return "", status.Error(codes.Unauthenticated, "no player id in context")
	}
	return playerID, nil
}

// callerActingAs is the single gate every verb that names a member passes
// through: the caller is authenticated, the member is present, and the caller
// controls it.
func (h *Handler) callerActingAs(ctx context.Context, member string) error {
	gate, err := h.accessGate()
	if err != nil {
		return err
	}
	return gate.CallerActingAs(ctx, member)
}

func (h *Handler) accessGate() (*sessionaccess.Access, error) {
	if h.access != nil {
		return h.access, nil
	}
	gate, err := sessionaccess.New(h.characters, h.roster)
	if err != nil {
		return nil, err
	}
	h.access = gate
	return gate, nil
}
