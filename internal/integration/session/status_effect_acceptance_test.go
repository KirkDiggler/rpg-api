package session_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	characterpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/character"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v2/character"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
)

// The cook's status (the-mad-king.yaml, rpg-project#1024), carried onto the
// small castle kitchen site so the run launches quickly.
const (
	kitchenCook  = "cook-1"
	helpLine     = "help"
	wellFedName  = "Well fed"
	statusPlayer = "player-alice"
)

// wellFedRows is how many rows of alice's own character sheet, read through
// the v2 character view, name the status.
func wellFedRows(t *testing.T, sheet *characterhandler.Handler) int {
	t.Helper()
	view, err := sheet.GetCharacterData(auth.WithPlayerID(context.Background(), statusPlayer),
		&characterpb.GetCharacterDataRequest{CharacterId: "alice"})
	require.NoError(t, err)
	rows := 0
	for _, condition := range view.GetCharacter().GetConditions() {
		if condition.GetName() == wellFedName {
			rows++
		}
	}
	return rows
}

// wellFedKitchen is the castle kitchen with the castle's own `statuses:`
// section, read from the-mad-king.yaml, and a plain line on its first cook
// whose answer gives it: the castle's help, without the check.
func wellFedKitchen(t *testing.T) []byte {
	t.Helper()
	castle := madKingRaw(t)
	start := bytes.Index(castle, []byte("\nstatuses:\n"))
	require.Positive(t, start, "the castle writes a statuses section")
	end := bytes.Index(castle[start:], []byte("\nexits:\n"))
	require.Positive(t, end, "the statuses section ends at exits")
	statuses := castle[start+1 : start+end+1]

	raw := dungeonstest.CastleKitchenYAML(t)
	room := []byte("\nroom:\n")
	require.Equal(t, 1, bytes.Count(raw, room))
	raw = bytes.Replace(raw, room, append(append([]byte("\n"), statuses...), room[1:]...), 1)
	cook := []byte("      cook-1: {faction: kitchen}\n")
	require.Equal(t, 1, bytes.Count(raw, cook))
	return bytes.Replace(raw, cook, []byte(`      cook-1:
        faction: kitchen
        options:
          - id: help
            say: (lend a hand with the pies)
            success:
              - say: Bless you. Here, eat.
                apply_status: well-fed
`), 1)
}

// TestAcceptance_TheCooksHelpLeavesAliceWellFedOnHerSheet is rpg-project#1024
// through the api's own doors: alice, beside the cook, lends a hand with the
// pies; her own character sheet, read through the v2 character view, names
// the status the castle wrote ("Well fed"). Helping again leaves one.
func TestAcceptance_TheCooksHelpLeavesAliceWellFedOnHerSheet(t *testing.T) {
	const run = "status-run"
	h := newAcceptanceHarness(t)
	_, err := h.charRepo.Create(auth.WithPlayerID(context.Background(), statusPlayer), characterrepo.CreateInput{
		Character: &entities.Character{Data: armedFighter("alice", statusPlayer)},
	})
	require.NoError(t, err)
	dungeon, err := sessionworld.Compile(wellFedKitchen(t))
	require.NoError(t, err)
	out := h.launch(t, run, dungeon.Spec, seatAt("alice", 0, 1))
	require.Empty(t, out.Formed, "precondition: nobody fights")

	sheet, err := characterhandler.New(&characterhandler.HandlerConfig{CharacterService: newAcceptanceCharacterService(t, h)})
	require.NoError(t, err)
	require.Zero(t, wellFedRows(t, sheet), "precondition: alice is not well fed")

	_, err = sayTo(t, h, run, "alice", statusPlayer, kitchenCook, helpLine)
	require.NoError(t, err, "alice lends the cook a hand")
	require.Equal(t, 1, wellFedRows(t, sheet), "her sheet names the status the castle wrote")

	_, err = sayTo(t, h, run, "alice", statusPlayer, kitchenCook, helpLine)
	require.NoError(t, err, "alice helps again")
	require.Equal(t, 1, wellFedRows(t, sheet), "helping again leaves one")
}
