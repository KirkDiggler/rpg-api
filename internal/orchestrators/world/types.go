package world

import "github.com/KirkDiggler/rpg-api/internal/entities"

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
