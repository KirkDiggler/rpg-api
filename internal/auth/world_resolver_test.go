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
	worldGetMethod       = "/api.world.v1alpha1.WorldService/GetWorld"
	canonicalGuildID     = "123456789012345678"
	// devWorldB is the second local simulation fixture world (issue #522).
	devWorldB = "223456789012345678"
)

type ownershipStub struct{ owner bool }

func (s ownershipStub) GetGuildOwnership(context.Context, *auth.GetGuildOwnershipInput) (*auth.GetGuildOwnershipOutput, error) {
	return &auth.GetGuildOwnershipOutput{Owner: s.owner}, nil
}

func newDevAllowlistResolver(t *testing.T, verifier auth.MembershipVerifier, defaultWorld string, list []string) auth.WorldResolver {
	t.Helper()
	resolver, err := auth.NewWorldResolver(&auth.WorldResolverConfig{
		MembershipVerifier: verifier,
		IdentityCache:      auth.NewTokenCache(5 * time.Minute),
		MembershipCache:    newMembershipCache(t),
		DevWorldID:         defaultWorld,
		DevWorldIDs:        list,
	})
	require.NoError(t, err)
	return resolver
}

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

func TestWorldResolverExpiredMembershipDoesNotFallBackOnProviderFailure(t *testing.T) {
	now := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	membershipCache, err := auth.NewMembershipCache(&auth.MembershipCacheConfig{
		TTL:        30 * time.Second,
		MaxEntries: 1024,
		Now:        func() time.Time { return now },
	})
	require.NoError(t, err)
	membershipCache.Set("expiring-credential", canonicalGuildID, auth.MembershipDecision{PlayerID: "player-1", WorldID: canonicalGuildID})
	identityCache := auth.NewTokenCache(5 * time.Minute)
	identityCache.Set("expiring-credential", "player-1")
	verifier := &verifierStub{err: auth.ErrDiscordUnavailable}
	resolver, err := auth.NewWorldResolver(&auth.WorldResolverConfig{
		MembershipVerifier: verifier,
		IdentityCache:      identityCache,
		MembershipCache:    membershipCache,
	})
	require.NoError(t, err)

	now = now.Add(30 * time.Second)
	_, err = invokeWorldChain(t, &validatorStub{}, identityCache, resolver, false, "Discord expiring-credential", []string{canonicalGuildID}, compositionGetMethod)
	require.Equal(t, codes.Unavailable, status.Code(err))
	require.Equal(t, 1, verifier.callCount())
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

func TestWorldResolverDevAllowlistSelectsConfiguredWorld(t *testing.T) {
	verifier := &verifierStub{}
	identityCache := auth.NewTokenCache(5 * time.Minute)
	resolver := newDevAllowlistResolver(t, verifier, canonicalGuildID, []string{canonicalGuildID, devWorldB})

	tests := []struct {
		name  string
		guild []string
		want  string
	}{
		{name: "absent selector uses configured default", guild: nil, want: canonicalGuildID},
		{name: "selector A", guild: []string{canonicalGuildID}, want: canonicalGuildID},
		{name: "selector B", guild: []string{devWorldB}, want: devWorldB},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, err := invokeWorldChain(t, &validatorStub{}, identityCache, resolver, true, "Dev local-player", tt.guild, compositionGetMethod)
			require.NoError(t, err)
			value, ok := worldcontext.Get(ctx)
			require.True(t, ok)
			require.Equal(t, tt.want, value.WorldID)
		})
	}
	require.Zero(t, verifier.callCount(), "the Dev allowlist must not consult Discord membership")
}

