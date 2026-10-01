package world_test

import (
	"context"
	"testing"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	worldrepo "github.com/KirkDiggler/rpg-api/internal/repositories/world"
)

type RedisSuite struct {
	suite.Suite
	repo   worldrepo.Repository
	server *miniredis.Miniredis
	world  entities.World
}

func TestRedisSuite(t *testing.T) { suite.Run(t, new(RedisSuite)) }
func (s *RedisSuite) SetupTest() {
	s.server = miniredis.RunT(s.T())
	client := goredis.NewClient(&goredis.Options{Addr: s.server.Addr()})
	s.T().Cleanup(func() { _ = client.Close() })
	var err error
	s.repo, err = worldrepo.NewRedis(&worldrepo.RedisConfig{Client: client})
	s.Require().NoError(err)
	s.world = entities.World{WorldID: "123456789012345678", AdminRoleID: "223456789012345678", BuilderRoleID: "323456789012345678", PlayerRoleID: "423456789012345678"}
}
func (s *RedisSuite) TestWorldsAreIndependentAndReturnedValuesAreDetached() {
	ctx := context.Background()
	first, err := s.repo.SetRoles(ctx, &worldrepo.SetRolesInput{World: &s.world})
	s.Require().NoError(err)
	second := s.world
	second.WorldID = "523456789012345678"
	second.AdminRoleID = "623456789012345678"
	_, err = s.repo.SetRoles(ctx, &worldrepo.SetRolesInput{World: &second})
	s.Require().NoError(err)
	first.World.AdminRoleID = "723456789012345678"
	for _, want := range []entities.World{s.world, second} {
		out, err := s.repo.Get(ctx, &worldrepo.GetInput{WorldID: want.WorldID})
		s.Require().NoError(err)
		s.Equal(want, *out.World)
	}
}
func (s *RedisSuite) TestMissingAndInvalidInputs() {
	ctx := context.Background()
	_, err := worldrepo.NewRedis(nil)
	s.Equal(apierr.CodeInvalidArgument, apierr.GetCode(err))
	out, err := s.repo.Get(ctx, &worldrepo.GetInput{WorldID: s.world.WorldID})
	s.Nil(out)
	s.True(apierr.IsNotFound(err))
	for _, input := range []*worldrepo.GetInput{nil, {}, {WorldID: "01"}} {
		_, err = s.repo.Get(ctx, input)
		s.Equal(apierr.CodeInvalidArgument, apierr.GetCode(err))
	}
	for _, input := range []*worldrepo.SetRolesInput{nil, {}, {World: &entities.World{WorldID: s.world.WorldID}}} {
		_, err = s.repo.SetRoles(ctx, input)
		s.Equal(apierr.CodeInvalidArgument, apierr.GetCode(err))
	}
	_, err = s.repo.SetMemberRoles(ctx, nil)
	s.Equal(apierr.CodeInvalidArgument, apierr.GetCode(err))
	s.Empty(s.server.Keys())
}
func (s *RedisSuite) TestCorruptedStoredPolicyFailsClosed() {
	ctx := context.Background()
	for _, data := range []string{`{`, `null`, `{"world_id":"523456789012345678"}`, `{"world_id":"123456789012345678","admin_role_id":"0"}`} {
		s.server.Set("world:v1:"+s.world.WorldID, data)
		out, err := s.repo.Get(ctx, &worldrepo.GetInput{WorldID: s.world.WorldID})
		s.Error(err)
		s.Nil(out)
	}
}
func (s *RedisSuite) TestMemberUpdateRequiresExistingWorldAndPreservesAdmin() {
	ctx := context.Background()
	input := &worldrepo.SetMemberRolesInput{WorldID: s.world.WorldID, ExpectedAdminRoleID: s.world.AdminRoleID, BuilderRoleID: s.world.PlayerRoleID, PlayerRoleID: s.world.BuilderRoleID}
	_, err := s.repo.SetMemberRoles(ctx, input)
	s.True(apierr.IsNotFound(err))
	_, err = s.repo.SetRoles(ctx, &worldrepo.SetRolesInput{World: &s.world})
	s.Require().NoError(err)
	out, err := s.repo.SetMemberRoles(ctx, input)
	s.Require().NoError(err)
	s.Equal(s.world.AdminRoleID, out.World.AdminRoleID)
	s.Equal(input.BuilderRoleID, out.World.BuilderRoleID)
	s.Equal(input.PlayerRoleID, out.World.PlayerRoleID)
	input.ExpectedAdminRoleID = "523456789012345678"
	_, err = s.repo.SetMemberRoles(ctx, input)
	s.Equal(apierr.CodeAborted, apierr.GetCode(err))
}
