package sessionv1alpha1

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/sdkerr"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// errSpeakNamesNoTarget is the request-shape refusal for a Speak that names
// neither a member nor a prop, or names one as the empty string.
const errSpeakNamesNoTarget = "speak: name exactly one target, a member_id or a prop_id"

// Speak says one authored line to a creature or a prop (rpg-project#569). It
// replaces Intimidate and Persuade: the line is the option the target offers,
// the check is the option's own, and the skill it rolls decides the deed.
//
// THE RESPONSE CARRIES NO beaten, total OR dc, under the ruling the two verbs
// it replaces kept (Kirk, rpg-api-protos#339). The outcome reaches the actor
// the way it reaches everyone else -- as the `spoke` beat on the stream, and
// the target's answer as the `answered` beat after it. A response repeating
// the numbers would be a second, differently shaped account of one throw. So
// SpeakOutput.Beaten, .Total, .DC, .Applied and the echoes are read and
// dropped here.
//
// WHAT IS LEFT IS THE CALLER'S ALONE: Paused and Roll are the offer window
// (Bardic Inspiration), asked before any beat exists, and Saved/Delivery are
// S6's partial-write report that every mutating verb at this seam carries.
//
// EXACTLY ONE TARGET ARM, checked here because it is the request's shape and
// not a rule: the oneof already forbids both, and an absent or empty arm is
// refused as InvalidArgument before the SDK is called. Which lines a target
// offers, and whether this speaker may say them, is the SDK's answer
// (ErrNoOption), never this handler's.
//
// A MISSED LINE IS AN OUTCOME, NOT AN ERROR. The option's `failure` rows
// still fire, which is where the goblin's bad directions come from.
func (h *Handler) Speak(
	ctx context.Context, req *sessionpb.SpeakRequest,
) (*sessionpb.SpeakResponse, error) {
	if err := h.callerActingAs(ctx, req.GetMember()); err != nil {
		return nil, err
	}

	in, err := speakInputFromProto(req)
	if err != nil {
		return nil, err
	}

	out, err := h.manager.Speak(ctx, in)
	if err != nil {
		return nil, sdkerr.StatusError(err)
	}

	return &sessionpb.SpeakResponse{
		Paused:   out.Paused,
		Roll:     speakRollToProto(out.Roll),
		Saved:    saveReportToProto(out.Saved),
		Delivery: deliveryReportToProto(out.Delivery),
	}, nil
}

// speakInputFromProto is the request's one conversion point: the oneof arm
// becomes exactly one of SpeakInput's Target and Prop.
func speakInputFromProto(req *sessionpb.SpeakRequest) (*sdk.SpeakInput, error) {
	in := &sdk.SpeakInput{
		Session:  req.GetSession(),
		Member:   req.GetMember(),
		OptionID: req.GetOptionId(),
	}
	switch target := req.GetTarget().(type) {
	case *sessionpb.SpeakRequest_MemberId:
		in.Target = target.MemberId
	case *sessionpb.SpeakRequest_PropId:
		in.Prop = target.PropId
	}
	if in.Target == "" && in.Prop == "" {
		return nil, status.Error(codes.InvalidArgument, errSpeakNamesNoTarget)
	}
	return in, nil
}

// speakRollToProto mirrors SpeakOutput.Roll's presence law -- a *int
// meaningful only while Paused -- onto the wire's optional int32. Nil stays
// nil rather than crossing as a meaningless zero, as unlockRollToProto's does.
func speakRollToProto(roll *int) *int32 {
	if roll == nil {
		return nil
	}
	converted := int32(*roll)
	return &converted
}
