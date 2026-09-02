package sessionv1alpha1

import (
	"context"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// GetAtlas returns a session's world map AS ONE MEMBER KNOWS IT. Construction
// truth for what that member has learned -- unchanged by movement, joins,
// exits or endings -- so a client should fetch it once per encounter and
// cache it, never per frame, and re-fetch (or patch from reveal beats) when
// something new is found.
//
// Member is bound to the authenticated caller, never trusted from the
// request -- the same law GetWhere and Afford keep (rpg-api-protos#266):
// concealed structure makes this a member-scoped answer, and wiring a
// client-supplied id through unchecked would let one player read another's
// reveals.
func (h *Handler) GetAtlas(ctx context.Context, req *sessionpb.GetAtlasRequest) (*sessionpb.GetAtlasResponse, error) {
	if err := h.callerActingAs(ctx, req.GetMember()); err != nil {
		return nil, err
	}

	atlas, err := h.manager.Atlas(ctx, &sdk.AtlasInput{Session: req.GetSession(), Member: req.GetMember()})
	if err != nil {
		return nil, statusError(err)
	}

	return AtlasToProto(atlas), nil
}
