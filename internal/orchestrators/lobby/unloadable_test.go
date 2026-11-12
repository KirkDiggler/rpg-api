package lobby_test

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/mock/gomock"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
)

// unloadable_test.go is a run that cannot be loaded holds no seat
// (rpg-project#942): the lobby marks a run whose stored record the SDK
// refuses as invalid encounter data, and nothing else.

// markedRuns records every run the lobby marks unloadable.
type markedRuns struct{ marked []string }

func (m *markedRuns) Mark(_ context.Context, session, _ string) error {
	m.marked = append(m.marked, session)
	return nil
}

// orchestratorMarking is the fixture's orchestrator over repo, marking runs
// in marks.
func (f *lobbyFixture) orchestratorMarking(repo lobbyrepo.Repository, marks *markedRuns) *lobbyorch.Orchestrator {
	orch, err := lobbyorch.New(&lobbyorch.Config{
		LobbyRepo:            repo,
		LobbyBroker:          f.broker,
		CharacterRepo:        f.charRepo,
		LobbyIDGenerator:     idgen.NewSequential("lobby"),
		JoinRefGenerator:     idgen.NewSequential("ref"),
		EncounterIDGenerator: idgen.NewSequential("enc"),
		Now:                  func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) },
		SessionManager:       f.manager,
		Dungeons:             f.registry,
		UnloadableRuns:       marks,
	})
	f.Require().NoError(err)
	return orch
}

// errUnloadable is the SDK refusing a run's stored record.
var errUnloadable = fmt.Errorf("status: %q: %w: load encounter: interaction %q is saved addressing %q, who is not in the run",
	"enc-dead", sdk.ErrInvalidWorld, "front-gate", "char-a")

// TestStartEncounter_ASeatHeldByAnUnloadableRunIsReleased: char-a is seated
// in a run whose stored record cannot be loaded. The launch is refused
// seated-elsewhere; the lobby finds that run, sees the SDK refuse its record
// as invalid encounter data, marks it, and launches once more.
func (s *StartContractSuite) TestStartEncounter_ASeatHeldByAnUnloadableRunIsReleased() {
	seated := fmt.Errorf("launch: character %q is seated in session %q: %w", "char-a", "enc-dead", sdk.ErrSeatedElsewhere)
	const key = "custom-dungeon"
	s.seedLobby(readyContractLobby())
	marks := &markedRuns{}
	orch := s.orchestratorMarking(s.lobbyRepo, marks)

	gomock.InOrder(
		s.registry.EXPECT().Get(s.ctx, hostCaller, key).Return(contractEntry(key), nil),
		s.manager.EXPECT().Launch(s.ctx, gomock.Any()).Return(nil, seated),
		s.manager.EXPECT().Seat(s.ctx, &sdk.SeatInput{Character: "char-b"}).Return(nil, fmt.Errorf("seat: %w", sdk.ErrNoSeat)),
		s.manager.EXPECT().Seat(s.ctx, &sdk.SeatInput{Character: "char-a"}).
			Return(&sdk.SeatOutput{Seat: &sdk.SeatData{Character: "char-a", Session: "enc-dead"}}, nil),
		s.manager.EXPECT().Status(s.ctx, &sdk.StatusInput{Session: "enc-dead"}).Return(nil, errUnloadable),
		s.manager.EXPECT().Launch(s.ctx, gomock.Any()).Return(&sdk.LaunchOutput{}, nil),
	)

	out, err := orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		WorldID: testWorldID, PlayerID: "alice", LobbyID: contractLobbyID, DungeonKey: lobbyorch.DungeonKey(key),
	})
	s.Require().NoError(err, "the second launch seats the party")
	s.Require().NotEmpty(out.EncounterID)
	s.Equal([]string{"enc-dead"}, marks.marked, "the dead run is marked, and only it")
}

