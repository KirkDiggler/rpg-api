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
	// ForceRefresh is used for live stream admission and renewal so an old
	// cached snapshot cannot extend an already-running stream's authority.
	ForceRefresh bool
}

// ResolveWorldOutput contains only the trusted toolkit-domain identity.
type ResolveWorldOutput struct {
	WorldID         string
	AssignedRoleIDs []string
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
	// DevWorldIDs is the optional development allowlist (RPG_DEV_WORLD_IDS).
	// When non-empty, NewWorldResolver requires every entry to be a canonical
	// non-zero uint64, refuses duplicates, and requires DevWorldID to appear in
	// the list. An empty list preserves the fixed single-world Dev behavior.
	DevWorldIDs      []string
	DevelopmentRoles map[string][]string
}

type worldResolver struct {
	membershipVerifier MembershipVerifier
	identityCache      *TokenCache
	membershipCache    *MembershipCache
	devWorldID         string
	devWorldIDs        map[string]struct{}
	developmentRoles   map[string][]string
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
	roles := make(map[string][]string, len(cfg.DevelopmentRoles))
	for player, assigned := range cfg.DevelopmentRoles {
		roles[player] = append([]string(nil), assigned...)
	}
	devWorldIDs, err := developmentWorldAllowlist(cfg.DevWorldID, cfg.DevWorldIDs)
	if err != nil {
		return nil, err
	}
	return &worldResolver{
		developmentRoles:   roles,
		membershipVerifier: cfg.MembershipVerifier,
		identityCache:      cfg.IdentityCache,
		membershipCache:    cfg.MembershipCache,
		devWorldID:         cfg.DevWorldID,
		devWorldIDs:        devWorldIDs,
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
		worldID, err := r.selectDevWorld(ctx)
		if err != nil {
			return nil, err
		}
		return &ResolveWorldOutput{WorldID: worldID, AssignedRoleIDs: append([]string(nil), r.developmentRoles[playerID]...)}, nil
	case authSchemeDiscord:
		return r.resolveDiscord(ctx, input, request.value, playerID)
	default:
		return nil, status.Error(codes.Unauthenticated, "unsupported authentication scheme")
	}
}

// selectDevWorld applies the optional development allowlist to an already
// authenticated Dev request. Without an allowlist the configured default is
// authoritative and any selector is ignored, preserving the fixed-dev-world
// behavior. With one, the selector is read from this authenticated branch so
// no untrusted value can reach gameplay without matching the allowlist.
func (r *worldResolver) selectDevWorld(ctx context.Context) (string, error) {
	if len(r.devWorldIDs) == 0 {
		return r.devWorldID, nil
	}
	selector, present, err := optionalGuildSelector(ctx)
	if err != nil {
		return "", err
	}
	if !present {
		return r.devWorldID, nil
	}
	if _, allowed := r.devWorldIDs[selector]; !allowed {
		return "", status.Error(codes.PermissionDenied, "selected development world is not allowed")
	}
	return selector, nil
}

// developmentWorldAllowlist validates the optional RPG_DEV_WORLD_IDS fixture.
// It never silently repairs malformed configuration: a non-canonical or
// duplicated entry, or a list that omits the configured default, fails server
// construction rather than broadening access.
func developmentWorldAllowlist(defaultWorldID string, configured []string) (map[string]struct{}, error) {
	if len(configured) == 0 {
		return nil, nil
	}
	allowlist := make(map[string]struct{}, len(configured))
	for _, worldID := range configured {
		if !isCanonicalUint64(worldID) {
			return nil, fmt.Errorf("development world allowlist entry %q must be a canonical non-zero uint64", worldID)
		}
		if _, duplicate := allowlist[worldID]; duplicate {
			return nil, fmt.Errorf("development world allowlist entry %q is duplicated", worldID)
		}
		allowlist[worldID] = struct{}{}
	}
	if _, present := allowlist[defaultWorldID]; !present {
		return nil, fmt.Errorf("development world allowlist must include the default world %q", defaultWorldID)
	}
	return allowlist, nil
}

func (r *worldResolver) resolveDiscord(ctx context.Context, input *ResolveWorldInput, token, playerID string) (*ResolveWorldOutput, error) {
	if input == nil || input.GuildID == "" {
		return nil, status.Error(codes.FailedPrecondition, "guild selector is required")
	}
	if decision, ok := r.membershipCache.Get(token, input.GuildID); !input.ForceRefresh && ok &&
		decision.PlayerID == playerID && decision.WorldID == input.GuildID {
		return &ResolveWorldOutput{WorldID: decision.WorldID, AssignedRoleIDs: decision.AssignedRoleIDs}, nil
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

	decision := MembershipDecision{PlayerID: playerID, WorldID: input.GuildID, AssignedRoleIDs: append([]string(nil), member.Roles...)}
	r.membershipCache.Set(token, input.GuildID, decision)
	return &ResolveWorldOutput{WorldID: input.GuildID, AssignedRoleIDs: decision.AssignedRoleIDs}, nil
}
