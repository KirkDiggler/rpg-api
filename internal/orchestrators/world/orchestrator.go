// Package world implements world configuration policy over a World repository.
package world

import (
	"context"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	worldrepo "github.com/KirkDiggler/rpg-api/internal/repositories/world"
)

type Config struct{ Repository worldrepo.Repository }
type Orchestrator struct{ repository worldrepo.Repository }

func New(cfg *Config) (*Orchestrator, error) {
	if cfg == nil || cfg.Repository == nil {
		return nil, apierr.InvalidArgument("world repository is required")
	}
	return &Orchestrator{repository: cfg.Repository}, nil
}

func (o *Orchestrator) Get(ctx context.Context, input *GetInput) (*GetOutput, error) {
	if input == nil {
		return nil, apierr.InvalidArgument("get world input is required")
	}
	world, err := o.authorizedWorld(ctx, input.Caller)
	if err != nil {
		return nil, err
	}
	return &GetOutput{World: world}, nil
}

func (o *Orchestrator) SetRoles(ctx context.Context, input *SetRolesInput) (*SetRolesOutput, error) {
	if input == nil {
		return nil, apierr.InvalidArgument("set world roles input is required")
	}
	if err := validateCaller(input.Caller); err != nil {
		return nil, err
	}
	if !input.Caller.Owner {
		return nil, apierr.PermissionDenied("only the Discord server owner can configure admin authority")
	}
	world := &entities.World{
		WorldID: input.Caller.WorldID, AdminRoleID: input.AdminRoleID,
		BuilderRoleID: input.BuilderRoleID, PlayerRoleID: input.PlayerRoleID,
	}
	if _, err := auth.EvaluatePermissions(&auth.EvaluatePermissionsInput{Config: *world, WorldID: world.WorldID}); err != nil {
		return nil, apierr.InvalidArgument(err.Error())
	}
	out, err := o.repository.SetRoles(ctx, &worldrepo.SetRolesInput{World: world})
	if err != nil {
		return nil, err
	}
	if out == nil || out.World == nil {
		return nil, apierr.Internal("world repository returned no configuration")
	}
	return &SetRolesOutput{World: out.World}, nil
}

func (o *Orchestrator) SetMemberRoles(ctx context.Context, input *SetMemberRolesInput) (*SetMemberRolesOutput, error) {
	if input == nil {
		return nil, apierr.InvalidArgument("set member roles input is required")
	}
	world, err := o.authorizedWorld(ctx, input.Caller)
	if err != nil {
		return nil, err
	}
	out, err := o.repository.SetMemberRoles(ctx, &worldrepo.SetMemberRolesInput{
		WorldID: world.WorldID, ExpectedAdminRoleID: world.AdminRoleID,
		BuilderRoleID: input.BuilderRoleID, PlayerRoleID: input.PlayerRoleID,
	})
	if err != nil {
		return nil, err
	}
	if out == nil || out.World == nil {
		return nil, apierr.Internal("world repository returned no configuration")
	}
	return &SetMemberRolesOutput{World: out.World}, nil
}

func (o *Orchestrator) authorizedWorld(ctx context.Context, caller Caller) (*entities.World, error) {
	if err := validateCaller(caller); err != nil {
		return nil, err
	}
	out, err := o.repository.Get(ctx, &worldrepo.GetInput{WorldID: caller.WorldID})
	if err != nil {
		return nil, err
	}
	if out == nil || out.World == nil || out.World.WorldID != caller.WorldID {
		return nil, apierr.Internal("world repository returned an invalid configuration")
	}
	permissions, err := auth.EvaluatePermissions(&auth.EvaluatePermissionsInput{
		Config: *out.World, WorldID: caller.WorldID, AssignedRoleIDs: caller.AssignedRoleIDs,
	})
	if err != nil {
		return nil, apierr.Wrap(err, "evaluate world access")
	}
	if !caller.Owner && !permissions.Permissions.Allows(auth.PermissionAdmin) {
		return nil, apierr.PermissionDenied("world admin access is required")
	}
	return out.World, nil
}

func validateCaller(caller Caller) error {
	if caller.PlayerID == "" {
		return apierr.Unauthenticated("verified player is required")
	}
	if caller.WorldID == "" {
		return apierr.FailedPrecondition("verified world is required")
	}
	return nil
}
