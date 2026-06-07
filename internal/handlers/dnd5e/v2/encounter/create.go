package encounter

import (
	"context"

	"github.com/google/uuid"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
)

// CreateEncounter handles the CreateEncounter RPC. It validates auth and
// request fields, constructs a fresh encounter with the caller as the sole
// player, persists it, and returns the proto Encounter projected for the
// caller's view.
func (h *Handler) CreateEncounter(ctx context.Context, req *encounterv2pb.CreateEncounterRequest) (*encounterv2pb.CreateEncounterResponse, error) {
	playerID := auth.GetPlayerID(ctx)
	if playerID == "" {
		return nil, status.Error(codes.Unauthenticated, "no player id in context")
	}
	if req.GetCampaignId() == "" {
		return nil, status.Error(codes.InvalidArgument, "campaign_id is required")
	}
	// Wave 2.6 supported FREE_ROAM only. Wave 2.8 adds TURN_BASED via the
	// toolkit's Encounter.SetMode verb, which rolls initiative on flip.
	// UNSPECIFIED is treated as FREE_ROAM for forward compatibility with
	// older clients.
	switch req.GetInitialMode() {
	case encounterv2pb.EncounterMode_ENCOUNTER_MODE_UNSPECIFIED,
		encounterv2pb.EncounterMode_ENCOUNTER_MODE_FREE_ROAM,
		encounterv2pb.EncounterMode_ENCOUNTER_MODE_TURN_BASED:
	default:
		return nil, status.Errorf(codes.InvalidArgument,
			"initial_mode %s is not a recognized mode",
			req.GetInitialMode())
	}

	encID := core.EncounterID(uuid.NewString())
	// WithCharacterResolver wires the modifier-lookup hook needed for any
	// future SubmitCheck against this encounter. CreateEncounter doesn't
	// itself issue prompts, but persisting a resolver-less New encounter
	// would leave subsequent SubmitCheck calls with ErrNoCharacterResolver.
	// CreateEncounter builds a fresh encounter with no monsters yet. Pass nil
	// data to buildCombatResolver — the Dnd5eCombatResolver handles nil data
	// gracefully (no monster map means all entities fall back to stand-in).
	enc := tkenc.New(ctx, encID, h.broker, tkenc.WithCharacterResolver(h.resolver), tkenc.WithCombatResolver(h.buildCombatResolver(nil)))
	// The creator is automatically added as the encounter's first player.
	// Entity ID mirrors the player ID for the initial seat; future PRs can
	// wire in a character-selection step that replaces this with the
	// player's chosen character ID.
	if err := enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: core.PlayerID(playerID),
		EntityID: core.EntityID(playerID),
		Position: core.Hex{Q: 0, R: 0, S: 0},
	}); err != nil {
		return nil, status.Errorf(codes.Internal, "add creator to encounter: %v", err)
	}

	// Flip into TURN_BASED if requested. Mode flip publishes ModeChangedEvent +
	// initial TurnStartedEvent, but no subscribers exist at construction time so
	// these are dropped (the broker is fanout-only; events fire iff a viewer is
	// subscribed). Subsequent connect-time ProjectFor reads Mode/Initiative from
	// data so client state is correct on first stream.
	if req.GetInitialMode() == encounterv2pb.EncounterMode_ENCOUNTER_MODE_TURN_BASED {
		if err := enc.SetMode(core.ModeTurnBased); err != nil {
			return nil, status.Errorf(codes.Internal, "set turn-based mode: %v", err)
		}
	}

	data := enc.ToData()
	if err := h.encRepo.Save(ctx, data); err != nil {
		return nil, status.Errorf(codes.Internal, "save encounter: %v", err)
	}

	pbEncounter, err := ProjectFor(ctx, data, core.PlayerID(playerID), h.broker, h.combatResolverConfig.CharacterRepo, h.now())
	if err != nil {
		return nil, status.Errorf(codes.Internal, "project encounter: %v", err)
	}

	return &encounterv2pb.CreateEncounterResponse{
		Encounter: pbEncounter,
	}, nil
}
