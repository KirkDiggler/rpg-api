package lobby_test

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

// start_encounter_armed_test.go is the launch half of rpg-project#448: the
// weapons an author armed a placement with have to cross the seam into
// session.Spawn, or the file says something the board never hears.

// TestStartEncounter_TheThreeMindsLaunchesWithItsTwoArmedGoblins is the
// shipped walk scene, end to end: two goblins from one stat block, armed
// differently, and a run that starts.
func (s *SessionStackSuite) TestStartEncounter_TheThreeMindsLaunchesWithItsTwoArmedGoblins() {
	s.seedCharacter("char-alice", "alice", "Alice")
	s.seedReadyLobby("lobby-1", "alice")

	out, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-1", DungeonKey: "reference-minds",
	})
	s.Require().NoError(err, "the shipped minds dungeon must launch with its armed placements")

	for _, id := range []string{"goblin-1", "goblin-2"} {
		_, turnErr := s.sessOrch.Manager.Turn(s.ctx, &sdk.TurnInput{Session: out.EncounterID, Member: id})
		s.Require().NoError(turnErr, "%s is a member of the run", id)
	}
}

// TestStartEncounter_AWeaponTheCatalogDoesNotHaveRefusesTheLaunch is what
// makes the test above a claim about FORWARDING rather than about launching.
//
// A launch that quietly dropped `actions:` would pass the scene above — the
// goblins would still be members, just holding their stat block's own arms.
// Here the file names a weapon nothing can build, so the only way the launch
// can fail is if the list reached session.Spawn. It refuses by name, and the
// run does not start half-built.
//
// It is also decision 5 at the last seam that can enforce it: a bad weapon
// takes down the launch, not a turn.
func (s *SessionStackSuite) TestStartEncounter_AWeaponTheCatalogDoesNotHaveRefusesTheLaunch() {
	s.seedCharacter("char-alice", "alice", "Alice")
	s.seedReadyLobby("lobby-1", "alice")

	dir := s.T().TempDir()
	src := dungeonstest.ContentDir(s.T())
	entries, err := os.ReadDir(src)
	s.Require().NoError(err)
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".yaml" {
			continue
		}
		raw, readErr := os.ReadFile(filepath.Join(src, e.Name())) //nolint:gosec // a ReadDir entry under the repo content dir
		s.Require().NoError(readErr)
		if e.Name() == "reference-minds.yaml" {
			raw = []byte(strings.Replace(string(raw),
				"dnd5e:weapons:scimitar", "dnd5e:weapons:trebuchet", 1))
		}
		s.Require().NoError(os.WriteFile(filepath.Join(dir, e.Name()), raw, 0o600))
	}

	registry, err := dungeons.NewFileRegistry(dir, false, dungeonstest.Projector(s.T()))
	s.Require().NoError(err, "the file still COMPILES — a weapon ref's shape is all the dungeon can check")

	orch, err := lobbyorch.New(&lobbyorch.Config{
		LobbyRepo: s.lobbyRepo, LobbyBroker: s.broker,
		CharacterRepo:    s.charRepo,
		LobbyIDGenerator: idgen.NewSequential("lobby"), JoinRefGenerator: idgen.NewSequential("ref"),
		EncounterIDGenerator: idgen.NewSequential("enc"),
		SessionManager:       s.sessOrch.Manager, Dungeons: registry,
	})
	s.Require().NoError(err)

	_, err = orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-1", DungeonKey: "reference-minds",
	})
	s.Require().Error(err, "a weapon nothing can build must take the launch down")
	s.ErrorIs(err, sdk.ErrUnknownContent)
	s.ErrorContains(err, "dnd5e:weapons:trebuchet", "and the refusal names the ref the author wrote")
}
