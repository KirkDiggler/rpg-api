package sessionv1alpha1

import (
	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

// The SDK has already validated/captured these owner-scoped visual identities.
func materialOverridesToProto(values []sdk.MaterialOverride) []*sessionpb.MaterialOverride {
	if len(values) == 0 {
		return nil
	}
	out := make([]*sessionpb.MaterialOverride, len(values))
	for i, value := range values {
		out[i] = &sessionpb.MaterialOverride{SurfaceId: value.SurfaceID, MaterialRef: value.MaterialRef}
	}
	return out
}
