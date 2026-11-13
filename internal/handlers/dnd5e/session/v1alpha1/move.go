package sessionv1alpha1

import (
	"context"

	"github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/sdkerr"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// Move walks a member along a path of cells, one at a time.
//
// A world-clock walk may answer before it ends (rpg-project#879 R2):
// continues is set, the status is CONTINUING, the steps are those before the
// one that pays a round, and through names the record id the rest continues
// from on the stream. Saved and Delivery are then empty, because the save
// and the delivery belong to the rest of the walk.
func (h *Handler) Move(ctx context.Context, req *sessionpb.MoveRequest) (*sessionpb.MoveResponse, error) {
	if err := h.callerActingAs(ctx, req.GetMember()); err != nil {
		return nil, err
	}

	path := make([]spatial.Position, len(req.GetPath()))
	for i, p := range req.GetPath() {
		path[i] = positionFromProto(p)
	}

	out, err := h.manager.Move(ctx, &sdk.MoveInput{
		Session:       req.GetSession(),
		Member:        req.GetMember(),
		DeclarationID: req.GetDeclarationId(),
		Path:          path,
	})
	if err != nil {
		return nil, sdkerr.StatusErrorFor(err, req.GetMember())
	}

	return &sessionpb.MoveResponse{
		JoinedCombat: out.JoinedCombat,
		Status:       movementStatusToProto(out.Status),
		StopReason:   out.StopReason,
		Steps:        stepsToProto(out.Steps),
		Discovered:   discoveriesToProto(out.Discovered),
		Outcome:      outcomeToProto(out.Outcome),
		Formed:       formedToProto(out.Formed),
		Saved:        saveReportToProto(ctx, req.GetSession(), out.Saved),
		Delivery:     deliveryReportToProto(out.Delivery),
		Continues:    out.Continues,
		Through:      out.Through,
	}, nil
}

func movementStatusToProto(status sdk.MovementStatus) sessionpb.MovementStatus {
	switch status {
	case sdk.MovementCompleted:
		return sessionpb.MovementStatus_MOVEMENT_STATUS_COMPLETED
	case sdk.MovementPaused:
		return sessionpb.MovementStatus_MOVEMENT_STATUS_PAUSED
	case sdk.MovementStopped:
		return sessionpb.MovementStatus_MOVEMENT_STATUS_STOPPED
	case sdk.MovementContinuing:
		return sessionpb.MovementStatus_MOVEMENT_STATUS_CONTINUING
	default:
		return sessionpb.MovementStatus_MOVEMENT_STATUS_UNSPECIFIED
	}
}
