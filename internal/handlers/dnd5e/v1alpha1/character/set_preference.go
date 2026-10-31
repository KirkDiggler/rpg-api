package character

import (
	"context"
	"errors"

	"google.golang.org/grpc/status"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/apierr"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
)

//go:generate mockgen -destination=mock/mock_preferences.go -package=charactermock github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v1alpha1/character Preferences

// Preferences writes character configuration (rpg-project#753 R2) under the
// character's own guard. *sessionorch.Preferences satisfies it.
type Preferences interface {
	SetPreference(ctx context.Context, in *sessionorch.SetPreferenceInput) (*sessionorch.SetPreferenceOutput, error)
}

// SetPreference changes the caller's own character's configuration. Each
// preference is optional; an absent one is left as it is. A session takes the
// change up at that character's next write verb.
func (h *Handler) SetPreference(
	ctx context.Context,
	req *dnd5ev1alpha1.SetPreferenceRequest,
) (*dnd5ev1alpha1.SetPreferenceResponse, error) {
	if req.GetCharacterId() == "" {
		return nil, apierr.ToGRPCError(apierr.InvalidArgument("character_id is required"))
	}
	if h.preferences == nil {
		return nil, apierr.ToGRPCError(apierr.Unimplemented("set preference: no character configuration is wired"))
	}
	if ownErr := h.verifyCallerOwnsCharacter(ctx, req.GetCharacterId()); ownErr != nil {
		return nil, ownErr
	}
	in := &sessionorch.SetPreferenceInput{CharacterID: req.GetCharacterId()}
	if req.DiscoverySharing != nil {
		sharing := req.GetDiscoverySharing()
		in.DiscoverySharing = &sharing
	}
	out, err := h.preferences.SetPreference(ctx, in)
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		// Waiting on the character's guard was given up.
		return nil, status.FromContextError(err).Err()
	}
	if err != nil {
		return nil, apierr.ToGRPCError(apierr.Internal("set preference: " + err.Error()))
	}
	return &dnd5ev1alpha1.SetPreferenceResponse{DiscoverySharing: out.DiscoverySharing}, nil
}
