package lobby_test

import (
	"encoding/json"
	"testing"
	"time"

	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-api/internal/entities"
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
	rpgcore "github.com/KirkDiggler/rpg-toolkit/core"
	coreresources "github.com/KirkDiggler/rpg-toolkit/core/resources"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
	encevents "github.com/KirkDiggler/rpg-toolkit/encounter/events"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	tkresources "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/resources"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/saves"
)

// seedStartedLobbyWithFreeRoamEncounter builds a STARTED lobby whose
// encounter is a real, live tkenc.Encounter in ModeFreeRoam — the exact
// shape of the reported bug (rpg-toolkit#483's movement deadlock, no
// monster ever added, so checkEncounterEnd's victory/TPK predicates can
// never fire naturally). Returns the encounter id.
func (s *LobbySuite) seedStartedLobbyWithFreeRoamEncounter(lobbyID, host, hostEntityID, encID string) string {
	enc := tkenc.New(s.ctx, core.EncounterID(encID), s.encBroker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: core.PlayerID(host), EntityID: core.EntityID(hostEntityID),
		Position: core.Hex{}, SightRange: 10,
		HP: 12, MaxHP: 12, AC: 14,
	}))
	s.Require().Equal(core.ModeFreeRoam, enc.Mode(), "setup sanity: no monster added, must stay FREE_ROAM")
	s.Require().NoError(s.encRepo.Save(s.ctx, enc.ToData()))

	s.seedLobby(&lobbyrepo.Data{
		ID: lobbyID, HostPlayerID: host, Status: lobbyrepo.StatusStarted, EncounterID: encID,
		Members:     map[string]*lobbyrepo.Member{host: {PlayerID: host, CharacterID: hostEntityID, IsHost: true}},
		MemberOrder: []string{host},
	})
	return encID
}

func (s *LobbySuite) TestAbandonEncounter_Success_EndsEncounter_PersistsAsEnded() {
	s.seedStartedLobbyWithFreeRoamEncounter("lobby-a1", "alice", "char-alice", "enc-a1")

	_, err := s.orch.AbandonEncounter(s.ctx, &lobbyorch.AbandonEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-a1",
	})
	s.Require().NoError(err)

	encData, err := s.encRepo.Get(s.ctx, "enc-a1")
	s.Require().NoError(err)
	s.Require().Equal(core.ModeEnded, encData.Mode, "the encounter must persist as ended")
}

// TestAbandonEncounter_PublishesEncounterEndedReasonAbandoned proves the
// terminal event actually reaches the encounter's own StreamEncounter
// channel with the right reason — the signal the web's Leave button (a
// separate dispatch) will render, and downstream consumers rely on to
// distinguish an abandon from a victory/TPK end.
func (s *LobbySuite) TestAbandonEncounter_PublishesEncounterEndedReasonAbandoned() {
	s.seedStartedLobbyWithFreeRoamEncounter("lobby-a2", "alice", "char-alice", "enc-a2")

	sub, err := s.encBroker.Subscribe(core.EncounterID("enc-a2"), core.PlayerID("alice"))
	s.Require().NoError(err)
	defer func() { _ = sub.Close() }()

	_, err = s.orch.AbandonEncounter(s.ctx, &lobbyorch.AbandonEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-a2",
	})
	s.Require().NoError(err)

	ended := waitForEncounterEndedEvent(s.T(), sub, time.Second)
	s.Require().NotNil(ended, "AbandonEncounter must publish EncounterEndedEvent")
	s.Require().Equal(tkenc.EncounterEndedReasonAbandoned, ended.Reason)
}

func (s *LobbySuite) TestAbandonEncounter_NotHost_PermissionDenied() {
	enc := tkenc.New(s.ctx, core.EncounterID("enc-a3"), s.encBroker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: "alice", EntityID: "char-alice", Position: core.Hex{}, SightRange: 10,
		HP: 12, MaxHP: 12, AC: 14,
	}))
	s.Require().NoError(s.encRepo.Save(s.ctx, enc.ToData()))
	s.seedLobby(&lobbyrepo.Data{
		ID: "lobby-a3", HostPlayerID: "alice", Status: lobbyrepo.StatusStarted, EncounterID: "enc-a3",
		Members: map[string]*lobbyrepo.Member{
			"alice": {PlayerID: "alice", CharacterID: "char-alice", IsHost: true},
			"bob":   {PlayerID: "bob", CharacterID: "char-bob"},
		},
		MemberOrder: []string{"alice", "bob"},
	})

	_, err := s.orch.AbandonEncounter(s.ctx, &lobbyorch.AbandonEncounterInput{
		PlayerID: "bob", LobbyID: "lobby-a3",
	})
	s.Require().ErrorIs(err, lobbyorch.ErrNotHost)
}

