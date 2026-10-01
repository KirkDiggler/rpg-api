package world_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	grpc_logging "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	grpc_recovery "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	compositionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/api/composition/v1alpha1"
	worldpb "github.com/KirkDiggler/rpg-api-protos/gen/go/api/world/v1alpha1"
	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	compositionhandler "github.com/KirkDiggler/rpg-api/internal/handlers/api/composition/v1alpha1"
	worldhandler "github.com/KirkDiggler/rpg-api/internal/handlers/api/world/v1alpha1"
	compositionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/composition"
	worldorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/world"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	compositionrepo "github.com/KirkDiggler/rpg-api/internal/repositories/composition"
	worldrepo "github.com/KirkDiggler/rpg-api/internal/repositories/world"
	"github.com/KirkDiggler/rpg-api/internal/worldcontext"
)

const (
	worldID      = "123456789012345678"
	adminRole    = "223456789012345678"
	builderRole  = "323456789012345678"
	playerRole   = "423456789012345678"
	newAdminRole = "523456789012345678"
)

type WorldAccessSuite struct {
	suite.Suite
	client        worldpb.WorldServiceClient
	repository    worldrepo.Repository
	redis         *miniredis.Miniredis
	compositions  compositionpb.CompositionServiceClient
	sessions      sessionpb.SessionServiceClient
	adminRevoked  atomic.Bool
	playerRevoked atomic.Bool
	adminLookups  atomic.Int32
	playerLookups atomic.Int32
}

func TestWorldAccessSuite(t *testing.T) { suite.Run(t, new(WorldAccessSuite)) }

func (s *WorldAccessSuite) SetupTest() {
	s.adminRevoked.Store(false)
	s.playerRevoked.Store(false)
	s.adminLookups.Store(0)
	s.playerLookups.Store(0)
	s.redis = miniredis.RunT(s.T())
	redisClient := goredis.NewClient(&goredis.Options{Addr: s.redis.Addr()})
	s.T().Cleanup(func() { _ = redisClient.Close() })
	var err error
	s.repository, err = worldrepo.NewRedis(&worldrepo.RedisConfig{Client: redisClient})
	s.Require().NoError(err)
	orchestrator, err := worldorch.New(&worldorch.Config{Repository: s.repository})
	s.Require().NoError(err)
	handler, err := worldhandler.New(&worldhandler.HandlerConfig{Orchestrator: orchestrator})
	s.Require().NoError(err)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token == "invalid" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if token == "unavailable" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		var response any
		switch r.URL.Path {
		case "/api/users/@me":
			response = map[string]string{"id": token + "-player"}
		case "/api/users/@me/guilds/" + worldID + "/member":
			roles := []string{}
			switch token {
			case "admin":
				s.adminLookups.Add(1)
				if !s.adminRevoked.Load() {
					roles = []string{adminRole}
				}
			case "builder":
				roles = []string{builderRole}
			case "player":
				s.playerLookups.Add(1)
				if !s.playerRevoked.Load() {
					roles = []string{playerRole}
				}
			}
			response = map[string]any{"user": map[string]string{"id": token + "-player"}, "roles": roles}
		case "/api/users/@me/guilds":
			response = []map[string]any{{"id": worldID, "owner": token == "owner"}}
		default:
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if encodeErr := json.NewEncoder(w).Encode(response); encodeErr != nil {
			s.T().Error(encodeErr)
		}
	}))
	s.T().Cleanup(provider.Close)
	discord := auth.NewDiscordClient(auth.WithBaseURL(provider.URL))
	identity := auth.NewTokenCache(time.Minute)
	membership, err := auth.NewMembershipCache(&auth.MembershipCacheConfig{TTL: 30 * time.Second, MaxEntries: 100, Now: time.Now})
	s.Require().NoError(err)
	resolver, err := auth.NewWorldResolver(&auth.WorldResolverConfig{MembershipVerifier: discord, IdentityCache: identity, MembershipCache: membership})
	s.Require().NoError(err)
	roleAccess, err := auth.NewRoleAccess(&auth.RoleAccessConfig{Resolver: resolver, Worlds: s.repository, StreamRefresh: 20 * time.Millisecond})
	s.Require().NoError(err)
	logger := grpc_logging.LoggerFunc(func(context.Context, grpc_logging.Level, string, ...any) {})
	identityConfig := &auth.InterceptorConfig{WorldScopedStreams: true}
	// Match production auth/management/role/logging/recovery order. Tests use
	// the real HTTP resolver and repositories; only renewal time is shortened.
	server := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			auth.UnaryAuthInterceptor(discord, identity, identityConfig),
			auth.UnaryWorldManagementInterceptor(&auth.WorldManagementConfig{Resolver: resolver, Ownership: discord, IdentityCache: identity, MembershipCache: membership}),
			roleAccess.UnaryInterceptor(),
			grpc_logging.UnaryServerInterceptor(logger),
			grpc_recovery.UnaryServerInterceptor(),
		),
		grpc.ChainStreamInterceptor(
			auth.StreamAuthInterceptor(discord, identity, identityConfig),
			roleAccess.StreamInterceptor(),
			grpc_logging.StreamServerInterceptor(logger),
			grpc_recovery.StreamServerInterceptor(),
		),
	)
	worldpb.RegisterWorldServiceServer(server, handler)
	compositions, err := compositionrepo.NewRedis(&compositionrepo.RedisConfig{Client: redisClient})
	s.Require().NoError(err)
	compositionOrchestrator, err := compositionorch.New(&compositionorch.Config{Repository: compositions, IDGenerator: idgen.NewUUID("composition")})
	s.Require().NoError(err)
	compositionHandler, err := compositionhandler.New(&compositionhandler.HandlerConfig{Service: compositionOrchestrator, AuthoringEnabled: true})
	s.Require().NoError(err)
	compositionpb.RegisterCompositionServiceServer(server, compositionHandler)
	sessionpb.RegisterSessionServiceServer(server, &roleGateStreamServer{})
	listener := bufconn.Listen(1024 * 1024)
	go func() { _ = server.Serve(listener) }()
	s.T().Cleanup(func() { server.Stop(); _ = listener.Close() })
	connection, err := grpc.NewClient("passthrough:///world-test", grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = connection.Close() })
	s.client = worldpb.NewWorldServiceClient(connection)
	s.compositions = compositionpb.NewCompositionServiceClient(connection)
	s.sessions = sessionpb.NewSessionServiceClient(connection)
}

