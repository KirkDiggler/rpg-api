package sessionv1alpha1

import sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

func (s *MaterialOverrideSuite) TestDoorFrameAndOpeningSpanCopyWithoutEnrichment() {
	phase := float64(0)
	frame := &sdk.DoorFramePresentation{Span: &sdk.WallFrameSpan{Before: 3, After: 4}, FaceFinishes: []sdk.DoorFrameFaceFinish{{Side: "positive-z", MaterialRef: "look:gray", PhaseU: &phase}}}
	door := atlasStructuralDoorsToProto([]sdk.AtlasStructuralDoor{{ID: "door", FramePresentation: frame}})[0]
	s.Require().NotNil(door.FramePresentation)
	s.Equal(float64(3), door.FramePresentation.Span.Before)
	s.Equal(float64(4), door.FramePresentation.Span.After)
	s.Require().Len(door.FramePresentation.FaceFinishes, 1)
	face := door.FramePresentation.FaceFinishes[0]
	s.Equal("positive-z", face.Side)
	s.Equal("look:gray", face.MaterialRef)
	s.Require().NotNil(face.PhaseU)
	s.Equal(float64(0), *face.PhaseU)
	*face.PhaseU = 0.9
	door.FramePresentation.Span.Before = 99
	s.Equal(float64(0), phase)
	s.Equal(float64(3), frame.Span.Before)
	frame.Span.After = 7
	s.Equal(float64(4), door.FramePresentation.Span.After)
	opening := atlasStructuralOpeningsToProto([]sdk.AtlasStructuralOpening{{ID: "gap", Width: 2, FrameSpan: frame.Span}})[0]
	s.Require().NotNil(opening.FrameSpan)
	s.Equal(float64(2), opening.Width)
	opening.FrameSpan.Before = 88
	s.Equal(float64(3), frame.Span.Before)
	s.Nil(atlasStructuralDoorsToProto([]sdk.AtlasStructuralDoor{{ID: "legacy"}})[0].FramePresentation)
	s.Nil(atlasStructuralOpeningsToProto([]sdk.AtlasStructuralOpening{{ID: "legacy"}})[0].FrameSpan)
	frame.FaceFinishes[0].PhaseU = nil
	s.Nil(atlasStructuralDoorsToProto([]sdk.AtlasStructuralDoor{{ID: "bad", FramePresentation: frame}})[0].FramePresentation.FaceFinishes[0].PhaseU, "translator never invents a missing phase")
}
