// Copyright (C) 2024 Kirk Diggler
// SPDX-License-Identifier: GPL-3.0-or-later

package harness

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"

	"github.com/testcontainers/testcontainers-go"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

// RedisContainer is a shared Redis testcontainer fixture, owned at the
// package/process level (typically by a TestMain) rather than by any
// single TestServer. Start one with StartRedis, hand its Addr to
// NewWithRedis for every test in the process, and call Terminate exactly
// once when the process is done running tests.
//
// A RedisContainer is not itself a TestServer: it owns only the container
// and connection address. Each test still builds its own fresh TestServer
// via NewWithRedis(ctx, cfg, rc.Addr) — fresh gRPC server, repos, brokers,
// and clients — and is responsible for calling FlushRedis against it
// before relying on a clean slate.
type RedisContainer struct {
	// Addr is the host:port address of the running Redis container,
	// suitable for NewWithRedis.
	Addr string

	container testcontainers.Container

	leaseMu sync.Mutex

	termOnce sync.Once
	termErr  error
}

// StartRedis starts a single redis:7-alpine testcontainer and returns a
// handle to it. The caller owns the returned *RedisContainer and is
// responsible for cleaning it up — typically a single call to Terminate
// (from TestMain, after m.Run() returns) — though Terminate is idempotent,
// so an explicit call plus a deferred one is also safe.
func StartRedis(ctx context.Context) (*RedisContainer, error) {
	redisC, err := tcredis.Run(ctx, "redis:7-alpine")
	if err != nil {
		return nil, fmt.Errorf("failed to start redis container: %w", err)
	}

	addr, err := redisC.ConnectionString(ctx)
	if err != nil {
		_ = redisC.Terminate(ctx)
		return nil, fmt.Errorf("failed to get redis connection string: %w", err)
	}
	// Strip redis:// prefix if present (go-redis expects host:port).
	addr = strings.TrimPrefix(addr, "redis://")

	log.Printf("harness: redis container started at %s", addr)

	return &RedisContainer{Addr: addr, container: redisC}, nil
}

// Terminate stops and removes the underlying container. It is safe to call
// more than once — only the first call actually terminates the container;
// later calls are no-ops that return the same result. This lets a TestMain
// call Terminate unconditionally in a deferred cleanup without worrying
// about whether an earlier explicit call already ran.
func (rc *RedisContainer) Terminate(ctx context.Context) error {
	rc.termOnce.Do(func() {
		rc.termErr = rc.container.Terminate(ctx)
		if rc.termErr != nil {
			log.Printf("harness: failed to terminate redis container (addr=%s): %v", rc.Addr, rc.termErr)
		} else {
			log.Printf("harness: redis container terminated (addr=%s)", rc.Addr)
		}
	})
	return rc.termErr
}

// Lease acquires exclusive access to the shared container for the
// duration of one test and returns a release function to call when that
// test is done (typically paired in SetupTest/TearDownTest). Integration
// suites are expected to stay serial (no t.Parallel()), so in normal use
// Lease never actually blocks — it exists as a belt-and-suspenders guard
// so that if a suite method is ever accidentally parallelized, concurrent
// FlushRedis calls against the shared instance serialize instead of racing.
func (rc *RedisContainer) Lease() (release func()) {
	rc.leaseMu.Lock()
	return rc.leaseMu.Unlock
}