func (s *WorldAccessSuite) ctx(token string) context.Context {
	return metadata.NewOutgoingContext(context.Background(), metadata.Pairs("authorization", "Discord "+token, "x-rpg-guild-id", worldID))
}

func (s *WorldAccessSuite) setupWorld() {
	out, err := s.client.SetWorldRoles(s.ctx("owner"), &worldpb.SetWorldRolesRequest{WorldId: worldID, Roles: &worldpb.WorldRoles{
		AdminRoleId: adminRole, BuilderRoleId: builderRole, PlayerRoleId: playerRole,
	}})
	s.Require().NoError(err)
	s.Equal(worldID, out.GetWorld().GetWorldId())
}

func (s *WorldAccessSuite) TestOwnerBootstrapsWithoutAnyGameRole() {
	_, err := s.client.GetWorld(s.ctx("owner"), &worldpb.GetWorldRequest{WorldId: worldID})
	s.Equal(codes.NotFound, status.Code(err))
	s.setupWorld()
	out, err := s.client.GetWorld(s.ctx("owner"), &worldpb.GetWorldRequest{WorldId: worldID})
	s.Require().NoError(err)
	s.Equal(adminRole, out.GetWorld().GetRoles().GetAdminRoleId())
	s.Zero(s.redis.TTL("world:v1:" + worldID))
	s.redis.FastForward(48 * time.Hour)
	s.True(s.redis.Exists("world:v1:" + worldID))
}

