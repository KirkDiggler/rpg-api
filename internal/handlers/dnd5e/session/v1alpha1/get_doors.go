package sessionv1alpha1

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"

	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	rosterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/roster"
)

// GetDoors reports every door's live state — the dynamic half of GetAtlas's
// doorways (rpg-project#268). The atlas says where a door's edges are and
// never changes; this says what each door is doing now. A client fetches it
// once and keeps it fresh from DOOR events on the stream.
//
// Authorization: the caller must be SEATED — the same gate GetRoster ruled
// (rpg-project#264): no member parameter to gate on, so the entitlement is
// membership itself, checked against the launch-written roster row.
func (h *Handler) GetDoors(ctx context.Context, req *sessionpb.GetDoorsRequest) (*sessionpb.GetDoorsResponse, error) {
	playerID, err := authenticatedPlayerID(ctx)
	if err != nil {
		return nil, err
	}
	if req.GetSession() == "" {
		return nil, status.Error(codes.InvalidArgument, "session is required")
	}

	if seatedErr := h.callerSeated(ctx, req.GetSession(), playerID); seatedErr != nil {
		return nil, seatedErr
	}

	out, err := h.manager.Doors(ctx, &sdk.DoorsInput{Session: req.GetSession()})
	if err != nil {
		return nil, statusError(err)
	}

	doors := make([]*sessionpb.DoorInfo, 0, len(out.Doors))
	for _, d := range out.Doors {
		doors = append(doors, doorToProto(d))
	}
	return &sessionpb.GetDoorsResponse{Doors: doors}, nil
}

// callerSeated is the membership gate a read with no member parameter uses:
// some player row of the launch-written roster must belong to the caller.
// The same entitlement GetRoster computes inline; factored here because
// GetDoors needs the verdict without the projection.
func (h *Handler) callerSeated(ctx context.Context, session, playerID string) error {
	row, err := h.roster.Get(ctx, session)
	if err != nil {
		if errors.Is(err, rosterrepo.ErrNotFound) {
			return status.Errorf(codes.NotFound, "session %q has no roster", session)
		}
		return status.Errorf(codes.Internal, "load roster for session %q: %v", session, err)
	}

	for _, m := range row.Members {
		if m.Kind != rosterrepo.KindPlayer {
			continue
		}
		got, err := h.characters.Get(ctx, characterrepo.GetInput{ID: m.ID})
		if err != nil || got == nil || got.Character == nil || got.Character.Data == nil {
			return status.Errorf(codes.Internal, "roster member %q has no character record", m.ID)
		}
		if got.Character.Data.PlayerID == playerID {
			return nil
		}
	}
	return status.Error(codes.PermissionDenied, "caller is not seated in this session")
}
