package sessionv1alpha1

import (
	"context"

	"github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/sdkerr"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// errInteractNamesOneThing is the request-shape refusal for an Interact that
// names both a world NPC and a prop, or neither (rpg-project#959).
const errInteractNamesOneThing = "interact: name exactly one thing, a target or a prop"

// Interact reaches for a placed world NPC (rpg-toolkit#1404) and reports
// what it is, or does something to a prop (rpg-project#959): one of the
// prop's offered things to do, named by option_id at the revision it was
// offered. Identity, reach and sight are the SDK's own to confirm; this
// handler is pure proto <-> SDK translation, no rule lives here (design rule
// 8). EXACTLY ONE of target and prop is named, checked here because it is
// the request's shape.
func (h *Handler) Interact(ctx context.Context, req *sessionpb.InteractRequest) (*sessionpb.InteractResponse, error) {
	if err := h.callerActingAs(ctx, req.GetActor()); err != nil {
		return nil, err
	}
	if (req.GetTarget() == "") == (req.GetProp() == "") {
		return nil, status.Error(codes.InvalidArgument, errInteractNamesOneThing)
	}

	out, err := h.manager.Interact(ctx, &sdk.InteractInput{
		Session:  req.GetSession(),
		Actor:    req.GetActor(),
		Target:   req.GetTarget(),
		Range:    int(req.GetRange()),
		Prop:     req.GetProp(),
		OptionID: req.GetOptionId(),
		Revision: req.GetRevision(),
	})
	if err != nil {
		return nil, sdkerr.StatusErrorFor(err, req.GetActor())
	}

	resp := &sessionpb.InteractResponse{
		Saved:    saveReportToProto(ctx, req.GetSession(), out.Saved),
		Delivery: deliveryReportToProto(out.Delivery),
		Done:     interactDoneToProto(out.Done),
	}
	if req.GetTarget() != "" {
		descriptor, err := worldNPCDescriptorToProto(out.Descriptor)
		if err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
		resp.Descriptor_ = descriptor
	}
	return resp, nil
}

// interactDoneToProto mirrors what was done to a prop (rpg-project#959).
// Nil in is nil out: a world NPC reached carries no done. Roll keeps its
// presence law: set only while the check waits on an offer.
func interactDoneToProto(d *sdk.DoneOutput) *sessionpb.InteractDone {
	if d == nil {
		return nil
	}
	out := &sessionpb.InteractDone{
		Prop: d.Prop, OptionId: d.OptionID, Beaten: d.Beaten,
		Total: int32(d.Total), Dc: int32(d.DC), //nolint:gosec // a check's numbers fit int32
		Paused: d.Paused,
	}
	// The approach the check was made with; a thing done without a check
	// crosses none rather than an empty approach.
	if d.Applied != (sdk.DoorApproach{}) {
		out.Applied = doorApproachToProto(d.Applied)
	}
	if d.Roll != nil {
		roll := int32(*d.Roll) //nolint:gosec // a die fits int32
		out.Roll = &roll
	}
	return out
}