func (s *WorldAccessSuite) TestDelegatedAdminCannotReplaceAdminAuthority() {
	s.setupWorld()
	_, err := s.client.SetWorldRoles(s.ctx("admin"), &worldpb.SetWorldRolesRequest{WorldId: worldID, Roles: &worldpb.WorldRoles{
		AdminRoleId: newAdminRole, BuilderRoleId: builderRole, PlayerRoleId: playerRole,
	}})
	s.Equal(codes.PermissionDenied, status.Code(err))
	out, err := s.client.SetWorldMemberRoles(s.ctx("admin"), &worldpb.SetWorldMemberRolesRequest{
		WorldId: worldID, BuilderRoleId: playerRole, PlayerRoleId: builderRole,
	})
	s.Require().NoError(err)
	s.Equal(adminRole, out.GetWorld().GetRoles().GetAdminRoleId())
	s.Equal(playerRole, out.GetWorld().GetRoles().GetBuilderRoleId())
}

func (s *WorldAccessSuite) TestBuilderPlayerAndUnrelatedMemberCannotConfigure() {
	s.setupWorld()
	for _, token := range []string{"builder", "player", "unrelated"} {
		_, err := s.client.GetWorld(s.ctx(token), &worldpb.GetWorldRequest{WorldId: worldID})
		s.Equal(codes.PermissionDenied, status.Code(err))
		_, err = s.client.SetWorldMemberRoles(s.ctx(token), &worldpb.SetWorldMemberRolesRequest{WorldId: worldID, BuilderRoleId: builderRole, PlayerRoleId: playerRole})
		s.Equal(codes.PermissionDenied, status.Code(err))
	}
}

func (s *WorldAccessSuite) TestOwnerRevocationUsesCurrentConfigDespiteCachedMembership() {
	s.setupWorld()
	_, err := s.client.GetWorld(s.ctx("admin"), &worldpb.GetWorldRequest{WorldId: worldID})
	s.Require().NoError(err)
	_, err = s.client.SetWorldRoles(s.ctx("owner"), &worldpb.SetWorldRolesRequest{WorldId: worldID, Roles: &worldpb.WorldRoles{
		AdminRoleId: newAdminRole, BuilderRoleId: builderRole, PlayerRoleId: playerRole,
	}})
	s.Require().NoError(err)
	_, err = s.client.GetWorld(s.ctx("admin"), &worldpb.GetWorldRequest{WorldId: worldID})
	s.Equal(codes.PermissionDenied, status.Code(err))
	_, err = s.repository.SetMemberRoles(context.Background(), &worldrepo.SetMemberRolesInput{
		WorldID: worldID, ExpectedAdminRoleID: adminRole, BuilderRoleID: builderRole, PlayerRoleID: playerRole,
	})
	s.Equal(apierr.CodeAborted, apierr.GetCode(err))
}

func (s *WorldAccessSuite) TestWrongWorldAndIncompleteConfigurationAreRefused() {
	_, err := s.client.SetWorldRoles(s.ctx("owner"), &worldpb.SetWorldRolesRequest{WorldId: worldID})
	s.Equal(codes.InvalidArgument, status.Code(err))
	_, err = s.client.SetWorldRoles(s.ctx("owner"), &worldpb.SetWorldRolesRequest{WorldId: "923456789012345678", Roles: &worldpb.WorldRoles{
		AdminRoleId: adminRole, BuilderRoleId: builderRole, PlayerRoleId: playerRole,
	}})
	s.Equal(codes.PermissionDenied, status.Code(err))
	s.Empty(s.redis.Keys())
	_, err = s.client.SetWorldMemberRoles(s.ctx("owner"), &worldpb.SetWorldMemberRolesRequest{WorldId: worldID, BuilderRoleId: builderRole, PlayerRoleId: playerRole})
	s.Equal(codes.NotFound, status.Code(err))
}

func (s *WorldAccessSuite) TestDiscordAdminRoleRemovalCannotReuseCachedAuthority() {
	s.setupWorld()
	_, err := s.client.GetWorld(s.ctx("admin"), &worldpb.GetWorldRequest{WorldId: worldID})
	s.Require().NoError(err)
	s.Equal(int32(1), s.adminLookups.Load())
	s.adminRevoked.Store(true)
	_, err = s.client.SetWorldMemberRoles(s.ctx("admin"), &worldpb.SetWorldMemberRolesRequest{
		WorldId: worldID, BuilderRoleId: playerRole, PlayerRoleId: builderRole,
	})
	s.Equal(codes.PermissionDenied, status.Code(err), "revoked Discord role must not reuse a cached admin snapshot")
	s.Equal(int32(2), s.adminLookups.Load(), "management must re-fetch membership before granting authority")
	stored, err := s.repository.Get(context.Background(), &worldrepo.GetInput{WorldID: worldID})
	s.Require().NoError(err)
	s.Equal(builderRole, stored.World.BuilderRoleID)
	s.Equal(playerRole, stored.World.PlayerRoleID)
}

