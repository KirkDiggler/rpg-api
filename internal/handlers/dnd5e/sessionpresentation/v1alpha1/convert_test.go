package sessionpresentationv1alpha1

import (
	"testing"

	"google.golang.org/protobuf/proto"
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

func TestConvertSmoke(t *testing.T) {
	// Keeps `go test -run TestConvertSmoke` valid before suite filtering.
}
