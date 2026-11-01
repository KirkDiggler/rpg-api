package sessionv1alpha2

import (
	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

func (s *ConvertActSuite) TestSayActivityMapsDirectAndNestedAnswersWithoutSubstitution() {
	for _, cue := range []*sdk.ActivityCue{
		nil,
		{Ref: "dnd5e:activities:thoughtful", Cycles: 2},
		{Ref: "future:activity:unknown", Cycles: 4294967295},
	} {
		source := answer()
		source.Activity = cue
		direct, err := actToProto(sdk.Act{ID: "answer-1", Family: sdk.FamilyTableAnswer, Body: source})
		s.Require().NoError(err)
		nested, err := actToProto(sdk.Act{ID: "speak-1", Family: sdk.FamilyAct,
			Body: sdk.SpeakAct{By: alice, To: gate1, Line: "What happened?", Answer: &source}})
		s.Require().NoError(err)
		for _, got := range []*sessionpb.ActivityCue{
			direct.GetTableAnswer().GetActivity(), nested.GetSpeak().GetAnswer().GetActivity(),
		} {
			if cue == nil {
				s.Nil(got)
				continue
			}
			s.Require().NotNil(got)
			s.Equal(cue.Ref, got.Ref)
			s.Equal(cue.Cycles, got.Cycles)
		}
	}
}
