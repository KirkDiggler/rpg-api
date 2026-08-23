package sessionv1alpha1

import (
	"context"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// GetAtlas returns a session's static world map. Construction truth --
// unchanged by movement, joins, exits or endings -- so a client should fetch
// it once per encounter and cache it, never per frame.
func (h *Handler) GetAtlas(ctx context.Context, req *sessionpb.GetAtlasRequest) (*sessionpb.GetAtlasResponse, error) {
	if _, err := authenticatedPlayerID(ctx); err != nil {
		return nil, err
	}

	atlas, err := h.manager.Atlas(ctx, &sdk.AtlasInput{Session: req.GetSession()})
	if err != nil {
		return nil, statusError(err)
	}

	return AtlasToProto(atlas), nil
}
