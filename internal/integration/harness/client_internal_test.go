package harness

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/test/bufconn"
)

func TestNewBufconnClientConn_ReturnsBeforeServerReady(t *testing.T) {
	const serverStartDelay = time.Second

	listener := bufconn.Listen(bufSize)
	defer listener.Close()

	server := grpc.NewServer()
	defer server.Stop()

	go func() {
		time.Sleep(serverStartDelay)
		_ = server.Serve(listener)
	}()

	started := time.Now()
	conn, err := newBufconnClientConn(func(ctx context.Context, _ string) (net.Conn, error) {
		return listener.DialContext(ctx)
	})
	require.NoError(t, err)
	defer func() {
		require.NoError(t, conn.Close())
	}()

	require.Less(t, time.Since(started), serverStartDelay/2,
		"client creation should not wait for server readiness")
	require.Eventually(t, func() bool {
		return conn.GetState() != connectivity.Idle
	}, 250*time.Millisecond, 10*time.Millisecond,
		"Connect should kick the client out of idle")
}
