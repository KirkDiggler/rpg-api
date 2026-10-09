package session_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	tkdungeonspec "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter/dungeonspec"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"
)

// partySeat is one party member and the authored cell [col,row] they start on.
type partySeat struct {
	ID       string
	Col, Row int
}

// seatAt is a party seat on an authored cell. A character's id is also its
// member id, so ID names both the saved character and who stands there.
func seatAt(id string, col, row int) partySeat { return partySeat{ID: id, Col: col, Row: row} }

// monsterAt is a monster placement on an authored cell, member id and all, the
// way the dungeon compile mints them.
func monsterAt(id, ref string, col, row int) tkdungeonspec.MonsterPlacement {
	return tkdungeonspec.MonsterPlacement{
		Ref: ref, ID: id, MemberID: id, At: spatial.Position{X: float64(col), Y: float64(row)},
	}
}

// withMonsters is dungeon with these monsters placed, leaving dungeon itself
// untouched.
func withMonsters(dungeon *tkdungeonspec.Compiled, monsters ...tkdungeonspec.MonsterPlacement) *tkdungeonspec.Compiled {
	out := *dungeon
	out.Monsters = append(append([]tkdungeonspec.MonsterPlacement{}, dungeon.Monsters...), monsters...)

	return &out
}

// launchInput is what h.launch hands the SDK: the compiled dungeon with the
// party's seats written onto it, the run under its own id.
func launchInput(run string, dungeon *tkdungeonspec.Compiled, party []partySeat) *sdk.LaunchInput {
	out := *dungeon
	out.PartyStart = make([]tkdungeonspec.Seat, 0, len(party))
	ids := make([]string, 0, len(party))
	for _, p := range party {
		out.PartyStart = append(out.PartyStart, tkdungeonspec.Seat{
			At: spatial.Position{X: float64(p.Col), Y: float64(p.Row)},
		})
		ids = append(ids, p.ID)
	}

	return &sdk.LaunchInput{Session: run, Dungeon: &out, Party: ids}
}

// launch starts the run the way the lobby does: one Launch of the compiled
// dungeon with the party seated where the test says, in seat order. Every
// party character must already be saved. It replaces the start-session, join
// and Spawn sequence these suites used to play by hand.
//
// A launch is not that sequence, and a test should know the three ways:
// it long-rests each character at admission, it stores the run under the
// session id, and the whole board forms fights at once — a monster that
// emits a beat at placement reaches the party members already standing.
func (h *acceptanceHarness) launch(
	t *testing.T, run string, dungeon *tkdungeonspec.Compiled, party ...partySeat,
) *sdk.LaunchOutput {
	t.Helper()

	out, err := h.manager.Manager.Launch(context.Background(), launchInput(run, dungeon, party))
	require.NoError(t, err, "launching %s", run)

	return out
}

// memberIDs is who a launch placed on the board, party then monsters.
func memberIDs(out *sdk.LaunchOutput) []string {
	ids := make([]string, 0, len(out.Members))
	for _, m := range out.Members {
		ids = append(ids, m.ID)
	}

	return ids
}

// launchAtStart is launch for a dungeon compiled from authored YAML: the party
// takes the file's own seats, best seat first, as the lobby seats them.
func (h *acceptanceHarness) launchAtStart(
	t *testing.T, run string, dungeon *tkdungeonspec.Compiled, party ...string,
) *sdk.LaunchOutput {
	t.Helper()

	out, err := h.manager.Manager.Launch(context.Background(), &sdk.LaunchInput{
		Session: run, Dungeon: dungeon, Party: party,
	})
	require.NoError(t, err, "launching %s", run)

	return out
}

// launchErr is launch for a test whose subject is a refused launch: it returns
// the refusal instead of failing on it.
func (h *acceptanceHarness) launchErr(
	t *testing.T, run string, dungeon *tkdungeonspec.Compiled, party ...partySeat,
) (*sdk.LaunchOutput, error) {
	t.Helper()

	return h.manager.Manager.Launch(context.Background(), launchInput(run, dungeon, party))
}
