package sessionv1alpha1

import (
	"context"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// Cast casts a cantrip the member knows -- True Strike, Vicious Mockery --
// at the target the offer named.
//
// ACTIVATE'S TWIN, and separate from it on purpose (rpg-project#405 R1).
// Physically a cast could ride Activate: ActivateRequest is
// {session, member, declaration_id, target} with no ability ref, and so is
// this. It does not, because a cantrip is on neither list an activation
// reads -- not the sheet's abilities, not the activation machine's effect
// set, which has no damage -- and because what comes next is cast-shaped
// rather than activation-shaped: a slot at the door, Counterspell, upcast.
// The rename would be owed either now or later at a higher price.
//
// NO SPELL REF ON THE REQUEST. The declaration id already names it: Afford
// compiles one offer per castable cantrip, so a caller naming the spell
// would be deciding something Afford already decided, and the two would be
// free to disagree. Exactly Activate's argument, one verb over.
//
// The response is an acknowledgement carrying only the two S6 reports. What
// the cast DID reaches every client on the stream -- CAST, then SAVED when
// the spell forced a roll, then one ACTIVATION_RESULT per delivered effect
// -- and putting any of it here would make this a second, rival account of
// beats the stream already tells. That is why CastOutput.Saved, the saving
// throw itself, is deliberately not copied onto this response: the SAVED
// beat is where a save is read, and the wire has no field for it here.
//
// statusError owns the refusals unchanged; it is verb-agnostic and needed no
// new arm. ErrBadCast (a target named on a self-cast, or omitted on one that
// needs it) is INVALID_ARGUMENT; a target that drifted out of range is
// ErrStaleDeclaration, FAILED_PRECONDITION, exactly as today.
func (h *Handler) Cast(
	ctx context.Context, req *sessionpb.CastRequest,
) (*sessionpb.CastResponse, error) {
	if err := h.callerActingAs(ctx, req.GetMember()); err != nil {
		return nil, err
	}

	// Keep both wire forms separate for the SDK boundary to normalize and
	// reject on conflict. Copy the canonical list so provider code cannot
	// mutate protobuf-owned request memory.
	targets := append([]string(nil), req.GetTargets()...)
	out, err := h.manager.Cast(ctx, &sdk.CastInput{
		Session:       req.GetSession(),
		Member:        req.GetMember(),
		DeclarationID: req.GetDeclarationId(),
		Target:        req.GetTarget(), //nolint:staticcheck // Required deprecated scalar request compatibility.
		Targets:       targets,
		// A REFERENCE, copied and not read. The cell is where the caster
		// pointed a shape they never computed: which shape it is, whether
		// the caster's own cell is a legal aim, and whether this declaration
		// needed a cell at all are rules, and session refuses on every one of
		// them. Nothing here inspects the cell, exactly as nothing here
		// inspects the path a Move carries.
		Cell: positionPtrFromProto(req.GetCell()),
	})
	if err != nil {
		return nil, statusError(err)
	}

	// Persisted, not Saved. The SDK renamed Activate's persistence report to
	// Persisted here precisely because Cast has a SECOND thing called a save
	// -- the gate's saving throw -- and the wire's `saved` field is the
	// persistence one in both responses.
	return &sessionpb.CastResponse{
		Saved:    saveReportToProto(out.Persisted),
		Delivery: deliveryReportToProto(out.Delivery),
		Caught:   caughtMembersToProto(out.Caught),
	}, nil
}
