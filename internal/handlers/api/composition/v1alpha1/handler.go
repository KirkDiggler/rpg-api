// Package compositionv1alpha1 implements the local-dev CompositionService wire boundary.
package compositionv1alpha1

import (
	"context"
	"encoding/json"

	compositionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/api/composition/v1alpha1"
	worldcomposition "github.com/KirkDiggler/rpg-toolkit/world/composition"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	compositionservice "github.com/KirkDiggler/rpg-api/internal/services/composition"
)

// HandlerConfig configures a CompositionService handler.
type HandlerConfig struct {
	Service          compositionservice.Service
	WorldID          string
	AuthoringEnabled bool
}

// Handler translates CompositionService protobuf messages to service inputs.
type Handler struct {
	compositionpb.UnimplementedCompositionServiceServer

	service          compositionservice.Service
	worldID          string
	authoringEnabled bool
}

// New creates a CompositionService handler.
func New(cfg *HandlerConfig) (*Handler, error) {
	if cfg == nil {
		return nil, apierr.InvalidArgument("composition handler config is required")
	}
	if cfg.Service == nil {
		return nil, apierr.InvalidArgument("composition service is required")
	}
	if cfg.WorldID == "" {
		return nil, apierr.InvalidArgument("composition world ID is required")
	}
	return &Handler{
		service:          cfg.Service,
		worldID:          cfg.WorldID,
		authoringEnabled: cfg.AuthoringEnabled,
	}, nil
}

// CreateComposition saves one new immutable composition snapshot.
func (h *Handler) CreateComposition(ctx context.Context, req *compositionpb.CreateCompositionRequest) (*compositionpb.CreateCompositionResponse, error) {
	playerID, err := h.authorizeWorld(ctx, req.GetWorldId())
	if err != nil {
		return nil, apierr.ToGRPCError(err)
	}
	if !h.authoringEnabled {
		return nil, apierr.ToGRPCError(apierr.FailedPrecondition("composition authoring is disabled"))
	}
	if req.GetJson() == "" {
		return nil, apierr.ToGRPCError(apierr.InvalidArgument("composition JSON is required"))
	}
	if !json.Valid([]byte(req.GetJson())) {
		return nil, apierr.ToGRPCError(apierr.InvalidArgument("composition JSON must be valid JSON"))
	}

	output, err := h.service.Create(ctx, &compositionservice.CreateInput{
		PlayerID: playerID,
		WorldID:  h.worldID,
		JSON:     json.RawMessage(req.GetJson()),
	})
	if err != nil {
		return nil, apierr.ToGRPCError(err)
	}
	if output == nil || output.Composition == nil {
		return nil, apierr.ToGRPCError(apierr.Internal("composition service returned no created composition"))
	}
	return &compositionpb.CreateCompositionResponse{Composition: compositionToProto(output.Composition)}, nil
}

// GetComposition returns one immutable composition snapshot.
func (h *Handler) GetComposition(ctx context.Context, req *compositionpb.GetCompositionRequest) (*compositionpb.GetCompositionResponse, error) {
	playerID, err := h.authorizeWorld(ctx, req.GetWorldId())
	if err != nil {
		return nil, apierr.ToGRPCError(err)
	}
	if req.GetId() == "" {
		return nil, apierr.ToGRPCError(apierr.InvalidArgument("composition ID is required"))
	}

	output, err := h.service.Get(ctx, &compositionservice.GetInput{
		PlayerID:      playerID,
		WorldID:       h.worldID,
		CompositionID: req.GetId(),
	})
	if err != nil {
		return nil, apierr.ToGRPCError(err)
	}
	if output == nil || output.Composition == nil {
		return nil, apierr.ToGRPCError(apierr.Internal("composition service returned no composition"))
	}
	return &compositionpb.GetCompositionResponse{Composition: compositionToProto(output.Composition)}, nil
}

// ListCompositions returns all immutable composition snapshots in the configured world.
func (h *Handler) ListCompositions(ctx context.Context, req *compositionpb.ListCompositionsRequest) (*compositionpb.ListCompositionsResponse, error) {
	playerID, err := h.authorizeWorld(ctx, req.GetWorldId())
	if err != nil {
		return nil, apierr.ToGRPCError(err)
	}

	output, err := h.service.List(ctx, &compositionservice.ListInput{
		PlayerID: playerID,
		WorldID:  h.worldID,
	})
	if err != nil {
		return nil, apierr.ToGRPCError(err)
	}
	if output == nil {
		return nil, apierr.ToGRPCError(apierr.Internal("composition service returned no list output"))
	}

	response := &compositionpb.ListCompositionsResponse{
		Compositions: make([]*compositionpb.Composition, 0, len(output.Compositions)),
	}
	for _, composition := range output.Compositions {
		response.Compositions = append(response.Compositions, compositionToProto(composition))
	}
	return response, nil
}

func (h *Handler) authorizeWorld(ctx context.Context, requestedWorldID string) (string, error) {
	playerID := auth.GetPlayerID(ctx)
	if playerID == "" {
		return "", apierr.Unauthenticated("player is not authenticated")
	}
	if requestedWorldID == "" {
		return "", apierr.InvalidArgument("world ID is required")
	}
	if requestedWorldID != h.worldID {
		return "", apierr.PermissionDenied("requested world is not available")
	}
	return playerID, nil
}

func compositionToProto(composition *worldcomposition.Data) *compositionpb.Composition {
	if composition == nil {
		return nil
	}
	return &compositionpb.Composition{
		Id:      composition.ID,
		WorldId: composition.WorldID,
		Json:    string(composition.JSON),
	}
}
