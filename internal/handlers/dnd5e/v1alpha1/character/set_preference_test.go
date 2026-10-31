package character

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	charactermock "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v1alpha1/character/mock"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	orchmock "github.com/KirkDiggler/rpg-api/internal/orchestrators/character/mock"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
)

// SetPreferenceHandlerSuite: CharacterService.SetPreference writes the
// caller's own character's configuration (rpg-project#753 R2).
type SetPreferenceHandlerSuite struct {
	suite.Suite
	ctx         context.Context
	service     *orchmock.MockService
	preferences *charactermock.MockPreferences
	handler     *Handler
}

func TestSetPreferenceHandlerSuite(t *testing.T) { suite.Run(t, new(SetPreferenceHandlerSuite)) }

func (s *SetPreferenceHandlerSuite) SetupTest() {
	ctrl := gomock.NewController(s.T())
	s.service = orchmock.NewMockService(ctrl)
	s.preferences = charactermock.NewMockPreferences(ctrl)
	var err error
	s.handler, err = NewHandler(&HandlerConfig{CharacterService: s.service, Sessions: &fakeSessions{}, Preferences: s.preferences})
	s.Require().NoError(err)
	s.ctx = auth.WithPlayerID(context.Background(), "player-alice")
}

func (s *SetPreferenceHandlerSuite) ownedBy(player string) {
	s.service.EXPECT().GetCharacterRecord(gomock.Any(), &character.GetCharacterRecordInput{CharacterID: "alice"}).
		Return(&character.GetCharacterRecordOutput{Character: &entities.Character{Data: &toolkitchar.Data{ID: "alice", PlayerID: player}}}, nil)
}

func (s *SetPreferenceHandlerSuite) TestWritesTheCallersOwnCharacter() {
	s.ownedBy("player-alice")
	s.preferences.EXPECT().SetPreference(gomock.Any(), &sessionorch.SetPreferenceInput{CharacterID: "alice", DiscoverySharing: proto.Bool(true)}).
		Return(&sessionorch.SetPreferenceOutput{DiscoverySharing: proto.Bool(true)}, nil)
	out, err := s.handler.SetPreference(s.ctx, &dnd5ev1alpha1.SetPreferenceRequest{CharacterId: "alice", DiscoverySharing: proto.Bool(true)})
	s.Require().NoError(err)
	s.Equal(proto.Bool(true), out.DiscoverySharing)
}

func (s *SetPreferenceHandlerSuite) TestAnAbsentPreferenceIsPassedAsUnchanged() {
	s.ownedBy("player-alice")
	s.preferences.EXPECT().SetPreference(gomock.Any(), &sessionorch.SetPreferenceInput{CharacterID: "alice"}).
		Return(&sessionorch.SetPreferenceOutput{}, nil)
	out, err := s.handler.SetPreference(s.ctx, &dnd5ev1alpha1.SetPreferenceRequest{CharacterId: "alice"})
	s.Require().NoError(err)
	s.Nil(out.DiscoverySharing, "never chosen stays absent on the wire")
}

func (s *SetPreferenceHandlerSuite) TestAnotherPlayersCharacterIsRefused() {
	s.ownedBy("player-mallory")
	_, err := s.handler.SetPreference(s.ctx, &dnd5ev1alpha1.SetPreferenceRequest{CharacterId: "alice", DiscoverySharing: proto.Bool(true)})
	s.Equal(codes.NotFound, status.Code(err))
}

func (s *SetPreferenceHandlerSuite) TestNoCharacterIsRefused() {
	_, err := s.handler.SetPreference(s.ctx, &dnd5ev1alpha1.SetPreferenceRequest{DiscoverySharing: proto.Bool(true)})
	s.Equal(codes.InvalidArgument, status.Code(err))
}

func (s *SetPreferenceHandlerSuite) TestAGivenUpWaitIsReportedAsSuch() {
	s.ownedBy("player-alice")
	s.preferences.EXPECT().SetPreference(gomock.Any(), gomock.Any()).Return(nil, context.DeadlineExceeded)
	_, err := s.handler.SetPreference(s.ctx, &dnd5ev1alpha1.SetPreferenceRequest{CharacterId: "alice", DiscoverySharing: proto.Bool(true)})
	s.Equal(codes.DeadlineExceeded, status.Code(err))
}

func (s *SetPreferenceHandlerSuite) TestWithoutPreferencesItFailsClosed() {
	s.handler.preferences = nil
	_, err := s.handler.SetPreference(s.ctx, &dnd5ev1alpha1.SetPreferenceRequest{CharacterId: "alice", DiscoverySharing: proto.Bool(true)})
	s.Equal(codes.Unimplemented, status.Code(err))
}
