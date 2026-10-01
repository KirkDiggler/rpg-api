// Package world defines game-facing world configuration operations.
package world

import (
	"context"

	"github.com/KirkDiggler/rpg-api/internal/entities"
)

//go:generate mockgen -destination=mock/mock_service.go -package=worldmock github.com/KirkDiggler/rpg-api/internal/services/world Service

type Service interface {
	Get(context.Context, *GetInput) (*GetOutput, error)
	SetRoles(context.Context, *SetRolesInput) (*SetRolesOutput, error)
	SetMemberRoles(context.Context, *SetMemberRolesInput) (*SetMemberRolesOutput, error)
}

// Caller contains API-verified identity only; handlers never construct it from body claims.
type Caller struct {
	PlayerID        string
	WorldID         string
	Owner           bool
	AssignedRoleIDs []string
}

type GetInput struct{ Caller Caller }
type GetOutput struct{ World *entities.World }
type SetRolesInput struct {
	Caller        Caller
	AdminRoleID   string
	BuilderRoleID string
	PlayerRoleID  string
}
type SetRolesOutput struct{ World *entities.World }
type SetMemberRolesInput struct {
	Caller        Caller
	BuilderRoleID string
	PlayerRoleID  string
}
type SetMemberRolesOutput struct{ World *entities.World }
