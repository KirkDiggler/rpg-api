package lobby_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	encounterhandlerv2 "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v2/encounter"
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	encountersv2 "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/v2"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
)

// LobbySuite is the shared fixture for every lobby-orchestrator RPC test
// file in this package (create_lobby_test.go, join_lobby_test.go, etc.) —
// one Go type whose methods live across files, avoiding six copies of the
// same Config wiring.
type LobbySuite struct {
	suite.Suite

	ctx       context.Context
	ctrl      *gomock.Controller
	charRepo  *charactermock.MockRepository
	lobbyRepo lobbyrepo.Repository
	broker    *lobbyorch.Broker
	encBroker *tkenc.Broker
	encRepo   encountersv2.Repository
	orch      *lobbyorch.Orchestrator
}

func (s *LobbySuite) SetupTest() {
	s.ctx = context.Background()
	s.ctrl = gomock.NewController(s.T())
	s.charRepo = charactermock.NewMockRepository(s.ctrl)
	s.lobbyRepo = lobbyrepo.NewInMemory()
	s.broker = lobbyorch.NewBroker()
	s.encBroker = tkenc.NewBroker(tkenc.NewInMemoryTransport())
	s.encRepo = encountersv2.NewInMemory()

	orch, err := lobbyorch.New(&lobbyorch.Config{
		LobbyRepo:         s.lobbyRepo,
		LobbyBroker:       s.broker,
		CharacterRepo:     s.charRepo,
		EncounterRepo:     s.encRepo,
		EncounterBroker:   s.encBroker,
		CharacterResolver: encounterhandlerv2.StubCharacterResolver{},
		BuildCombatResolver: func(_ *tkenc.Data) tkenc.CombatResolver {
			return nil
		},
		BuildMovementResolver: func(_ *tkenc.Data) tkenc.MovementResolver {
			return nil
		},
		LobbyIDGenerator:     idgen.NewSequential("lobby"),
		JoinRefGenerator:     idgen.NewSequential("ref"),
		EncounterIDGenerator: idgen.NewSequential("enc"),
		Now:                  func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) },
	})
	s.Require().NoError(err)
	s.orch = orch
}

func (s *LobbySuite) TearDownTest() {
	s.ctrl.Finish()
}

// expectCharacter arms s.charRepo to return, on the next Get(characterID), a
// character owned by playerID with the given display name and HP.
func (s *LobbySuite) expectCharacter(characterID, playerID, name string, hp, maxHP int) {
	s.charRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: characterID}).
		Return(&characterrepo.GetOutput{
			Character: &entities.Character{
				Data: &toolkitchar.Data{
					PlayerID: playerID, Name: name, HitPoints: hp, MaxHitPoints: maxHP,
				},
			},
		}, nil)
}

// expectCharacterWithAC is expectCharacter plus a stored ArmorClass, for
// tests that assert StartEncounter's honest combat-snapshot seeding
// (rpg-api#634): AC is a real stored field, copied verbatim onto
// tkenc.PlayerInput.AC.
func (s *LobbySuite) expectCharacterWithAC(characterID, playerID, name string, hp, maxHP, ac int) {
	s.charRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: characterID}).
		Return(&characterrepo.GetOutput{
			Character: &entities.Character{
				Data: &toolkitchar.Data{
					PlayerID: playerID, Name: name,
					HitPoints: hp, MaxHitPoints: maxHP, ArmorClass: ac,
				},
			},
		}, nil)
}

// expectCharacterNotFound arms s.charRepo to return NotFound for characterID.
func (s *LobbySuite) expectCharacterNotFound(characterID string) {
	s.charRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: characterID}).
		Return(nil, apierr.NotFound("character not found"))
}

// newOrchestratorWithLobbyRepo builds a second Orchestrator sharing every
// other suite dependency (broker, character repo, encounter repo/broker,
// generators) but backed by repo instead of s.lobbyRepo — for tests that
// need to observe behavior when the lobby repository itself misbehaves
// (e.g. a wrapped repo that forces one method to fail).
func (s *LobbySuite) newOrchestratorWithLobbyRepo(repo lobbyrepo.Repository) *lobbyorch.Orchestrator {
	orch, err := lobbyorch.New(&lobbyorch.Config{
		LobbyRepo:         repo,
		LobbyBroker:       s.broker,
		CharacterRepo:     s.charRepo,
		EncounterRepo:     s.encRepo,
		EncounterBroker:   s.encBroker,
		CharacterResolver: encounterhandlerv2.StubCharacterResolver{},
		BuildCombatResolver: func(_ *tkenc.Data) tkenc.CombatResolver {
			return nil
		},
		BuildMovementResolver: func(_ *tkenc.Data) tkenc.MovementResolver {
			return nil
		},
		LobbyIDGenerator:     idgen.NewSequential("lobby"),
		JoinRefGenerator:     idgen.NewSequential("ref"),
		EncounterIDGenerator: idgen.NewSequential("enc"),
		Now:                  func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) },
	})
	s.Require().NoError(err)
	return orch
}

