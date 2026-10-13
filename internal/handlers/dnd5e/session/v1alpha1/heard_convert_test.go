package sessionv1alpha1

import (
	"testing"

	"github.com/stretchr/testify/require"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// heard_convert_test.go is hearing's wire half (rpg-project#569, H5): a sound
// reaching one listener, scoped to that listener by the session.

// TestEventHeardKindToProto is the kind half. An unmapped kind demotes to
// EVENT_KIND_UNKNOWN and the body stays nil, so a missing arm would tell the
// listener nothing rather than degrade one field.
func TestEventHeardKindToProto(t *testing.T) {
	require.Equal(t, sessionpb.EventKind_EVENT_KIND_HEARD, eventKindToProto(sdk.EventHeard))
}

// TestHeardBodyToProto_CarriesKindPositionAndHops is the acceptance case: the
// listener, the sealed kind word verbatim, the cell the sound was made on and
// the region distance it crossed.
func TestHeardBodyToProto_CarriesKindPositionAndHops(t *testing.T) {
	event := &sessionpb.Event{}
	require.NoError(t, setEventBody(event, sdk.HeardBody{
		Listener: "gate-guard", Kind: "clang", Position: spatial.Position{X: 7, Y: 3}, Hops: 2,
	}))

	got := event.GetHeard()
	require.NotNil(t, got, "a sound that reached this seam must reach the wire")
	require.Equal(t, "gate-guard", got.GetListener())
	require.Equal(t, "clang", got.GetKind(), "the rulebook's word, carried not interpreted")
	require.Equal(t, 7.0, got.GetPosition().GetX())
	require.Equal(t, 3.0, got.GetPosition().GetY())
	require.Equal(t, int32(2), got.GetHops())
}

// TestHeardBodyToProto_SameRoomIsZeroHops pins zero as an answer: a sound in
// the listener's own region crosses as hops 0 with its position intact.
func TestHeardBodyToProto_SameRoomIsZeroHops(t *testing.T) {
	event := &sessionpb.Event{}
	require.NoError(t, setEventBody(event, sdk.HeardBody{
		Listener: "char-alice", Kind: "voice", Position: spatial.Position{X: 1, Y: 1},
	}))

	got := event.GetHeard()
	require.NotNil(t, got)
	require.Equal(t, int32(0), got.GetHops())
	require.NotNil(t, got.GetPosition(), "where the sound was made always crosses")
}

// TestHeardBodyToProto_EmptyKindOrListenerIsRefused pins the producer-defect
// refusals: a sound of no kind is not "an unknown sound", and a sound heard
// by nobody has no recipient.
func TestHeardBodyToProto_EmptyKindOrListenerIsRefused(t *testing.T) {
	for name, body := range map[string]sdk.HeardBody{
		"no kind":     {Listener: "gate-guard"},
		"no listener": {Kind: "clang"},
	} {
		t.Run(name, func(t *testing.T) {
			event := &sessionpb.Event{}
			require.Error(t, setEventBody(event, body))
			require.Nil(t, event.GetHeard(), "a refused beat sets no body")
		})
	}
}
