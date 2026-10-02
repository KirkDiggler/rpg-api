package sessionv1alpha1

import (
	"context"
	"errors"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/sdkerr"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

var errKnowledgeOutputRequired = errors.New("knowledge returned no output")

// GetKnowledge binds the caller and returns the SDK's coherent snapshot. The
// SDK verifies ownership of the exact requested seat before projecting knowledge.
func (h *Handler) GetKnowledge(ctx context.Context, req *sessionpb.GetKnowledgeRequest) (*sessionpb.GetKnowledgeResponse, error) {
	if err := h.callerActingAs(ctx, req.GetMember()); err != nil {
		return nil, err
	}
	out, err := h.manager.Knowledge(ctx, &sdk.KnowledgeInput{Session: req.GetSession(), Member: req.GetMember(), Player: auth.GetPlayerID(ctx)})
	if err != nil {
		return nil, sdkerr.StatusError(err)
	}
	if out == nil {
		return nil, sdkerr.StatusError(errKnowledgeOutputRequired)
	}
	return &sessionpb.GetKnowledgeResponse{
		Atlas: AtlasToProto(&out.Atlas), View: viewToProto(&out.View), Where: whereToProto(&out.Where),
		Roster: rosterToProto(&out.Roster), Holding: append([]string(nil), out.Holding...), Seq: out.Seq,
	}, nil
}
