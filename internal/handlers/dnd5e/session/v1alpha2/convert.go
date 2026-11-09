package sessionv1alpha2

import (
	"encoding/json"
	"fmt"

	"google.golang.org/protobuf/types/known/structpb"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
)

// recordEventToProto carries original payload bytes without interpreting them.
// The legacy Struct is still an approximate projection for older readers; exact
// inspection uses FieldsJson only. A legacy projection error never discards the
// raw payload or record identity, so clients can report unavailable/malformed
// evidence rather than silently substituting a different representation.
func recordEventToProto(evt sdk.RecordEvent) (*sessionpb.RecordEvent, error) {
	out := &sessionpb.RecordEvent{Id: evt.ID, At: evt.At, Act: evt.Act, Beat: evt.Beat,
		FieldsJson: append([]byte(nil), evt.Fields...)}
	fields, err := fieldsToStruct(evt.Fields)
	if err != nil {
		return out, fmt.Errorf("record event %s (%s): %w", evt.ID, evt.Beat, err)
	}
	out.Fields = fields //nolint:staticcheck // additive wire migration retains the approximate field; exact readers use FieldsJson.
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
