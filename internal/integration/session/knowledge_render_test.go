package session_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// renderedKnowledgeAtlas assembles only supplied geometry and observations,
// as the game renderer does. GetAtlas itself must contain no holdable objects.
// It performs no visibility inference and never substitutes a world-truth read.
func renderedKnowledgeAtlas(ctx context.Context, t *testing.T, h *acceptanceHarness, session, member string) *sessionpb.GetAtlasResponse {
	t.Helper()
	atlas, err := h.handler.GetAtlas(ctx, &sessionpb.GetAtlasRequest{Session: session, Member: member})
	require.NoError(t, err)
	for _, p := range atlas.Props {
		require.False(t, p.Holdable, "mutable props belong to GetView")
	}
	for _, p := range atlas.Placed {
		require.False(t, p.Holdable, "mutable footprints belong to GetView")
	}
	view, err := h.handler.GetView(ctx, &sessionpb.GetViewRequest{Session: session, Member: member})
	require.NoError(t, err)
	for _, s := range view.Props {
		if s.ObservedEmpty {
			continue
		}
		if p := s.GetProp(); p != nil && p.At != nil {
			atlas.Props = append(atlas.Props, p)
		}
		if p := s.GetPlaced(); p != nil && p.Placement != nil {
			atlas.Placed = append(atlas.Placed, p)
		}
	}
	return atlas
}
