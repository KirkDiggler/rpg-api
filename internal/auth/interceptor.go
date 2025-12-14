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
	devScheme           = "Dev "
)

// InterceptorConfig configures the auth interceptors.
type InterceptorConfig struct {
	// DevMode allows the "Dev <player_id>" scheme for local development.
	// When true, requests with Dev scheme bypass Discord validation.
	// NEVER enable in production!
	DevMode bool
}

// UnaryAuthInterceptor returns a gRPC unary interceptor that validates Discord tokens.
func UnaryAuthInterceptor(validator TokenValidator, cache *TokenCache, cfg *InterceptorConfig) grpc.UnaryServerInterceptor {
	devMode := cfg != nil && cfg.DevMode
	return func(
		ctx context.Context,
		req interface{},
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (interface{}, error) {
		newCtx, err := authenticate(ctx, validator, cache, devMode)
		if err != nil {
			return nil, err
		}
		return handler(newCtx, req)
	}
}

// StreamAuthInterceptor returns a gRPC stream interceptor that validates Discord tokens.
func StreamAuthInterceptor(validator TokenValidator, cache *TokenCache, cfg *InterceptorConfig) grpc.StreamServerInterceptor {
	devMode := cfg != nil && cfg.DevMode
	return func(
		srv interface{},
		ss grpc.ServerStream,
		info *grpc.StreamServerInfo,
		handler grpc.StreamHandler,
	) error {
		ctx := ss.Context()
		newCtx, err := authenticate(ctx, validator, cache, devMode)
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

// authenticate extracts and validates authentication from the request context.
func authenticate(ctx context.Context, validator TokenValidator, cache *TokenCache, devMode bool) (context.Context, error) {
	info, err := extractAuthInfo(ctx)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, err.Error())
	}

	// Handle Dev scheme (development only)
	if info.scheme == "dev" {
		if !devMode {
			return nil, status.Error(codes.Unauthenticated, "Dev authentication not allowed")
		}
		// In dev mode, use the player ID directly without Discord validation
		return WithPlayerID(ctx, info.value), nil
	}

	// Handle Discord scheme - validate token
	token := info.value

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

// authInfo contains extracted authentication information.
type authInfo struct {
	scheme string
	value  string // Token for Discord scheme, player ID for Dev scheme
}

// extractAuthInfo extracts authentication info from the authorization header.
// Supports two schemes:
//   - "Discord <token>" - Real Discord auth, token validated with Discord API
//   - "Dev <player_id>" - Development mode, player ID used directly (requires devMode)
func extractAuthInfo(ctx context.Context) (*authInfo, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, ErrMissingToken
	}

	values := md.Get(authorizationHeader)
	if len(values) == 0 {
		return nil, ErrMissingToken
	}

	authHeader := values[0]

	// Check for Discord scheme
	if strings.HasPrefix(authHeader, discordScheme) {
		token := strings.TrimPrefix(authHeader, discordScheme)
		if token == "" {
			return nil, ErrMissingToken
		}
		return &authInfo{scheme: "discord", value: token}, nil
	}

	// Check for Dev scheme
	if strings.HasPrefix(authHeader, devScheme) {
		playerID := strings.TrimPrefix(authHeader, devScheme)
		if playerID == "" {
			return nil, ErrMissingToken
		}
		return &authInfo{scheme: "dev", value: playerID}, nil
	}

	return nil, ErrInvalidTokenFormat
}
