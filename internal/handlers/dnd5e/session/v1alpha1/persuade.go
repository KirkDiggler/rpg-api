package sessionv1alpha1

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// errPersuadeRetired is the sentence a stale client reads.
const errPersuadeRetired = "Persuade is retired: use Speak with the target's option id (rpg-project#569)"

// Persuade is deprecated on the wire and retired here (rpg-project#569),
// Intimidate's twin: an appeal is now one of the lines a target offers, said
// through Speak. The RPC answers Unimplemented naming its replacement.
//
//nolint:staticcheck // The deprecated RPC must still be served to answer Unimplemented.
func (h *Handler) Persuade(
	_ context.Context, _ *sessionpb.PersuadeRequest,
) (*sessionpb.PersuadeResponse, error) {
	return nil, status.Error(codes.Unimplemented, errPersuadeRetired)
}
