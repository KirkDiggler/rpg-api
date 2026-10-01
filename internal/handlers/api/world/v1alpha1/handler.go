// Package worldv1alpha1 translates the WorldService contract at the API boundary.
package worldv1alpha1

import (
	"context"

	worldpb "github.com/KirkDiggler/rpg-api-protos/gen/go/api/world/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	worldservice "github.com/KirkDiggler/rpg-api/internal/services/world"
	"github.com/KirkDiggler/rpg-api/internal/worldcontext"
)

type Handler struct {
	worldpb.UnimplementedWorldServiceServer
	service worldservice.Service
}
type HandlerConfig struct{ Service worldservice.Service }

func New(cfg *HandlerConfig) (*Handler, error) {
	if cfg == nil || cfg.Service == nil {
		return nil, apierr.InvalidArgument("world service is required")
	}
	return &Handler{service: cfg.Service}, nil
}

func (h *Handler) GetWorld(ctx context.Context, req *worldpb.GetWorldRequest) (*worldpb.GetWorldResponse, error) {
	caller, err := verifiedCaller(ctx, req.GetWorldId())
	if err != nil {
		return nil, apierr.ToGRPCError(err)
	}
	out, err := h.service.Get(ctx, &worldservice.GetInput{Caller: caller})
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
	out, err := h.service.SetRoles(ctx, &worldservice.SetRolesInput{
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
	out, err := h.service.SetMemberRoles(ctx, &worldservice.SetMemberRolesInput{
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

func verifiedCaller(ctx context.Context, requestedWorldID string) (worldservice.Caller, error) {
	playerID := auth.GetPlayerID(ctx)
	if playerID == "" {
		return worldservice.Caller{}, apierr.Unauthenticated("verified player is required")
	}
	world, ok := worldcontext.Get(ctx)
	if !ok || world.WorldID == "" {
		return worldservice.Caller{}, apierr.FailedPrecondition("verified world is required")
	}
	if requestedWorldID == "" {
		return worldservice.Caller{}, apierr.InvalidArgument("world ID is required")
	}
	if requestedWorldID != world.WorldID {
		return worldservice.Caller{}, apierr.PermissionDenied("world ID does not match verified guild")
	}
	return worldservice.Caller{PlayerID: playerID, WorldID: world.WorldID, Owner: world.Owner, AssignedRoleIDs: world.AssignedRoleIDs}, nil
}

func toProto(world *entities.World) *worldpb.World {
	return &worldpb.World{WorldId: world.WorldID, Roles: &worldpb.WorldRoles{
		AdminRoleId: world.AdminRoleID, BuilderRoleId: world.BuilderRoleID, PlayerRoleId: world.PlayerRoleID,
	}}
}
