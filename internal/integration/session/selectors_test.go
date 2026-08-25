package session_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	sessionhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1"
)

// currentDeclarationID asks the same public Afford surface a client uses and
// returns the opaque selector for one currently available verb. Tests never
// construct or parse selectors: the provider remains their sole author.
func currentDeclarationID(
	ctx context.Context,
	t *testing.T,
	h *sessionhandler.Handler,
	session string,
	member string,
	verb sessionpb.Verb,
) string {
	t.Helper()

	out, err := h.Afford(ctx, &sessionpb.AffordRequest{Session: session, Member: member})
	require.NoError(t, err)
	for _, declaration := range out.GetDeclarations() {
		if declaration.GetVerb() != verb {
			continue
		}
		require.True(t, declaration.GetAvailable(), "verb %s must be available: %v", verb, declaration.GetWhy())
		require.NotEmpty(t, declaration.GetId(), "verb %s must carry an opaque selector", verb)
		return declaration.GetId()
	}
	t.Fatalf("Afford returned no %s declaration", verb)
	return ""
}
