package main

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/KirkDiggler/rpg-api/internal/auth"
	worldrepo "github.com/KirkDiggler/rpg-api/internal/repositories/world"
	"github.com/KirkDiggler/rpg-api/internal/worldcontext"
)

const (
	rolloutWorld       = "123456789012345678"
	rolloutCharacter   = "/dnd5e.api.v1alpha1.CharacterService/ListCharacters"
	rolloutStream      = "/dnd5e.api.session.v1alpha1.SessionService/StreamEvents"
	rolloutComposition = "/api.composition.v1alpha1.CompositionService/ListCompositions"
)

type rolloutValidator struct{}

func (rolloutValidator) GetCurrentUser(_ context.Context, token string) (*auth.DiscordUser, error) {
	if token != "valid" {
		return nil, auth.ErrInvalidToken
	}
	return &auth.DiscordUser{ID: "discord-player"}, nil
}

type rolloutResolver struct {
	calls int
	err   error
}

func (r *rolloutResolver) Resolve(_ context.Context, _ *auth.ResolveWorldInput) (*auth.ResolveWorldOutput, error) {
	r.calls++
	if r.err != nil {
		return nil, r.err
	}
	return &auth.ResolveWorldOutput{WorldID: rolloutWorld}, nil
}

type rolloutServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (s *rolloutServerStream) Context() context.Context { return s.ctx }

type rolloutRequest struct {
	enforce       bool
	dev           bool
	authorization string
	method        string
	guild         string
}

type worldAccessRolloutSuite struct {
	suite.Suite
	resolver *rolloutResolver
	roles    *auth.RoleAccessConfig
	reached  bool
	world    string
}

func TestWorldAccessRollout(t *testing.T) { suite.Run(t, new(worldAccessRolloutSuite)) }

func (s *worldAccessRolloutSuite) SetupTest() {
	redisServer := miniredis.RunT(s.T())
	client := goredis.NewClient(&goredis.Options{Addr: redisServer.Addr()})
	s.T().Cleanup(func() { s.Require().NoError(client.Close()) })
	worlds, err := worldrepo.NewRedis(&worldrepo.RedisConfig{Client: client})
	s.Require().NoError(err)
	s.resolver = &rolloutResolver{}
	s.roles = &auth.RoleAccessConfig{Resolver: s.resolver, Worlds: worlds}
	s.reached, s.world = false, ""
}

func (s *worldAccessRolloutSuite) TestConfigurationDefaultsOffAndRejectsTypos() {
	for _, tc := range []struct {
		value   string
		enabled bool
		invalid bool
	}{{"", false, false}, {"false", false, false}, {"true", true, false}, {"yes", false, true}, {"TRUE", false, true}} {
		s.Run(tc.value, func() {
			s.T().Setenv(envWorldAccessEnforcement, tc.value)
			enabled, err := configuredWorldAccessEnforcement()
			if tc.invalid {
				s.Require().Error(err)
				return
			}
			s.Require().NoError(err)
			s.Equal(tc.enabled, enabled)
		})
	}
}

func (s *worldAccessRolloutSuite) context(in rolloutRequest) context.Context {
	md := metadata.MD{}
	if in.authorization != "" {
		md.Set("authorization", in.authorization)
	}
	if in.guild != "" {
		md.Set("x-rpg-guild-id", in.guild)
	}
	return metadata.NewIncomingContext(context.Background(), md)
}

func (s *worldAccessRolloutSuite) observe(ctx context.Context) {
	s.reached = true
	s.NotEmpty(auth.GetPlayerID(ctx))
	if world, ok := worldcontext.Get(ctx); ok {
		s.world = world.WorldID
	}
}

func (s *worldAccessRolloutSuite) unary(in rolloutRequest) error {
	gate, err := newGameplayAccess(&gameplayAccessInput{Enforce: in.enforce, Roles: s.roles})
	s.Require().NoError(err)
	info := &grpc.UnaryServerInfo{FullMethod: in.method}
	authenticate := auth.UnaryAuthInterceptor(rolloutValidator{}, auth.NewTokenCache(0), &auth.InterceptorConfig{DevMode: in.dev})
	_, err = authenticate(s.context(in), nil, info, func(ctx context.Context, req interface{}) (interface{}, error) {
		return gate.Unary(ctx, req, info, func(ctx context.Context, _ interface{}) (interface{}, error) {
			s.observe(ctx)
			return struct{}{}, nil
		})
	})
	return err
}

func (s *worldAccessRolloutSuite) stream(in rolloutRequest) error {
	gate, err := newGameplayAccess(&gameplayAccessInput{Enforce: in.enforce, Roles: s.roles})
	s.Require().NoError(err)
	info := &grpc.StreamServerInfo{FullMethod: in.method, IsServerStream: true}
	authenticate := auth.StreamAuthInterceptor(rolloutValidator{}, auth.NewTokenCache(0), &auth.InterceptorConfig{
		DevMode: in.dev, WorldScopedStreams: in.enforce,
	})
	return authenticate(nil, &rolloutServerStream{ctx: s.context(in)}, info, func(srv interface{}, stream grpc.ServerStream) error {
		return gate.Stream(srv, stream, info, func(_ interface{}, stream grpc.ServerStream) error {
			s.observe(stream.Context())
			return nil
		})
	})
}

