package sessionv1alpha1

import (
	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

// Copy already validated, recipient-permitted choices without resolving assets.
func wallFaceFinishesToProto(values []sdk.WallFaceFinish) []*sessionpb.WallFaceFinish {
	if len(values) == 0 {
		return nil
	}
	out := make([]*sessionpb.WallFaceFinish, len(values))
	for i, value := range values {
		out[i] = &sessionpb.WallFaceFinish{Side: value.Side, MaterialRef: value.MaterialRef}
	}
	return out
}
