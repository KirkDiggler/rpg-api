package sessionv1alpha1

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// errIntimidateRetired is the sentence a stale client reads.
const errIntimidateRetired = "Intimidate is retired: use Speak with the target's option id (rpg-project#569)"

// Intimidate is deprecated on the wire and retired here (rpg-project#569). A
// threat is now one of the lines a target offers, said through Speak; the
// session has no Intimidate verb left to call. The RPC answers Unimplemented
// naming its replacement rather than falling through to the generated stub,
// whose message would not say where to go.
//
//nolint:staticcheck // The deprecated RPC must still be served to answer Unimplemented.
func (h *Handler) Intimidate(
	_ context.Context, _ *sessionpb.IntimidateRequest,
) (*sessionpb.IntimidateResponse, error) {
	return nil, status.Error(codes.Unimplemented, errIntimidateRetired)
}