func (s *LobbySuite) TestAbandonEncounter_LobbyNotFound() {
	_, err := s.orch.AbandonEncounter(s.ctx, &lobbyorch.AbandonEncounterInput{
		PlayerID: "alice", LobbyID: "no-such-lobby",
	})
	s.Require().ErrorIs(err, lobbyorch.ErrLobbyNotFound)
}

func (s *LobbySuite) TestAbandonEncounter_WaitingLobby_ErrLobbyNotStarted() {
	s.seedLobby(&lobbyrepo.Data{
		ID: "lobby-a4", HostPlayerID: "alice", Status: lobbyrepo.StatusWaiting,
		Members:     map[string]*lobbyrepo.Member{"alice": {PlayerID: "alice", IsHost: true}},
		MemberOrder: []string{"alice"},
	})

	_, err := s.orch.AbandonEncounter(s.ctx, &lobbyorch.AbandonEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-a4",
	})
	s.Require().ErrorIs(err, lobbyorch.ErrLobbyNotStarted)
}

// TestAbandonEncounter_AlreadyEnded_ErrEncounterAlreadyEnded is the
// negative control: a second abandon attempt against an encounter that
// already ended (via the first abandon) must reject, not silently
// re-transition or double-publish.
func (s *LobbySuite) TestAbandonEncounter_AlreadyEnded_ErrEncounterAlreadyEnded() {
	s.seedStartedLobbyWithFreeRoamEncounter("lobby-a5", "alice", "char-alice", "enc-a5")

	_, err := s.orch.AbandonEncounter(s.ctx, &lobbyorch.AbandonEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-a5",
	})
	s.Require().NoError(err)

	_, err = s.orch.AbandonEncounter(s.ctx, &lobbyorch.AbandonEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-a5",
	})
	s.Require().ErrorIs(err, lobbyorch.ErrEncounterAlreadyEnded)
}

