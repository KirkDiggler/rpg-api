package sessionv1alpha1

import (
	"context"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/sdkerr"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// GetStatus reports whether a session's encounter is open. Encounter-wide and
// never per-member (design's own pin, mirrored in sdk.Status): a convenience
// field answering "the" active actor would create a privileged clock by
// implication -- ask Turn instead.
func (h *Handler) GetStatus(ctx context.Context, req *sessionpb.GetStatusRequest) (*sessionpb.GetStatusResponse, error) {
	if _, err := authenticatedPlayerID(ctx); err != nil {
		return nil, err
	}

	out, err := h.manager.Status(ctx, &sdk.StatusInput{Session: req.GetSession()})
	if err != nil {
		return nil, sdkerr.StatusError(err)
	}

	sharing, err := partySharingToProto(out.Sharing)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return &sessionpb.GetStatusResponse{
		Open:         out.Open,
		Outcome:      outcomeToProto(out.Outcome),
		PartySharing: sharing,
	}, nil
}

// partySharingToProto mirrors the run's sharing dial (rpg-project#935) onto
// the wire enum. FAIL CLOSED: a word this build has no spelling for is a
// producer defect, refused rather than sent as UNSPECIFIED, which a client
// would have to guess at.
func partySharingToProto(s sdk.PartySharing) (sessionpb.PartySharing, error) {
	switch s {
	case sdk.PartySharingOwn:
		return sessionpb.PartySharing_PARTY_SHARING_OWN, nil
	case sdk.PartySharingView:
		return sessionpb.PartySharing_PARTY_SHARING_VIEW, nil
	case sdk.PartySharingKnowledge:
		return sessionpb.PartySharing_PARTY_SHARING_KNOWLEDGE, nil
	default:
		return sessionpb.PartySharing_PARTY_SHARING_UNSPECIFIED, fmt.Errorf("party sharing %q has no wire spelling", string(s))
	}
}
