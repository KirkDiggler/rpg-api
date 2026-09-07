package auth_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/worldcontext"
)

const (
	compositionGetMethod = "/api.composition.v1alpha1.CompositionService/GetComposition"
	canonicalGuildID     = "123456789012345678"
)

type verifierStub struct {
	mu     sync.Mutex
	calls  int
	inputs []*auth.GetCurrentUserGuildMemberInput
	member *auth.DiscordGuildMember
	err    error
}

func (s *verifierStub) GetCurrentUserGuildMember(_ context.Context, input *auth.GetCurrentUserGuildMemberInput) (*auth.DiscordGuildMember, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	s.inputs = append(s.inputs, input)
	return s.member, s.err
}

func (s *verifierStub) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

type validatorStub struct {
	user *auth.DiscordUser
	err  error
}

func (s *validatorStub) GetCurrentUser(context.Context, string) (*auth.DiscordUser, error) {
	return s.user, s.err
}

func newMembershipCache(t *testing.T) *auth.MembershipCache {
	t.Helper()
	cache, err := auth.NewMembershipCache(&auth.MembershipCacheConfig{
		TTL:        30 * time.Second,
		MaxEntries: 1024,
		Now:        time.Now,
	})
	require.NoError(t, err)
	return cache
}

func invokeWorldChain(
	t *testing.T,
	validator auth.TokenValidator,
	identityCache *auth.TokenCache,
	resolver auth.WorldResolver,
	devMode bool,
	authorization string,
	guildValues []string,
	method string,
) (context.Context, error) {
	t.Helper()
	md := metadata.MD{}
	if authorization != "" {
		md.Set("authorization", authorization)
	}
	if guildValues != nil {
		md["x-rpg-guild-id"] = guildValues
	}
	ctx := metadata.NewIncomingContext(context.Background(), md)
	var handlerContext context.Context
	worldInterceptor := auth.UnaryWorldContextInterceptor(resolver)
	globalInterceptor := auth.UnaryAuthInterceptor(validator, identityCache, &auth.InterceptorConfig{DevMode: devMode})
	_, err := globalInterceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: method}, func(ctx context.Context, req interface{}) (interface{}, error) {
		return worldInterceptor(ctx, req, &grpc.UnaryServerInfo{FullMethod: method}, func(ctx context.Context, _ interface{}) (interface{}, error) {
			handlerContext = ctx
			return nil, nil
		})
	})
	return handlerContext, err
}

func TestWorldResolverDiscordSameRequestTokenOnIdentityCacheHit(t *testing.T) {
	identityCache := auth.NewTokenCache(5 * time.Minute)
	identityCache.Set("same-request-credential", "player-1")
	membershipCache := newMembershipCache(t)
	verifier := &verifierStub{member: &auth.DiscordGuildMember{User: &auth.DiscordUser{ID: "player-1"}}}
	resolver, err := auth.NewWorldResolver(&auth.WorldResolverConfig{
		MembershipVerifier: verifier,
		IdentityCache:      identityCache,
		MembershipCache:    membershipCache,
		DevWorldID:         "test-world",
	})
	require.NoError(t, err)

	ctx, err := invokeWorldChain(t, &validatorStub{}, identityCache, resolver, false, "Discord same-request-credential", []string{canonicalGuildID}, compositionGetMethod)
	require.NoError(t, err)
	value, ok := worldcontext.Get(ctx)
	require.True(t, ok)
	require.Equal(t, canonicalGuildID, value.WorldID)
	require.Equal(t, 1, verifier.callCount())
	require.Len(t, verifier.inputs, 1)
	require.Equal(t, "same-request-credential", verifier.inputs[0].Token)
	require.Equal(t, canonicalGuildID, verifier.inputs[0].GuildID)

	_, err = invokeWorldChain(t, &validatorStub{}, identityCache, resolver, false, "Discord same-request-credential", []string{canonicalGuildID}, compositionGetMethod)
	require.NoError(t, err)
	require.Equal(t, 1, verifier.callCount(), "positive membership decision should be cached")
}

