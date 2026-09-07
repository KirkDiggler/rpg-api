// Package compositionv1alpha1 implements the world-scoped CompositionService wire boundary.
package compositionv1alpha1

import (
	"context"
	"encoding/json"

	compositionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/api/composition/v1alpha1"
	worldcomposition "github.com/KirkDiggler/rpg-toolkit/world/composition"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	compositionservice "github.com/KirkDiggler/rpg-api/internal/services/composition"
	"github.com/KirkDiggler/rpg-api/internal/worldcontext"
)

// HandlerConfig configures a CompositionService handler.
type HandlerConfig struct {
	Service          compositionservice.Service
	AuthoringEnabled bool
}

// Handler translates CompositionService protobuf messages to service inputs.
type Handler struct {
	compositionpb.UnimplementedCompositionServiceServer

	service          compositionservice.Service
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
	return &Handler{
		service:          cfg.Service,
		authoringEnabled: cfg.AuthoringEnabled,
	}, nil
}

// CreateComposition saves one new immutable composition snapshot.
func (h *Handler) CreateComposition(ctx context.Context, req *compositionpb.CreateCompositionRequest) (*compositionpb.CreateCompositionResponse, error) {
	playerID, worldID, err := h.authorizeWorld(ctx, req.GetWorldId())
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
		WorldID:  worldID,
		JSON:     json.RawMessage(req.GetJson()),
	})
	if err != nil {
		return nil, apierr.ToGRPCError(err)
	}
	if output == nil || validateComposition(output.Composition, worldID, "") != nil {
		return nil, apierr.ToGRPCError(apierr.Internal("composition service returned an invalid created composition"))
	}
	return &compositionpb.CreateCompositionResponse{Composition: compositionToProto(output.Composition)}, nil
}

// GetComposition returns one immutable composition snapshot.
func (h *Handler) GetComposition(ctx context.Context, req *compositionpb.GetCompositionRequest) (*compositionpb.GetCompositionResponse, error) {
	playerID, worldID, err := h.authorizeWorld(ctx, req.GetWorldId())
	if err != nil {
		return nil, apierr.ToGRPCError(err)
	}
	if req.GetId() == "" {
		return nil, apierr.ToGRPCError(apierr.InvalidArgument("composition ID is required"))
	}

	output, err := h.service.Get(ctx, &compositionservice.GetInput{
		PlayerID:      playerID,
		WorldID:       worldID,
		CompositionID: req.GetId(),
	})
	if err != nil {
		return nil, apierr.ToGRPCError(err)
	}
	if output == nil || validateComposition(output.Composition, worldID, req.GetId()) != nil {
		return nil, apierr.ToGRPCError(apierr.Internal("composition service returned an invalid composition"))
	}
	return &compositionpb.GetCompositionResponse{Composition: compositionToProto(output.Composition)}, nil
}

// DeleteComposition permanently deletes one composition snapshot.
func (h *Handler) DeleteComposition(ctx context.Context, req *compositionpb.DeleteCompositionRequest) (*compositionpb.DeleteCompositionResponse, error) {
	playerID, worldID, err := h.authorizeWorld(ctx, req.GetWorldId())
	if err != nil {
		return nil, apierr.ToGRPCError(err)
	}
	if !h.authoringEnabled {
		return nil, apierr.ToGRPCError(apierr.FailedPrecondition("composition authoring is disabled"))
	}
	if req.GetId() == "" {
		return nil, apierr.ToGRPCError(apierr.InvalidArgument("composition ID is required"))
	}

	output, err := h.service.Delete(ctx, &compositionservice.DeleteInput{
		PlayerID:      playerID,
		WorldID:       worldID,
		CompositionID: req.GetId(),
	})
	if err != nil {
		return nil, apierr.ToGRPCError(err)
	}
	if output == nil {
		return nil, apierr.ToGRPCError(apierr.Internal("composition service returned no delete output"))
	}
	return &compositionpb.DeleteCompositionResponse{}, nil
}

// ListCompositions returns all immutable composition snapshots in the trusted world.
func (h *Handler) ListCompositions(ctx context.Context, req *compositionpb.ListCompositionsRequest) (*compositionpb.ListCompositionsResponse, error) {
	playerID, worldID, err := h.authorizeWorld(ctx, req.GetWorldId())
	if err != nil {
		return nil, apierr.ToGRPCError(err)
	}

	output, err := h.service.List(ctx, &compositionservice.ListInput{
		PlayerID: playerID,
		WorldID:  worldID,
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
		if validateComposition(composition, worldID, "") != nil {
			return nil, apierr.ToGRPCError(apierr.Internal("composition service returned an invalid list composition"))
		}
		response.Compositions = append(response.Compositions, compositionToProto(composition))
	}
	return response, nil
}

func (h *Handler) authorizeWorld(ctx context.Context, requestedWorldID string) (string, string, error) {
	playerID := auth.GetPlayerID(ctx)
	if playerID == "" {
		return "", "", apierr.Unauthenticated("player is not authenticated")
	}
	trustedWorld, ok := worldcontext.Get(ctx)
	if !ok || trustedWorld.WorldID == "" {
		return "", "", apierr.FailedPrecondition("trusted world context is required")
	}
	if requestedWorldID == "" {
		return "", "", apierr.InvalidArgument("world ID is required")
	}
	if requestedWorldID != trustedWorld.WorldID {
		return "", "", apierr.PermissionDenied("requested world is not available")
	}
	return playerID, trustedWorld.WorldID, nil
}

func validateComposition(composition *worldcomposition.Data, worldID, expectedID string) error {
	if composition == nil || composition.ID == "" || composition.WorldID != worldID {
		return apierr.Internal("composition does not match trusted world")
	}
	if expectedID != "" && composition.ID != expectedID {
		return apierr.Internal("composition does not match requested ID")
	}
	return nil
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
