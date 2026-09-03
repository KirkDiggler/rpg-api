package sessionv1alpha1

import (
	"context"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// DeathSave executes the authenticated member's selected provider declaration.
// It is deliberately one ownership gate, one Manager call, and a field-for-
// field projection: all eligibility, roll, progress, and continuation rules
// remain in the released session provider.
func (h *Handler) DeathSave(
	ctx context.Context,
	req *sessionpb.DeathSaveRequest,
) (*sessionpb.DeathSaveResponse, error) {
	if err := h.callerActingAs(ctx, req.GetMember()); err != nil {
		return nil, err
	}

	out, err := h.manager.DeathSave(ctx, &sdk.DeathSaveInput{
		Session:       req.GetSession(),
		Member:        req.GetMember(),
		DeclarationID: req.GetDeclarationId(),
	})
	if err != nil {
		return nil, statusError(err)
	}

	return &sessionpb.DeathSaveResponse{
		Roll:              int32(out.Roll),
		Outcome:           deathSaveOutcomeToProto(out.Outcome),
		SuccessesAdded:    int32(out.SuccessesAdded),
		FailuresAdded:     int32(out.FailuresAdded),
		Successes:         int32(out.Successes),
		Failures:          int32(out.Failures),
		SuccessesNeeded:   int32(out.SuccessesNeeded),
		FailuresRemaining: int32(out.FailuresRemaining),
		Stabilized:        out.Stabilized,
		Dead:              out.Dead,
		Recovered:         out.Recovered,
		HpRestored:        int32(out.HPRestored),
		Continuation:      deathSaveContinuationToProto(out.Continuation),
		PresentationId:    out.PresentationID,
		Seq:               out.Seq,
		Saved:             saveReportToProto(out.Saved),
		Delivery:          deliveryReportToProto(out.Delivery),
	}, nil
}
