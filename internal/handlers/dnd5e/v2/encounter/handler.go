package encounter

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	encountersv2 "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/v2"
	"github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
)

// HandlerConfig configures a v2 encounter Handler.
type HandlerConfig struct {
	Broker *encounter.Broker
	Repo   encountersv2.Repository
	Now    func() time.Time // optional; defaults to time.Now
}

// Handler implements dnd5e.api.v1alpha2.encounter.EncounterServiceServer.
//
// Only MoveEntity and StreamEncounter ship in slice 1. Every other RPC
// returns codes.Unimplemented via the embedded server.
type Handler struct {
	encounterv2pb.UnimplementedEncounterServiceServer
	broker  *encounter.Broker
	encRepo encountersv2.Repository
	now     func() time.Time
}

// New constructs a Handler. Returns error on missing required deps.
func New(cfg *HandlerConfig) (*Handler, error) {
	if cfg == nil {
		return nil, errors.New("HandlerConfig is required")
	}
	if cfg.Broker == nil {
		return nil, errors.New("HandlerConfig.Broker is required")
	}
	if cfg.Repo == nil {
		return nil, errors.New("HandlerConfig.Repo is required")
	}
	now := cfg.Now
	if now == nil {
		now = time.Now
	}
	return &Handler{broker: cfg.Broker, encRepo: cfg.Repo, now: now}, nil
}

// MoveEntity loads the encounter, validates that the request's entity_id matches
// the auth player's controlled entity, dispatches to the toolkit's Encounter.Move
// verb (which publishes per-viewer events to the broker), and saves the updated
// state. The empty MoveEntityResponse is by proto design — world changes flow as
// events on StreamEncounter.
func (h *Handler) MoveEntity(ctx context.Context, req *encounterv2pb.MoveEntityRequest) (*encounterv2pb.MoveEntityResponse, error) {
	playerID := auth.GetPlayerID(ctx)
	if playerID == "" {
		return nil, status.Error(codes.Unauthenticated, "no player id in context")
	}
	if req.GetEncounterId() == "" {
		return nil, status.Error(codes.InvalidArgument, "encounter_id is required")
	}

	data, err := h.encRepo.Get(ctx, req.GetEncounterId())
	if err != nil {
		if errors.Is(err, encountersv2.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "encounter not found")
		}
		return nil, status.Errorf(codes.Internal, "load encounter: %v", err)
	}

	// entity_id validation: the player's controlled entity must match the
	// request's entity_id. Read from data BEFORE LoadFromData consumes it.
	pd, ok := data.Players[core.PlayerID(playerID)]
	if !ok {
		return nil, status.Error(codes.PermissionDenied, "player is not in this encounter")
	}
	if string(pd.EntityID) != req.GetEntityId() {
		return nil, status.Error(codes.PermissionDenied, "entity_id does not match player's controlled entity")
	}

	enc, err := encounter.LoadFromData(data, h.broker)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "load from data: %v", err)
	}

	path := make([]core.Hex, 0, len(req.GetProposedPath()))
	for _, p := range req.GetProposedPath() {
		path = append(path, core.Hex{Q: int(p.X), R: int(p.Y), S: int(p.Z)})
	}

	if err := enc.Move(core.PlayerID(playerID), path); err != nil {
		// Toolkit-level errors map to InvalidArgument — they're the toolkit
		// telling us the move violated rules (empty path, player not found, etc.).
		return nil, status.Errorf(codes.InvalidArgument, "move: %v", err)
	}

	if err := h.encRepo.Save(ctx, enc.ToData()); err != nil {
		return nil, status.Errorf(codes.Internal, "save encounter: %v", err)
	}

	return &encounterv2pb.MoveEntityResponse{}, nil
}
