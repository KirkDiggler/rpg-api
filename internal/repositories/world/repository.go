// Package world persists server-owned world configuration.
package world

import (
	"context"

	"github.com/KirkDiggler/rpg-api/internal/entities"
)

//go:generate mockgen -destination=mock/mock_repository.go -package=worldmock github.com/KirkDiggler/rpg-api/internal/repositories/world Repository

// Repository stores world policy independently of Discord credentials.
type Repository interface {
	Get(context.Context, *GetInput) (*GetOutput, error)
	SetRoles(context.Context, *SetRolesInput) (*SetRolesOutput, error)
	SetMemberRoles(context.Context, *SetMemberRolesInput) (*SetMemberRolesOutput, error)
}

type GetInput struct{ WorldID string }
type GetOutput struct{ World *entities.World }

type SetRolesInput struct{ World *entities.World }
type SetRolesOutput struct{ World *entities.World }

// SetMemberRolesInput changes member roles while preserving current admin authority.
// ExpectedAdminRoleID is the policy against which the caller was authorized.
// A concurrent admin-role change aborts the update instead of admitting a revoked admin.
type SetMemberRolesInput struct {
	WorldID             string
	ExpectedAdminRoleID string
	BuilderRoleID       string
	PlayerRoleID        string
}
type SetMemberRolesOutput struct{ World *entities.World }
