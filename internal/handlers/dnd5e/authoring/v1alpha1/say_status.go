package authoringv1alpha1

import (
	"context"
	"encoding/json"

	authoringpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/authoring/v1alpha1"
	authoringorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/authoring"
)

// SayStatus says one whole status effect in a sentence built from the rules'
// own labels (status.Say, rpg-project#1036, #1097), or marks the row the rules
// refuse.
//
// status_json crosses as the engine's input untouched: this api neither
// parses nor checks it. A refusal is one FieldError carrying the rules'
// Path and Says exactly as given; a refusal about the whole status has an
// empty path, and none is invented. Any other failure is an error, never an
// empty sentence.
func (h *Handler) SayStatus(
	ctx context.Context, req *authoringpb.SayStatusRequest,
) (*authoringpb.SayStatusResponse, error) {
	if err := requireAuthenticated(ctx); err != nil {
		return nil, err
	}

	out, err := h.orch.SayStatus(ctx, &authoringorch.SayStatusInput{
		ID:     req.GetId(),
		Status: json.RawMessage(req.GetStatusJson()),
	})
	if err != nil {
		return nil, statusError(err)
	}

	return sayStatusToProto(out), nil
}

// sayStatusToProto is the sentence, or the refusal, on the wire.
func sayStatusToProto(out *authoringorch.SayStatusOutput) *authoringpb.SayStatusResponse {
	resp := &authoringpb.SayStatusResponse{}
	if r := out.Refusal; r != nil {
		resp.Errors = []*authoringpb.FieldError{{Path: r.Path, Message: r.Says}}
		return resp
	}
	resp.Name = out.Said.Name
	resp.Sentence = out.Said.Sentence
	resp.Parts = make([]*authoringpb.StatusSaidPart, 0, len(out.Said.Parts))
	for _, p := range out.Said.Parts {
		resp.Parts = append(resp.Parts, &authoringpb.StatusSaidPart{Path: p.Path, Text: p.Text})
	}
	return resp
}
