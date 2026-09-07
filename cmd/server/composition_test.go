package main

import (
	"testing"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"

	compositionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/api/composition/v1alpha1"
)

func TestConfiguredDevWorldID(t *testing.T) {
	t.Setenv(envDevWorldID, "")
	require.Empty(t, configuredDevWorldID(false))
	require.Equal(t, defaultWorldID, configuredDevWorldID(true))

	t.Setenv(envDevWorldID, "configured-world")
	require.Empty(t, configuredDevWorldID(false))
	require.Equal(t, "configured-world", configuredDevWorldID(true))
}

func TestCompositionServiceRegistrationInProductionAndDev(t *testing.T) {
	t.Setenv(envDevWorldID, "production-must-ignore-this-stub")
	redisServer := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: redisServer.Addr()})
	t.Cleanup(func() { require.NoError(t, client.Close()) })

	for _, authMode := range []string{"production", "development"} {
		t.Run(authMode, func(t *testing.T) {
			server := grpc.NewServer()
			registered, err := registerCompositionService(server, &compositionRegistrationConfig{
				AuthoringEnabled: true,
				Redis:            client,
			})
			require.NoError(t, err)
			require.True(t, registered)
			_, present := server.GetServiceInfo()[compositionpb.CompositionService_ServiceDesc.ServiceName]
			require.True(t, present)
		})
	}
	require.Equal(t, compositionpb.CompositionService_ServiceDesc.ServiceName, compositionv1alpha1ServiceName)
}
