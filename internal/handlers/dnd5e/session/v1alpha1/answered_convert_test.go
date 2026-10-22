package sessionv1alpha1

import (
	"testing"

	"github.com/stretchr/testify/require"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

// answered_convert_test.go is Speak's wire half (rpg-project#458, migrated by
// rpg-project#569): the line's own `spoke` beat, and the WORLD's roll on the
// option's rows.

// TestVerbSpeakToProto is the dock's half. Afford emits this row on BOTH
// clocks, and a client drops a verb it cannot name rather than drawing it
// wrong, so unmapped the menu would simply never appear.
func TestVerbSpeakToProto(t *testing.T) {
	require.Equal(t, sessionpb.Verb_VERB_SPEAK, verbToProto(sdk.VerbSpeak))
}

// TestSightingStanceIsCarriedPerViewer is the ring's own seam
// (rpg-project#458). `session.Sighting` gained a believed stance beside Name
// and Kind, filled from `encounter.BelievedStance` per viewer, and this
// converter copies it verbatim.
//
// IT REPLACED A GAP MARKER. Until the seam carried the field this test asserted
// its ABSENCE, so that its arrival would be noticed rather than sit unwired.
// It arrived; the marker is gone and this is what took its place.
func TestSightingStanceIsCarriedPerViewer(t *testing.T) {
	for _, word := range []string{"hostile", "neutral", "allied"} {
		got := sightingToProto(sdk.Sighting{
			Subject: "front-goblin", Name: "Goblin", Kind: sdk.KindMonster,
			Status: "current", Stance: word,
		})
		require.Equal(t, word, got.GetStance(),
			"the seam's own word crosses verbatim; this converter knows no vocabulary")
	}
}

// TestSightingStanceCrossesEmptyRatherThanBorrowingTheFaction pins the value
// itself, because "we could not fill it" and "we filled it with something
// plausible" look identical from the outside and are opposite decisions. The
// wire says empty means the observer has no word; a faction color smuggled in
// here would be shared truth wearing a per-viewer field's clothes, and the
// first authored `pretend` would have to undo it.
func TestSightingStanceEmptyStaysEmptyForACreatureInNoFaction(t *testing.T) {
	got := sightingToProto(sdk.Sighting{
		Subject: "world-npc-1",
		Name:    "Merchant",
		Kind:    sdk.KindWorld,
		Status:  "current",
		// The seam leaves it empty when the run cannot answer: a subject who
		// is not a member, or one in NO FACTION AT ALL, which a world NPC is.
	})
	require.Equal(t, "world-npc-1", got.GetSubject(), "the fields that DO cross still cross")
	require.Empty(t, got.GetStance(),
		"empty is the wire's own \"no word for it\"; mapping it to neutral would "+
			"invent a belief nobody holds, and the client has a roster color to fall back to")
}
