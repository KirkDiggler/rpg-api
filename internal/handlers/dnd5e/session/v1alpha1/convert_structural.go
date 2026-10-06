package sessionv1alpha1

import (
	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

// These are already permitted SDK records. Preserve their identity, order,
// units and complete cut lists; the host neither filters nor enriches them.
func atlasStructuralWallsToProto(walls []sdk.AtlasStructuralWall) []*sessionpb.AtlasStructuralWall {
	out := make([]*sessionpb.AtlasStructuralWall, 0, len(walls))
	for _, wall := range walls {
		openings := make([]*sessionpb.AtlasStructuralOpening, 0, len(wall.Openings))
		for _, opening := range wall.Openings {
			openings = append(openings, &sessionpb.AtlasStructuralOpening{
				Id: opening.ID, Position: opening.Position, Width: opening.Width,
			})
		}
		out = append(out, &sessionpb.AtlasStructuralWall{
			Id: wall.ID, Ref: wall.Ref,
			From:   &sessionpb.FootprintPoint{X: wall.From.X, Y: wall.From.Y},
			To:     &sessionpb.FootprintPoint{X: wall.To.X, Y: wall.To.Y},
			Height: wall.Height, Thickness: wall.Thickness, Elevation: wall.Elevation,
			Openings: openings,
		})
	}
	return out
}

func atlasStructuralDoorsToProto(doors []sdk.AtlasStructuralDoor) []*sessionpb.AtlasStructuralDoor {
	out := make([]*sessionpb.AtlasStructuralDoor, 0, len(doors))
	for _, door := range doors {
		out = append(out, &sessionpb.AtlasStructuralDoor{
			Id: door.ID, Ref: door.Ref,
			From:   &sessionpb.FootprintPoint{X: door.From.X, Y: door.From.Y},
			To:     &sessionpb.FootprintPoint{X: door.To.X, Y: door.To.Y},
			Height: door.Height, Thickness: door.Thickness, Elevation: door.Elevation,
		})
	}
	return out
}
