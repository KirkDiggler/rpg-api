package main

import (
	"encoding/json"
	"fmt"
	"os"

	"google.golang.org/grpc"

	worldpb "github.com/KirkDiggler/rpg-api-protos/gen/go/api/world/v1alpha1"

	"github.com/KirkDiggler/rpg-api/internal/auth"
	worldhandler "github.com/KirkDiggler/rpg-api/internal/handlers/api/world/v1alpha1"
	worldorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/world"
	worldrepo "github.com/KirkDiggler/rpg-api/internal/repositories/world"
)

const (
	envDevWorldOwner        = "RPG_DEV_WORLD_OWNER"
	envDevWorldOwnerPlayer  = "RPG_DEV_WORLD_OWNER_PLAYER_ID"
	envDevPermissionLevel   = "RPG_DEV_PERMISSION_LEVEL"
	envDevPlayerPermissions = "RPG_DEV_PLAYER_PERMISSIONS"
	envDevPlayerRoles       = "RPG_DEV_PLAYER_ROLES"
)

type developmentAccess struct {
	Default auth.Permissions
	Players map[string]auth.Permissions
	Roles   map[string][]string
}

func configuredDevelopmentAccess(devMode bool) (*developmentAccess, error) {
	out := &developmentAccess{Players: map[string]auth.Permissions{}, Roles: map[string][]string{}}
	if !devMode {
		return out, nil
	}
	var err error
	out.Default, err = permissionLevel(os.Getenv(envDevPermissionLevel))
	if err != nil {
		return nil, err
	}
	levels := map[string]string{}
	if value := os.Getenv(envDevPlayerPermissions); value != "" {
		if err := json.Unmarshal([]byte(value), &levels); err != nil {
			return nil, fmt.Errorf("decode development permissions: %w", err)
		}
	}
	for player, level := range levels {
		if player == "" {
			return nil, fmt.Errorf("development permission player ID cannot be empty")
		}
		permissions, err := permissionLevel(level)
		if err != nil {
			return nil, err
		}
		out.Players[player] = permissions
	}
	if value := os.Getenv(envDevPlayerRoles); value != "" {
		if err := json.Unmarshal([]byte(value), &out.Roles); err != nil {
			return nil, fmt.Errorf("decode development role fixtures: %w", err)
		}
	}
	return out, nil
}

func permissionLevel(level string) (auth.Permissions, error) {
	switch level {
	case "", "none":
		return 0, nil
	case "player":
		return auth.PermissionPlay, nil
	case "builder":
		return auth.PermissionBuild | auth.PermissionPlay, nil
	case "admin":
		return auth.PermissionAdmin | auth.PermissionBuild | auth.PermissionPlay, nil
	default:
		return 0, fmt.Errorf("unknown development permission level %q", level)
	}
}

func configuredDevWorldOwner(devMode bool) bool {
	return devMode && os.Getenv(envDevWorldOwner) == "true"
}

func registerWorldService(registrar grpc.ServiceRegistrar, repository worldrepo.Repository) error {
	orchestrator, err := worldorch.New(&worldorch.Config{Repository: repository})
	if err != nil {
		return fmt.Errorf("create world orchestrator: %w", err)
	}
	handler, err := worldhandler.New(&worldhandler.HandlerConfig{Orchestrator: orchestrator})
	if err != nil {
		return fmt.Errorf("create world handler: %w", err)
	}
	worldpb.RegisterWorldServiceServer(registrar, handler)
	return nil
}
