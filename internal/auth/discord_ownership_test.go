package auth_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-api/internal/auth"
)

type OwnershipSuite struct{ suite.Suite }

func TestOwnershipSuite(t *testing.T) { suite.Run(t, new(OwnershipSuite)) }

func (s *OwnershipSuite) TestOwnerAndNonOwnerAreProviderDecisions() {
	for _, owner := range []bool{true, false} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			s.Equal("/api/users/@me/guilds", r.URL.Path)
			s.Equal("Bearer same-credential", r.Header.Get("Authorization"))
			s.Equal("200", r.URL.Query().Get("limit"))
			err := json.NewEncoder(w).Encode([]map[string]any{{"id": "123456789012345678", "owner": owner}})
			s.NoError(err)
		}))
		client := auth.NewDiscordClient(auth.WithBaseURL(server.URL))
		out, err := client.GetGuildOwnership(context.Background(), &auth.GetGuildOwnershipInput{Token: "same-credential", GuildID: "123456789012345678"})
		server.Close()
		s.Require().NoError(err)
		s.Equal(owner, out.Owner)
	}
}

func (s *OwnershipSuite) TestPaginatesBeyondFirstTwoHundredGuilds() {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		guilds := []map[string]any{}
		if r.URL.Query().Get("after") == "" {
			for i := uint64(0); i < 200; i++ {
				guilds = append(guilds, map[string]any{"id": strconv.FormatUint(100000000000000000+i, 10), "owner": false})
			}
		} else {
			s.Equal("100000000000000199", r.URL.Query().Get("after"))
			guilds = append(guilds, map[string]any{"id": "123456789012345678", "owner": true})
		}
		s.NoError(json.NewEncoder(w).Encode(guilds))
	}))
	defer server.Close()
	client := auth.NewDiscordClient(auth.WithBaseURL(server.URL))
	out, err := client.GetGuildOwnership(context.Background(), &auth.GetGuildOwnershipInput{Token: "credential", GuildID: "123456789012345678"})
	s.Require().NoError(err)
	s.True(out.Owner)
	s.Equal(2, calls)
}

func (s *OwnershipSuite) TestMissingGuildInvalidScopeAndProviderFailuresDoNotGrantOwnership() {
	for _, tt := range []struct {
		code int
		body string
		want error
	}{
		{http.StatusOK, `[]`, auth.ErrGuildMembershipDenied},
		{http.StatusOK, `{`, auth.ErrDiscordUnavailable},
		{http.StatusOK, `[{"id":"bad","owner":true}]`, auth.ErrDiscordUnavailable},
		{http.StatusUnauthorized, ``, auth.ErrInvalidToken},
		{http.StatusForbidden, ``, auth.ErrDiscordUnavailable},
		{http.StatusTooManyRequests, ``, auth.ErrDiscordUnavailable},
		{http.StatusServiceUnavailable, ``, auth.ErrDiscordUnavailable},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tt.code)
			_, err := w.Write([]byte(tt.body))
			s.NoError(err)
		}))
		client := auth.NewDiscordClient(auth.WithBaseURL(server.URL))
		out, err := client.GetGuildOwnership(context.Background(), &auth.GetGuildOwnershipInput{Token: "credential", GuildID: "123456789012345678"})
		server.Close()
		s.Nil(out)
		s.ErrorIs(err, tt.want)
	}
}

func (s *OwnershipSuite) TestMembershipCacheDoesNotExposeMutableRoleAuthority() {
	cache, err := auth.NewMembershipCache(&auth.MembershipCacheConfig{TTL: time.Second, MaxEntries: 2, Now: time.Now})
	s.Require().NoError(err)
	roles := []string{"223456789012345678"}
	cache.Set("credential", "123456789012345678", auth.MembershipDecision{PlayerID: "player", WorldID: "123456789012345678", AssignedRoleIDs: roles})
	roles[0] = "323456789012345678"
	first, ok := cache.Get("credential", "123456789012345678")
	s.Require().True(ok)
	s.Equal("223456789012345678", first.AssignedRoleIDs[0])
	first.AssignedRoleIDs[0] = "423456789012345678"
	second, ok := cache.Get("credential", "123456789012345678")
	s.Require().True(ok)
	s.Equal("223456789012345678", second.AssignedRoleIDs[0])
}
