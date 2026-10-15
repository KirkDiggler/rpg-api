package authoringv1alpha1_test

import (
	"context"

	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	"github.com/KirkDiggler/rpg-api/internal/worldcontext"
)

// worldCtx is a signed-in player in the test registries' world, as the auth
// boundary installs both.
func worldCtx(playerID string) context.Context {
	return worldcontext.With(auth.WithPlayerID(context.Background(), playerID),
		worldcontext.Value{WorldID: dungeonstest.Caller.WorldID})
}
