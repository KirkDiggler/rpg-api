package auth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-api/internal/auth"
)

type PermissionsSuite struct {
	suite.Suite
	config auth.WorldRoleConfig
}

func TestPermissionsSuite(t *testing.T) {
	suite.Run(t, new(PermissionsSuite))
}

func (s *PermissionsSuite) SetupTest() {
	s.config = auth.WorldRoleConfig{
		WorldID: "123456789012345678", AdminRoleID: "223456789012345678",
		BuilderRoleID: "323456789012345678", PlayerRoleID: "423456789012345678",
	}
}

func (s *PermissionsSuite) TestCumulativeGrants() {
	tests := []struct {
		name  string
		roles []string
		want  auth.Permissions
	}{
		{name: "no roles"},
		{name: "unrelated role", roles: []string{"523456789012345678"}},
		{name: "player", roles: []string{s.config.PlayerRoleID}, want: auth.PermissionPlay},
		{name: "builder", roles: []string{s.config.BuilderRoleID}, want: auth.PermissionBuild | auth.PermissionPlay},
		{name: "admin", roles: []string{s.config.AdminRoleID}, want: auth.PermissionAdmin | auth.PermissionBuild | auth.PermissionPlay},
		{name: "multiple roles", roles: []string{s.config.PlayerRoleID, s.config.BuilderRoleID, s.config.AdminRoleID}, want: auth.PermissionAdmin | auth.PermissionBuild | auth.PermissionPlay},
		{name: "duplicates", roles: []string{s.config.PlayerRoleID, s.config.PlayerRoleID}, want: auth.PermissionPlay},
	}
	for _, tt := range tests {
		s.Run(tt.name, func() {
			out, err := auth.EvaluatePermissions(&auth.EvaluatePermissionsInput{
				Config: s.config, WorldID: s.config.WorldID, AssignedRoleIDs: tt.roles,
			})
			s.Require().NoError(err)
			s.Require().NotNil(out)
			s.Equal(tt.want, out.Permissions)
			s.Equal(tt.want&auth.PermissionPlay != 0, out.Permissions.Allows(auth.PermissionPlay))
			s.Equal(tt.want&auth.PermissionBuild != 0, out.Permissions.Allows(auth.PermissionBuild))
			s.Equal(tt.want&auth.PermissionAdmin != 0, out.Permissions.Allows(auth.PermissionAdmin))
			s.False(out.Permissions.Allows(0))
		})
	}
}

func (s *PermissionsSuite) TestSharedConfiguredRoleGetsHighestGrant() {
	s.config.PlayerRoleID = s.config.AdminRoleID
	out, err := auth.EvaluatePermissions(&auth.EvaluatePermissionsInput{
		Config: s.config, WorldID: s.config.WorldID, AssignedRoleIDs: []string{s.config.AdminRoleID},
	})
	s.Require().NoError(err)
	s.True(out.Permissions.Allows(auth.PermissionAdmin | auth.PermissionBuild | auth.PermissionPlay))
}

func (s *PermissionsSuite) TestRefusesMissingInputAndWrongWorld() {
	out, err := auth.EvaluatePermissions(nil)
	s.Error(err)
	s.Nil(out)
	for _, world := range []string{"", "623456789012345678"} {
		out, err = auth.EvaluatePermissions(&auth.EvaluatePermissionsInput{
			Config: s.config, WorldID: world, AssignedRoleIDs: []string{s.config.AdminRoleID},
		})
		s.Error(err)
		s.Nil(out)
	}
}

func (s *PermissionsSuite) TestRefusesIncompleteOrInvalidConfiguration() {
	for _, invalid := range []string{"", "0", "01", "-1", " 1", "18446744073709551616"} {
		for _, field := range []string{"world", "admin", "builder", "player"} {
			s.Run(field+"/"+invalid, func() {
				config := s.config
				switch field {
				case "world":
					config.WorldID = invalid
				case "admin":
					config.AdminRoleID = invalid
				case "builder":
					config.BuilderRoleID = invalid
				case "player":
					config.PlayerRoleID = invalid
				}
				out, err := auth.EvaluatePermissions(&auth.EvaluatePermissionsInput{Config: config, WorldID: s.config.WorldID})
				s.Error(err)
				s.Nil(out)
			})
		}
	}
}

func (s *PermissionsSuite) TestDiscordMemberPreservesRolesForEvaluation() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Equal("/api/users/@me/guilds/"+s.config.WorldID+"/member", r.URL.Path)
		s.Equal("Bearer credential", r.Header.Get("Authorization"))
		_, err := w.Write([]byte(`{"user":{"id":"player-1"},"roles":["323456789012345678"]}`))
		s.NoError(err)
	}))
	defer server.Close()
	client := auth.NewDiscordClient(auth.WithBaseURL(server.URL))
	member, err := client.GetCurrentUserGuildMember(context.Background(), &auth.GetCurrentUserGuildMemberInput{
		Token: "credential", GuildID: s.config.WorldID,
	})
	s.Require().NoError(err)
	s.Require().NotNil(member)
	s.Equal([]string{s.config.BuilderRoleID}, member.Roles)
	out, err := auth.EvaluatePermissions(&auth.EvaluatePermissionsInput{
		Config: s.config, WorldID: s.config.WorldID, AssignedRoleIDs: member.Roles,
	})
	s.Require().NoError(err)
	s.True(out.Permissions.Allows(auth.PermissionBuild | auth.PermissionPlay))
	s.False(out.Permissions.Allows(auth.PermissionAdmin))
}
