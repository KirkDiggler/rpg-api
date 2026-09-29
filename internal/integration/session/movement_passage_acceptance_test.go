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
	// The deterministic actor is adjacent to the defeated skeleton. This
	// scenario pins standing on its cell and persistence across requests.
	s.Require().True(proto.Equal(where.GetPosition(), pbAt(18, 3)))
	view, err := scene.h.handler.GetView(scene.ctx, &sessionpb.GetViewRequest{
		Session: scene.session, Member: scene.actor,
	})
	s.Require().NoError(err)
	var body *sessionpb.Sighting
	for _, sighting := range view.GetSightings() {
		if sighting.GetSubject() == "skel-1" {
			body = sighting
			break
		}
	}
	s.Require().NotNil(body)
	s.Equal(sessionpb.Passage_PASSAGE_STANDABLE, body.GetPassage())
	path := []*sessionpb.Position{pbAt(19, 3)}
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
