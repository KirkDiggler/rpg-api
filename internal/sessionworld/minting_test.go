package sessionworld

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
)

// minting_test.go pins how a monster gets its member id (rpg-project#375,
// ruled on the hold-out): the author's own when the placement has one, the
// ref plus an ordinal when it does not, and never one id for two monsters.

// TestAnAuthoredIdIsTheMemberId is the ruling itself, on both shipped
// dungeons that name a placement: the chief is `chief`, the scout is
// `scout`, the heirloom's captain is `captain` — not skeleton-captain-1.
func TestAnAuthoredIdIsTheMemberId(t *testing.T) {
	camp, _ := compileRaiderCamp(t)
	ids := map[string]string{}
	for _, m := range camp.Monsters {
		ids[m.PlacementID] = m.MemberID
	}
	require.Equal(t, "chief", ids["chief"])
	require.Equal(t, "scout", ids["scout"])

	raw, err := os.ReadFile(heirloomPath)
	require.NoError(t, err)
	heirloom, err := Compile(raw)
	require.NoError(t, err)
	for _, m := range heirloom.Monsters {
		if m.PlacementID == "captain" {
			require.Equal(t, "captain", m.MemberID, "the author named him, so that is his id")
		} else {
			require.Empty(t, m.PlacementID)
			require.True(t, strings.HasPrefix(m.MemberID, "skeleton-"),
				"an unnamed placement is still minted from its ref: %q", m.MemberID)
		}
	}
}

// TestTheMindIsTheMemberIdTheRunKnows is the reason for the ruling, proved
// against the composition rather than by string equality alone.
//
// The file says `factions: [{ id: raiders, mind: chief }]`, and the compiler
// carries that word into the world as the faction's Mind (a MEMBER id, in
// the composition's own vocabulary). The composition cannot check it at
// construction — the world is empty of members on purpose — so it checks
// it when the member ENTERS: a member joining under a mind's id must join
// that faction, or it is refused. Which means the member id this package
// mints for the chief is not a naming preference; it is the id the run's
// graph is waiting for. Mint anything else and the chief joins under a name
// nobody is the mind of, the camp never learns, and the hold-out cannot end
// — in a run that otherwise works.
//
// So, both halves: the chief enters his own faction under the id the file
// gave him; and entering under that id into the DEFAULT faction — which is
// what a launch that dropped Faction on the way to Spawn would do — is
// refused by the composition, naming him as the mind.
func TestTheMindIsTheMemberIdTheRunKnows(t *testing.T) {
	camp, _ := compileRaiderCamp(t)

	var chief *Monster
	for i := range camp.Monsters {
		if camp.Monsters[i].PlacementID == "chief" {
			chief = &camp.Monsters[i]
		}
	}
	require.NotNil(t, chief)

	minds := map[tkencounter.FactionID]tkencounter.MemberID{}
	for _, fa := range camp.World.Field.Factions {
		minds[fa.ID] = fa.Mind
	}
	require.Equal(t, tkencounter.MemberID(chief.MemberID), minds[chief.Faction],
		"the faction's mind in the world is the id the launch will spawn the chief under")

	// The composition's own answer, twice.
	enc := loadWorld(t, camp)
	_, err := enc.Join(&tkencounter.JoinInput{
		Member: tkencounter.MemberID(chief.MemberID), Kind: tkencounter.KindMonster,
		Name: "chief", Cell: chief.At, Faction: chief.Faction,
	})
	require.NoError(t, err, "the chief enters the raiders under the id the file named as their mind")

	enc = loadWorld(t, camp)
	_, err = enc.Join(&tkencounter.JoinInput{
		Member: tkencounter.MemberID(chief.MemberID), Kind: tkencounter.KindMonster,
		Name: "chief", Cell: chief.At,
	})
	require.ErrorIs(t, err, tkencounter.ErrNoFaction,
		"the same id into the default faction is refused: a mind belongs to its own faction or it is nobody's")
	require.Contains(t, err.Error(), "mind")
	require.Contains(t, err.Error(), chief.MemberID)
}

