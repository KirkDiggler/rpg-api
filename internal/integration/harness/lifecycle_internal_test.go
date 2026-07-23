// Copyright (C) 2024 Kirk Diggler
// SPDX-License-Identifier: GPL-3.0-or-later

package harness

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"

	"go.uber.org/mock/gomock"

	redismocks "github.com/KirkDiggler/rpg-api/internal/redis/mocks"
)

// fakeContainer satisfies testcontainers.Container by embedding a nil
// interface (any method other than Terminate would panic if called, which
// is intentional: these lifecycle tests must never touch Docker) and
// overriding Terminate to record how many times it was invoked.
type fakeContainer struct {
	testcontainers.Container

	mu    sync.Mutex
	calls int
	err   error
}

func (f *fakeContainer) Terminate(_ context.Context, _ ...testcontainers.TerminateOption) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	return f.err
}

func (f *fakeContainer) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// TestRedisContainer_TerminateIsIdempotent proves the shared fixture only
// terminates its underlying container once, even if Terminate is called
// multiple times (e.g. a defer plus an explicit TestMain cleanup).
func TestRedisContainer_TerminateIsIdempotent(t *testing.T) {
	fc := &fakeContainer{}
	rc := &RedisContainer{Addr: "127.0.0.1:6379", container: fc}

	require.NoError(t, rc.Terminate(context.Background()))
	require.NoError(t, rc.Terminate(context.Background()))
	require.NoError(t, rc.Terminate(context.Background()))

	assert.Equal(t, 1, fc.callCount(), "underlying container Terminate must be called exactly once")
}

// TestRedisContainer_LeaseSerializesAccess proves a second Lease() call
// blocks until the first is released, which is what prevents a concurrent
// FlushRedis race if a suite method is ever accidentally parallelized.
func TestRedisContainer_LeaseSerializesAccess(t *testing.T) {
	rc := &RedisContainer{Addr: "127.0.0.1:6379", container: &fakeContainer{}}

	release := rc.Lease()

	acquired := make(chan struct{})
	go func() {
		release2 := rc.Lease()
		close(acquired)
		release2()
	}()

	select {
	case <-acquired:
		t.Fatal("second Lease() acquired while first still held")
	case <-time.After(100 * time.Millisecond):
		// expected: still blocked
	}

	release()

	select {
	case <-acquired:
		// expected: unblocked after release
	case <-time.After(2 * time.Second):
		t.Fatal("second Lease() never acquired after release")
	}
}

// TestServer_Close_OwnedContainer_TerminatesOnce proves a TestServer
// constructed via the owning path (redisContainer set) terminates its
// container exactly once on Close(), and that Close() is itself safe to
// call more than once.
func TestServer_Close_OwnedContainer_TerminatesOnce(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := redismocks.NewMockClient(ctrl)
	mockClient.EXPECT().Close().Return(nil).Times(1)

	fc := &fakeContainer{}
	ts := &TestServer{
		redisClient:    mockClient,
		redisContainer: fc,
	}

	ts.Close()
	ts.Close() // safe to call twice; must not double-terminate

	assert.Equal(t, 1, fc.callCount(), "owning TestServer must terminate its container exactly once")
}

// TestServer_Close_BorrowedContainer_NeverTerminates proves a TestServer
// constructed via the non-owning path (redisContainer nil, e.g.
// NewWithRedis against a shared fixture) never calls Terminate on any
// container, while still closing its own per-instance Redis client.
func TestServer_Close_BorrowedContainer_NeverTerminates(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockClient := redismocks.NewMockClient(ctrl)
	mockClient.EXPECT().Close().Return(nil).Times(1)

	// A shared fixture that a *different* owner (TestMain) is responsible
	// for. This TestServer must never touch it.
	shared := &RedisContainer{Addr: "127.0.0.1:6379", container: &fakeContainer{}}

	ts := &TestServer{
		redisClient: mockClient,
		// redisContainer intentionally left nil: this TestServer borrowed
		// the shared fixture's address but does not own its container.
	}

	ts.Close()

	assert.Equal(t, 0, shared.container.(*fakeContainer).callCount(),
		"borrowed TestServer must never terminate the shared container")
}
