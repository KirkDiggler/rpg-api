package sessionv1alpha2

import (
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

// TestSayActivityMapsTheAnswersCueWithoutSubstitution: a line's answer
// arrives as a standalone table answer under the line's `speak:` tag
// (rpg-project#934), and its activity cue crosses as the SDK wrote it --
// absent, known, or a ref this build has never heard of.
func (s *ConvertActSuite) TestSayActivityMapsTheAnswersCueWithoutSubstitution() {
	for _, cue := range []*sdk.ActivityCue{
		nil,
		{Ref: "dnd5e:activities:thoughtful", Cycles: 2},
		{Ref: "future:activity:unknown", Cycles: 4294967295},
	} {
		source := answer()
		source.Activity = cue
		got, err := actToProto(sdk.Act{ID: "answer-1", Act: "speak:line-1", Family: sdk.FamilyTableAnswer, Body: source})
		s.Require().NoError(err)
		activity := got.GetTableAnswer().GetActivity()
		if cue == nil {
			s.Nil(activity)
			continue
		}
		s.Require().NotNil(activity)
		s.Equal(cue.Ref, activity.Ref)
		s.Equal(cue.Cycles, activity.Cycles)
	}
}