// loadWorld rebuilds a live encounter from the world this package produced,
// with the same construction-time capabilities, so a scene can ask the
// composition a question the way session.StartSession would.
func loadWorld(t *testing.T, d *Dungeon) *tkencounter.Encounter {
	t.Helper()

	enc, err := tkencounter.LoadEncounter(&tkencounter.LoadEncounterInput{
		Data:       *d.World,
		Initiative: orderAsGiven{}, Standing: nobodyDown{}, Sight: nobodySees{},
		TurnDriver: tkencounter.PassDriver{}, Striker: tkencounter.RefusingStriker{},
		Announcer: tkencounter.RefusingAnnouncer{},
	})
	require.NoError(t, err, "the world this package produced must be one the composition accepts back")

	return enc
}

// twoSkeletons is a one-room file placing two skeletons, the second with
// whatever id a case wants to test, and a third placement a case may name.
func twoSkeletons(secondID, thirdID string) string {
	id := func(s string) string {
		if s == "" {
			return ""
		}
		return "id: " + s + ", "
	}
	return `
version: 2
key: two-skeletons
name: Two Skeletons
orientation: pointy
void: opaque
start: [0, 0]
regions:
  - id: room
    name: Room
    archetype: crypt
    lighting: { intensity: 1 }
    cells:
      - [[0,0],[1,0],[2,0],[3,0]]
      - [[0,1],[1,1],[2,1],[3,1]]
place:
  - { ref: "dnd5e:monsters:skeleton", at: [2, 0] }
  - { ` + id(secondID) + `ref: "dnd5e:monsters:skeleton", at: [3, 1] }
  - { ` + id(thirdID) + `ref: "dnd5e:monsters:skeleton", at: [1, 1] }
`
}

// TestAnAuthoredIdThatSpellsAMintedOneIsRefusedNamingBoth is the collision
// the ruling closes: two monsters, one id. dungeonspec already refuses two
// AUTHORED ids alike, so the case left is an authored id that spells what
// the ordinal minted for an unnamed sibling — in either order. Refused at
// compile, naming both placements the way an author would find them,
// rather than at the second Spawn of a launch, which the session reports
// as "no such member" (the opposite of what happened).
func TestAnAuthoredIdThatSpellsAMintedOneIsRefusedNamingBoth(t *testing.T) {
	t.Run("the named one comes second", func(t *testing.T) {
		_, err := Compile([]byte(twoSkeletons("skeleton-1", "")))
		require.Error(t, err)
		require.Contains(t, err.Error(), `"skeleton-1" is claimed twice`)
		require.Contains(t, err.Error(), "dnd5e:monsters:skeleton at [2,0]", "the unnamed one, by ref and cell")
		require.Contains(t, err.Error(), `"skeleton-1" (dnd5e:monsters:skeleton at [3,1])`, "the named one, by name")
	})
	t.Run("the named one comes first", func(t *testing.T) {
		_, err := Compile([]byte(twoSkeletons("skeleton-3", "")))
		require.Error(t, err, "the third skeleton is minted skeleton-3, which the second already spelled")
		require.Contains(t, err.Error(), `"skeleton-3" is claimed twice`)
		require.Contains(t, err.Error(), `"skeleton-3" (dnd5e:monsters:skeleton at [3,1])`)
		require.Contains(t, err.Error(), "dnd5e:monsters:skeleton at [1,1]")
	})
}

// TestNamingOneMonsterDoesNotRenumberItsSiblings pins that an ordinal is
// spent on every placement of a ref, named or not: skeleton-1, scout,
// skeleton-3 — never skeleton-1, scout, skeleton-2. That follows the rule
// MemberID's doc already states for props and reordering: a monster nothing
// about which changed must not be silently renamed, and giving its sibling
// a name is exactly such a change. The gap in the numbering is the visible
// cost of the stable one.
func TestNamingOneMonsterDoesNotRenumberItsSiblings(t *testing.T) {
	d, err := Compile([]byte(twoSkeletons("scout", "")))
	require.NoError(t, err)

	ids := make([]string, len(d.Monsters))
	for i, m := range d.Monsters {
		ids[i] = m.MemberID
	}
	require.Equal(t, []string{"skeleton-1", "scout", "skeleton-3"}, ids)
}
