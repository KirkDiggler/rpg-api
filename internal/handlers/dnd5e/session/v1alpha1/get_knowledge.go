package sessionv1alpha1

import (
	"context"
	"errors"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/sdkerr"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
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
	sharing, err := h.configuredSharing(ctx, req.GetMember(), out.DiscoverySharing)
	if err != nil {
		return nil, sdkerr.StatusError(err)
	}
	return &sessionpb.GetKnowledgeResponse{
		DiscoverySharing: sharing,
		Atlas:            AtlasToProto(&out.Atlas), View: viewToProto(&out.View), Where: whereToProto(&out.Where),
		Roster: rosterToProto(&out.Roster), Holding: append([]string(nil), out.Holding...),
	}, nil
}

// configuredSharing is the discovery-sharing value GetKnowledge reports. The
// encounter's copy is restored from character configuration only at the
// member's next write verb (rpg-project#753 R2), so right after a change the
// configuration is ahead of it: a choice the character made is reported as
// made. A character who never chose keeps the encounter's resolved default,
// so the default stays the session's to decide. A producer without the
// capability (nil) stays absent.
func (h *Handler) configuredSharing(ctx context.Context, member string, encounterSharing *bool) (*bool, error) {
	if encounterSharing == nil || h.preferences == nil {
		return encounterSharing, nil
	}
	configured, err := h.preferences.GetPreference(ctx, &sessionorch.GetPreferenceInput{CharacterID: member})
	if err != nil {
		return nil, err
	}
	if configured.DiscoverySharing == nil {
		return encounterSharing, nil
	}
	chosen := *configured.DiscoverySharing
	return &chosen, nil
}
