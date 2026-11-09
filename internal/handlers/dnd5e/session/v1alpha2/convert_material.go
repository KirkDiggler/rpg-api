package sessionv1alpha2

import (
	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

// No resource lookup or audience calculation belongs at this wire boundary.
func materialOverridesToProto(values []sdk.MaterialOverride) []*sessionpb.MaterialOverride {
	return mapSlice(values, func(value sdk.MaterialOverride) *sessionpb.MaterialOverride {
		return &sessionpb.MaterialOverride{SurfaceId: value.SurfaceID, MaterialRef: value.MaterialRef}
	})
}
