package sessionv1alpha2

import sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

func (s *ConvertActSuite) TestBothAppearanceOwnersCarryCapturedMaterialChoices() {
	values := []sdk.MaterialOverride{{SurfaceID: "atlas", MaterialRef: "look:04_C"}}
	walls := structuralWallsToProto([]sdk.AtlasStructuralWall{{ID: "wall", MaterialOverrides: values}})
	props := propPresentationsToProto([]sdk.PropPresentation{{ID: "prop", MaterialOverrides: values}})
	s.Require().Len(walls[0].MaterialOverrides, 1)
	s.Require().Len(props[0].MaterialOverrides, 1)
	s.Equal("atlas", walls[0].MaterialOverrides[0].SurfaceId)
	s.Equal("look:04_C", walls[0].MaterialOverrides[0].MaterialRef)
	s.Equal("atlas", props[0].MaterialOverrides[0].SurfaceId)
	s.Equal("look:04_C", props[0].MaterialOverrides[0].MaterialRef)
	walls[0].MaterialOverrides[0].MaterialRef = "changed"
	props[0].MaterialOverrides[0].MaterialRef = "changed"
	s.Equal("look:04_C", values[0].MaterialRef)
}
