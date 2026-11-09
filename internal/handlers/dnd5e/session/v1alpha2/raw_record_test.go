package sessionv1alpha2

import (
	"encoding/json"

	"go.uber.org/mock/gomock"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

func (s *RecordHandlerSuite) TestRecordEventRawJSONIsVerbatim() {
	raw := json.RawMessage(` { "value":9007199254740993,"zero":0,"flag":false,"label":"","why":"a < b" } `)
	evt := beat("raw-1")
	evt.Fields = raw
	out, err := recordEventToProto(evt)
	s.Require().NoError(err)
	s.Equal([]byte(raw), out.GetFieldsJson(), "the host must not parse or re-encode exact evidence")
	before := string(out.GetFieldsJson())
	raw[0] = '!'
	s.Equal(before, string(out.GetFieldsJson()), "wire bytes own their copy")
}

func (s *RecordHandlerSuite) TestMalformedAndAbsentRawRemainExplicit() {
	for _, raw := range []json.RawMessage{nil, []byte(`{broken`), []byte(`[1,2]`)} {
		evt := beat("raw-invalid")
		evt.Fields = raw
		out, _ := recordEventToProto(evt)
		s.Require().NotNil(out)
		s.Equal("raw-invalid", out.GetId())
		s.Equal([]byte(raw), out.GetFieldsJson(), "a degraded record keeps its identity and original evidence, not a fabricated object")
	}
}

func (s *RecordHandlerSuite) TestBothRecordRPCsPreserveRawPayload() {
	broker := sessionorch.NewRecordBroker(8)
	h := s.handler(broker)
	evt := beat("id-01")
	evt.Fields = json.RawMessage(`{"value":9007199254740993,"flag":false}`)
	s.seated()
	s.manager.EXPECT().Record(gomock.Any(), gomock.Any()).Return(&sdk.RecordReadOutput{Events: []sdk.RecordEvent{evt}}, nil)
	page, err := h.GetRecord(s.ctx, &sessionpb.GetRecordRequest{Session: "sess-1", Member: "char-1"})
	s.Require().NoError(err)
	s.Require().Len(page.Events, 1)
	s.Equal([]byte(evt.Fields), page.Events[0].GetFieldsJson())
	s.seated()
	s.manager.EXPECT().Record(gomock.Any(), gomock.Any()).Return(&sdk.RecordReadOutput{Events: []sdk.RecordEvent{evt}}, nil)
	stream, done := s.run(h, "")
	s.Equal([]byte(evt.Fields), stream.next(s.T()).GetEvent().GetFieldsJson())
	live := evt
	live.ID = "id-02"
	s.Require().NoError(broker.PublishRecord(s.ctx, []sdk.RecordEvent{live}))
	s.Equal([]byte(evt.Fields), stream.next(s.T()).GetEvent().GetFieldsJson())
	s.cancel()
	s.NoError(s.ended(done))
}
