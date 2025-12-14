package auth_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/KirkDiggler/rpg-api/internal/auth"
	authmock "github.com/KirkDiggler/rpg-api/internal/auth/mock"
)

func TestUnaryAuthInterceptor_Success_CacheMiss(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockValidator := authmock.NewMockTokenValidator(ctrl)
	cache := auth.NewTokenCache(5 * time.Minute)

	mockValidator.EXPECT().
		GetCurrentUser(gomock.Any(), "valid-token").
		Return(&auth.DiscordUser{ID: "user-123", Username: "testuser"}, nil)

	interceptor := auth.UnaryAuthInterceptor(mockValidator, cache)

	// Create context with authorization metadata
	md := metadata.New(map[string]string{"authorization": "Discord valid-token"})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	var capturedCtx context.Context
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		capturedCtx = ctx
		return "response", nil
	}

	resp, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, handler)

	require.NoError(t, err)
	assert.Equal(t, "response", resp)
	assert.Equal(t, "user-123", auth.GetPlayerID(capturedCtx))
}

func TestUnaryAuthInterceptor_Success_CacheHit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockValidator := authmock.NewMockTokenValidator(ctrl)
	cache := auth.NewTokenCache(5 * time.Minute)

	// Pre-populate cache
	cache.Set("cached-token", "cached-user-456")

	// No Discord call expected - cache hit
	interceptor := auth.UnaryAuthInterceptor(mockValidator, cache)

	md := metadata.New(map[string]string{"authorization": "Discord cached-token"})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	var capturedCtx context.Context
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		capturedCtx = ctx
		return "response", nil
	}

	resp, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, handler)

	require.NoError(t, err)
	assert.Equal(t, "response", resp)
	assert.Equal(t, "cached-user-456", auth.GetPlayerID(capturedCtx))
}

func TestUnaryAuthInterceptor_MissingToken(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockValidator := authmock.NewMockTokenValidator(ctrl)
	cache := auth.NewTokenCache(5 * time.Minute)

	interceptor := auth.UnaryAuthInterceptor(mockValidator, cache)

	// No authorization metadata
	ctx := context.Background()

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		t.Fatal("handler should not be called")
		return nil, nil
	}

	resp, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, handler)

	assert.Nil(t, resp)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestUnaryAuthInterceptor_InvalidTokenFormat(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockValidator := authmock.NewMockTokenValidator(ctrl)
	cache := auth.NewTokenCache(5 * time.Minute)

	interceptor := auth.UnaryAuthInterceptor(mockValidator, cache)

	// Wrong format - not "Discord <token>"
	md := metadata.New(map[string]string{"authorization": "Bearer some-token"})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		t.Fatal("handler should not be called")
		return nil, nil
	}

	resp, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, handler)

	assert.Nil(t, resp)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestUnaryAuthInterceptor_InvalidToken(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockValidator := authmock.NewMockTokenValidator(ctrl)
	cache := auth.NewTokenCache(5 * time.Minute)

	mockValidator.EXPECT().
		GetCurrentUser(gomock.Any(), "invalid-token").
		Return(nil, auth.ErrInvalidToken)

	interceptor := auth.UnaryAuthInterceptor(mockValidator, cache)

	md := metadata.New(map[string]string{"authorization": "Discord invalid-token"})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		t.Fatal("handler should not be called")
		return nil, nil
	}

	resp, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, handler)

	assert.Nil(t, resp)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

func TestUnaryAuthInterceptor_DiscordUnavailable(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockValidator := authmock.NewMockTokenValidator(ctrl)
	cache := auth.NewTokenCache(5 * time.Minute)

	mockValidator.EXPECT().
		GetCurrentUser(gomock.Any(), "some-token").
		Return(nil, auth.ErrDiscordUnavailable)

	interceptor := auth.UnaryAuthInterceptor(mockValidator, cache)

	md := metadata.New(map[string]string{"authorization": "Discord some-token"})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		t.Fatal("handler should not be called")
		return nil, nil
	}

	resp, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, handler)

	assert.Nil(t, resp)
	assert.Equal(t, codes.Unavailable, status.Code(err))
}
