package main

import (
	"fmt"
	"os"

	"google.golang.org/grpc"

	"github.com/KirkDiggler/rpg-api/internal/auth"
)

const (
	envWorldAccessEnforcement = "RPG_WORLD_ACCESS_ENFORCEMENT"
	envEnabledValue           = "true"
)

// configuredWorldAccessEnforcement separates publishing role-access support from
// enabling it. Unset preserves the existing authenticated gameplay contract.
// Invalid settings fail startup rather than silently selecting an access policy.
func configuredWorldAccessEnforcement() (bool, error) {
	switch os.Getenv(envWorldAccessEnforcement) {
	case "", "false":
		return false, nil
	case envEnabledValue:
		return true, nil
	default:
		return false, fmt.Errorf("%s must be true or false", envWorldAccessEnforcement)
	}
}

type gameplayAccessInput struct {
	Enforce bool
	Roles   *auth.RoleAccessConfig
}

type gameplayAccessOutput struct {
	Unary  grpc.UnaryServerInterceptor
	Stream grpc.StreamServerInterceptor
}

// newGameplayAccess keeps the pre-role composition world boundary in compatibility
// mode. Authentication and owner-only world-management authorization remain
// separate interceptors in both modes; this switch never admits anonymous users.
func newGameplayAccess(in *gameplayAccessInput) (*gameplayAccessOutput, error) {
	if in == nil || in.Roles == nil || in.Roles.Resolver == nil {
		return nil, fmt.Errorf("gameplay access requires a world resolver")
	}
	if !in.Enforce {
		return &gameplayAccessOutput{
			Unary: auth.UnaryWorldContextInterceptor(in.Roles.Resolver),
			Stream: func(srv interface{}, stream grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
				return handler(srv, stream)
			},
		}, nil
	}
	roles, err := auth.NewRoleAccess(in.Roles)
	if err != nil {
		return nil, fmt.Errorf("role access: %w", err)
	}
	return &gameplayAccessOutput{
		Unary: roles.UnaryInterceptor(), Stream: roles.StreamInterceptor(),
	}, nil
}
