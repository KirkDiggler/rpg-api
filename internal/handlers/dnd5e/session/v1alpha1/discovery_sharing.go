package sessionv1alpha1

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/sdkerr"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
)

// SetDiscoverySharing updates only the authenticated caller's exact seated
// character.
//
// Deprecated: discovery sharing is character configuration (rpg-project#753
// R2); CharacterService.SetPreference is its verb. This handler stays for the
// web until it moves, and writes the same store for the seated character
// under the character's guard, never the session's. The encounter takes the
// change up at that character's next write verb, so the response's reports
// are empty: nothing in the session was saved or delivered.
func (h *Handler) SetDiscoverySharing(ctx context.Context, req *sessionpb.SetDiscoverySharingRequest) (*sessionpb.SetDiscoverySharingResponse, error) {
	gate, err := h.accessGate()
	if err != nil {
		return nil, err
	}
	if seatErr := gate.CallerMemberSeated(ctx, req.GetSession(), req.GetMember()); seatErr != nil {
		return nil, seatErr
	}
	if req.Sharing == nil {
		return nil, status.Error(codes.InvalidArgument, "sharing must be explicitly supplied")
	}
	if h.preferences == nil {
		return nil, status.Error(codes.Unimplemented, "discovery sharing: no character configuration is wired")
	}
	sharing := req.GetSharing()
	out, err := h.preferences.SetPreference(ctx, &sessionorch.SetPreferenceInput{
		CharacterID: req.GetMember(), DiscoverySharing: &sharing,
	})
	if err != nil {
		return nil, sdkerr.StatusError(err)
	}
	if out.DiscoverySharing == nil {
		return nil, status.Error(codes.Internal, "discovery sharing: the stored choice is missing after the write")
	}
	return &sessionpb.SetDiscoverySharingResponse{
		Sharing:  *out.DiscoverySharing,
		Saved:    &sessionpb.SaveReport{},
		Delivery: &sessionpb.DeliveryReport{},
	}, nil
}
