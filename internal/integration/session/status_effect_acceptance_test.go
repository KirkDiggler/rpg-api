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

// The cook's statuses (the-mad-king.yaml, rpg-project#1024, #990), carried
// onto the small castle kitchen site so the run launches quickly.
const (
	kitchenCook   = "cook-1"
	helpLine      = "help"
	tasteLine     = "taste"
	warmedUpName  = "Warmed up"
	feastedName   = "Feasted"
	statusPlayer  = "player-alice"
	feastedMaxHP  = 2
	feastedHealed = 2
)

// aliceSheet is alice's own character sheet, read through the v2 character
// view.
func aliceSheet(t *testing.T, sheet *characterhandler.Handler) *characterpb.GetCharacterDataResponse {
	t.Helper()
	view, err := sheet.GetCharacterData(auth.WithPlayerID(context.Background(), statusPlayer),
		&characterpb.GetCharacterDataRequest{CharacterId: "alice"})
	require.NoError(t, err)
	return view
}

// rowsNamed is how many condition rows of a sheet carry name.
func rowsNamed(view *characterpb.GetCharacterDataResponse, name string) int {
	rows := 0
	for _, condition := range view.GetCharacter().GetConditions() {
		if condition.GetName() == name {
			rows++
		}
	}
	return rows
}

// kitchenWithCookLines is the castle kitchen with the castle's own
// `statuses:` section, read from the-mad-king.yaml, and lines (the YAML of an
// `options:` list, at its indent) on its first cook.
func kitchenWithCookLines(t *testing.T, lines string) []byte {
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
	return bytes.Replace(raw, cook, []byte("      cook-1:\n        faction: kitchen\n        options:\n"+lines), 1)
}

// kitchenRewards is the kitchen with the castle's two lines that give its
// rewards: help (warmed-up) and taste (feasted), both plain here so no die
// decides them.
func kitchenRewards(t *testing.T) []byte {
	t.Helper()
	return kitchenWithCookLines(t, `          - id: help
            say: (lend a hand with the pies)
            success:
              - say: Bless you. Here, eat.
                apply_status: warmed-up
          - id: taste
            say: (taste the feast)
            success:
              - say: Go on, then. One plate, mind.
                apply_status: feasted
`)
}

// TestAcceptance_TheKitchensRewardsLandOnAlicesSheet is rpg-project#1024
// through the api's own doors: alice, beside the cook and wounded by 4 (set
// on her stored sheet after she joins), lends a hand and is "Warmed up" once;
// she tastes the feast and is "Feasted": her own sheet, read through the v2
// character view, shows her maximum hit points 2 higher and her current hit
// points healed by 2. Tasting again changes nothing, and she holds both.
func TestAcceptance_TheKitchensRewardsLandOnAlicesSheet(t *testing.T) {
	const run = "status-run"
	h := newAcceptanceHarness(t)
	_, err := h.charRepo.Create(auth.WithPlayerID(context.Background(), statusPlayer), characterrepo.CreateInput{
		Character: &entities.Character{Data: armedFighter("alice", statusPlayer)},
	})
	require.NoError(t, err)
	dungeon, err := sessionworld.Compile(kitchenRewards(t))
	require.NoError(t, err)
	out := h.launch(t, run, dungeon.Spec, seatAt("alice", 0, 1))
	require.Empty(t, out.Formed, "precondition: nobody fights")

	// Join owns first-admission rest recovery, so alice is wounded after it:
	// a persistence fixture, as the activation test sets its own.
	stored, err := h.charRepo.Get(context.Background(), characterrepo.GetInput{ID: "alice"})
	require.NoError(t, err)
	stored.Character.Data.HitPoints = stored.Character.Data.MaxHitPoints - 4
	_, err = h.charRepo.Update(context.Background(), characterrepo.UpdateInput{Character: stored.Character})
	require.NoError(t, err)

	sheet, err := characterhandler.New(&characterhandler.HandlerConfig{CharacterService: newAcceptanceCharacterService(t, h)})
	require.NoError(t, err)
	before := aliceSheet(t, sheet)
	require.Zero(t, rowsNamed(before, warmedUpName), "precondition: alice is not warmed up")
	require.Zero(t, rowsNamed(before, feastedName), "precondition: alice has not feasted")
	hp := before.GetCharacter().GetHitPoints()
	require.Less(t, hp.GetCurrent(), hp.GetMax(), "precondition: alice starts wounded")

	_, err = sayTo(t, h, run, "alice", statusPlayer, kitchenCook, helpLine)
	require.NoError(t, err, "alice lends the cook a hand")
	_, err = sayTo(t, h, run, "alice", statusPlayer, kitchenCook, helpLine)
	require.NoError(t, err, "alice helps again")
	require.Equal(t, 1, rowsNamed(aliceSheet(t, sheet), warmedUpName), "warmed up, once")

	_, err = sayTo(t, h, run, "alice", statusPlayer, kitchenCook, tasteLine)
	require.NoError(t, err, "alice tastes the feast")
	fed := aliceSheet(t, sheet)
	require.Equal(t, 1, rowsNamed(fed, feastedName), "feasted")
	require.Equal(t, 1, rowsNamed(fed, warmedUpName), "and still warmed up: the two are held together")
	require.Equal(t, hp.GetMax()+feastedMaxHP, fed.GetCharacter().GetHitPoints().GetMax(), "maximum hit points 2 higher")
	require.Equal(t, hp.GetCurrent()+feastedHealed, fed.GetCharacter().GetHitPoints().GetCurrent(), "healed by 2")

	_, err = sayTo(t, h, run, "alice", statusPlayer, kitchenCook, tasteLine)
	require.NoError(t, err, "alice tastes again")
	again := aliceSheet(t, sheet)
	require.Equal(t, 1, rowsNamed(again, feastedName), "still one")
	require.Equal(t, fed.GetCharacter().GetHitPoints().GetMax(), again.GetCharacter().GetHitPoints().GetMax(), "no higher")
	require.Equal(t, fed.GetCharacter().GetHitPoints().GetCurrent(), again.GetCharacter().GetHitPoints().GetCurrent(), "no second heal")
}
