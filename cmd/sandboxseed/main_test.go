// Copyright (C) 2024 Kirk Diggler
// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"net"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	grpc_health_v1 "google.golang.org/grpc/health/grpc_health_v1"

	"github.com/stretchr/testify/require"

	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-api/internal/sandboxseed"
)

func TestParseConfig_DefaultsToEnvoyAddressAndDefaultFixture(t *testing.T) {
	config, err := parseConfig(nil)
	require.NoError(t, err)
	require.Equal(t, defaultAddress, config.address)
	require.Equal(t, fixtureDefault, config.fixture)
	require.Equal(t, defaultRedisAddress, config.redisAddress)
	require.False(t, config.health)
}

func TestParseConfig_RejectsUnknownFixture(t *testing.T) {
	_, err := parseConfig([]string{"--fixture", "wave-2-monk"})
	require.ErrorContains(t, err, "fixture must be default or weapon-gallery")
}

func TestParseConfig_GalleryRequiresRedisAddressUnlessHealthOnly(t *testing.T) {
	_, err := parseConfig([]string{"--fixture", fixtureWeaponGallery, "--redis-address", ""})
	require.ErrorContains(t, err, "redis address is required")

	config, err := parseConfig([]string{"--health", "--fixture", fixtureWeaponGallery, "--redis-address", ""})
	require.NoError(t, err)
	require.True(t, config.health)
}

func TestRunWithDeps_GalleryWiresRedisBackedStoreWithoutNetworkInUnitTest(t *testing.T) {
	var stdout bytes.Buffer
	var connectedAddress string
	var openedRedisAddress string
	fakeClient := &commandFakeCharacterClient{}
	fakeStore := &commandFakeGalleryStore{}
	deps := commandDeps{
		connect: func(address string) (*grpc.ClientConn, error) {
			connectedAddress = address
			return nil, nil
		},
		characterClient: func(conn *grpc.ClientConn) sandboxseed.CharacterRPC {
			require.Nil(t, conn)
			return fakeClient
		},
		openGalleryStore: func(_ context.Context, redisAddress string) (galleryStoreHandle, error) {
			openedRedisAddress = redisAddress
			return fakeStore, nil
		},
		seedGallery: func(_ context.Context, input *sandboxseed.SeedWeaponGalleryInput) (*sandboxseed.SeedWeaponGalleryOutput, error) {
			require.Same(t, fakeClient, input.Client)
			require.Same(t, fakeStore, input.Store)
			return &sandboxseed.SeedWeaponGalleryOutput{CharacterID: "stable-id", WeaponCount: 30}, nil
		},
		stdout: &stdout,
	}

	err := runWithDeps([]string{"--fixture", fixtureWeaponGallery, "--address", "bufnet", "--redis-address", "redis:6380"}, deps)

	require.NoError(t, err)
	require.Equal(t, "bufnet", connectedAddress)
	require.Equal(t, "redis:6380", openedRedisAddress)
	require.True(t, fakeStore.closed)
	require.Contains(t, stdout.String(), "character_id=stable-id")
	require.Contains(t, stdout.String(), "weapon_count=30")
}

func TestRunWithDeps_HealthIgnoresGalleryFixtureSeeding(t *testing.T) {
	var healthCalled bool
	deps := commandDeps{
		connect: func(_ string) (*grpc.ClientConn, error) { return nil, nil },
		checkHealth: func(context.Context, *grpc.ClientConn) error {
			healthCalled = true
			return nil
		},
		openGalleryStore: func(context.Context, string) (galleryStoreHandle, error) {
			t.Fatal("health must not open Redis")
			return nil, nil
		},
		seedGallery: func(context.Context, *sandboxseed.SeedWeaponGalleryInput) (*sandboxseed.SeedWeaponGalleryOutput, error) {
			t.Fatal("health must not seed gallery")
			return nil, nil
		},
	}

	err := runWithDeps([]string{"--health", "--fixture", fixtureWeaponGallery, "--redis-address", ""}, deps)

	require.NoError(t, err)
	require.True(t, healthCalled)
}

func TestRun_HealthReturnsSuccessOnlyForServing(t *testing.T) {
	listener, server := healthServer(t, grpc_health_v1.HealthCheckResponse_SERVING)

	err := run([]string{"--address", listener.Addr().String(), "--health"})
	require.NoError(t, err)

	server.Stop()
}

func TestRun_HealthRejectsNonServingResponse(t *testing.T) {
	listener, _ := healthServer(t, grpc_health_v1.HealthCheckResponse_NOT_SERVING)

	err := run([]string{"--address", listener.Addr().String(), "--health"})
	require.ErrorContains(t, err, "not serving")
}

type commandFakeGalleryStore struct{ closed bool }

func (s *commandFakeGalleryStore) Get(context.Context, characterrepo.GetInput) (*characterrepo.GetOutput, error) {
	return &characterrepo.GetOutput{Character: &entities.Character{}}, nil
}

func (s *commandFakeGalleryStore) Update(context.Context, characterrepo.UpdateInput) (*characterrepo.UpdateOutput, error) {
	return &characterrepo.UpdateOutput{Character: &entities.Character{}}, nil
}

func (s *commandFakeGalleryStore) Close() error {
	s.closed = true
	return nil
}

type commandFakeCharacterClient struct{ sandboxseed.CharacterRPC }

func healthServer(t *testing.T, servingStatus grpc_health_v1.HealthCheckResponse_ServingStatus) (net.Listener, *grpc.Server) {
	t.Helper()

	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	require.NoError(t, err)

	server := grpc.NewServer()
	healthService := health.NewServer()
	healthService.SetServingStatus("", servingStatus)
	grpc_health_v1.RegisterHealthServer(server, healthService)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})

	return listener, server
}
