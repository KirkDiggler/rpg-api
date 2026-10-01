package world_test

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	worldpb "github.com/KirkDiggler/rpg-api-protos/gen/go/api/world/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	worldhandler "github.com/KirkDiggler/rpg-api/internal/handlers/api/world/v1alpha1"
	worldorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/world"
	worldrepo "github.com/KirkDiggler/rpg-api/internal/repositories/world"
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
	client     worldpb.WorldServiceClient
	repository worldrepo.Repository
	redis      *miniredis.Miniredis
}

func TestWorldAccessSuite(t *testing.T) { suite.Run(t, new(WorldAccessSuite)) }

func (s *WorldAccessSuite) SetupTest() {
	s.redis = miniredis.RunT(s.T())
	redisClient := goredis.NewClient(&goredis.Options{Addr: s.redis.Addr()})
	s.T().Cleanup(func() { _ = redisClient.Close() })
	var err error
	s.repository, err = worldrepo.NewRedis(&worldrepo.RedisConfig{Client: redisClient})
	s.Require().NoError(err)
	service, err := worldorch.New(&worldorch.Config{Repository: s.repository})
	s.Require().NoError(err)
	handler, err := worldhandler.New(&worldhandler.HandlerConfig{Service: service})
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
				roles = []string{adminRole}
			case "builder":
				roles = []string{builderRole}
			case "player":
				roles = []string{playerRole}
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
	server := grpc.NewServer(grpc.ChainUnaryInterceptor(
		auth.UnaryAuthInterceptor(discord, identity, &auth.InterceptorConfig{}),
		auth.UnaryWorldManagementInterceptor(&auth.WorldManagementConfig{Resolver: resolver, Ownership: discord, IdentityCache: identity, MembershipCache: membership}),
		auth.UnaryWorldContextInterceptor(resolver),
	))
	worldpb.RegisterWorldServiceServer(server, handler)
	listener := bufconn.Listen(1024 * 1024)
	go func() { _ = server.Serve(listener) }()
	s.T().Cleanup(func() { server.Stop(); _ = listener.Close() })
	connection, err := grpc.NewClient("passthrough:///world-test", grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return listener.Dial() }))
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = connection.Close() })
	s.client = worldpb.NewWorldServiceClient(connection)
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

func (s *WorldAccessSuite) TestInvalidCredentialAndProviderFailureCannotConfigure() {
	for token, want := range map[string]codes.Code{"invalid": codes.Unauthenticated, "unavailable": codes.Unavailable} {
		_, err := s.client.SetWorldRoles(s.ctx(token), &worldpb.SetWorldRolesRequest{WorldId: worldID})
		s.Equal(want, status.Code(err))
	}
	s.Empty(s.redis.Keys())
}
