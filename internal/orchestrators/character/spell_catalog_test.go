package character

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSharedSpellCatalogStatus(t *testing.T) {
	catalog := map[string]SpellInfo{}
	o := &Orchestrator{}
	for _, level := range []int{0, 1} {
		result, err := o.ListSpellsByLevel(context.Background(), &ListSpellsByLevelInput{Level: level})
		require.NoError(t, err)
		for _, spell := range result.Spells {
			catalog[spell.Ref] = spell
		}
	}
	for _, id := range []string{"light", "charm-person", "disguise-self", "identify"} {
		spell, ok := catalog["dnd5e:spells:"+id]
		require.True(t, ok, id)
		require.True(t, spell.NotYetImplemented, id)
		require.NotEmpty(t, spell.Name)
	}
	for _, id := range []string{"command", "guidance", "resistance"} {
		spell, ok := catalog["dnd5e:spells:"+id]
		require.True(t, ok, id)
		require.False(t, spell.NotYetImplemented, id)
	}
}
