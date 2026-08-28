// Copyright (C) 2024 Kirk Diggler
// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	grpc_health_v1 "google.golang.org/grpc/health/grpc_health_v1"
)

func TestParseConfig_DefaultsToEnvoyAddress(t *testing.T) {
	config, err := parseConfig(nil)
	require.NoError(t, err)
	require.Equal(t, defaultAddress, config.address)
	require.False(t, config.health)
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
