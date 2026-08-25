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
	Afford(ctx context.Context, in *sdk.AffordInput) (*sdk.AffordOutput, error)
	Turn(ctx context.Context, in *sdk.TurnInput) (*sdk.TurnOutput, error)
	EndTurn(ctx context.Context, in *sdk.EndTurnInput) (*sdk.EndTurnOutput, error)
	Dissolve(ctx context.Context, in *sdk.DissolveInput) (*sdk.DissolveOutput, error)
	End(ctx context.Context, in *sdk.EndInput) (*sdk.EndOutput, error)
	Status(ctx context.Context, in *sdk.StatusInput) (*sdk.Status, error)
	Story(ctx context.Context, in *sdk.StoryInput) ([]sdk.Event, error)
	View(ctx context.Context, in *sdk.ViewInput) ([]sdk.Sighting, error)
	Atlas(ctx context.Context, in *sdk.AtlasInput) (*sdk.Atlas, error)
	Where(ctx context.Context, in *sdk.WhereInput) (*sdk.WhereOutput, error)
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
	return &Handler{manager: cfg.Manager, broker: cfg.Broker, characters: cfg.Characters, roster: cfg.Roster}, nil
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
//
// One helper rather than three lines in each handler, because the failure mode
// this closes is a handler that forgets one of them -- and eleven of the twelve
// did. An empty member is refused here rather than deeper: the SDK would answer
// ErrNoMember and translate to the same INVALID_ARGUMENT, but a member that is
// not named cannot be one this caller owns, so the ownership question has no
// meaning until it is.
func (h *Handler) callerActingAs(ctx context.Context, member string) error {
	playerID, err := authenticatedPlayerID(ctx)
	if err != nil {
		return err
	}
	if member == "" {
		return status.Error(codes.InvalidArgument, "member is required")
	}
	return h.verifyMemberOwnership(ctx, playerID, member)
}

// verifyMemberOwnership confirms playerID controls the character named member.
//
// The session SDK has no notion of a caller's identity -- a member ID is just a
// character ID it was handed, and Where/Move/Attack answer for whichever one
// arrives -- so ownership is rpg-api's to enforce, the same way the old
// encounter path's ErrEntityOwnershipMismatch is: a security boundary concern,
// not a game rule (design rule 8 governs rules, not authz).
//
// THIS USED TO GUARD ONLY StreamEvents. Every other verb passed `member`
// straight through, which meant any authenticated player could act as any
// member ID they could guess -- move somebody else's fighter, swing their
// sword, end their turn. On the read side it was worse than impersonation: a
// client-supplied member on GetWhere or GetView re-opened exactly the
// unperceived-roster leak the toolkit deliberately refused a batch positions
// read to prevent (rpg-toolkit#1051), and monster IDs are learnable from story
// beats. Both halves were flagged independently, by Copilot on the mutating
// verbs and by this PR's own gate item on GetWhere; see callerActingAs.
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
