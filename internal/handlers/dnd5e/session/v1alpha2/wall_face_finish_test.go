package sessionv1alpha2

import sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

func (s *ConvertActSuite) TestWallFaceChoicesAreCopiedWithoutEnrichment() {
	values := []sdk.WallFaceFinish{{Side: "positive-z", MaterialRef: "look:brick"}, {Side: "negative-z", MaterialRef: "look:stucco"}}
	walls := structuralWallsToProto([]sdk.AtlasStructuralWall{{ID: "wall", FaceFinishes: values}})
	s.Require().Len(walls[0].FaceFinishes, 2)
	s.Equal("positive-z", walls[0].FaceFinishes[0].Side)
	s.Equal("look:brick", walls[0].FaceFinishes[0].MaterialRef)
	s.Equal("negative-z", walls[0].FaceFinishes[1].Side)
	s.Equal("look:stucco", walls[0].FaceFinishes[1].MaterialRef)
	walls[0].FaceFinishes[0].MaterialRef = "caller:mutation"
	s.Equal("look:brick", values[0].MaterialRef)
	s.Empty(structuralWallsToProto([]sdk.AtlasStructuralWall{{ID: "legacy"}})[0].FaceFinishes)
}
