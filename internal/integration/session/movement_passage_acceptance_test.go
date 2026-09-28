package session_test

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/proto"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

type MovementPassageSuite struct {
	suite.Suite
}

func TestMovementPassageSuite(t *testing.T) {
	suite.Run(t, new(MovementPassageSuite))
}

func (s *MovementPassageSuite) TestDefeatedMonsterCellCanBeOccupiedAndReloaded() {
	scene := newDeathSaveScene(s.T())
	attack := attackDeclaration(s.T(), scene)
	scene.dice.reset(20, 8, 8)
	_, err := scene.h.handler.Attack(scene.ctx, &sessionpb.AttackRequest{
		Session: scene.session, Attacker: scene.actor, Target: "skel-1", DeclarationId: attack.GetId(),
	})
	s.Require().NoError(err)

	where, err := scene.h.handler.GetWhere(scene.ctx, &sessionpb.GetWhereRequest{
		Session: scene.session, Member: scene.actor,
	})
	s.Require().NoError(err)
	// The scene places the skeleton at (19,3) and both party members to
	// its left. Route through any intervening ally and stop on the body.
	path := []*sessionpb.Position{pbAt(19, 3)}
	if proto.Equal(where.GetPosition(), pbAt(17, 3)) {
		path = []*sessionpb.Position{pbAt(18, 3), pbAt(19, 3)}
	} else {
		s.Require().True(proto.Equal(where.GetPosition(), pbAt(18, 3)))
	}
	s.Require().NotEmpty(path)
	moved, err := scene.h.handler.Move(scene.ctx, &sessionpb.MoveRequest{
		Session: scene.session, Member: scene.actor, Path: path,
	})
	s.Require().NoError(err)
	s.Require().Len(moved.GetSteps(), len(path))
	s.True(proto.Equal(pbAt(19, 3), moved.GetSteps()[len(path)-1].GetPosition()))

	// A separate handler request reloads the persisted encounter.
	reloaded, err := scene.h.handler.GetWhere(scene.ctx, &sessionpb.GetWhereRequest{
		Session: scene.session, Member: scene.actor,
	})
	s.Require().NoError(err)
	s.True(proto.Equal(pbAt(19, 3), reloaded.GetPosition()))
}