func (s *worldAccessRolloutSuite) TestLegacyDevNeedsNoNewRoleConfiguration() {
	s.Require().NoError(s.unary(rolloutRequest{dev: true, authorization: "Dev local-player", method: rolloutCharacter}))
	s.True(s.reached)
	s.Empty(s.world, "ordinary legacy gameplay does not invent a world")
	s.Zero(s.resolver.calls)
}

func (s *worldAccessRolloutSuite) TestLegacyDiscordNeedsNoNewGuildOrRoleSetup() {
	s.Require().NoError(s.unary(rolloutRequest{authorization: "Discord valid", method: rolloutCharacter}))
	s.True(s.reached)
	s.Empty(s.world)
	s.Zero(s.resolver.calls)
}

func (s *worldAccessRolloutSuite) TestLegacyStillRejectsUnauthenticatedRequests() {
	s.Equal(codes.Unauthenticated, status.Code(s.unary(rolloutRequest{method: rolloutCharacter})))
	s.False(s.reached)
}

func (s *worldAccessRolloutSuite) TestLegacyStillRejectsInvalidDiscordToken() {
	s.Equal(codes.Unauthenticated, status.Code(s.unary(rolloutRequest{authorization: "Discord invalid", method: rolloutCharacter})))
	s.False(s.reached)
}

func (s *worldAccessRolloutSuite) TestLegacyProductionStillRejectsDevCredentials() {
	s.Equal(codes.Unauthenticated, status.Code(s.unary(rolloutRequest{authorization: "Dev local-player", method: rolloutCharacter})))
	s.False(s.reached)
}

func (s *worldAccessRolloutSuite) TestLegacyCompositionStillRequiresVerifiedWorld() {
	s.Require().NoError(s.unary(rolloutRequest{authorization: "Discord valid", method: rolloutComposition, guild: rolloutWorld}))
	s.True(s.reached)
	s.Equal(rolloutWorld, s.world)
	s.Equal(1, s.resolver.calls)
}

func (s *worldAccessRolloutSuite) TestLegacyCompositionStillRejectsUnverifiedMembership() {
	s.resolver.err = status.Error(codes.PermissionDenied, "not a member")
	s.Equal(codes.PermissionDenied, status.Code(s.unary(rolloutRequest{authorization: "Discord valid", method: rolloutComposition, guild: rolloutWorld})))
	s.False(s.reached)
}

func (s *worldAccessRolloutSuite) TestEnabledDevWithoutPermissionFailsClosed() {
	s.Equal(codes.PermissionDenied, status.Code(s.unary(rolloutRequest{enforce: true, dev: true, authorization: "Dev local-player", method: rolloutCharacter})))
	s.False(s.reached)
}

func (s *worldAccessRolloutSuite) TestEnabledDevPermissionInstallsWorld() {
	s.roles.DevelopmentPermissions = auth.PermissionPlay
	s.Require().NoError(s.unary(rolloutRequest{enforce: true, dev: true, authorization: "Dev local-player", method: rolloutCharacter}))
	s.True(s.reached)
	s.Equal(rolloutWorld, s.world)
}

func (s *worldAccessRolloutSuite) TestEnabledDiscordRequiresOwnerConfiguration() {
	s.Equal(codes.FailedPrecondition, status.Code(s.unary(rolloutRequest{enforce: true, authorization: "Discord valid", method: rolloutCharacter, guild: rolloutWorld})))
	s.False(s.reached)
}

func (s *worldAccessRolloutSuite) TestLegacyStreamsNeedNoNewGuildSetup() {
	s.Require().NoError(s.stream(rolloutRequest{authorization: "Discord valid", method: rolloutStream}))
	s.True(s.reached)
	s.Empty(s.world)
	s.Zero(s.resolver.calls)
}

func (s *worldAccessRolloutSuite) TestLegacyStreamsStillRequireAuthentication() {
	s.Equal(codes.Unauthenticated, status.Code(s.stream(rolloutRequest{method: rolloutStream})))
	s.False(s.reached)
}

func (s *worldAccessRolloutSuite) TestEnabledStreamsRetainStrictRoleGate() {
	s.Equal(codes.FailedPrecondition, status.Code(s.stream(rolloutRequest{enforce: true, authorization: "Discord valid", method: rolloutStream, guild: rolloutWorld})))
	s.False(s.reached)
}

func (s *worldAccessRolloutSuite) TestEnabledStreamWithDevPermissionInstallsWorld() {
	s.roles.DevelopmentPermissions = auth.PermissionPlay
	s.Require().NoError(s.stream(rolloutRequest{enforce: true, dev: true, authorization: "Dev local-player", method: rolloutStream}))
	s.True(s.reached)
	s.Equal(rolloutWorld, s.world)
}
