package sessionv1alpha1

import (
	"context"
	"fmt"
	"net"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	sessionv1alpha1mock "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1/mock"
)

// overTheWire serves h on an in-process gRPC server and returns a client of
// it: every call is made as player "alice", the way the auth interceptor
// would set her on a real server.
func overTheWire(t *testing.T, h *Handler) sessionpb.SessionServiceClient {
	t.Helper()
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer(grpc.UnaryInterceptor(func(
		ctx context.Context, req any, _ *grpc.UnaryServerInfo, next grpc.UnaryHandler,
	) (any, error) {
		return next(auth.WithPlayerID(ctx, "alice"), req)
	}))
	sessionpb.RegisterSessionServiceServer(server, h)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return listener.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return sessionpb.NewSessionServiceClient(conn)
}

// TestStaleRefusalsArriveAbortedOverTheWire pins rpg-project#925 past the
// handler: a stale Move selector and a stale Speak reply, served by a real
// gRPC server, reach the client as ABORTED with the session's sentence --
// the one code that says "what you were offered has moved on", apart from
// the FAILED_PRECONDITION refusals that are sentences for the player.
func TestStaleRefusalsArriveAbortedOverTheWire(t *testing.T) {
	ctrl := gomock.NewController(t)
	mgr := sessionv1alpha1mock.NewMockManager(ctrl)
	mgr.EXPECT().Move(gomock.Any(), gomock.Any()).Return(nil, fmt.Errorf("move: %w", sdk.ErrStaleDeclaration))
	mgr.EXPECT().Speak(gomock.Any(), gomock.Any()).Return(nil, fmt.Errorf("speak: %w", sdk.ErrStaleOption))
	client := overTheWire(t, &Handler{manager: mgr, characters: anyMemberOwnedBy(ctrl, "alice")})

	_, err := client.Move(context.Background(), &sessionpb.MoveRequest{Session: "sess-1", Member: "char-1", DeclarationId: "decl-move"})
	require.Equal(t, codes.Aborted, status.Code(err))
	require.Equal(t, "move: declaration is stale", status.Convert(err).Message())

	_, err = client.Speak(context.Background(), &sessionpb.SpeakRequest{
		Session: "sess-1", Member: "char-1", Target: &sessionpb.SpeakRequest_MemberId{MemberId: "gate-1"}, OptionId: "guest", Revision: 3,
	})
	require.Equal(t, codes.Aborted, status.Code(err))
}
