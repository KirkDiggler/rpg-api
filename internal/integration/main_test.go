// Copyright (C) 2024 Kirk Diggler
// SPDX-License-Identifier: GPL-3.0-or-later

package integration_test

import (
	"context"
	"log"
	"os"
	"testing"
	"time"

	"github.com/KirkDiggler/rpg-api/internal/integration/harness"
)

// sharedRedis is the single Redis testcontainer shared by every test in
// this package's process (rpg-api#699 — see
// docs/how-to/run-integration-tests.md). It is started once here in
// TestMain before any test runs, and terminated exactly once after
// m.Run() returns. Every individual test still gets its own fresh
// TestServer (gRPC server, bufconn listener, repos, brokers, proto
// clients) via harness.NewWithRedis(ctx, cfg, sharedRedis.Addr) in that
// suite's SetupTest — only the Redis container/connection endpoint is
// shared, and each SetupTest flushes it before running.
//
// The suites in this package (DungeonCryptSuite, EncounterV2Integration
// Suite, LobbyV1alpha1IntegrationSuite, LobbyStartThenMoveSuite) run
// serially today (no t.Parallel()); sharedRedis.Lease() is taken in each
// SetupTest and released in TearDownTest as a belt-and-suspenders guard
// so an accidental future t.Parallel() cannot race FlushRedis against
// this shared instance.
var sharedRedis *harness.RedisContainer

// TestMain owns the one Redis container this whole test binary uses.
// `internal/integration/character` and `internal/integration/harness` are
// separate packages/processes with their own TestMain and their own
// shared container each — see rpg-api#699's PR description for the exact
// per-process container count this achieves.
func TestMain(m *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	rc, err := harness.StartRedis(ctx)
	if err != nil {
		log.Fatalf("integration_test TestMain: failed to start shared redis container: %v", err)
	}
	sharedRedis = rc
	log.Printf("integration_test TestMain: shared redis container STARTED (addr=%s)", rc.Addr)

	code := m.Run()

	termCtx, termCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer termCancel()
	if err := rc.Terminate(termCtx); err != nil {
		log.Printf("integration_test TestMain: failed to terminate shared redis container: %v", err)
	} else {
		log.Printf("integration_test TestMain: shared redis container TERMINATED (addr=%s)", rc.Addr)
	}

	os.Exit(code)
}
