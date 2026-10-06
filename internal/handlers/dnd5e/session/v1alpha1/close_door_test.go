package sessionv1alpha1

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	sessionmock "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1/mock"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

type CloseDoorHandlerSuite struct {
	suite.Suite
	manager *sessionmock.MockManager
	handler *Handler
	ctx     context.Context
}

func TestCloseDoorHandlerSuite(t *testing.T) { suite.Run(t, new(CloseDoorHandlerSuite)) }

func (s *CloseDoorHandlerSuite) SetupTest() {
	ctrl := gomock.NewController(s.T())
	s.manager = sessionmock.NewMockManager(ctrl)
	s.handler = &Handler{manager: s.manager, characters: charactersOf(ctrl, map[string]rosterCharacter{
		"hero":  {owner: "alice", name: "Hero", class: "fighter", race: "human"},
		"other": {owner: "bob", name: "Other", class: "rogue", race: "elf"},
	})}
	s.ctx = auth.WithPlayerID(context.Background(), "alice")
}

func (s *CloseDoorHandlerSuite) TestBindsTheActorAndPassesThroughClosedState() {
	s.manager.EXPECT().CloseDoor(gomock.Any(), &sdk.CloseDoorInput{Session: "run", Member: "hero", Door: "keep/gate"}).Return(
		&sdk.CloseDoorOutput{Door: sdk.Door{ID: "keep/gate", State: "closed"}}, nil,
	)
	out, err := s.handler.CloseDoor(s.ctx, &sessionpb.CloseDoorRequest{Session: "run", Member: "hero", Door: "keep/gate"})
	s.Require().NoError(err)
	s.Equal("keep/gate", out.Door.Door)
	s.Equal(sessionpb.DoorState_DOOR_STATE_CLOSED, out.Door.State)
	s.Nil(out.Door.Lock, "closing must not invent a lock")
}

func (s *CloseDoorHandlerSuite) TestUnauthenticatedNeverCallsTheSDK() {
	out, err := s.handler.CloseDoor(context.Background(), &sessionpb.CloseDoorRequest{Session: "run", Member: "hero", Door: "gate"})
	s.Nil(out)
	requireCode(s.T(), err, codes.Unauthenticated)
}

func (s *CloseDoorHandlerSuite) TestMissingMemberNeverCallsTheSDK() {
	out, err := s.handler.CloseDoor(s.ctx, &sessionpb.CloseDoorRequest{Session: "run", Door: "gate"})
	s.Nil(out)
	requireCode(s.T(), err, codes.InvalidArgument)
}

func (s *CloseDoorHandlerSuite) TestForeignMemberNeverCallsTheSDK() {
	out, err := s.handler.CloseDoor(s.ctx, &sessionpb.CloseDoorRequest{Session: "run", Member: "other", Door: "gate"})
	s.Nil(out)
	requireCode(s.T(), err, codes.PermissionDenied)
}

func (s *CloseDoorHandlerSuite) TestProviderRefusalsUseTheExistingMapping() {
	for _, tc := range []struct {
		name string
		err  error
		code codes.Code
	}{
		{"missing, concealed or not open", sdk.ErrNoConnection, codes.NotFound},
		{"out of reach", sdk.ErrOutOfRange, codes.FailedPrecondition},
		{"missing session", sdk.ErrNoSessionID, codes.InvalidArgument},
	} {
		s.Run(tc.name, func() {
			s.manager.EXPECT().CloseDoor(gomock.Any(), gomock.Any()).Return(nil, tc.err)
			out, err := s.handler.CloseDoor(s.ctx, &sessionpb.CloseDoorRequest{Session: "run", Member: "hero", Door: "keep/gate"})
			s.Nil(out)
			requireCode(s.T(), err, tc.code)
		})
	}
}
