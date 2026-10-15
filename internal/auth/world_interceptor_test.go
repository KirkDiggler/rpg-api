package auth_test

import (
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

// dungeonWorldMethods are the seven dungeon methods whose store is per world
// (rpg-project#585).
var dungeonWorldMethods = []string{
	"/dnd5e.api.authoring.v1alpha1.AuthoringService/PutDungeon",
	"/dnd5e.api.authoring.v1alpha1.AuthoringService/GetDungeon",
	"/dnd5e.api.authoring.v1alpha1.AuthoringService/ListDungeons",
	"/dnd5e.api.authoring.v1alpha1.AuthoringService/DeleteDungeon",
	"/dnd5e.api.authoring.v1alpha1.AuthoringService/RenameDungeon",
	"/dnd5e.api.lobby.v1alpha1.LobbyService/ListDungeons",
	"/dnd5e.api.lobby.v1alpha1.LobbyService/StartEncounter",
}

func TestWorldInterceptorAppliesOnlyToWorldScopedUnaryMethods(t *testing.T) {
	methods := append([]string{
		"/api.composition.v1alpha1.CompositionService/CreateComposition",
		"/api.composition.v1alpha1.CompositionService/GetComposition",
		"/api.composition.v1alpha1.CompositionService/ListCompositions",
		"/api.composition.v1alpha1.CompositionService/DeleteComposition",
	}, dungeonWorldMethods...)
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
	require.False(t, ok, "methods outside the world-scoped set must not acquire world context")
}

// TestWorldInterceptorDungeonMethodsUnderTheDevScheme: under AUTH_DEV_MODE
// the Dev scheme resolves to the configured dev world with no guild
// selector; a Discord caller without one is refused, the composition
// precedent (S7-Q9).
func TestWorldInterceptorDungeonMethodsUnderTheDevScheme(t *testing.T) {
	for _, method := range dungeonWorldMethods {
		t.Run(method, func(t *testing.T) {
			identityCache := auth.NewTokenCache(5 * time.Minute)
			identityCache.Set("discord-credential", "player-1")
			verifier := &verifierStub{member: &auth.DiscordGuildMember{User: &auth.DiscordUser{ID: "player-1"}}}
			resolver, err := auth.NewWorldResolver(&auth.WorldResolverConfig{
				MembershipVerifier: verifier,
				IdentityCache:      identityCache,
				MembershipCache:    newMembershipCache(t),
				DevWorldID:         "test-world",
			})
			require.NoError(t, err)

			ctx, err := invokeWorldChain(t, &validatorStub{}, identityCache, resolver, true, "Dev local-player", nil, method)
			require.NoError(t, err)
			value, ok := worldcontext.Get(ctx)
			require.True(t, ok)
			require.Equal(t, "test-world", value.WorldID)
			require.Equal(t, "local-player", auth.GetPlayerID(ctx))

			ctx, err = invokeWorldChain(t, &validatorStub{}, identityCache, resolver, false, "Discord discord-credential", nil, method)
			require.Nil(t, ctx)
			require.Equal(t, codes.FailedPrecondition, status.Code(err))
			require.Equal(t, "guild selector is required", status.Convert(err).Message())
		})
	}

	identityCache := auth.NewTokenCache(5 * time.Minute)
	resolver, err := auth.NewWorldResolver(&auth.WorldResolverConfig{
		MembershipVerifier: &verifierStub{},
		IdentityCache:      identityCache,
		MembershipCache:    newMembershipCache(t),
		DevWorldID:         "test-world",
	})
	require.NoError(t, err)
	ctx, err := invokeWorldChain(t, &validatorStub{}, identityCache, resolver, true, "Dev local-player", nil,
		"/dnd5e.api.authoring.v1alpha1.AuthoringService/ListScenarios")
	require.NoError(t, err)
	_, ok := worldcontext.Get(ctx)
	require.False(t, ok, "an authoring method outside the set gets none")
}
