package sessionv1alpha2

import (
	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

// Copy the captured section facts without a source lookup, unit conversion or
// gameplay interpretation. This version owns its mapper, not v1alpha1's types.
func wallSectionsToProto(sections []sdk.WallSection) []*sessionpb.WallSection {
	out := make([]*sessionpb.WallSection, len(sections))
	for i, section := range sections {
		out[i] = &sessionpb.WallSection{Id: section.ID, Ref: section.Ref, Position: section.Position, Width: section.Width}
	}
	return out
}
