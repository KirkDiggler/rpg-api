package auth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

type internalResolverStub struct{}

func (internalResolverStub) Resolve(context.Context, *ResolveWorldInput) (*ResolveWorldOutput, error) {
	return &ResolveWorldOutput{WorldID: "123456789012345678"}, nil
}

func TestWorldInterceptorRemovesPrivateRequestAuthBeforeHandler(t *testing.T) {
	ctx := withAuthenticatedRequest(context.Background(), "player-1", &requestAuth{
		scheme: authSchemeDiscord,
		value:  "private-request-credential",
	})
	ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(guildSelectorHeader, "123456789012345678"))
	interceptor := UnaryWorldContextInterceptor(internalResolverStub{})

	_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/api.composition.v1alpha1.CompositionService/GetComposition"}, func(handlerContext context.Context, _ interface{}) (interface{}, error) {
		_, ok := getRequestAuth(handlerContext)
		require.False(t, ok)
		require.Equal(t, "player-1", GetPlayerID(handlerContext))
		return nil, nil
	})
	require.NoError(t, err)
}

func TestWorldInterceptorRemovesPrivateRequestAuthForGlobalHandler(t *testing.T) {
	ctx := withAuthenticatedRequest(context.Background(), "player-1", &requestAuth{
		scheme: authSchemeDiscord,
		value:  "private-request-credential",
	})
	interceptor := UnaryWorldContextInterceptor(internalResolverStub{})

	_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/dnd5e.api.v1alpha1.CharacterService/ListCharacters"}, func(handlerContext context.Context, _ interface{}) (interface{}, error) {
		_, ok := getRequestAuth(handlerContext)
		require.False(t, ok)
		require.Equal(t, "player-1", GetPlayerID(handlerContext))
		return nil, nil
	})
	require.NoError(t, err)
}