// TestStartEncounter_ATransientFailureReleasesNoSeat: the run holding the
// seat could not be read for a reason that is not its record (here, a
// deadline). Nothing is marked, the launch is not tried again, and the seat
// refusal stands with the failure beside it.
func (s *StartContractSuite) TestStartEncounter_ATransientFailureReleasesNoSeat() {
	seated := fmt.Errorf("launch: character %q is seated in session %q: %w", "char-a", "enc-busy", sdk.ErrSeatedElsewhere)
	const key = "custom-dungeon"
	s.seedLobby(readyContractLobby())
	marks := &markedRuns{}
	orch := s.orchestratorMarking(s.lobbyRepo, marks)

	gomock.InOrder(
		s.registry.EXPECT().Get(s.ctx, hostCaller, key).Return(contractEntry(key), nil),
		s.manager.EXPECT().Launch(s.ctx, gomock.Any()).Return(nil, seated),
		s.manager.EXPECT().Seat(s.ctx, &sdk.SeatInput{Character: "char-b"}).Return(nil, fmt.Errorf("seat: %w", sdk.ErrNoSeat)),
		s.manager.EXPECT().Seat(s.ctx, &sdk.SeatInput{Character: "char-a"}).
			Return(&sdk.SeatOutput{Seat: &sdk.SeatData{Character: "char-a", Session: "enc-busy"}}, nil),
		s.manager.EXPECT().Status(s.ctx, &sdk.StatusInput{Session: "enc-busy"}).Return(nil, context.DeadlineExceeded),
	)

	out, err := orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		WorldID: testWorldID, PlayerID: "alice", LobbyID: contractLobbyID, DungeonKey: lobbyorch.DungeonKey(key),
	})
	s.Nil(out)
	s.Require().ErrorIs(err, lobbyorch.ErrCharacterSeatedElsewhere, "the seat refusal stands")
	s.Require().ErrorIs(err, context.DeadlineExceeded, "with the failure that kept it from being judged")
	s.Empty(marks.marked, "a transient failure marks nothing")
}

// startedLobbyOn seeds alice's started lobby on encounter enc.
func (s *LobbySuite) startedLobbyOn(enc string) {
	s.seedLobby(&lobbyrepo.Data{
		ID: "lobby-dead", HostPlayerID: "alice", Status: lobbyrepo.StatusStarted, EncounterID: enc,
		Members:     map[string]*lobbyrepo.Member{"alice": {PlayerID: "alice", IsHost: true}},
		MemberOrder: []string{"alice"},
	})
}

// TestGetMyActiveLobby_AnUnloadableRunIsNotLive: the run's stored record is
// refused as invalid encounter data. The player has no live run to resume,
// and the run is marked so it holds no seat.
func (s *LobbySuite) TestGetMyActiveLobby_AnUnloadableRunIsNotLive() {
	marks := &markedRuns{}
	orch := s.orchestratorMarking(s.lobbyRepo, marks)
	s.manager.EXPECT().Status(s.ctx, &sdk.StatusInput{Session: "enc-dead"}).Return(nil, errUnloadable)
	s.startedLobbyOn("enc-dead")

	out, err := orch.GetMyActiveLobby(s.ctx, &lobbyorch.GetMyActiveLobbyInput{PlayerID: "alice"})
	s.Require().NoError(err)
	s.Empty(out.LobbyID)
	s.Empty(out.EncounterID)
	s.Equal([]string{"enc-dead"}, marks.marked)
}

// TestGetMyActiveLobby_ATransientFailureMarksNothing: a lock timeout is not
// a verdict on the record: the error is returned as before and nothing is
// marked.
func (s *LobbySuite) TestGetMyActiveLobby_ATransientFailureMarksNothing() {
	marks := &markedRuns{}
	orch := s.orchestratorMarking(s.lobbyRepo, marks)
	s.manager.EXPECT().Status(s.ctx, &sdk.StatusInput{Session: "enc-busy"}).
		Return(nil, fmt.Errorf("lock session %q: %w", "enc-busy", context.DeadlineExceeded))
	s.startedLobbyOn("enc-busy")

	_, err := orch.GetMyActiveLobby(s.ctx, &lobbyorch.GetMyActiveLobbyInput{PlayerID: "alice"})
	s.Require().Error(err)
	s.Empty(marks.marked)
}
