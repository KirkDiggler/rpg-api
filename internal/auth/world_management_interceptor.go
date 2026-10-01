package auth

import (
	"context"
	"errors"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	worldpb "github.com/KirkDiggler/rpg-api-protos/gen/go/api/world/v1alpha1"

	"github.com/KirkDiggler/rpg-api/internal/worldcontext"
)

// WorldManagementConfig supplies provider verification independently of gameplay admission.
type WorldManagementConfig struct {
	Resolver        WorldResolver
	Ownership       OwnershipVerifier
	IdentityCache   *TokenCache
	MembershipCache *MembershipCache
	// DevelopmentOwner is honored only for an already-authenticated Dev scheme.
	// Production server wiring never enables it.
	DevelopmentOwner         bool
	DevelopmentOwnerPlayerID string
}

// UnaryWorldManagementInterceptor permits verified owner bootstrap without a configured game role.
// It runs after identity auth and before the role gate strips the private credential.
func UnaryWorldManagementInterceptor(cfg *WorldManagementConfig) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		switch info.FullMethod {
		case worldpb.WorldService_GetWorld_FullMethodName, worldpb.WorldService_SetWorldRoles_FullMethodName,
			worldpb.WorldService_SetWorldMemberRoles_FullMethodName:
		default:
			return handler(ctx, req)
		}
		if cfg == nil || cfg.Resolver == nil || cfg.Ownership == nil {
			return nil, status.Error(codes.Internal, "world management verifier is not configured")
		}
		credential, ok := getRequestAuth(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "authenticated request is required")
		}
		// Administrative authority must not survive Discord-side role removal
		// through an otherwise valid cached membership snapshot.
		input := &ResolveWorldInput{ForceRefresh: true}
		if credential.scheme != authSchemeDev {
			guildID, err := guildSelector(ctx)
			if err != nil {
				return nil, err
			}
			input.GuildID = guildID
		}
		world, err := cfg.Resolver.Resolve(ctx, input)
		if err != nil {
			return nil, err
		}
		if world == nil || world.WorldID == "" {
			return nil, status.Error(codes.Internal, "world resolver returned no world")
		}
		owner := cfg.DevelopmentOwner && credential.scheme == authSchemeDev &&
			cfg.DevelopmentOwnerPlayerID != "" && GetPlayerID(ctx) == cfg.DevelopmentOwnerPlayerID
		if credential.scheme == authSchemeDiscord {
			ownership, err := cfg.Ownership.GetGuildOwnership(ctx, &GetGuildOwnershipInput{Token: credential.value, GuildID: world.WorldID})
			if err != nil {
				switch {
				case errors.Is(err, ErrInvalidToken):
					if cfg.IdentityCache != nil {
						cfg.IdentityCache.Delete(credential.value)
					}
					if cfg.MembershipCache != nil {
						cfg.MembershipCache.DeleteToken(credential.value)
					}
					return nil, status.Error(codes.Unauthenticated, "invalid Discord token")
				case errors.Is(err, ErrGuildMembershipDenied):
					return nil, status.Error(codes.PermissionDenied, "Discord guild membership denied")
				default:
					return nil, status.Error(codes.Unavailable, "Discord ownership verification unavailable")
				}
			}
			if ownership == nil {
				return nil, status.Error(codes.Unavailable, "Discord returned no ownership decision")
			}
			owner = ownership.Owner
		}
		ctx = worldcontext.With(withoutRequestAuth(ctx), worldcontext.Value{
			WorldID: world.WorldID, Owner: owner, AssignedRoleIDs: append([]string(nil), world.AssignedRoleIDs...),
		})
		return handler(ctx, req)
	}
}
