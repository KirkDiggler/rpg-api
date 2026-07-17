package lobby_test

import (
	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
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

// TestStartEncounter_SeedsHonestCombatSnapshot proves the rpg-api#634 fix:
// AC is seeded from the stored character (a real field, copied verbatim),
// while AttackBonus/DamageDice/DamageType stay zero — a character carries no
// precomputed value for them, and rpg-api must not compute one (that's rules
// math the toolkit owns). isPlayerCombatant instead treats a hydrated seat
// as combat-ready; that hydration comes from the v2 encounter orchestrator's
// characterData.Attach cascade on the first combat-capable RPC, not from
// StartEncounter, so DataJSON is correctly absent here.
func (s *LobbySuite) TestStartEncounter_SeedsHonestCombatSnapshot() {
	s.seedReadyLobby("lobby-s7", "alice")
	s.expectCharacterWithAC("char-alice", "alice", "Alice", 12, 12, 15)

	out, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-s7",
	})
	s.Require().NoError(err)

	encData, err := s.encRepo.Get(s.ctx, out.EncounterID)
	s.Require().NoError(err)
	alice := encData.Players[core.PlayerID("alice")]
	s.Require().Equal(15, alice.AC, "AC must be seeded honestly from the stored character")
	s.Require().Zero(alice.AttackBonus, "no stored field to honestly derive an attack bonus from")
	s.Require().Empty(alice.DamageDice, "no stored field to honestly derive damage dice from")
	s.Require().Empty(alice.DamageType, "no stored field to honestly derive a damage type from")
	s.Require().Empty(alice.DataJSON, "hydration is the v2 orchestrator's characterData.Attach cascade's job, not StartEncounter's")
}

// TestStartEncounter_ClearsStaleActionEconomy is the wave-close playtest
// blocker regression test (rpg-api#644): a character carrying a non-nil
// ActionEconomy left over from a PRIOR encounter (character.ActionEconomy
// has no encounter scoping — see clearStaleActionEconomy's doc) must have
// it cleared by StartEncounter, or the toolkit's Move() budget gate
// (InCombat() == ActionEconomy != nil) rejects every move on the brand-new
// FREE_ROAM encounter with "insufficient movement remaining", even though
// nothing about the new encounter has happened yet — reproduced live on the
// dev stack with alice's real character data (movement_remaining: 0,
// turn_number: 1, carried over from an earlier playtest's combat).
func (s *LobbySuite) TestStartEncounter_ClearsStaleActionEconomy() {
	s.seedReadyLobby("lobby-stale-economy", "alice")

	staleEconomy := &toolkitchar.ActionEconomyData{
		TurnNumber: 1, ActionsRemaining: 0, BonusActionsRemaining: 0,
		ReactionsRemaining: 0, MovementRemaining: 0,
	}
	s.charRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-alice"}).
		Return(&characterrepo.GetOutput{
			Character: &entities.Character{
				Data: &toolkitchar.Data{
					ID: "char-alice", PlayerID: "alice", Name: "Alice",
					HitPoints: 12, MaxHitPoints: 12,
					ActionEconomy: staleEconomy,
				},
			},
		}, nil)
	var persisted *toolkitchar.Data
	s.charRepo.EXPECT().
		Update(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ interface{}, input characterrepo.UpdateInput) (*characterrepo.UpdateOutput, error) {
			persisted = input.Character.Data
			return &characterrepo.UpdateOutput{Character: input.Character}, nil
		})

	out, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-stale-economy",
	})
	s.Require().NoError(err, "a stale action economy must not block StartEncounter")
	s.Require().NotEmpty(out.EncounterID)

	s.Require().NotNil(persisted, "the cleared character must be persisted back to the character store")
	s.Require().Nil(persisted.ActionEconomy, "ActionEconomy must be cleared (ExitCombat) so the fresh encounter's Move() is not budget-gated")

	// HP/MaxHP still seed correctly onto the new encounter — clearing the
	// stale economy must not disturb the honest combat-snapshot seed.
	encData, err := s.encRepo.Get(s.ctx, out.EncounterID)
	s.Require().NoError(err)
	s.Require().Equal(12, encData.Players[core.PlayerID("alice")].HP)
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

