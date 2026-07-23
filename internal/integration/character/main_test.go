// Copyright (C) 2024 Kirk Diggler
// SPDX-License-Identifier: GPL-3.0-or-later

package character_integration

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
// docs/how-to/run-integration-tests.md). internal/integration/character is
// its own Go test binary/process, separate from internal/integration and
// internal/integration/harness, so it owns its own single container — it
// cannot literally share the container object across process boundaries,
// only the *pattern* (one container per process, started once, terminated
// once) is shared with those other packages.
//
// Started once here in TestMain before any test runs, terminated exactly
// once after m.Run() returns. Every individual test still gets its own
// fresh TestServer (gRPC server, bufconn listener, repos, brokers, proto
// clients) via harness.NewWithRedis(ctx, cfg, sharedRedis.Addr) in
// CharacterCreationSuite.SetupTest — only the Redis container/connection
// endpoint is shared, and SetupTest flushes it before each test runs.
var sharedRedis *harness.RedisContainer

func TestMain(m *testing.M) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	rc, err := harness.StartRedis(ctx)
	cancel()
	if err != nil {
		log.Fatalf("character_integration TestMain: failed to start shared redis container: %v", err)
	}
	sharedRedis = rc
	log.Printf("character_integration TestMain: shared redis container STARTED (addr=%s)", rc.Addr)

	code := m.Run()

	termCtx, termCancel := context.WithTimeout(context.Background(), 30*time.Second)
	if err := rc.Terminate(termCtx); err != nil {
		log.Printf("character_integration TestMain: failed to terminate shared redis container: %v", err)
	} else {
		log.Printf("character_integration TestMain: shared redis container TERMINATED (addr=%s)", rc.Addr)
	}
	termCancel()

	os.Exit(code)
}
