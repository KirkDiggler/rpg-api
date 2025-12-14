package auth

import (
	"context"
	"errors"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

const (
	authorizationHeader = "authorization"
	discordScheme       = "Discord "
)

// UnaryAuthInterceptor returns a gRPC unary interceptor that validates Discord tokens.
func UnaryAuthInterceptor(validator TokenValidator, cache *TokenCache) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		newCtx, err := authenticate(ctx, validator, cache)
		if err != nil {
			return nil, err
		}
		return handler(newCtx, req)
	}
}

// StreamAuthInterceptor returns a gRPC stream interceptor that validates Discord tokens.
func StreamAuthInterceptor(validator TokenValidator, cache *TokenCache) grpc.StreamServerInterceptor {
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		ctx := ss.Context()
		newCtx, err := authenticate(ctx, validator, cache)
		if err != nil {
			return err
		}
		wrapped := &wrappedServerStream{
			ServerStream: ss,
			ctx:          newCtx,
		}
		return handler(srv, wrapped)
	}
}

type wrappedServerStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedServerStream) Context() context.Context {
	return w.ctx
}

// authenticate extracts and validates the Discord token from the request context.
func authenticate(ctx context.Context, validator TokenValidator, cache *TokenCache) (context.Context, error) {
	token, err := extractToken(ctx)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}

	// Check cache first
	if userID, ok := cache.Get(token); ok {
		return WithPlayerID(ctx, userID), nil
	}

	// Cache miss - validate with Discord
	user, err := validator.GetCurrentUser(ctx, token)
	if err != nil {
		if errors.Is(err, ErrInvalidToken) {
			return nil, status.Error(codes.Unauthenticated, "invalid Discord token")
		}
		if errors.Is(err, ErrDiscordUnavailable) {
			return nil, status.Error(codes.Unavailable, "Discord API unavailable")
		}
		return nil, status.Error(codes.Internal, "authentication failed")
	}

	// Cache the result
	cache.Set(token, user.ID)

	return WithPlayerID(ctx, user.ID), nil
}

// extractToken extracts the Discord token from the authorization header.
func extractToken(ctx context.Context) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", ErrMissingToken
	}

	values := md.Get(authorizationHeader)
	if len(values) == 0 {
		return "", ErrMissingToken
	}

	authHeader := values[0]
	if !strings.HasPrefix(authHeader, discordScheme) {
		return "", ErrInvalidTokenFormat
	}

	token := strings.TrimPrefix(authHeader, discordScheme)
	if token == "" {
		return "", ErrMissingToken
	}

	return token, nil
}
