package sessionv1alpha1

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/proto"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	sessionmock "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1/mock"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

type DefaultActivityHandlerSuite struct {
	suite.Suite
	manager *sessionmock.MockManager
	handler *Handler
	ctx     context.Context
}

func TestDefaultActivityHandlerSuite(t *testing.T) { suite.Run(t, new(DefaultActivityHandlerSuite)) }

func (s *DefaultActivityHandlerSuite) SetupTest() {
	ctrl := gomock.NewController(s.T())
	s.manager = sessionmock.NewMockManager(ctrl)
	s.handler = &Handler{manager: s.manager, characters: anyMemberOwnedBy(ctrl, "alice")}
	s.ctx = auth.WithPlayerID(context.Background(), "alice")
}

func defaultActivityRosterFixture() sdk.RosterOutput {
	return sdk.RosterOutput{Members: []sdk.PublicMember{
		{ID: "thoughtful", Kind: sdk.KindMonster, Name: "Prisoner", MonsterRef: "dnd5e:monsters:commoner", AppearanceRef: "dnd5e:npcs:prisoner-01", DefaultActivityRef: "dnd5e:activities:thoughtful", Faction: "camp"},
		{ID: "unsupported", Kind: sdk.KindMonster, DefaultActivityRef: "other:activities:unpublished"},
		{ID: "legacy", Kind: sdk.KindMonster},
	}}
}

func (s *DefaultActivityHandlerSuite) assertCapturedDefaults(got []*sessionpb.PublicMemberInfo) {
	want := []*sessionpb.PublicMemberInfo{
		{Id: "thoughtful", Kind: sessionpb.MemberKind_MEMBER_KIND_MONSTER, Name: "Prisoner", MonsterRef: "dnd5e:monsters:commoner", AppearanceRef: "dnd5e:npcs:prisoner-01", DefaultActivityRef: "dnd5e:activities:thoughtful", Faction: "camp", Customization: &sessionpb.Customization{}},
		{Id: "unsupported", Kind: sessionpb.MemberKind_MEMBER_KIND_MONSTER, DefaultActivityRef: "other:activities:unpublished", Customization: &sessionpb.Customization{}},
		{Id: "legacy", Kind: sessionpb.MemberKind_MEMBER_KIND_MONSTER, Customization: &sessionpb.Customization{}},
	}
	s.Require().Len(got, len(want))
	for i := range want {
		s.True(proto.Equal(want[i], got[i]), "captured identity must be transcribed without activity lookup: want %s, got %s", want[i], got[i])
	}
}

func (s *DefaultActivityHandlerSuite) TestRosterCopiesCapturedDefaultsAndLegacyAbsence() {
	roster := defaultActivityRosterFixture()
	s.manager.EXPECT().Roster(gomock.Any(), &sdk.RosterInput{Session: "run", Member: "hero", Player: "alice"}).Return(&roster, nil)
	out, err := s.handler.GetRoster(s.ctx, &sessionpb.GetRosterRequest{Session: "run", Member: "hero"})
	s.Require().NoError(err)
	s.assertCapturedDefaults(out.Members)
}

func (s *DefaultActivityHandlerSuite) TestKnowledgeCopiesCapturedDefaultsAndLegacyAbsence() {
	s.manager.EXPECT().Knowledge(gomock.Any(), &sdk.KnowledgeInput{Session: "run", Member: "hero", Player: "alice"}).Return(&sdk.KnowledgeOutput{Roster: defaultActivityRosterFixture()}, nil)
	out, err := s.handler.GetKnowledge(s.ctx, &sessionpb.GetKnowledgeRequest{Session: "run", Member: "hero"})
	s.Require().NoError(err)
	s.Require().NotNil(out.Roster)
	s.assertCapturedDefaults(out.Roster.Members)
}
