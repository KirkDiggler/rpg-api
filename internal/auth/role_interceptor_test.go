package auth

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/KirkDiggler/rpg-api/internal/entities"
	worldrepo "github.com/KirkDiggler/rpg-api/internal/repositories/world"
	"github.com/KirkDiggler/rpg-api/internal/worldcontext"
)

const roleTestWorldID = "123456789012345678"
const roleTestAdmin = "223456789012345678"
const roleTestBuilder = "323456789012345678"
const roleTestPlayer = "423456789012345678"

type roleTestResolver struct {
	mu      sync.Mutex
	roles   []string
	failure error
	calls   []bool
}

func (r *roleTestResolver) Resolve(_ context.Context, input *ResolveWorldInput) (*ResolveWorldOutput, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, input.ForceRefresh)
	if r.failure != nil {
		return nil, r.failure
	}
	return &ResolveWorldOutput{WorldID: roleTestWorldID, AssignedRoleIDs: append([]string(nil), r.roles...)}, nil
}
func (r *roleTestResolver) change(roles []string, failure error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.roles = roles
	r.failure = failure
}

type roleTestStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *roleTestStream) Context() context.Context { return s.ctx }

type RoleInterceptorSuite struct {
	suite.Suite
	gate     *RoleAccess
	resolver *roleTestResolver
	worlds   worldrepo.Repository
	redis    *miniredis.Miniredis
	ctx      context.Context
}

func TestRoleInterceptorSuite(t *testing.T) { suite.Run(t, new(RoleInterceptorSuite)) }
func (s *RoleInterceptorSuite) SetupTest() {
	redis := miniredis.RunT(s.T())
	s.redis = redis
	client := goredis.NewClient(&goredis.Options{Addr: redis.Addr()})
	s.T().Cleanup(func() { _ = client.Close() })
	var err error
	s.worlds, err = worldrepo.NewRedis(&worldrepo.RedisConfig{Client: client})
	s.Require().NoError(err)
	_, err = s.worlds.SetRoles(context.Background(), &worldrepo.SetRolesInput{World: &entities.World{
		WorldID: roleTestWorldID, AdminRoleID: roleTestAdmin, BuilderRoleID: roleTestBuilder, PlayerRoleID: roleTestPlayer,
	}})
	s.Require().NoError(err)
	s.resolver = &roleTestResolver{roles: []string{roleTestPlayer}}
	s.gate, err = NewRoleAccess(&RoleAccessConfig{Resolver: s.resolver, Worlds: s.worlds, StreamRefresh: 5 * time.Millisecond})
	s.Require().NoError(err)
	s.ctx = metadata.NewIncomingContext(context.Background(), metadata.Pairs(guildSelectorHeader, roleTestWorldID))
	s.ctx = withAuthenticatedRequest(s.ctx, "player", &requestAuth{scheme: authSchemeDiscord, value: "credential"})
}

func (s *RoleInterceptorSuite) TestUnaryAdmissionCleansCredentialsAndRetainsWorld() {
	for _, role := range []string{roleTestPlayer, roleTestBuilder, roleTestAdmin} {
		s.resolver.change([]string{role}, nil)
		called := false
		_, err := s.gate.UnaryInterceptor()(s.ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/dnd5e.api.lobby.v1alpha1.LobbyService/ListDungeons"}, func(ctx context.Context, _ interface{}) (interface{}, error) {
			called = true
			_, present := getRequestAuth(ctx)
			s.False(present)
			world, ok := worldcontext.Get(ctx)
			s.True(ok)
			s.Equal(roleTestWorldID, world.WorldID)
			return nil, nil
		})
		s.NoError(err)
		s.True(called)
	}
}

func (s *RoleInterceptorSuite) TestPlayerCannotAuthorAndUnconfiguredWorldCannotPlay() {
	for _, method := range []string{"/api.composition.v1alpha1.CompositionService/CreateComposition", "/dnd5e.api.session.v1alpha1.SessionService/UnknownVerb"} {
		_, err := s.gate.UnaryInterceptor()(s.ctx, nil, &grpc.UnaryServerInfo{FullMethod: method}, func(context.Context, interface{}) (interface{}, error) {
			s.Fail("denied request reached handler")
			return nil, nil
		})
		s.Equal(codes.PermissionDenied, status.Code(err))
	}
	s.resolver.change([]string{roleTestBuilder}, nil)
	_, err := s.gate.authorize(s.ctx, "/api.composition.v1alpha1.CompositionService/CreateComposition", false)
	s.NoError(err)
	s.resolver.change(nil, nil)
	_, err = s.gate.authorize(s.ctx, "/dnd5e.api.lobby.v1alpha1.LobbyService/ListDungeons", false)
	s.Equal(codes.PermissionDenied, status.Code(err))
}

func (s *RoleInterceptorSuite) TestUnconfiguredWorldDoesNotGrantGameplay() {
	s.redis.Del("world:v1:" + roleTestWorldID)
	_, err := s.gate.authorize(s.ctx, "/dnd5e.api.lobby.v1alpha1.LobbyService/ListDungeons", false)
	s.Equal(codes.FailedPrecondition, status.Code(err))
}

func (s *RoleInterceptorSuite) TestIdleStreamRevocationAndProviderFailureEndSubscription() {
	for _, failure := range []error{nil, status.Error(codes.Unavailable, "provider unavailable")} {
		s.resolver.change([]string{roleTestPlayer}, nil)
		ready := make(chan struct{})
		result := make(chan error, 1)
		go func() {
			result <- s.gate.StreamInterceptor()(nil, &roleTestStream{ctx: s.ctx}, &grpc.StreamServerInfo{FullMethod: "/dnd5e.api.session.v1alpha1.SessionService/StreamEvents"}, func(_ interface{}, stream grpc.ServerStream) error {
				_, present := getRequestAuth(stream.Context())
				s.False(present)
				close(ready)
				<-stream.Context().Done()
				return nil
			})
		}()
		select {
		case <-ready:
		case <-time.After(time.Second):
			s.FailNow("stream was not admitted")
		}
		s.resolver.change(nil, failure)
		select {
		case err := <-result:
			want := codes.PermissionDenied
			if failure != nil {
				want = codes.Unavailable
			}
			s.Equal(want, status.Code(err))
		case <-time.After(time.Second):
			s.FailNow("idle stream kept revoked access")
		}
	}
	s.resolver.mu.Lock()
	defer s.resolver.mu.Unlock()
	for _, fresh := range s.resolver.calls {
		s.True(fresh, "stream checks must not extend a cached snapshot")
	}
}

func (s *RoleInterceptorSuite) TestStreamCancellationStopsRefreshWorker() {
	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()
	err := s.gate.StreamInterceptor()(nil, &roleTestStream{ctx: ctx}, &grpc.StreamServerInfo{FullMethod: "/dnd5e.api.session.v1alpha1.SessionService/StreamEvents"}, func(_ interface{}, stream grpc.ServerStream) error {
		cancel()
		<-stream.Context().Done()
		return nil
	})
	s.NoError(err)
}
