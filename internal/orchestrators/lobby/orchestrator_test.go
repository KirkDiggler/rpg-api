package lobby_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"go.uber.org/mock/gomock"

	encounterhandlerv2 "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v2/encounter"
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	charactermock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	encountersv2 "github.com/KirkDiggler/rpg-api/internal/repositories/encounters/v2"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
)

// TestNew_NegativePartyCap_ReturnsError proves Config.PartyCap < 0 is
// rejected at construction rather than silently making every JoinLobby fail
// (a negative cap makes len(members) >= partyCap always true).
func TestNew_NegativePartyCap_ReturnsError(t *testing.T) {
	ctrl := gomock.NewController(t)
	_, err := lobbyorch.New(&lobbyorch.Config{
		LobbyRepo:         lobbyrepo.NewInMemory(),
		LobbyBroker:       lobbyorch.NewBroker(),
		CharacterRepo:     charactermock.NewMockRepository(ctrl),
		EncounterRepo:     encountersv2.NewInMemory(),
		EncounterBroker:   tkenc.NewBroker(tkenc.NewInMemoryTransport()),
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
		PartyCap:             -1,
	})
	require.Error(t, err)
}
