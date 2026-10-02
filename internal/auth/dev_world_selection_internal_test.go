package auth

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	worldrepo "github.com/KirkDiggler/rpg-api/internal/repositories/world"
	"github.com/KirkDiggler/rpg-api/internal/worldcontext"
)

const (
	devAllowlistWorldA = "123456789012345678"
	devAllowlistWorldB = "223456789012345678"
)

// devAllowlistVerifier fails every membership lookup: the Dev allowlist path
// must never reach the Discord provider.
type devAllowlistVerifier struct{}

func (devAllowlistVerifier) GetCurrentUserGuildMember(context.Context, *GetCurrentUserGuildMemberInput) (*DiscordGuildMember, error) {
	return nil, ErrGuildMembershipDenied
}

// TestRoleAccessDevAllowlistSelectorReachesGameplayAdmission proves the
// composed gameplay entry path (global auth -> RoleAccess world resolution)
// forwards the selected Dev world to the handler, and denies unusable or
// unknown selectors before admission.
func TestRoleAccessDevAllowlistSelectorReachesGameplayAdmission(t *testing.T) {
	redis := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: redis.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	worlds, err := worldrepo.NewRedis(&worldrepo.RedisConfig{Client: client})
	require.NoError(t, err)

	membershipCache, err := NewMembershipCache(&MembershipCacheConfig{TTL: 30 * time.Second, MaxEntries: 8, Now: time.Now})
	require.NoError(t, err)
	resolver, err := NewWorldResolver(&WorldResolverConfig{
		MembershipVerifier: devAllowlistVerifier{},
		IdentityCache:      NewTokenCache(5 * time.Minute),
		MembershipCache:    membershipCache,
		DevWorldID:         devAllowlistWorldA,
		DevWorldIDs:        []string{devAllowlistWorldA, devAllowlistWorldB},
	})
	require.NoError(t, err)
	gate, err := NewRoleAccess(&RoleAccessConfig{Resolver: resolver, Worlds: worlds, DevelopmentPermissions: PermissionPlay})
	require.NoError(t, err)

	devContext := func(values []string) context.Context {
		md := metadata.MD{}
		md.Set("authorization", "Dev local-player")
		if values != nil {
			md["x-rpg-guild-id"] = values
		}
		ctx, err := authenticate(metadata.NewIncomingContext(context.Background(), md), nil, NewTokenCache(time.Minute), true)
		require.NoError(t, err)
		return ctx
	}

	tests := []struct {
		name     string
		guild    []string
		world    string
		code     codes.Code
		admitted bool
	}{
		{name: "selector B", guild: []string{devAllowlistWorldB}, world: devAllowlistWorldB, admitted: true},
		{name: "absent selector uses default", guild: nil, world: devAllowlistWorldA, admitted: true},
		{name: "unknown selector denied", guild: []string{"323456789012345678"}, code: codes.PermissionDenied},
		{name: "malformed selector denied", guild: []string{"not-a-world"}, code: codes.InvalidArgument},
		{name: "multiple selectors denied", guild: []string{devAllowlistWorldA, devAllowlistWorldB}, code: codes.InvalidArgument},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			admitted := false
			_, err := gate.UnaryInterceptor()(devContext(tt.guild), nil, &grpc.UnaryServerInfo{FullMethod: "/dnd5e.api.lobby.v1alpha1.LobbyService/ListDungeons"}, func(ctx context.Context, _ interface{}) (interface{}, error) {
				admitted = true
				value, ok := worldcontext.Get(ctx)
				require.True(t, ok)
				require.Equal(t, tt.world, value.WorldID)
				return nil, nil
			})
			require.Equal(t, tt.admitted, admitted)
			if tt.admitted {
				require.NoError(t, err)
				return
			}
			require.Equal(t, tt.code, status.Code(err))
		})
	}
}