func TestWorldResolverProviderStatusMappingAndNoDenialCache(t *testing.T) {
	tests := []struct {
		name   string
		member *auth.DiscordGuildMember
		err    error
		code   codes.Code
	}{
		{name: "forbidden", err: auth.ErrGuildMembershipDenied, code: codes.PermissionDenied},
		{name: "unavailable", err: auth.ErrDiscordUnavailable, code: codes.Unavailable},
		{name: "unknown provider failure", err: errors.New("unexpected"), code: codes.Unavailable},
		{name: "missing member", code: codes.Unavailable},
		{name: "missing user", member: &auth.DiscordGuildMember{}, code: codes.Unavailable},
		{name: "empty user", member: &auth.DiscordGuildMember{User: &auth.DiscordUser{}}, code: codes.Unavailable},
		{name: "mismatched user", member: &auth.DiscordGuildMember{User: &auth.DiscordUser{ID: "another-player"}}, code: codes.Unavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			identityCache := auth.NewTokenCache(5 * time.Minute)
			identityCache.Set("provider-credential", "player-1")
			verifier := &verifierStub{member: tt.member, err: tt.err}
			resolver, err := auth.NewWorldResolver(&auth.WorldResolverConfig{
				MembershipVerifier: verifier,
				IdentityCache:      identityCache,
				MembershipCache:    newMembershipCache(t),
			})
			require.NoError(t, err)
			for range 2 {
				_, err = invokeWorldChain(t, &validatorStub{}, identityCache, resolver, false, "Discord provider-credential", []string{canonicalGuildID}, compositionGetMethod)
				require.Equal(t, tt.code, status.Code(err))
			}
			require.Equal(t, 2, verifier.callCount(), "failed membership decisions must not be cached")
		})
	}
}

func TestWorldResolverUnauthorizedInvalidatesIdentityAndEveryTokenMembership(t *testing.T) {
	identityCache := auth.NewTokenCache(5 * time.Minute)
	identityCache.Set("revoked-credential", "player-1")
	membershipCache := newMembershipCache(t)
	membershipCache.Set("revoked-credential", "223456789012345678", auth.MembershipDecision{PlayerID: "player-1", WorldID: "223456789012345678"})
	membershipCache.Set("revoked-credential", "323456789012345678", auth.MembershipDecision{PlayerID: "player-1", WorldID: "323456789012345678"})
	membershipCache.Set("other-credential", canonicalGuildID, auth.MembershipDecision{PlayerID: "player-2", WorldID: canonicalGuildID})
	verifier := &verifierStub{err: auth.ErrInvalidToken}
	resolver, err := auth.NewWorldResolver(&auth.WorldResolverConfig{
		MembershipVerifier: verifier,
		IdentityCache:      identityCache,
		MembershipCache:    membershipCache,
	})
	require.NoError(t, err)

	_, err = invokeWorldChain(t, &validatorStub{}, identityCache, resolver, false, "Discord revoked-credential", []string{canonicalGuildID}, compositionGetMethod)
	require.Equal(t, codes.Unauthenticated, status.Code(err))
	_, ok := identityCache.Get("revoked-credential")
	require.False(t, ok)
	_, ok = membershipCache.Get("revoked-credential", "223456789012345678")
	require.False(t, ok)
	_, ok = membershipCache.Get("revoked-credential", "323456789012345678")
	require.False(t, ok)
	_, ok = membershipCache.Get("other-credential", canonicalGuildID)
	require.True(t, ok)
}

func TestWorldResolverDevUsesOnlyConfiguredWorld(t *testing.T) {
	identityCache := auth.NewTokenCache(5 * time.Minute)
	resolver, err := auth.NewWorldResolver(&auth.WorldResolverConfig{
		MembershipVerifier: &verifierStub{},
		IdentityCache:      identityCache,
		MembershipCache:    newMembershipCache(t),
		DevWorldID:         "configured-dev-world",
	})
	require.NoError(t, err)

	ctx, err := invokeWorldChain(t, &validatorStub{}, identityCache, resolver, true, "Dev local-player", []string{"hostile-selector"}, compositionGetMethod)
	require.NoError(t, err)
	value, ok := worldcontext.Get(ctx)
	require.True(t, ok)
	require.Equal(t, "configured-dev-world", value.WorldID)

	_, err = invokeWorldChain(t, &validatorStub{}, identityCache, resolver, false, "Dev local-player", nil, compositionGetMethod)
	require.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestNewWorldResolverValidation(t *testing.T) {
	_, err := auth.NewWorldResolver(nil)
	require.Error(t, err)
	_, err = auth.NewWorldResolver(&auth.WorldResolverConfig{})
	require.Error(t, err)
}
