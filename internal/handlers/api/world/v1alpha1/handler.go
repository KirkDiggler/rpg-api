// Package worldv1alpha1 translates the WorldService contract at the API boundary.
package worldv1alpha1

import (
	"context"

	worldpb "github.com/KirkDiggler/rpg-api-protos/gen/go/api/world/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	worldorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/world"
	"github.com/KirkDiggler/rpg-api/internal/worldcontext"
)

//go:generate mockgen -destination=mock/mock_orchestrator.go -package=worldv1alpha1mock github.com/KirkDiggler/rpg-api/internal/handlers/api/world/v1alpha1 Orchestrator

// Orchestrator is the handler's narrow consumer contract. Operation types belong
// to the business package; entities.World is shared with the repository.
type Orchestrator interface {
	Get(context.Context, *worldorch.GetInput) (*worldorch.GetOutput, error)
	SetRoles(context.Context, *worldorch.SetRolesInput) (*worldorch.SetRolesOutput, error)
	SetMemberRoles(context.Context, *worldorch.SetMemberRolesInput) (*worldorch.SetMemberRolesOutput, error)
}

type Handler struct {
	worldpb.UnimplementedWorldServiceServer
	orchestrator Orchestrator
}
type HandlerConfig struct{ Orchestrator Orchestrator }

func New(cfg *HandlerConfig) (*Handler, error) {
	if cfg == nil || cfg.Orchestrator == nil {
		return nil, apierr.InvalidArgument("world orchestrator is required")
	}
	return &Handler{orchestrator: cfg.Orchestrator}, nil
}

func (h *Handler) GetWorld(ctx context.Context, req *worldpb.GetWorldRequest) (*worldpb.GetWorldResponse, error) {
	caller, err := verifiedCaller(ctx, req.GetWorldId())
	if err != nil {
		return nil, apierr.ToGRPCError(err)
	}
	out, err := h.orchestrator.Get(ctx, &worldorch.GetInput{Caller: caller})
	if err != nil {
		return nil, apierr.ToGRPCError(err)
	}
	if out == nil || out.World == nil {
		return nil, apierr.ToGRPCError(apierr.Internal("world service returned no configuration"))
	}
	return &worldpb.GetWorldResponse{World: toProto(out.World)}, nil
}

func (h *Handler) SetWorldRoles(ctx context.Context, req *worldpb.SetWorldRolesRequest) (*worldpb.SetWorldRolesResponse, error) {
	caller, err := verifiedCaller(ctx, req.GetWorldId())
	if err != nil {
		return nil, apierr.ToGRPCError(err)
	}
	roles := req.GetRoles()
	out, err := h.orchestrator.SetRoles(ctx, &worldorch.SetRolesInput{
		Caller: caller, AdminRoleID: roles.GetAdminRoleId(),
		BuilderRoleID: roles.GetBuilderRoleId(), PlayerRoleID: roles.GetPlayerRoleId(),
	})
	if err != nil {
		return nil, apierr.ToGRPCError(err)
	}
	if out == nil || out.World == nil {
		return nil, apierr.ToGRPCError(apierr.Internal("world service returned no configuration"))
	}
	return &worldpb.SetWorldRolesResponse{World: toProto(out.World)}, nil
}

func (h *Handler) SetWorldMemberRoles(ctx context.Context, req *worldpb.SetWorldMemberRolesRequest) (*worldpb.SetWorldMemberRolesResponse, error) {
	caller, err := verifiedCaller(ctx, req.GetWorldId())
	if err != nil {
		return nil, apierr.ToGRPCError(err)
	}
	out, err := h.orchestrator.SetMemberRoles(ctx, &worldorch.SetMemberRolesInput{
		Caller: caller, BuilderRoleID: req.GetBuilderRoleId(), PlayerRoleID: req.GetPlayerRoleId(),
	})
	if err != nil {
		return nil, apierr.ToGRPCError(err)
	}
	if out == nil || out.World == nil {
		return nil, apierr.ToGRPCError(apierr.Internal("world service returned no configuration"))
	}
	return &worldpb.SetWorldMemberRolesResponse{World: toProto(out.World)}, nil
}

func verifiedCaller(ctx context.Context, requestedWorldID string) (worldorch.Caller, error) {
	playerID := auth.GetPlayerID(ctx)
	if playerID == "" {
		return worldorch.Caller{}, apierr.Unauthenticated("verified player is required")
	}
	world, ok := worldcontext.Get(ctx)
	if !ok || world.WorldID == "" {
		return worldorch.Caller{}, apierr.FailedPrecondition("verified world is required")
	}
	if requestedWorldID == "" {
		return worldorch.Caller{}, apierr.InvalidArgument("world ID is required")
	}
	if requestedWorldID != world.WorldID {
		return worldorch.Caller{}, apierr.PermissionDenied("world ID does not match verified guild")
	}
	return worldorch.Caller{PlayerID: playerID, WorldID: world.WorldID, Owner: world.Owner, AssignedRoleIDs: world.AssignedRoleIDs}, nil
}

func toProto(world *entities.World) *worldpb.World {
	return &worldpb.World{WorldId: world.WorldID, Roles: &worldpb.WorldRoles{
		AdminRoleId: world.AdminRoleID, BuilderRoleId: world.BuilderRoleID, PlayerRoleId: world.PlayerRoleID,
	}}
}
