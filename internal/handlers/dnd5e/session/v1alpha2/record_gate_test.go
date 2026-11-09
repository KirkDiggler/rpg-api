package sessionv1alpha2

import (
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
)

func (s *RecordHandlerSuite) TestDisabledRecordRefusesReadAndStreamBeforeAccessingData() {
	h, err := New(&HandlerConfig{Manager: s.manager, Broker: sessionorch.NewRecordBroker(1), Gate: s.gate, Enabled: false})
	s.Require().NoError(err)
	// No seat/data expectations: a disabled diagnostic surface cannot probe
	// a session or subscribe, even for an authenticated caller.
	out, err := h.GetRecord(s.ctx, &sessionpb.GetRecordRequest{Session: "sess-1", Member: "char-1"})
	s.Nil(out)
	s.Equal(codes.PermissionDenied, status.Code(err))
	stream := newRecordStream(s.ctx)
	err = h.StreamRecord(&sessionpb.StreamRecordRequest{Session: "sess-1", Member: "char-1"}, stream)
	s.Equal(codes.PermissionDenied, status.Code(err))
	s.Empty(stream.sent)
}
