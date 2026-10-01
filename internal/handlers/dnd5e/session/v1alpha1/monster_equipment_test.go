package sessionv1alpha1

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/proto"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	sessionv1alpha1mock "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1/mock"
)

// This suite verifies API translation only. Ordered weapon selection and
// observed/remembered snapshots belong to the released session provider.
type MonsterEquipmentTransportSuite struct {
	suite.Suite
	ctrl *gomock.Controller
}

func TestMonsterEquipmentTransportSuite(t *testing.T) {
	suite.Run(t, new(MonsterEquipmentTransportSuite))
}

func (s *MonsterEquipmentTransportSuite) SetupTest() {
	s.ctrl = gomock.NewController(s.T())
}

func (s *MonsterEquipmentTransportSuite) TestMonsterEquipmentSurvivesViewDiscoveryAndWireRoundTrip() {
	for _, tc := range []struct {
		name string
		id   string
		ref  string
	}{
		{"scimitar", "scimitar", "dnd5e:item:scimitar"},
		{"shortbow", "shortbow", "dnd5e:item:shortbow"},
		{"shortsword", "shortsword", "dnd5e:item:shortsword"},
		{"unsupported-visual-still-translates", "longsword", "dnd5e:item:longsword"},
		{"unarmed-is-not-a-missing-observation", "unarmed-strike", "dnd5e:item:unarmed-strike"},
		{"unknown-or-natural", "", ""},
	} {
		s.Run(tc.name, func() {
			seen := &sdk.Seen{}
			if tc.id != "" {
				seen.Equipment = &sdk.SeenEquipment{MainHand: tc.id}
			}
			for _, via := range [][]string{{"sight"}, nil} {
				view := &sessionpb.GetViewResponse{Sightings: sightingsToProto([]sdk.Sighting{{
					Subject: "monster", Kind: sdk.KindMonster, CurrentVia: via, Seen: seen,
				}})}
				encoded, err := proto.Marshal(view)
				s.Require().NoError(err)
				reloaded := &sessionpb.GetViewResponse{}
				s.Require().NoError(proto.Unmarshal(encoded, reloaded))
				s.Require().Len(reloaded.GetSightings(), 1)
				got := reloaded.GetSightings()[0]
				s.Equal(sessionpb.MemberKind_MEMBER_KIND_MONSTER, got.GetKind())
				s.Equal(len(via), len(got.GetCurrentVia()))
				s.Require().NotNil(got.GetSeen())
				if tc.id == "" {
					s.Nil(got.GetSeen().GetEquipment(), "unknown must not become observed empty hands")
				} else {
					s.Require().NotNil(got.GetSeen().GetEquipment())
					s.Equal(tc.ref, got.GetSeen().GetEquipment().GetMainHand())
					s.Empty(got.GetSeen().GetEquipment().GetOffHand(), "do not invent a monster off-hand asset")
				}
				discovery := discoveryToProto(sdk.Discovery{FirstContact: []sdk.Report{{Subject: "monster", Seen: seen}}})
				s.Require().Len(discovery.GetFirstContact(), 1)
				s.True(proto.Equal(got.GetSeen(), discovery.GetFirstContact()[0].GetSeen()),
					"first contact and reconnect must share the same equipment mapping")
			}
		})
	}
}

func (s *MonsterEquipmentTransportSuite) TestGetViewTranslatesOnlyTheCallingObserversTestimony() {
	manager := sessionv1alpha1mock.NewMockManager(s.ctrl)
	h := &Handler{manager: manager, characters: anyMemberOwnedBy(s.ctrl, "owner")}
	ctx := auth.WithPlayerID(context.Background(), "owner")
	for _, tc := range []struct {
		member string
		weapon string
		via    []string
	}{
		{"current-observer", "shortbow", []string{"sight"}},
		{"remembering-observer", "scimitar", nil},
	} {
		in := &sdk.ViewInput{Session: "sess", Member: tc.member}
		manager.EXPECT().View(gomock.Any(), in).Return([]sdk.Sighting{{
			Subject: "monster", Kind: sdk.KindMonster, CurrentVia: tc.via,
			Seen: &sdk.Seen{Equipment: &sdk.SeenEquipment{MainHand: tc.weapon}},
		}}, nil)
		manager.EXPECT().Areas(gomock.Any(), in).Return([]sdk.SightArea{}, nil)
		out, err := h.GetView(ctx, &sessionpb.GetViewRequest{Session: "sess", Member: tc.member})
		s.Require().NoError(err)
		s.Require().Len(out.GetSightings(), 1)
		got := out.GetSightings()[0]
		s.Equal("dnd5e:item:"+tc.weapon, got.GetSeen().GetEquipment().GetMainHand())
		s.Equal(len(tc.via), len(got.GetCurrentVia()))
	}
}

func (s *MonsterEquipmentTransportSuite) TestSightedEventKeepsRecipientScopedInvalidationForLiveAndCatchUp() {
	e := sdk.Event{
		Session: "sess", Recipient: "observer", Kind: sdk.EventSighted,
		Body: sdk.SightedBody{Changed: []string{"monster"}},
	}
	live, err := eventToProto(e)
	s.Require().NoError(err)
	s.Equal("observer", live.GetRecipient())
	s.Require().NotNil(live.GetSighted())
	s.Equal([]string{"monster"}, live.GetSighted().GetChanged(), "event names subjects, not a second equipment answer")
	catchUp, err := eventsToProto([]sdk.Event{e})
	s.Require().NoError(err)
	s.Require().Len(catchUp, 1)
	s.True(proto.Equal(live, catchUp[0]))
}
