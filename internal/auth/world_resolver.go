package auth

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ResolveWorldInput contains the untrusted, already syntax-validated selector.
type ResolveWorldInput struct {
	GuildID string
}

// ResolveWorldOutput contains only the trusted toolkit-domain identity.
type ResolveWorldOutput struct {
	WorldID string
}

// WorldResolver derives a trusted world for an authenticated request.
type WorldResolver interface {
	Resolve(context.Context, *ResolveWorldInput) (*ResolveWorldOutput, error)
}

// WorldResolverConfig configures Discord membership and explicit Dev resolution.
type WorldResolverConfig struct {
	MembershipVerifier MembershipVerifier
	IdentityCache      *TokenCache
	MembershipCache    *MembershipCache
	DevWorldID         string
}

type worldResolver struct {
	membershipVerifier MembershipVerifier
	identityCache      *TokenCache
	membershipCache    *MembershipCache
	devWorldID         string
}

// NewWorldResolver creates the narrow resolver used by composition requests.
func NewWorldResolver(cfg *WorldResolverConfig) (WorldResolver, error) {
	if cfg == nil {
		return nil, fmt.Errorf("world resolver config is required")
	}
	if cfg.MembershipVerifier == nil {
		return nil, fmt.Errorf("membership verifier is required")
	}
	if cfg.IdentityCache == nil {
		return nil, fmt.Errorf("identity cache is required")
	}
	if cfg.MembershipCache == nil {
		return nil, fmt.Errorf("membership cache is required")
	}
	return &worldResolver{
		membershipVerifier: cfg.MembershipVerifier,
		identityCache:      cfg.IdentityCache,
		membershipCache:    cfg.MembershipCache,
		devWorldID:         cfg.DevWorldID,
	}, nil
}

func (r *worldResolver) Resolve(ctx context.Context, input *ResolveWorldInput) (*ResolveWorldOutput, error) {
	request, ok := getRequestAuth(ctx)
	if !ok {
		return nil, status.Error(codes.FailedPrecondition, "authenticated request context is missing")
	}
	playerID := GetPlayerID(ctx)
	if playerID == "" {
		return nil, status.Error(codes.Unauthenticated, "player is not authenticated")
	}

	switch request.scheme {
	case authSchemeDev:
		if r.devWorldID == "" {
			return nil, status.Error(codes.FailedPrecondition, "development world is not configured")
		}
		return &ResolveWorldOutput{WorldID: r.devWorldID}, nil
	case authSchemeDiscord:
		return r.resolveDiscord(ctx, input, request.value, playerID)
	default:
		return nil, status.Error(codes.Unauthenticated, "unsupported authentication scheme")
	}
}

func (r *worldResolver) resolveDiscord(ctx context.Context, input *ResolveWorldInput, token, playerID string) (*ResolveWorldOutput, error) {
	if input == nil || input.GuildID == "" {
		return nil, status.Error(codes.FailedPrecondition, "guild selector is required")
	}
	if decision, ok := r.membershipCache.Get(token, input.GuildID); ok &&
		decision.PlayerID == playerID && decision.WorldID == input.GuildID {
		return &ResolveWorldOutput{WorldID: decision.WorldID}, nil
	}

	member, err := r.membershipVerifier.GetCurrentUserGuildMember(ctx, &GetCurrentUserGuildMemberInput{
		Token: token, GuildID: input.GuildID,
	})
	if err != nil {
		switch {
		case errors.Is(err, ErrInvalidToken):
			r.identityCache.Delete(token)
			r.membershipCache.DeleteToken(token)
			return nil, status.Error(codes.Unauthenticated, "invalid Discord token")
		case errors.Is(err, ErrGuildMembershipDenied):
			return nil, status.Error(codes.PermissionDenied, "guild membership denied")
		default:
			return nil, status.Error(codes.Unavailable, "Discord membership verification unavailable")
		}
	}
	if member == nil || member.User == nil || member.User.ID == "" || member.User.ID != playerID {
		return nil, status.Error(codes.Unavailable, "Discord returned unusable membership data")
	}

	decision := MembershipDecision{PlayerID: playerID, WorldID: input.GuildID}
	r.membershipCache.Set(token, input.GuildID, decision)
	return &ResolveWorldOutput{WorldID: input.GuildID}, nil
}
