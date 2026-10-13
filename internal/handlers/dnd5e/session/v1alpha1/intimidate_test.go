package sessionv1alpha1

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// TestIntimidate_IsRetiredAndNamesSpeak pins rpg-project#569's ruling: the deprecated
// RPC answers Unimplemented with a sentence naming its replacement, and never
// reaches the SDK (the handler has no manager at all here).
func TestIntimidate_IsRetiredAndNamesSpeak(t *testing.T) {
	h := &Handler{}
	_, err := h.Intimidate(context.Background(), &sessionpb.IntimidateRequest{
		Session: "sess-1", Member: "char-1", Target: "front-goblin",
	})
	requireCode(t, err, codes.Unimplemented)
	require.Contains(t, status.Convert(err).Message(), "Speak")
}
