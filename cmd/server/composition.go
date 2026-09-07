package main

import (
	"fmt"
	"log"
	"os"

	"google.golang.org/grpc"

	compositionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/api/composition/v1alpha1"

	compositionhandler "github.com/KirkDiggler/rpg-api/internal/handlers/api/composition/v1alpha1"
	compositionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/composition"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	"github.com/KirkDiggler/rpg-api/internal/redis"
	compositionrepo "github.com/KirkDiggler/rpg-api/internal/repositories/composition"
)

const (
	envAuthDevMode                 = "AUTH_DEV_MODE"
	envDevWorldID                  = "RPG_DEV_WORLD_ID"
	defaultWorldID                 = "test-world"
	compositionv1alpha1ServiceName = "api.composition.v1alpha1.CompositionService"
)

type compositionRegistrationConfig struct {
	DevMode          bool
	AuthoringEnabled bool
	Redis            redis.Client
}

// registerCompositionService installs the local world library only in auth dev mode.
func registerCompositionService(registrar grpc.ServiceRegistrar, cfg *compositionRegistrationConfig) (bool, error) {
	if cfg == nil {
		return false, fmt.Errorf("composition registration config is required")
	}
	if !cfg.DevMode {
		return false, nil
	}

	worldID := configuredDevWorldID(cfg.DevMode)
	repository, err := compositionrepo.NewRedis(&compositionrepo.RedisConfig{Client: cfg.Redis})
	if err != nil {
		return false, fmt.Errorf("create composition repository: %w", err)
	}
	service, err := compositionorch.New(&compositionorch.Config{
		Repository:  repository,
		IDGenerator: idgen.NewUUID("composition"),
	})
	if err != nil {
		return false, fmt.Errorf("create composition service: %w", err)
	}
	handler, err := compositionhandler.New(&compositionhandler.HandlerConfig{
		Service:          service,
		WorldID:          worldID,
		AuthoringEnabled: cfg.AuthoringEnabled,
	})
	if err != nil {
		return false, fmt.Errorf("create composition handler: %w", err)
	}

	compositionpb.RegisterCompositionServiceServer(registrar, handler)
	// The world ID is operator-supplied and logged only on an explicitly local-dev service.
	log.Printf( //nolint:gosec
		"CompositionService registered for local world %q (%s=%t)",
		worldID,
		envAuthoringEnabled,
		cfg.AuthoringEnabled,
	)
	return true, nil
}

func configuredDevWorldID(devMode bool) string {
	if !devMode {
		return ""
	}
	if worldID := os.Getenv(envDevWorldID); worldID != "" {
		return worldID
	}
	return defaultWorldID
}
