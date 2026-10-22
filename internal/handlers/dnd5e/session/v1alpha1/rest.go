package sessionv1alpha1

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/sdkerr"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

const (
	errRestersRequired = "resters is required"
	errRestKindUnknown = "kind is required"
)

// Rest is the party's short rest (rpg-project#542): the named members rest
// together, one hour for all of them, each spending their own hit dice.
//
// TRANSPORT ONLY. Which rest is legal, what a hit die heals, what refills and
// what ends are the SDK's; this binds the caller, spells the request, calls
// Manager.Rest once and maps its refusal. The outcome rides each rester's
// RESTED beat (design rule 2), so the response is the save and delivery
// reports alone.
//
// THE CALLER IS BOUND TO THE SESSION through a rester they control: the
// caller must own at least one rester and be seated as that member. The SDK
// refuses a rester the session does not hold, so every rester named is a
// member of the caller's own session.
func (h *Handler) Rest(ctx context.Context, req *sessionpb.RestRequest) (*sessionpb.RestResponse, error) {
	if _, err := authenticatedPlayerID(ctx); err != nil {
		return nil, err
	}
	resters := restersFromProto(req)
	if len(resters) == 0 {
		return nil, status.Error(codes.InvalidArgument, errRestersRequired)
	}
	kind, err := restKindFromProto(req.GetKind())
	if err != nil {
		return nil, err
	}
	if seatErr := h.callerSeatedAsARester(ctx, req.GetSession(), resters); seatErr != nil {
		return nil, seatErr
	}

	out, err := h.manager.Rest(ctx, &sdk.RestInput{
		Session: req.GetSession(),
		Kind:    kind,
		Resters: resters,
	})
	if err != nil {
		return nil, sdkerr.StatusError(err)
	}

	return &sessionpb.RestResponse{
		Saved:    saveReportToProto(out.Saved),
		Delivery: deliveryReportToProto(out.Delivery),
	}, nil
}

// restersFromProto reads `resters`, or — for a request from before a rest
// was the party's act — the deprecated singular member and hit_dice as one
// rester, as the proto documents.
func restersFromProto(req *sessionpb.RestRequest) []sdk.Rester {
	if named := req.GetResters(); len(named) > 0 {
		out := make([]sdk.Rester, len(named))
		for i, r := range named {
			out[i] = sdk.Rester{Member: r.GetMember(), HitDice: int(r.GetHitDice())}
		}
		return out
	}
	//nolint:staticcheck // the documented legacy fallback for the deprecated singular fields
	if member := req.GetMember(); member != "" {
		//nolint:staticcheck // the documented legacy fallback for the deprecated singular fields
		return []sdk.Rester{{Member: member, HitDice: int(req.GetHitDice())}}
	}
	return nil
}

// callerSeatedAsARester passes when the caller controls one of the resters
// and is seated in the session as that member. The first refusal is the
// answer when none passes.
func (h *Handler) callerSeatedAsARester(ctx context.Context, session string, resters []sdk.Rester) error {
	gate, err := h.accessGate()
	if err != nil {
		return err
	}
	var first error
	for _, r := range resters {
		seatErr := gate.CallerMemberSeated(ctx, session, r.Member)
		if seatErr == nil {
			return nil
		}
		if first == nil {
			first = seatErr
		}
	}
	return first
}

// restKindFromProto spells the wire's rest kind in the SDK's words. LONG is
// passed through for the SDK to refuse (R13, deferred), so the refusal is the
// SDK's sentence, not a second copy of it here.
func restKindFromProto(kind sessionpb.RestKind) (sdk.RestKind, error) {
	switch kind {
	case sessionpb.RestKind_REST_KIND_SHORT:
		return sdk.RestShort, nil
	case sessionpb.RestKind_REST_KIND_LONG:
		return sdk.RestLong, nil
	default:
		return "", status.Error(codes.InvalidArgument, errRestKindUnknown)
	}
}
