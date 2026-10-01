package auth

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	worldrepo "github.com/KirkDiggler/rpg-api/internal/repositories/world"
	"github.com/KirkDiggler/rpg-api/internal/worldcontext"
)

const maximumStreamRefresh = 15 * time.Second

// RoleAccessConfig supplies world policy independently of handler ownership checks.
type RoleAccessConfig struct {
	Resolver WorldResolver
	Worlds   worldrepo.Repository
	// DevelopmentPermissions applies only to authenticated Dev credentials.
	DevelopmentPermissions Permissions
	DevelopmentPlayers     map[string]Permissions
	// StreamRefresh is at most 15 seconds; renewal has the same timeout so
	// permission loss/provider outage ends an idle stream within 30 seconds.
	StreamRefresh time.Duration
}

type RoleAccess struct{ config RoleAccessConfig }

func NewRoleAccess(cfg *RoleAccessConfig) (*RoleAccess, error) {
	if cfg == nil || cfg.Resolver == nil || cfg.Worlds == nil {
		return nil, fmt.Errorf("role access requires world resolution and repository")
	}
	config := *cfg
	if config.StreamRefresh == 0 {
		config.StreamRefresh = maximumStreamRefresh
	}
	if config.StreamRefresh < 0 || config.StreamRefresh > maximumStreamRefresh {
		return nil, fmt.Errorf("stream permission refresh must be positive and at most %s", maximumStreamRefresh)
	}
	config.DevelopmentPlayers = make(map[string]Permissions, len(cfg.DevelopmentPlayers))
	for player, permissions := range cfg.DevelopmentPlayers {
		config.DevelopmentPlayers[player] = permissions
	}
	return &RoleAccess{config: config}, nil
}

// UnaryInterceptor installs trusted world context only after role admission.
func (a *RoleAccess) UnaryInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		if shouldSkipAuth(info.FullMethod) || isWorldManagementMethod(info.FullMethod) {
			return handler(withoutRequestAuth(ctx), req)
		}
		world, err := a.authorize(ctx, info.FullMethod, false)
		if err != nil {
			return nil, err
		}
		return handler(worldcontext.With(withoutRequestAuth(ctx), *world), req)
	}
}

// StreamInterceptor gates initial subscription and renews authority even when idle.
// The handler sees trusted context, never the credential retained by the auth boundary.
func (a *RoleAccess) StreamInterceptor() grpc.StreamServerInterceptor {
	return func(srv interface{}, ss grpc.ServerStream, info *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
		if shouldSkipAuth(info.FullMethod) {
			return handler(srv, &wrappedServerStream{ServerStream: ss, ctx: withoutRequestAuth(ss.Context())})
		}
		world, err := a.authorize(ss.Context(), info.FullMethod, true)
		if err != nil {
			return err
		}
		ctx, cancel := context.WithCancelCause(ss.Context())
		defer cancel(nil)
		done := make(chan struct{})
		go func() {
			defer close(done)
			ticker := time.NewTicker(a.config.StreamRefresh)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					checkCtx, stop := context.WithTimeout(ctx, a.config.StreamRefresh)
					_, refreshErr := a.authorize(checkCtx, info.FullMethod, true)
					stop()
					if refreshErr != nil {
						cancel(refreshErr)
						return
					}
				}
			}
		}()
		wrapped := &wrappedServerStream{ServerStream: ss, ctx: worldcontext.With(withoutRequestAuth(ctx), *world)}
		handlerErr := handler(srv, wrapped)
		cause := context.Cause(ctx)
		cancel(nil)
		<-done
		if cause != nil && status.Code(cause) != codes.Unknown {
			return cause
		}
		return handlerErr
	}
}

func (a *RoleAccess) authorize(ctx context.Context, method string, fresh bool) (*worldcontext.Value, error) {
	required, known := GameMethodPermission(method)
	if !known {
		return nil, status.Error(codes.PermissionDenied, "RPC has no configured access policy")
	}
	credential, ok := getRequestAuth(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "authenticated request is required")
	}
	input := &ResolveWorldInput{ForceRefresh: fresh}
	if credential.scheme != authSchemeDev {
		guildID, err := guildSelector(ctx)
		if err != nil {
			return nil, err
		}
		input.GuildID = guildID
	}
	world, err := a.config.Resolver.Resolve(ctx, input)
	if err != nil {
		return nil, err
	}
	if world == nil || world.WorldID == "" {
		return nil, status.Error(codes.Internal, "world resolver returned no world")
	}
	var permissions Permissions
	if credential.scheme == authSchemeDev {
		permissions = a.config.DevelopmentPermissions
		if override, exists := a.config.DevelopmentPlayers[GetPlayerID(ctx)]; exists {
			permissions = override
		}
	} else {
		stored, err := a.config.Worlds.Get(ctx, &worldrepo.GetInput{WorldID: world.WorldID})
		if apierr.IsNotFound(err) {
			return nil, status.Error(codes.FailedPrecondition, "this server's world access must be configured by its owner")
		}
		if err != nil {
			return nil, apierr.ToGRPCError(err)
		}
		if stored == nil || stored.World == nil {
			return nil, status.Error(codes.Internal, "world repository returned no configuration")
		}
		grant, err := EvaluatePermissions(&EvaluatePermissionsInput{Config: *stored.World, WorldID: world.WorldID, AssignedRoleIDs: world.AssignedRoleIDs})
		if err != nil {
			return nil, status.Error(codes.Internal, "stored world access is invalid")
		}
		permissions = grant.Permissions
	}
	if !permissions.Allows(required) {
		return nil, status.Error(codes.PermissionDenied, "your Discord roles do not grant access to this action")
	}
	return &worldcontext.Value{WorldID: world.WorldID}, nil
}
