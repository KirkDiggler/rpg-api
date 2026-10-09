package sessionv1alpha1

import (
	"context"

	"github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/sdkerr"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// GetSeat answers which run holds a character. A character holding no live
// seat is NotFound (sdk.ErrNoSeat); a success always names a session. The
// server never exits anyone here: reading a seat is not a verb.
func (h *Handler) GetSeat(ctx context.Context, req *sessionpb.GetSeatRequest) (*sessionpb.GetSeatResponse, error) {
	// callerActingAs is the single member gate: it refuses an empty
	// character with InvalidArgument and a foreign one with PermissionDenied.
	if err := h.callerActingAs(ctx, req.GetCharacter()); err != nil {
		return nil, err
	}

	out, err := h.manager.Seat(ctx, &sdk.SeatInput{Character: req.GetCharacter()})
	if err != nil {
		return nil, sdkerr.StatusError(err)
	}

	return &sessionpb.GetSeatResponse{Session: out.Seat.Session}, nil
}
