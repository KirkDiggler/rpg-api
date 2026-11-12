package lobby_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
)

// TestAPlayerInARunNobodyCanLoadCanStartAnother is the seat rule on the real
// stack (rpg-project#942): alice's run is launched, and its stored record is
// then made one the loader refuses (here, a member on the world clock who is
// not on the roster, as a stand-in for the address naming a leaver). Every
// read of that run now fails with invalid encounter data. Without the rule
// her seat in it refuses every launch; with it, her new lobby starts, and
// the dead run's stored data is still there.
func TestAPlayerInARunNobodyCanLoadCanStartAnother(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	charRepo, err := characterrepo.NewRedis(&characterrepo.RedisConfig{Client: client})
	require.NoError(t, err)
	sessOrch, err := sessionorch.New(sessionorch.Config{
		Redis: client, Characters: charRepo, TTL: 24 * time.Hour,
		PresentationIDs: idgen.NewSequential("presentation"),
	})
	require.NoError(t, err)
	lobbies := lobbyrepo.NewInMemory()
	orch, err := lobbyorch.New(&lobbyorch.Config{
		LobbyRepo: lobbies, LobbyBroker: lobbyorch.NewBroker(), CharacterRepo: charRepo,
		LobbyIDGenerator: idgen.NewSequential("lobby"), JoinRefGenerator: idgen.NewSequential("ref"),
		EncounterIDGenerator: idgen.NewSequential("enc"),
		SessionManager:       sessOrch.Manager, Dungeons: dungeonstest.Shipped(t),
		UnloadableRuns: sessionorch.NewUnloadableRuns(client),
	})
	require.NoError(t, err)

	s := &SessionStackSuite{}
	s.SetT(t)
	s.ctx, s.charRepo, s.lobbyRepo = ctx, charRepo, lobbies
	s.seedCharacter("char-alice", "alice", "Alice")
	s.seedReadyLobby("lobby-1", "alice")
	first, err := orch.StartEncounter(ctx, &lobbyorch.StartEncounterInput{WorldID: testWorldID, PlayerID: "alice", LobbyID: "lobby-1"})
	require.NoError(t, err)

	// Make the stored run one the loader refuses.
	encounters := sessionorch.NewEncounterRepository(client, 24*time.Hour)
	world, err := encounters.GetEncounter(ctx, first.EncounterID)
	require.NoError(t, err)
	kept := world.Members[:0]
	for _, m := range world.Members {
		if string(m.ID) != "char-alice" {
			kept = append(kept, m)
		}
	}
	world.Members = kept
	require.NoError(t, encounters.SaveEncounter(ctx, first.EncounterID, world))
	_, err = sessOrch.Manager.Status(ctx, &sdk.StatusInput{Session: first.EncounterID})
	require.True(t, errors.Is(err, sdk.ErrInvalidWorld), "precondition: the run cannot be loaded: %v", err)

	s.seedReadyLobby("lobby-2", "alice")
	second, err := orch.StartEncounter(ctx, &lobbyorch.StartEncounterInput{WorldID: testWorldID, PlayerID: "alice", LobbyID: "lobby-2"})
	require.NoError(t, err, "alice starts another run")
	require.NotEqual(t, first.EncounterID, second.EncounterID)
	_, err = encounters.GetEncounter(ctx, first.EncounterID)
	require.NoError(t, err, "the dead run's stored data is left in place")
}