// newOrchestratorWithContentDir returns a fresh Orchestrator sharing every
// other suite dependency, constructed with RPG_CONTENT_DIR set to dir —
// for tests that need a deliberately broken (or deliberately augmented)
// content override. loadContentSpecs only ever reads RPG_CONTENT_DIR
// once, at New() time (Task E2's note #3: never per-request), so a test
// needing a specific override must build its OWN Orchestrator here rather
// than reuse s.orch, which was already constructed in SetupTest before
// the test body ever runs. Returns the error too (rather than requiring
// success) so this same helper covers both a successful construction
// (the disabled-key test) and a construction FAILURE (an unreadable
// RPG_CONTENT_DIR path).
func (s *LobbySuite) newOrchestratorWithContentDir(dir string) (*lobbyorch.Orchestrator, error) {
	s.T().Setenv("RPG_CONTENT_DIR", dir)
	return lobbyorch.New(&lobbyorch.Config{
		LobbyRepo:         s.lobbyRepo,
		LobbyBroker:       s.broker,
		CharacterRepo:     s.charRepo,
		EncounterRepo:     s.encRepo,
		EncounterBroker:   s.encBroker,
		CharacterResolver: encounterhandlerv2.StubCharacterResolver{},
		BuildCombatResolver: func(_ *tkenc.Data) tkenc.CombatResolver {
			return nil
		},
		BuildMovementResolver: func(_ *tkenc.Data) tkenc.MovementResolver {
			return nil
		},
		LobbyIDGenerator:     idgen.NewSequential("lobby"),
		JoinRefGenerator:     idgen.NewSequential("ref"),
		EncounterIDGenerator: idgen.NewSequential("enc"),
		Now:                  func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) },
	})
}

// newOrchestratorWithDungeonKeyOverride returns a fresh Orchestrator
// sharing every other suite dependency, constructed with
// Config.DungeonKeyOverride set to key (Task E2b's RPG_DUNGEON_KEY
// mechanism) — s.orch never has an override set, so a test exercising it
// must build its own Orchestrator here, exactly like
// newOrchestratorWithContentDir does for RPG_CONTENT_DIR.
func (s *LobbySuite) newOrchestratorWithDungeonKeyOverride(key string) *lobbyorch.Orchestrator {
	orch, err := lobbyorch.New(&lobbyorch.Config{
		LobbyRepo:         s.lobbyRepo,
		LobbyBroker:       s.broker,
		CharacterRepo:     s.charRepo,
		EncounterRepo:     s.encRepo,
		EncounterBroker:   s.encBroker,
		CharacterResolver: encounterhandlerv2.StubCharacterResolver{},
		BuildCombatResolver: func(_ *tkenc.Data) tkenc.CombatResolver {
			return nil
		},
		BuildMovementResolver: func(_ *tkenc.Data) tkenc.MovementResolver {
			return nil
		},
		LobbyIDGenerator:     idgen.NewSequential("lobby"),
		JoinRefGenerator:     idgen.NewSequential("ref"),
		EncounterIDGenerator: idgen.NewSequential("enc"),
		Now:                  func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) },
		DungeonKeyOverride:   key,
	})
	s.Require().NoError(err)
	return orch
}

// seedLobby writes data directly to s.lobbyRepo, bypassing CreateLobby /
// JoinLobby (and their character-repo lookups) so tests that exercise a
// LATER RPC (SetReady, LeaveLobby, StartEncounter, SetConnected) can set up
// their starting roster in one call instead of chaining gomock-backed
// CreateLobby/JoinLobby calls.
func (s *LobbySuite) seedLobby(data *lobbyrepo.Data) {
	s.Require().NoError(s.lobbyRepo.Save(s.ctx, data))
}

func TestLobbySuite(t *testing.T) {
	suite.Run(t, new(LobbySuite))
}
