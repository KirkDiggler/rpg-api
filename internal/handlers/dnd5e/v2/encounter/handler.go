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

	if len(req.GetProposedPath()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "proposed_path is required")
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
		// Toolkit Move errors are state-dependent (turn order, blocked path,
		// out-of-range, entity not in encounter, etc.) — these are
		// FailedPrecondition per gRPC convention. Empty-path is the only
		// genuinely argument-shaped error and is filtered before we reach Move.
		return nil, status.Errorf(codes.FailedPrecondition, "move: %v", err)
	}

	if err := h.encRepo.Save(ctx, enc.ToData()); err != nil {
		return nil, status.Errorf(codes.Internal, "save encounter: %v", err)
	}

	return &encounterv2pb.MoveEntityResponse{}, nil
}

// StreamEncounter opens a server-streaming session for the authenticated player.
// It emits an initial SnapshotDelivered event immediately, then forwards all
// subsequent broker events for the encounter until the client disconnects.
//
// Subscribe-before-snapshot ordering is intentional: subscribing first ensures
// no events are missed while the snapshot is being built. The broker's buffered
// channel holds any in-flight events until the forward loop starts.
func (h *Handler) StreamEncounter(req *encounterv2pb.StreamEncounterRequest, stream encounterv2pb.EncounterService_StreamEncounterServer) error {
	ctx := stream.Context()
	playerID := auth.GetPlayerID(ctx)
	if playerID == "" {
		return status.Error(codes.Unauthenticated, "no player id in context")
	}
	encID := core.EncounterID(req.GetEncounterId())
	if encID == "" {
		return status.Error(codes.InvalidArgument, "encounter_id is required")
	}

	// Subscribe FIRST so the broker holds events in its buffered channel while
	// we build the snapshot. Any event firing between Subscribe and the forward
	// loop is captured and delivered after the snapshot send.
	sub, err := h.broker.Subscribe(encID, core.PlayerID(playerID))
	if err != nil {
		return status.Errorf(codes.Internal, "subscribe: %v", err)
	}
	defer func() { _ = sub.Close() }()

	// Snapshot the encounter at-time-of-connect.
	data, err := h.encRepo.Get(ctx, string(encID))
	if err != nil {
		if errors.Is(err, encountersv2.ErrNotFound) {
			return status.Error(codes.NotFound, "encounter not found")
		}
		return status.Errorf(codes.Internal, "load encounter: %v", err)
	}
	enc, err := encounter.LoadFromData(data, h.broker)
	if err != nil {
		return status.Errorf(codes.Internal, "load from data: %v", err)
	}
	snap := enc.SnapshotFor(core.PlayerID(playerID))
	snapEvent := translateSnapshot(snap, h.now())
	if err := stream.Send(snapEvent); err != nil {
		return err
	}

	// Forward broker events until the client disconnects or the subscription closes.
	for {
		select {
		case <-ctx.Done():
			return nil
		case evt, ok := <-sub.Events():
			if !ok {
				return nil
			}
			out, translateErr := TranslateEvent(evt, core.PlayerID(playerID), h.now())
			switch {
			case errors.Is(translateErr, ErrViewerSawNothing):
				continue
			case errors.Is(translateErr, ErrUnknownEventType):
				// TODO(metric): increment translator-gap counter
				continue
			case translateErr != nil:
				return status.Errorf(codes.Internal, "translate: %v", translateErr)
			}
			if err := stream.Send(out); err != nil {
				return err
			}
		}
	}
}