func TestWorldResolverDevAllowlistDeniesBadSelector(t *testing.T) {
	identityCache := auth.NewTokenCache(5 * time.Minute)
	resolver := newDevAllowlistResolver(t, &verifierStub{}, canonicalGuildID, []string{canonicalGuildID, devWorldB})

	tests := []struct {
		name  string
		guild []string
		code  codes.Code
	}{
		{name: "unknown canonical world", guild: []string{"323456789012345678"}, code: codes.PermissionDenied},
		{name: "multiple selectors", guild: []string{canonicalGuildID, devWorldB}, code: codes.InvalidArgument},
		{name: "combined selectors", guild: []string{canonicalGuildID + "," + devWorldB}, code: codes.InvalidArgument},
		{name: "malformed selector", guild: []string{"not-a-guild"}, code: codes.InvalidArgument},
		{name: "leading zero selector", guild: []string{"0123456789012345678"}, code: codes.InvalidArgument},
		{name: "empty selector", guild: []string{""}, code: codes.InvalidArgument},
		{name: "overflowing selector", guild: []string{"18446744073709551616"}, code: codes.InvalidArgument},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, err := invokeWorldChain(t, &validatorStub{}, identityCache, resolver, true, "Dev local-player", tt.guild, compositionGetMethod)
			require.Nil(t, ctx)
			require.Equal(t, tt.code, status.Code(err))
		})
	}
}

func TestWorldResolverDevWithoutAllowlistIgnoresCanonicalSelector(t *testing.T) {
	resolver := newDevAllowlistResolver(t, &verifierStub{}, "configured-dev-world", nil)
	ctx, err := invokeWorldChain(t, &validatorStub{}, auth.NewTokenCache(5*time.Minute), resolver, true, "Dev local-player", []string{devWorldB}, compositionGetMethod)
	require.NoError(t, err)
	value, ok := worldcontext.Get(ctx)
	require.True(t, ok)
	require.Equal(t, "configured-dev-world", value.WorldID, "without an allowlist a canonical selector is still untrusted")
}

func TestNewWorldResolverRejectsMalformedDevAllowlist(t *testing.T) {
	tests := []struct {
		name         string
		defaultWorld string
		list         []string
	}{
		{name: "empty entry", defaultWorld: canonicalGuildID, list: []string{""}},
		{name: "non decimal entry", defaultWorld: canonicalGuildID, list: []string{"not-a-guild"}},
		{name: "leading zero entry", defaultWorld: canonicalGuildID, list: []string{"0123456789012345678"}},
		{name: "overflowing entry", defaultWorld: canonicalGuildID, list: []string{"18446744073709551616"}},
		{name: "zero entry", defaultWorld: canonicalGuildID, list: []string{"0"}},
		{name: "whitespace-bearing entry", defaultWorld: canonicalGuildID, list: []string{"123 456"}},
		{name: "duplicate entry", defaultWorld: canonicalGuildID, list: []string{canonicalGuildID, canonicalGuildID}},
		{name: "default omitted", defaultWorld: canonicalGuildID, list: []string{devWorldB}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := auth.NewWorldResolver(&auth.WorldResolverConfig{
				MembershipVerifier: &verifierStub{},
				IdentityCache:      auth.NewTokenCache(5 * time.Minute),
				MembershipCache:    newMembershipCache(t),
				DevWorldID:         tt.defaultWorld,
				DevWorldIDs:        tt.list,
			})
			require.Error(t, err, "malformed allowlist configuration must fail closed")
		})
	}
}

func TestWorldResolverDiscordUsesSelectedGuildDespiteDevAllowlist(t *testing.T) {
	verifier := &verifierStub{member: &auth.DiscordGuildMember{User: &auth.DiscordUser{ID: "player-1"}, Roles: []string{"role-1"}}}
	identityCache := auth.NewTokenCache(5 * time.Minute)
	identityCache.Set("discord-credential", "player-1")
	resolver := newDevAllowlistResolver(t, verifier, canonicalGuildID, []string{canonicalGuildID, devWorldB})

	ctx, err := invokeWorldChain(t, &validatorStub{}, identityCache, resolver, true, "Discord discord-credential", []string{devWorldB}, compositionGetMethod)
	require.NoError(t, err)
	value, ok := worldcontext.Get(ctx)
	require.True(t, ok)
	require.Equal(t, devWorldB, value.WorldID, "Discord credentials must resolve through real membership, not the Dev allowlist")
	require.Equal(t, 1, verifier.callCount())
	require.Len(t, verifier.inputs, 1)
	require.Equal(t, devWorldB, verifier.inputs[0].GuildID, "membership verification must use the selected guild")
}

