package sessionpresentationv1alpha1

import (
	"testing"

	"google.golang.org/protobuf/proto"

	presentationpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/presentation/v1alpha1"
	orchsessionpresentation "github.com/KirkDiggler/rpg-api/internal/orchestrators/sessionpresentation"
)

func (s *HandlerSuite) TestDraftFromProto_ConvertsExactOrderedFields() {
	got := draftFromProto(s.testProtoDraft())
	s.Equal(s.testDomainDraft(), got)
}

func (s *HandlerSuite) TestPlanToProto_ConvertsExactOrderedFields() {
	want := s.testProtoPlan()
	got := planToProto(s.testDomainPlan())
	s.True(proto.Equal(want, got), "expected exact proto plan conversion")
}

func (s *HandlerSuite) TestDraftFromProto_UnknownEnumsNormalizeToUnspecified() {
	in := s.testProtoDraft()
	in.PhysicsSchema = presentationpb.DicePhysicsSchema(99)
	in.Bodies[0].Shape = presentationpb.DiceShape(99)
	in.Contacts[1].GetStaticCollider().Kind = presentationpb.DiceStaticContactKind(99)
	in.Terminal.Dice[0].Kind = presentationpb.DiceTerminalKind(99)

	got := draftFromProto(in)
	s.Equal(orchsessionpresentation.PhysicsSchemaUnspecified, got.PhysicsSchema)
	s.Equal(orchsessionpresentation.ShapeUnspecified, got.Bodies[0].Shape)
	s.Equal(orchsessionpresentation.StaticContactKindUnspecified, got.Contacts[1].StaticCollider.Kind)
	s.Equal(orchsessionpresentation.TerminalKindUnspecified, got.Terminal[0].Kind)
}

func TestConvertSmoke(t *testing.T) {
	// Keeps `go test -run TestConvertSmoke` valid before suite filtering.
}
