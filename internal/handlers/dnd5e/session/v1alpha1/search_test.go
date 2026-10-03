package sessionv1alpha1

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

type RetiredSearchSuite struct{ suite.Suite }

func TestRetiredSearchSuite(t *testing.T) { suite.Run(t, new(RetiredSearchSuite)) }
func (s *RetiredSearchSuite) TestCannotReachTheManagerOrRoll() {
	// No manager/repository is installed: the inherited wire tombstone must
	// return without reaching either, even when an old client sends valid IDs.
	h := &Handler{}
	out, err := h.Search(context.Background(), &sessionpb.SearchRequest{Session: "sess", Member: "alice", Region: "hall"})
	s.Nil(out)
	s.Equal(codes.Unimplemented, status.Code(err))
}
