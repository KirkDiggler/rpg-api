package lobby_test

import (
	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
)

func (s *LobbySuite) seedReadyLobby(id, host string, others ...string) {
	members := map[string]*lobbyrepo.Member{
		host: {PlayerID: host, CharacterID: "char-" + host, IsHost: true, IsReady: true},
	}
	order := make([]string, 0, 1+len(others))
	order = append(order, host)
	for _, p := range others {
		members[p] = &lobbyrepo.Member{PlayerID: p, CharacterID: "char-" + p, IsReady: true}
		order = append(order, p)
	}
	s.seedLobby(&lobbyrepo.Data{
		ID: id, HostPlayerID: host, Status: lobbyrepo.StatusWaiting,
		Members: members, MemberOrder: order,
	})
}

func (s *LobbySuite) TestStartEncounter_Success_ConstructsAndPersistsEncounter() {
	s.seedReadyLobby("lobby-s1", "alice", "bob")
	s.expectCharacter("char-alice", "alice", "Alice", 12, 12)
	s.expectCharacter("char-bob", "bob", "Bob", 10, 10)

	out, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-s1",
	})
	s.Require().NoError(err)
	s.Require().NotEmpty(out.EncounterID)

	encData, err := s.encRepo.Get(s.ctx, out.EncounterID)
	s.Require().NoError(err)
	s.Require().Len(encData.Players, 2, "both ready members become encounter players")
	s.Require().Contains(encData.Players, core.PlayerID("alice"))
	s.Require().Contains(encData.Players, core.PlayerID("bob"))
	s.Require().Equal(12, encData.Players[core.PlayerID("alice")].HP, "HP must be seeded from the character store")
	s.Require().Equal(10, encData.Players[core.PlayerID("bob")].HP)

	// rpg-api#632: an unseeded SightRange (0) reveals exactly one hex per
	// player, so nobody can see anybody else — the diagnosed bug. Assert each
	// member's cumulative reveal actually covers the other member's spawn hex,
	// not just their own.
	alice := encData.Players[core.PlayerID("alice")]
	bob := encData.Players[core.PlayerID("bob")]
	s.Require().NotZero(alice.View.SightRange, "SightRange must be seeded — a zero value reveals only the player's own hex")
	s.Require().True(alice.View.RevealedHexes.Has(bob.View.Position), "alice must be able to see bob's spawn hex")
	s.Require().True(bob.View.RevealedHexes.Has(alice.View.Position), "bob must be able to see alice's spawn hex")

	lobbyData, err := s.lobbyRepo.Get(s.ctx, "lobby-s1")
	s.Require().NoError(err)
	s.Require().Equal(lobbyrepo.StatusStarted, lobbyData.Status)
	s.Require().Equal(out.EncounterID, lobbyData.EncounterID)
}

func (s *LobbySuite) TestStartEncounter_CharacterNotFound_SeedsZeroHP_NotFatal() {
	s.seedReadyLobby("lobby-s2", "alice")
	s.charRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-alice"}).
		Return(nil, apierr.NotFound("character not found"))

	out, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-s2",
	})
	s.Require().NoError(err, "a missing character seeds 0/0 HP rather than failing StartEncounter")

	encData, err := s.encRepo.Get(s.ctx, out.EncounterID)
	s.Require().NoError(err)
	s.Require().Equal(0, encData.Players[core.PlayerID("alice")].HP)
	s.Require().Equal(0, encData.Players[core.PlayerID("alice")].MaxHP)
}

func (s *LobbySuite) TestStartEncounter_NotHost_PermissionDenied() {
	s.seedReadyLobby("lobby-s3", "alice", "bob")

	_, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "bob", LobbyID: "lobby-s3",
	})
	s.Require().ErrorIs(err, lobbyorch.ErrNotHost)
}

func (s *LobbySuite) TestStartEncounter_NotAllReady_FailedPrecondition() {
	s.seedLobby(&lobbyrepo.Data{
		ID: "lobby-s4", HostPlayerID: "alice", Status: lobbyrepo.StatusWaiting,
		Members: map[string]*lobbyrepo.Member{
			"alice": {PlayerID: "alice", IsHost: true, IsReady: true},
			"bob":   {PlayerID: "bob", IsReady: false},
		},
		MemberOrder: []string{"alice", "bob"},
	})

	_, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-s4",
	})
	s.Require().ErrorIs(err, lobbyorch.ErrNotAllReady)
}

func (s *LobbySuite) TestStartEncounter_AlreadyStarted_FailedPrecondition() {
	s.seedLobby(&lobbyrepo.Data{
		ID: "lobby-s5", HostPlayerID: "alice", Status: lobbyrepo.StatusStarted,
		EncounterID: "enc-existing",
		Members:     map[string]*lobbyrepo.Member{"alice": {PlayerID: "alice", IsHost: true, IsReady: true}},
		MemberOrder: []string{"alice"},
	})

	_, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-s5",
	})
	s.Require().ErrorIs(err, lobbyorch.ErrLobbyAlreadyStarted)
}

func (s *LobbySuite) TestStartEncounter_LobbyNotFound() {
	_, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "no-such-lobby",
	})
	s.Require().ErrorIs(err, lobbyorch.ErrLobbyNotFound)
}

// TestStartEncounter_PublishesEncounterStarted_AfterPersist proves the
// persist-then-emit ordering (lobby-surface.md): by the time the broadcast
// event lands in a subscriber's channel, the encounter is already readable
// from the encounter repo.
func (s *LobbySuite) TestStartEncounter_PublishesEncounterStarted_AfterPersist() {
	s.seedReadyLobby("lobby-s6", "alice")
	s.expectCharacter("char-alice", "alice", "Alice", 12, 12)

	sub, err := s.broker.Subscribe("lobby-s6")
	s.Require().NoError(err)
	defer func() { _ = sub.Close() }()

	out, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-s6",
	})
	s.Require().NoError(err)

	evt := <-sub.Events()
	s.Require().Equal(lobbyorch.EventKindEncounterStarted, evt.Kind)
	s.Require().Equal(out.EncounterID, evt.EncounterStarted.EncounterID)

	// The encounter must already be persisted by the time this event is
	// observable — that ordering is what makes the event safe to act on.
	_, err = s.encRepo.Get(s.ctx, out.EncounterID)
	s.Require().NoError(err)
}