func (s *WorldAccessSuite) TestGameRPCsThroughProductionAuthorizationChain() {
	_, err := s.compositions.ListCompositions(s.ctx("player"), &compositionpb.ListCompositionsRequest{WorldId: worldID})
	s.Equal(codes.FailedPrecondition, status.Code(err), "unconfigured world cannot admit gameplay")
	s.setupWorld()
	created, err := s.compositions.CreateComposition(s.ctx("builder"), &compositionpb.CreateCompositionRequest{WorldId: worldID, Json: "{}"})
	s.Require().NoError(err)
	s.Require().NotEmpty(created.GetComposition().GetId())
	read, err := s.compositions.GetComposition(s.ctx("player"), &compositionpb.GetCompositionRequest{WorldId: worldID, Id: created.GetComposition().GetId()})
	s.Require().NoError(err)
	s.Equal(created.GetComposition().GetId(), read.GetComposition().GetId())
	_, err = s.compositions.CreateComposition(s.ctx("player"), &compositionpb.CreateCompositionRequest{WorldId: worldID, Json: "{}"})
	s.Equal(codes.PermissionDenied, status.Code(err))
	_, err = s.compositions.ListCompositions(s.ctx("unrelated"), &compositionpb.ListCompositionsRequest{WorldId: worldID})
	s.Equal(codes.PermissionDenied, status.Code(err))
}

func (s *WorldAccessSuite) TestIdleGRPCStreamRevocationThroughProductionAuthorizationChain() {
	s.setupWorld()
	// Warm the unary cache, then prove both stream admission and renewal
	// bypass it through the real HTTP provider and gRPC stream wrappers.
	_, err := s.compositions.ListCompositions(s.ctx("player"), &compositionpb.ListCompositionsRequest{WorldId: worldID})
	s.Require().NoError(err)
	ctx, cancel := context.WithTimeout(s.ctx("player"), time.Second)
	defer cancel()
	stream, err := s.sessions.StreamEvents(ctx, &sessionpb.StreamEventsRequest{Session: "gate-only", Member: "player"})
	s.Require().NoError(err)
	header, err := stream.Header()
	s.Require().NoError(err)
	s.Equal([]string{"admitted"}, header.Get("role-gate-proof"))
	s.playerRevoked.Store(true)
	_, err = stream.Recv()
	s.Equal(codes.PermissionDenied, status.Code(err))
	s.GreaterOrEqual(s.playerLookups.Load(), int32(3))
}

// This narrow consumer tests stream authorization/context wiring, not SDK
// seating or game events. World and composition operations above use real
// business handlers/repositories; session engine behavior has its own suites.
type roleGateStreamServer struct {
	sessionpb.UnimplementedSessionServiceServer
}

func (*roleGateStreamServer) StreamEvents(_ *sessionpb.StreamEventsRequest, stream sessionpb.SessionService_StreamEventsServer) error {
	world, ok := worldcontext.Get(stream.Context())
	if !ok || world.WorldID != worldID {
		return status.Error(codes.Internal, "verified world context was lost")
	}
	if err := stream.SendHeader(metadata.Pairs("role-gate-proof", "admitted")); err != nil {
		return err
	}
	<-stream.Context().Done()
	return nil
}

func (s *WorldAccessSuite) TestInvalidCredentialAndProviderFailureCannotConfigure() {
	for token, want := range map[string]codes.Code{"invalid": codes.Unauthenticated, "unavailable": codes.Unavailable} {
		_, err := s.client.SetWorldRoles(s.ctx(token), &worldpb.SetWorldRolesRequest{WorldId: worldID})
		s.Equal(want, status.Code(err))
	}
	s.Empty(s.redis.Keys())
}
