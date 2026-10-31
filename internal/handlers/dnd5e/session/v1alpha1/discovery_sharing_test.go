package sessionv1alpha1

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	sessionv1alpha1mock "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1/mock"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

// DiscoverySharingSuite: the deprecated session SetDiscoverySharing writes
// character configuration for the caller's seated character
// (rpg-project#753 R2), and GetKnowledge reports what the character chose.
type DiscoverySharingSuite struct {
	suite.Suite
	ctx         context.Context
	manager     *sessionv1alpha1mock.MockManager
	preferences *sessionv1alpha1mock.MockPreferences
	handler     *Handler
}

func TestDiscoverySharingSuite(t *testing.T) { suite.Run(t, new(DiscoverySharingSuite)) }

func (s *DiscoverySharingSuite) SetupTest() {
	ctrl := gomock.NewController(s.T())
	s.manager = sessionv1alpha1mock.NewMockManager(ctrl)
	s.preferences = sessionv1alpha1mock.NewMockPreferences(ctrl)
	s.handler = &Handler{
		manager: s.manager, preferences: s.preferences,
		characters: ownedBy(ctrl, map[string]string{"char-a": "alice"}),
	}
	s.ctx = auth.WithPlayerID(context.Background(), "alice")
}

func (s *DiscoverySharingSuite) TestTheDeprecatedHandlerWritesTheStore() {
	for _, sharing := range []bool{true, false} {
		seatedAs(s.manager, "alice", "char-a")
		s.preferences.EXPECT().SetPreference(gomock.Any(), &sessionorch.SetPreferenceInput{
			CharacterID: "char-a", DiscoverySharing: proto.Bool(sharing),
		}).Return(&sessionorch.SetPreferenceOutput{DiscoverySharing: proto.Bool(sharing)}, nil)

		out, err := s.handler.SetDiscoverySharing(s.ctx, &sessionpb.SetDiscoverySharingRequest{
			Session: "sess-1", Member: "char-a", Sharing: proto.Bool(sharing),
		})
		s.Require().NoError(err)
		s.Equal(sharing, out.Sharing)
		s.Empty(out.Saved.GetWritten(), "nothing in the session was saved")
		s.False(out.Delivery.GetFailed(), "nothing in the session was delivered, so nothing failed")
	}
}

func (s *DiscoverySharingSuite) TestAnOmittedChoiceIsRefused() {
	seatedAs(s.manager, "alice", "char-a")
	_, err := s.handler.SetDiscoverySharing(s.ctx, &sessionpb.SetDiscoverySharingRequest{Session: "sess-1", Member: "char-a"})
	s.Equal(codes.InvalidArgument, status.Code(err))
}

func (s *DiscoverySharingSuite) TestAnotherPlayersCharacterIsRefusedBeforeTheStore() {
	ctx := auth.WithPlayerID(context.Background(), "mallory")
	_, err := s.handler.SetDiscoverySharing(ctx, &sessionpb.SetDiscoverySharingRequest{
		Session: "sess-1", Member: "char-a", Sharing: proto.Bool(true),
	})
	s.Require().Error(err)
}

func (s *DiscoverySharingSuite) TestWithoutPreferencesItFailsClosed() {
	s.handler.preferences = nil
	seatedAs(s.manager, "alice", "char-a")
	_, err := s.handler.SetDiscoverySharing(s.ctx, &sessionpb.SetDiscoverySharingRequest{
		Session: "sess-1", Member: "char-a", Sharing: proto.Bool(true),
	})
	s.Equal(codes.Unimplemented, status.Code(err))
}

func (s *DiscoverySharingSuite) knowledge(encounterSharing *bool) *sessionpb.GetKnowledgeResponse {
	s.manager.EXPECT().Knowledge(gomock.Any(), gomock.Any()).Return(&sdk.KnowledgeOutput{DiscoverySharing: encounterSharing}, nil)
	out, err := s.handler.GetKnowledge(s.ctx, &sessionpb.GetKnowledgeRequest{Session: "sess-1", Member: "char-a"})
	s.Require().NoError(err)
	return out
}

func (s *DiscoverySharingSuite) TestGetKnowledgeReportsTheConfiguredChoice() {
	s.preferences.EXPECT().GetPreference(gomock.Any(), &sessionorch.GetPreferenceInput{CharacterID: "char-a"}).
		Return(&sessionorch.GetPreferenceOutput{DiscoverySharing: proto.Bool(true)}, nil)
	out := s.knowledge(proto.Bool(false))
	s.Require().NotNil(out.DiscoverySharing)
	s.True(*out.DiscoverySharing, "the choice is reported before the encounter's copy takes it up")
}

func (s *DiscoverySharingSuite) TestGetKnowledgeKeepsTheSessionDefaultWhenNotChosen() {
	s.preferences.EXPECT().GetPreference(gomock.Any(), gomock.Any()).
		Return(&sessionorch.GetPreferenceOutput{}, nil)
	out := s.knowledge(proto.Bool(false))
	s.Require().NotNil(out.DiscoverySharing)
	s.False(*out.DiscoverySharing)
}

func (s *DiscoverySharingSuite) TestGetKnowledgeLeavesAnAbsentCapabilityAbsent() {
	out := s.knowledge(nil)
	s.Nil(out.DiscoverySharing, "absent does not mean private, and the store is not asked")
}
