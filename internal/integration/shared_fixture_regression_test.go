// Copyright (C) 2024 Kirk Diggler
// SPDX-License-Identifier: GPL-3.0-or-later

package integration_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/KirkDiggler/rpg-api/internal/integration/harness"
)

// TestSharedRedisFixture_PerTestFreshnessAndStateIsolation is rpg-api#699's
// closing regression coverage, added in response to an independent
// review gap: the original PR asserted container-ownership lifecycle
// (owned Close terminates once, borrowed Close never terminates, the
// shared fixture's Terminate/Lease) with docker-free unit tests, but had
// no durable test proving the two behaviors every converted suite in this
// package actually depends on at runtime:
//
//  1. Two TestServers built via NewWithRedis against the SAME shared
//     Redis container are wired to distinct application-level resources
//     (gRPC server, in-memory encounter/lobby repos, brokers) — not just
//     distinct Go struct pointers wrapping shared internals.
//  2. The shared container's Redis STATE is actually isolated between
//     tests by the FlushRedis call every SetupTest in this package makes
//     — using a fixed key, so this cannot pass merely because two tests
//     happened to use different IDs.
//
// This lives in the primary internal/integration package (not a new
// package) so a targeted `-run` of just this test starts only this
// package's one shared container, not an additional one.
func TestSharedRedisFixture_PerTestFreshnessAndStateIsolation(t *testing.T) {
	t.Run("per_test_application_freshness", func(t *testing.T) {
		release := sharedRedis.Lease()
		t.Cleanup(release)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		srv1, err := harness.NewWithRedis(ctx, nil, sharedRedis.Addr)
		require.NoError(t, err, "failed to create first test server")
		t.Cleanup(srv1.Close)

		srv2, err := harness.NewWithRedis(ctx, nil, sharedRedis.Addr)
		require.NoError(t, err, "failed to create second test server")
		t.Cleanup(srv2.Close)

		// This is a characterization test: NewWithRedis's wireServices
		// path already constructs a fresh gRPC server, bufconn listener,
		// and in-memory repo/broker per call (see harness.go) — it has
		// always behaved this way since #699 introduced NewWithRedis.
		// This pins that behavior down as an explicit, durable assertion
		// rather than leaving it as an implicit, undocumented property
		// every suite's SetupTest happens to rely on.
		assertDistinctInstances(t, "gRPC server", srv1.GRPCServer(), srv2.GRPCServer())
		assertDistinctInstances(t, "v2 encounter broker", srv1.BrokerV2, srv2.BrokerV2)
		assertDistinctInstances(t, "lobby broker", srv1.LobbyBroker, srv2.LobbyBroker)
		assertDistinctInstances(t, "v2 encounter repo", srv1.EncRepoV2, srv2.EncRepoV2)
		assertDistinctInstances(t, "lobby repo", srv1.LobbyRepo, srv2.LobbyRepo)
		assertDistinctInstances(t, "character repo", srv1.CharacterRepo, srv2.CharacterRepo)
	})

	t.Run("shared_redis_state_isolation_via_flush", func(t *testing.T) {
		const fixedKey = "rpg-api-699-regression-fixed-key"

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		t.Cleanup(cancel)

		// Cycle A: models one test's SetupTest/TearDownTest, ending with
		// a fixed key written directly into the shared Redis instance —
		// standing in for whatever state a real test leaves behind.
		releaseA := sharedRedis.Lease()
		srvA, err := harness.NewWithRedis(ctx, nil, sharedRedis.Addr)
		require.NoError(t, err, "failed to create first test server")
		require.NoError(t, srvA.FlushRedis(ctx), "failed to flush shared redis before cycle A")
		require.NoError(t, srvA.RedisClient().Set(ctx, fixedKey, "leaked-if-isolation-is-broken", 0).Err(),
			"failed to seed fixed key")
		srvA.Close()
		releaseA()

		// Cycle B: the next test in this process. Its SetupTest does
		// exactly this — NewWithRedis against the same shared address,
		// then FlushRedis — before any test body runs.
		releaseB := sharedRedis.Lease()
		t.Cleanup(releaseB)
		srvB, err := harness.NewWithRedis(ctx, nil, sharedRedis.Addr)
		require.NoError(t, err, "failed to create second test server")
		t.Cleanup(srvB.Close)
		require.NoError(t, srvB.FlushRedis(ctx), "failed to flush shared redis before cycle B")

		exists, err := srvB.RedisClient().Exists(ctx, fixedKey).Result()
		require.NoError(t, err, "failed to check fixed key")
		assert.Zero(t, exists,
			"fixed key %q written by a prior test must not survive FlushRedis into the next test", fixedKey)
	})
}

// assertDistinctInstances fails the test if a and b are the same pointer.
// a and b are expected to be interface or pointer values wrapping a
// pointer-typed concrete instance (true for every TestServer field this
// test checks); %p reports that pointer's address regardless of the
// static type, so this works uniformly across gRPC servers, brokers, and
// repository interfaces without depending on their concrete types.
func assertDistinctInstances(t *testing.T, label string, a, b any) {
	t.Helper()
	pa := fmt.Sprintf("%p", a)
	pb := fmt.Sprintf("%p", b)
	assert.NotEqual(t, pa, pb, "%s must be a distinct instance per TestServer, both were %s", label, pa)
}
