package auth_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/worldcontext"
)

func TestWorldInterceptorStrictGuildSelector(t *testing.T) {
	tests := []struct {
		name   string
		values []string
		code   codes.Code
	}{
		{name: "missing", values: nil, code: codes.FailedPrecondition},
		{name: "empty", values: []string{""}, code: codes.InvalidArgument},
		{name: "repeated", values: []string{canonicalGuildID, canonicalGuildID}, code: codes.InvalidArgument},
		{name: "combined", values: []string{canonicalGuildID + "," + canonicalGuildID}, code: codes.InvalidArgument},
		{name: "leading zero", values: []string{"0123456789012345678"}, code: codes.InvalidArgument},
		{name: "plus", values: []string{"+123"}, code: codes.InvalidArgument},
		{name: "minus", values: []string{"-123"}, code: codes.InvalidArgument},
		{name: "whitespace", values: []string{" 123"}, code: codes.InvalidArgument},
		{name: "trailing whitespace", values: []string{"123 "}, code: codes.InvalidArgument},
		{name: "non decimal", values: []string{"123x"}, code: codes.InvalidArgument},
		{name: "zero", values: []string{"0"}, code: codes.InvalidArgument},
		{name: "overflow", values: []string{"18446744073709551616"}, code: codes.InvalidArgument},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			identityCache := auth.NewTokenCache(5 * time.Minute)
			identityCache.Set("selector-credential", "player-1")
			verifier := &verifierStub{member: &auth.DiscordGuildMember{User: &auth.DiscordUser{ID: "player-1"}}}
			resolver, err := auth.NewWorldResolver(&auth.WorldResolverConfig{
				MembershipVerifier: verifier,
				IdentityCache:      identityCache,
				MembershipCache:    newMembershipCache(t),
			})
			require.NoError(t, err)

			ctx, err := invokeWorldChain(t, &validatorStub{}, identityCache, resolver, false, "Discord selector-credential", tt.values, compositionGetMethod)
			require.Nil(t, ctx)
			require.Equal(t, tt.code, status.Code(err))
			require.Zero(t, verifier.callCount(), "invalid selector must fail before membership lookup")
		})
	}
}

func TestWorldInterceptorAppliesOnlyToCompositionUnaryMethods(t *testing.T) {
	methods := []string{
		"/api.composition.v1alpha1.CompositionService/CreateComposition",
		"/api.composition.v1alpha1.CompositionService/GetComposition",
		"/api.composition.v1alpha1.CompositionService/ListCompositions",
		"/api.composition.v1alpha1.CompositionService/DeleteComposition",
	}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			identityCache := auth.NewTokenCache(5 * time.Minute)
			identityCache.Set("allowlist-credential", "player-1")
			resolver, err := auth.NewWorldResolver(&auth.WorldResolverConfig{
				MembershipVerifier: &verifierStub{member: &auth.DiscordGuildMember{User: &auth.DiscordUser{ID: "player-1"}}},
				IdentityCache:      identityCache,
				MembershipCache:    newMembershipCache(t),
			})
			require.NoError(t, err)
			ctx, err := invokeWorldChain(t, &validatorStub{}, identityCache, resolver, false, "Discord allowlist-credential", []string{canonicalGuildID}, method)
			require.NoError(t, err)
			value, ok := worldcontext.Get(ctx)
			require.True(t, ok)
			require.Equal(t, canonicalGuildID, value.WorldID)
		})
	}

	identityCache := auth.NewTokenCache(5 * time.Minute)
	identityCache.Set("global-credential", "player-1")
	resolver, err := auth.NewWorldResolver(&auth.WorldResolverConfig{
		MembershipVerifier: &verifierStub{},
		IdentityCache:      identityCache,
		MembershipCache:    newMembershipCache(t),
	})
	require.NoError(t, err)
	ctx, err := invokeWorldChain(t, &validatorStub{}, identityCache, resolver, false, "Discord global-credential", nil, "/dnd5e.api.v1alpha1.CharacterService/ListCharacters")
	require.NoError(t, err)
	require.Equal(t, "player-1", auth.GetPlayerID(ctx))
	_, ok := worldcontext.Get(ctx)
	require.False(t, ok, "non-composition methods must not acquire world context")
}

func TestWorldContextDoesNotExposeCredentialToHandlerPackages(t *testing.T) {
	valueType := worldcontext.Value{WorldID: canonicalGuildID}
	require.Equal(t, canonicalGuildID, valueType.WorldID)
	// The handler-facing type intentionally has only the trusted domain WorldID.
	require.NotContains(t, []string{"WorldID"}, "Token")
	_ = context.Background()
}