// TestAbandonEncounter_ThenGetMyActiveLobby_ReturnsEmpty_ThenFreshLobbySeatsAlive
// is the whole-night integration proof: abandon a stuck encounter ->
// GetMyActiveLobby reports nothing to resume into (the existing
// get_my_active_lobby.go liveness check, now exercised end-to-end against a
// real abandoned encounter for the first time) -> a completely fresh
// CreateLobby -> StartEncounter for the SAME player, whose stored character
// is dead-and-drained (0 HP, 3 failed death saves, Unconscious, rage
// charges spent), seats them ALIVE at full HP with FULL RAGE CHARGES via
// rpg-toolkit#785/#786/#795's arcade recovery (rpg-api#671) — proving the
// escape hatch this wave built and the pre-existing revival guarantee
// compose cleanly, not just each in isolation. This is the whole night's
// chain in one test.
func (s *LobbySuite) TestAbandonEncounter_ThenGetMyActiveLobby_ReturnsEmpty_ThenFreshLobbySeatsAlive() {
	s.seedStartedLobbyWithFreeRoamEncounter("lobby-a6", "alice", "char-alice", "enc-a6")

	_, err := s.orch.AbandonEncounter(s.ctx, &lobbyorch.AbandonEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-a6",
	})
	s.Require().NoError(err)

	out, err := s.orch.GetMyActiveLobby(s.ctx, &lobbyorch.GetMyActiveLobbyInput{PlayerID: "alice"})
	s.Require().NoError(err)
	s.Require().Empty(out.LobbyID, "abandoned lobby must not resolve as an active lobby")
	s.Require().Empty(out.EncounterID)
	s.Require().Equal(lobbyrepo.StatusUnspecified, out.Status)

	// Fresh CreateLobby -> StartEncounter for the same player, whose stored
	// character is dead AND drained (rpg-api#670/#671 arcade-recovery
	// fixture, mirroring TestStartEncounter_DeadCharacter_ArcadeRecoveryRestoresAndPersists
	// plus rpg-toolkit#795's resource-pool restoration): 0 rage charges left
	// of a 2-charge max, on top of 0 HP / 3 failed death saves / Unconscious.
	unconsciousBlob, err := json.Marshal(struct {
		Ref *rpgcore.Ref `json:"ref"`
	}{Ref: refs.Conditions.Unconscious()})
	s.Require().NoError(err)

	deadCharacter := func() *entities.Character {
		return &entities.Character{
			Data: &toolkitchar.Data{
				ID: "char-alice", PlayerID: "alice", Name: "Alice",
				HitPoints: 0, MaxHitPoints: 20,
				DeathSaveState: &saves.DeathSaveState{Failures: 3},
				Conditions:     []json.RawMessage{unconsciousBlob},
				Resources: map[coreresources.ResourceKey]toolkitchar.RecoverableResourceData{
					tkresources.RageCharges: {Current: 0, Maximum: 2, ResetType: coreresources.ResetLongRest},
				},
			},
		}
	}
	// CreateLobby's resolveCharacter and StartEncounter's
	// seedMemberCombatSnapshot each independently Get the character.
	s.charRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-alice"}).
		Return(&characterrepo.GetOutput{Character: deadCharacter()}, nil)
	s.charRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-alice"}).
		Return(&characterrepo.GetOutput{Character: deadCharacter()}, nil)
	var persisted *toolkitchar.Data
	s.charRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ interface{}, input characterrepo.UpdateInput) (*characterrepo.UpdateOutput, error) {
			persisted = input.Character.Data
			return &characterrepo.UpdateOutput{Character: input.Character}, nil
		})

	createOut, err := s.orch.CreateLobby(s.ctx, &lobbyorch.CreateLobbyInput{
		PlayerID: "alice", CampaignID: "campaign-1", CharacterID: "char-alice",
	})
	s.Require().NoError(err)
	s.Require().NotEqual("lobby-a6", createOut.LobbyID, "must be a BRAND NEW lobby, not the abandoned one")

	_, err = s.orch.SetReady(s.ctx, &lobbyorch.SetReadyInput{
		PlayerID: "alice", LobbyID: createOut.LobbyID, Ready: true,
	})
	s.Require().NoError(err)

	startOut, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: createOut.LobbyID,
	})
	s.Require().NoError(err)
	s.Require().NotEqual("enc-a6", startOut.EncounterID, "must be a BRAND NEW encounter, not the abandoned one")

	freshEncData, err := s.encRepo.Get(s.ctx, startOut.EncounterID)
	s.Require().NoError(err)
	s.Require().Equal(20, freshEncData.Players[core.PlayerID("alice")].HP,
		"a character dead in the abandoned encounter must be seated ALIVE at full HP in the fresh one")

	s.Require().NotNil(persisted, "arcade recovery must persist the restored character back to the store")
	s.Require().Equal(20, persisted.HitPoints)
	s.Require().Nil(persisted.DeathSaveState)
	s.Require().Empty(persisted.Conditions)

	rage, ok := persisted.Resources[tkresources.RageCharges]
	s.Require().True(ok, "rage charges resource must survive the restore, not be dropped")
	s.Require().Equal(2, rage.Maximum)
	s.Require().Equal(rage.Maximum, rage.Current, "rage charges must be fully restored, not just HP — rpg-toolkit#795")
}

// waitForEncounterEndedEvent reads sub's channel until an
// *encevents.EncounterEndedEvent arrives or timeout elapses, whichever
// first — a bounded alternative to `for evt := range sub.Events()`, which
// blocks the whole suite forever on a genuine regression (no event ever
// published) instead of failing this one test fast (Copilot review, PR
// #675). Returns nil on timeout; callers assert non-nil themselves so the
// failure message stays test-specific.
func waitForEncounterEndedEvent(
	t *testing.T, sub *tkenc.Subscription, timeout time.Duration,
) *encevents.EncounterEndedEvent {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case evt, ok := <-sub.Events():
			if !ok {
				return nil
			}
			if e, ok := evt.(*encevents.EncounterEndedEvent); ok {
				return e
			}
		case <-deadline:
			return nil
		}
	}
}