// TestStartEncounter_WalledRoomAndGoblins_CombatStartsOnSightedMove is the
// rpg-api#644 headline proof (The Dungeon wave 1, design doc "Done when"
// bar): StartEncounter creates the encounter WITH a walled room and 2
// goblins, combat does NOT start at spawn (the goblins are placed outside
// alice's initial sight), and a Move that brings a goblin into sight flips
// the encounter to TURN_BASED BY RULE — the toolkit's own inline
// checkCombatEntry, not anything rpg-api triggers directly. No devseed
// --inject-combat involved.
func (s *LobbySuite) TestStartEncounter_WalledRoomAndGoblins_CombatStartsOnSightedMove() {
	s.seedReadyLobby("lobby-dungeon", "alice")
	s.expectCharacter("char-alice", "alice", "Alice", 12, 12)

	out, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-dungeon",
	})
	s.Require().NoError(err)

	encData, err := s.encRepo.Get(s.ctx, out.EncounterID)
	s.Require().NoError(err)

	// Walls on the wire: the encounter was created with a real room snapshot.
	s.Require().NotNil(encData.Space, "StartEncounter must create the encounter with a room (rpg-toolkit#757 SpaceData)")
	s.Require().Equal(20, encData.Space.Width)
	s.Require().Equal(20, encData.Space.Height)

	// 2 goblins seeded.
	s.Require().Len(encData.Monsters, 2, "StartEncounter must seed exactly 2 goblins")
	goblinPositions := make([]core.Hex, 0, len(encData.Monsters))
	for id, m := range encData.Monsters {
		s.Require().Positive(m.HP, "goblin %q must have positive HP", id)
		s.Require().NotEmpty(m.DataJSON, "goblin %q must carry DataJSON (monster.NewGoblin, not a hand-rolled stub)", id)
		goblinPositions = append(goblinPositions, m.Position)
	}
	// Distinct positions: monster.Monster doesn't implement spatial.Placeable,
	// so the spawn engine's baseline CanPlaceEntity occupancy check silently
	// no-ops for goblin-on-goblin placement (it only blocks when the existing
	// occupant type-asserts to Placeable and BlocksMovement() is true).
	// seedGoblins' PositionOracle explicitly rejects already-occupied cells
	// (room.GetEntitiesInRange(pos, 0)) to restore the distinctness guarantee
	// the deleted safeGoblinHexes gave for free by construction — this pins
	// that guarantee so a future regression here is caught, not silently
	// probable-but-unverified (gate finding on rpg-api#650's PR).
	s.Require().NotEqual(goblinPositions[0], goblinPositions[1], "goblins must not be placed on the same hex")

	// Combat must NOT have started at spawn: the goblins were placed outside
	// alice's initial sight, so AddMonster's inline combat-entry check
	// (rpg-toolkit#759) must not have fired.
	s.Require().Equal(core.ModeFreeRoam, encData.Mode, "combat must not start at spawn — goblins are placed outside initial LOS")

	// Rehydrate the live encounter (same broker StartEncounter used) and move
	// alice directly onto one goblin's hex — a Move that forms sight. This
	// must flip the encounter to TURN_BASED BY RULE (the toolkit's own inline
	// checkCombatEntry), not via any rpg-api-side trigger.
	enc, err := tkenc.LoadFromData(s.ctx, encData, s.encBroker)
	s.Require().NoError(err)
	s.Require().Equal(core.ModeFreeRoam, enc.Mode(), "rehydration must not itself flip mode")

	var targetGoblin core.Hex
	for _, m := range encData.Monsters {
		targetGoblin = m.Position
		break
	}
	s.Require().NoError(enc.Move("alice", []core.Hex{targetGoblin}),
		"moving onto a pre-vetted (non-wall) goblin hex must not be blocked")
	s.Require().Equal(core.ModeTurnBased, enc.Mode(),
		"a Move that brings a goblin into sight must flip the encounter to TURN_BASED by rule")
}
