package main

import (
	"fmt"
	"log"
	"os"

	"google.golang.org/grpc"

	compositionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/api/composition/v1alpha1"

	"github.com/KirkDiggler/rpg-api/internal/auth"
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
	AuthoringEnabled bool
	Redis            redis.Client
}

// registerCompositionService installs the world-scoped composition library.
func registerCompositionService(registrar grpc.ServiceRegistrar, cfg *compositionRegistrationConfig) (bool, error) {
	if cfg == nil {
		return false, fmt.Errorf("composition registration config is required")
	}
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
		AuthoringEnabled: cfg.AuthoringEnabled,
	})
	if err != nil {
		return false, fmt.Errorf("create composition handler: %w", err)
	}

	compositionpb.RegisterCompositionServiceServer(registrar, handler)
	log.Printf("CompositionService registered (%s=%t)", envAuthoringEnabled, cfg.AuthoringEnabled)
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

// configuredDevMode is THE ONE READ of AUTH_DEV_MODE: development mode is on
// exactly when the variable is the string "true". Unset, empty, "false", "1",
// "yes", "TRUE" and " true" are all off. Every development consumer reads the
// value this returns, through newAuthConfig: the Dev authentication scheme,
// development roles, the development world, and the record service, which in
// development mode serves any run's whole record (rpg-project#1001).
func configuredDevMode() bool {
	return os.Getenv(envAuthDevMode) == envEnabledValue
}

// newAuthConfig is the auth interceptor's configuration, its DevMode the one
// decision configuredDevMode makes.
func newAuthConfig(worldScopedStreams bool) *auth.InterceptorConfig {
	return &auth.InterceptorConfig{DevMode: configuredDevMode(), WorldScopedStreams: worldScopedStreams}
}
