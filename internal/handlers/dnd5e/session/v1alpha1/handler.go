package sessionv1alpha1

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
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
	Traverse(ctx context.Context, in *sdk.TraverseInput) (*sdk.TraverseOutput, error)
	Attack(ctx context.Context, in *sdk.AttackInput) (*sdk.AttackOutput, error)
	Turn(ctx context.Context, in *sdk.TurnInput) (*sdk.TurnOutput, error)
	EndTurn(ctx context.Context, in *sdk.EndTurnInput) (*sdk.EndTurnOutput, error)
	Dissolve(ctx context.Context, in *sdk.DissolveInput) (*sdk.DissolveOutput, error)
	End(ctx context.Context, in *sdk.EndInput) (*sdk.EndOutput, error)
	Status(ctx context.Context, in *sdk.StatusInput) (*sdk.Status, error)
	Story(ctx context.Context, in *sdk.StoryInput) ([]sdk.StoryEntry, error)
	View(ctx context.Context, in *sdk.ViewInput) ([]sdk.Sighting, error)
	Atlas(ctx context.Context, in *sdk.AtlasInput) (*sdk.Atlas, error)
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
	// characters backs the StreamEvents entitlement check only (verifyMember
	// below) -- ownership of a member beyond that read is Manager's own
	// concern via the SDK's sentinels, not rechecked here.
	characters characterrepo.Repository
}

// HandlerConfig carries what New needs to build a Handler. Every field is
// required.
type HandlerConfig struct {
	Manager    Manager
	Broker     *sessionorch.Broker
	Characters characterrepo.Repository
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
	return &Handler{manager: cfg.Manager, broker: cfg.Broker, characters: cfg.Characters}, nil
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

// verifyMemberOwnership confirms playerID controls the character named
// member, for the one RPC that needs it: StreamEvents (design's own
// instruction to entitlement-check the caller against the member, mirroring
// the old encounter path's StreamEncounter auth pattern). The session SDK
// itself has no notion of a caller's identity -- a member ID is just a
// character ID it was handed -- so ownership is rpg-api's to enforce, the
// same way the old path's ErrEntityOwnershipMismatch is: a security boundary
// concern, not a game rule (design rule 8 governs rules, not authz).
//
// Every other verb handler passes `member` straight through to the Manager,
// unchecked: this wave scopes the entitlement check to the read/subscribe
// path per the brief that authored it, not to every mutating verb.
func (h *Handler) verifyMemberOwnership(ctx context.Context, playerID, member string) error {
	out, err := h.characters.Get(ctx, characterrepo.GetInput{ID: member})
	if err != nil {
		return status.Errorf(codes.NotFound, "member %q not found", member)
	}
	if out == nil || out.Character == nil || out.Character.Data == nil {
		return status.Errorf(codes.NotFound, "member %q not found", member)
	}
	if out.Character.Data.PlayerID != playerID {
		return status.Error(codes.PermissionDenied, "caller does not control this member")
	}
	return nil
}
