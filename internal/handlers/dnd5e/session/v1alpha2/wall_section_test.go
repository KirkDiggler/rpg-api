package sessionv1alpha2

import sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

func (s *ConvertActSuite) TestWallSectionsCopyCanonicalFactsWithoutMakingOpenings() {
	source := []sdk.WallSection{{ID: "window", Ref: "opaque:window", Position: 8.25, Width: 3.75}}
	rows := structuralWallsToProto([]sdk.AtlasStructuralWall{{ID: "wall", Sections: source}})
	s.Require().Len(rows, 1)
	s.Require().Len(rows[0].Sections, 1)
	section := rows[0].Sections[0]
	s.Equal("window", section.Id)
	s.Equal("opaque:window", section.Ref)
	s.Equal(8.25, section.Position)
	s.Equal(3.75, section.Width)
	s.Empty(rows[0].Openings)
	section.Ref = "caller:mutation"
	s.Equal("opaque:window", source[0].Ref)
	s.Empty(structuralWallsToProto([]sdk.AtlasStructuralWall{{ID: "legacy"}})[0].Sections)
}
