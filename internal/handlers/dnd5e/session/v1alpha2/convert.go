package sessionv1alpha2

import (
	"encoding/json"
	"fmt"

	"google.golang.org/protobuf/types/known/structpb"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
)

// recordEventToProto spells one record beat on the wire: the beat as
// written, its fields as a Struct (Law 5). Every number becomes a double.
//
// The error reports fields that could not be spelled; the event it returns
// beside it is still whole but for its fields, and the caller sends it — a
// beat with unreadable fields is one degraded beat, not a gap (R22's spirit).
func recordEventToProto(evt sdk.RecordEvent) (*sessionpb.RecordEvent, error) {
	out := &sessionpb.RecordEvent{Id: evt.ID, At: evt.At, Act: evt.Act, Beat: evt.Beat}
	fields, err := fieldsToStruct(evt.Fields)
	if err != nil {
		return out, fmt.Errorf("record event %s (%s): %w", evt.ID, evt.Beat, err)
	}
	out.Fields = fields
	return out, nil
}

// fieldsToStruct decodes a JSON object into a Struct. An empty payload is no
// fields; anything that is not a JSON object is an error.
func fieldsToStruct(raw json.RawMessage) (*structpb.Struct, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("fields are not a JSON object: %w", err)
	}
	s, err := structpb.NewStruct(m)
	if err != nil {
		return nil, fmt.Errorf("fields: %w", err)
	}
	return s, nil
}