func TestWorldResolverDiscordDeniesInvalidMembershipWithDevAllowlist(t *testing.T) {
	verifier := &verifierStub{err: auth.ErrGuildMembershipDenied}
	identityCache := auth.NewTokenCache(5 * time.Minute)
	identityCache.Set("discord-credential", "player-1")
	resolver := newDevAllowlistResolver(t, verifier, canonicalGuildID, []string{canonicalGuildID, devWorldB})

	_, err := invokeWorldChain(t, &validatorStub{}, identityCache, resolver, true, "Discord discord-credential", []string{devWorldB}, compositionGetMethod)
	require.Equal(t, codes.PermissionDenied, status.Code(err))
	require.Equal(t, 1, verifier.callCount())
}

func TestWorldResolverProductionDevCredentialDeniedDespiteAllowlist(t *testing.T) {
	identityCache := auth.NewTokenCache(5 * time.Minute)
	resolver := newDevAllowlistResolver(t, &verifierStub{}, canonicalGuildID, []string{canonicalGuildID, devWorldB})

	// Dev mode is off: the same allowlist-configured resolver must still refuse
	// the Dev scheme before any world is selected.
	_, err := invokeWorldChain(t, &validatorStub{}, identityCache, resolver, false, "Dev local-player", []string{devWorldB}, compositionGetMethod)
	require.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestWorldManagementInterceptorDevAllowlistSelectsWorld(t *testing.T) {
	identityCache := auth.NewTokenCache(5 * time.Minute)
	resolver := newDevAllowlistResolver(t, &verifierStub{}, canonicalGuildID, []string{canonicalGuildID, devWorldB})
	management := auth.UnaryWorldManagementInterceptor(&auth.WorldManagementConfig{
		Resolver: resolver, Ownership: ownershipStub{},
		IdentityCache: identityCache, MembershipCache: newMembershipCache(t),
	})

	tests := []struct {
		name  string
		guild []string
		world string
		code  codes.Code
	}{
		{name: "selected world A reaches handler", guild: []string{devWorldB}, world: devWorldB},
		{name: "absent selector uses default", guild: nil, world: canonicalGuildID},
		{name: "unknown selector denied", guild: []string{"323456789012345678"}, code: codes.PermissionDenied},
		{name: "malformed selector denied", guild: []string{"not-a-guild"}, code: codes.InvalidArgument},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			md := metadata.MD{}
			md.Set("authorization", "Dev local-player")
			if tt.guild != nil {
				md["x-rpg-guild-id"] = tt.guild
			}
			ctx := metadata.NewIncomingContext(context.Background(), md)

			authInterceptor := auth.UnaryAuthInterceptor(&validatorStub{}, identityCache, &auth.InterceptorConfig{DevMode: true})
			handlerCalled := false
			var handlerCtx context.Context
			_, err := authInterceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: worldGetMethod}, func(ctx context.Context, req interface{}) (interface{}, error) {
				return management(ctx, req, &grpc.UnaryServerInfo{FullMethod: worldGetMethod}, func(ctx context.Context, _ interface{}) (interface{}, error) {
					handlerCalled = true
					handlerCtx = ctx
					return nil, nil
				})
			})
			if tt.code != codes.OK {
				require.Equal(t, tt.code, status.Code(err))
				require.False(t, handlerCalled, "a denied selector must not reach the handler")
				return
			}
			require.NoError(t, err)
			require.True(t, handlerCalled)
			value, present := worldcontext.Get(handlerCtx)
			require.True(t, present)
			require.Equal(t, tt.world, value.WorldID)
		})
	}
}
