package main

import (
	"fmt"
	"os"

	"google.golang.org/grpc"

	worldpb "github.com/KirkDiggler/rpg-api-protos/gen/go/api/world/v1alpha1"

	worldhandler "github.com/KirkDiggler/rpg-api/internal/handlers/api/world/v1alpha1"
	worldorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/world"
	redisclient "github.com/KirkDiggler/rpg-api/internal/redis"
	worldrepo "github.com/KirkDiggler/rpg-api/internal/repositories/world"
)

const envDevWorldOwner = "RPG_DEV_WORLD_OWNER"

func configuredDevWorldOwner(devMode bool) bool {
	return devMode && os.Getenv(envDevWorldOwner) == "true"
}

func registerWorldService(registrar grpc.ServiceRegistrar, client redisclient.Client) error {
	repository, err := worldrepo.NewRedis(&worldrepo.RedisConfig{Client: client})
	if err != nil {
		return fmt.Errorf("create world repository: %w", err)
	}
	service, err := worldorch.New(&worldorch.Config{Repository: repository})
	if err != nil {
		return fmt.Errorf("create world service: %w", err)
	}
	handler, err := worldhandler.New(&worldhandler.HandlerConfig{Service: service})
	if err != nil {
		return fmt.Errorf("create world handler: %w", err)
	}
	worldpb.RegisterWorldServiceServer(registrar, handler)
	return nil
}
