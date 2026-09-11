package sessionv1alpha1

import (
	"testing"

	"github.com/stretchr/testify/require"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

// seen_equipment_test.go covers rpg-toolkit#1615's half of this seam: the host
// mints the asset identity a client keys a model off, and the difference
// between "nobody looked" and "looked, and the hands were empty" survives it.

func TestSeenEquipmentAbsentStaysAbsent(t *testing.T) {
	require.Nil(t, seenEquipmentToProto(nil),
		"hands nobody observed must stay unset on the wire, not become an empty message")
}

func TestSeenEquipmentObservedEmptyStaysPresent(t *testing.T) {
	got := seenEquipmentToProto(&sdk.SeenEquipment{})
	require.NotNil(t, got,
		"hands that were looked at and found empty are an observation, not an absence")
	require.Empty(t, got.GetMainHand())
	require.Empty(t, got.GetOffHand())
}

func TestSeenEquipmentMintsTheAssetIdentity(t *testing.T) {
	got := seenEquipmentToProto(&sdk.SeenEquipment{MainHand: "longsword", OffHand: "shield"})
	require.NotNil(t, got)

	// The toolkit hands over bare ids; the full ref is this layer's job, and it
	// is what the shipped provider manifest is keyed by.
	require.Equal(t, "dnd5e:item:longsword", got.GetMainHand())

	// A shield is dnd5e:armor:shield to the rules and one item to the thing
	// that puts a model in a hand. The flattening is deliberate.
	require.Equal(t, "dnd5e:item:shield", got.GetOffHand())
}

// One hand full and one hand empty is the ordinary two-handed-weapon and
// sword-and-nothing case, and both facts have to survive together.
func TestSeenEquipmentMintsOnlyTheHandThatHoldsSomething(t *testing.T) {
	got := seenEquipmentToProto(&sdk.SeenEquipment{MainHand: "greataxe"})
	require.NotNil(t, got)
	require.Equal(t, "dnd5e:item:greataxe", got.GetMainHand())
	require.Empty(t, got.GetOffHand(),
		"an empty hand must stay empty rather than minting an unrenderable dnd5e:item:")
}
