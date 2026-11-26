package sessionv1alpha1

import (
	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

// These facts are already captured and recipient-permitted. Keep canonical
// units and wall-local identity; never infer an opening or resolve asset data.
func wallSectionsToProto(sections []sdk.WallSection) []*sessionpb.WallSection {
	out := make([]*sessionpb.WallSection, len(sections))
	for i, section := range sections {
		out[i] = &sessionpb.WallSection{Id: section.ID, Ref: section.Ref, Position: section.Position, Width: section.Width}
	}
	return out
}
