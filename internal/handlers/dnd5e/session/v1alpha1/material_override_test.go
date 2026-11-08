package sessionv1alpha1

import (
	"testing"

	"github.com/stretchr/testify/suite"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

type MaterialOverrideSuite struct{ suite.Suite }

func TestMaterialOverrideSuite(t *testing.T) { suite.Run(t, new(MaterialOverrideSuite)) }
func (s *MaterialOverrideSuite) TestBothCapturedOwnersCarryDetachedChoices() {
	values := []sdk.MaterialOverride{{SurfaceID: "atlas", MaterialRef: "look:04_C"}}
	walls := atlasStructuralWallsToProto([]sdk.AtlasStructuralWall{{ID: "wall", MaterialOverrides: values}})
	prop := propPresentationToProto(sdk.PropPresentation{ID: "prop", MaterialOverrides: values})
	s.Require().Len(walls[0].MaterialOverrides, 1)
	s.Require().Len(prop.MaterialOverrides, 1)
	s.Equal("atlas", walls[0].MaterialOverrides[0].SurfaceId)
	s.Equal("look:04_C", walls[0].MaterialOverrides[0].MaterialRef)
	s.Equal("atlas", prop.MaterialOverrides[0].SurfaceId)
	s.Equal("look:04_C", prop.MaterialOverrides[0].MaterialRef)
	walls[0].MaterialOverrides[0].MaterialRef = "changed"
	prop.MaterialOverrides[0].MaterialRef = "changed"
	s.Equal("look:04_C", values[0].MaterialRef)
	s.Empty(propPresentationToProto(sdk.PropPresentation{}).MaterialOverrides)
}
