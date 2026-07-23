package lobby_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	lobbyorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/lobby"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
	rpgcore "github.com/KirkDiggler/rpg-toolkit/core"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	"github.com/KirkDiggler/rpg-toolkit/encounter/core"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/saves"
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

// TestStartEncounter_DeadCharacter_ArcadeRecoveryRestoresAndPersists is the
// rpg-api#670 regression: a character carrying 0 HP from a PRIOR encounter
// (a confirmed death or an unresolved TPK snapshot) must be seated ALIVE at
// full HP in a BRAND NEW encounter — rpg-toolkit's arcade recovery
// (character.RestoreForNewEncounter, toolkit#785/#786) exists for exactly
// this, but only fires where a caller actually invokes it. StartEncounter's
// AddPlayer call deliberately carries no DataJSON (hydration is the v2
// orchestrator's job — see TestStartEncounter_SeedsHonestCombatSnapshot
// above), so the toolkit's OWN DataJSON-gated restoreForNewSeat path inside
// AddPlayer never fires for a real lobby-started encounter. character.go's
// restoreForNewEncounter step is what actually triggers the toolkit's
// restore rule here, and must persist the result back to the character
// store — otherwise the NEXT StartEncounter for this character would read
// the same pre-restore hp=0 record right back out of the store.
func (s *LobbySuite) TestStartEncounter_DeadCharacter_ArcadeRecoveryRestoresAndPersists() {
	s.seedReadyLobby("lobby-dead-alice", "alice")

	unconsciousBlob, err := json.Marshal(struct {
		Ref *rpgcore.Ref `json:"ref"`
	}{Ref: refs.Conditions.Unconscious()})
	s.Require().NoError(err)

	s.charRepo.EXPECT().
		Get(gomock.Any(), characterrepo.GetInput{ID: "char-alice"}).
		Return(&characterrepo.GetOutput{
			Character: &entities.Character{
				Data: &toolkitchar.Data{
					ID: "char-alice", PlayerID: "alice", Name: "Alice",
					HitPoints: 0, MaxHitPoints: 20,
					DeathSaveState: &saves.DeathSaveState{Failures: 3},
					Conditions:     []json.RawMessage{unconsciousBlob},
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
		PlayerID: "alice", LobbyID: "lobby-dead-alice",
	})
	s.Require().NoError(err)

	// Alive on the encounter wire: full HP, not the dead hp=0 the store had.
	encData, err := s.encRepo.Get(s.ctx, out.EncounterID)
	s.Require().NoError(err)
	s.Require().Equal(20, encData.Players[core.PlayerID("alice")].HP,
		"a dead character must be seated at full HP in a NEW encounter (arcade recovery)")
	s.Require().Equal(20, encData.Players[core.PlayerID("alice")].MaxHP)

	// Persisted back to the character store: the NEXT StartEncounter must not
	// read the same pre-restore hp=0 record again.
	s.Require().NotNil(persisted, "the restored character must be persisted back to the character store")
	s.Require().Equal(20, persisted.HitPoints, "persisted record must carry the restored HP")
	s.Require().Nil(persisted.DeathSaveState, "death-save state must be cleared")
	s.Require().Empty(persisted.Conditions, "the Unconscious condition must be stripped")
}

// TestStartEncounter_LivingCharacter_ArcadeRecoveryDoesNotFire proves the
// restore is death-scoped, not a free heal: a character already above 0 HP
// must be seated unchanged, with no extra Update call to the character
// store beyond StartEncounter's other necessary writes.
func (s *LobbySuite) TestStartEncounter_LivingCharacter_ArcadeRecoveryDoesNotFire() {
	s.seedReadyLobby("lobby-alive-alice", "alice")
	s.expectCharacter("char-alice", "alice", "Alice", 7, 20)
	// No Update EXPECT() is armed: gomock fails the test if StartEncounter
	// calls charRepo.Update for a character that never needed restoring.

	out, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-alive-alice",
	})
	s.Require().NoError(err)

	encData, err := s.encRepo.Get(s.ctx, out.EncounterID)
	s.Require().NoError(err)
	s.Require().Equal(7, encData.Players[core.PlayerID("alice")].HP,
		"a living character's HP must not be touched by arcade recovery")
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

// regionByArchetype finds the single region tagged with the given
// archetype in space.Regions. Returns ok=false for BOTH zero matches and
// more than one match — a duplicate archetype is exactly as wrong as a
// missing one for the crypt spec's "exactly one entrance/corridor/boss"
// invariant, and silently returning the first of several duplicates would
// let that invariant break without any test noticing (Copilot review
// catch on this PR).
func regionByArchetype(space *tkenc.SpaceData, archetype tkenc.RegionArchetype) (tkenc.RegionData, bool) {
	var found tkenc.RegionData
	count := 0
	for _, r := range space.Regions {
		if r.Archetype == archetype {
			found = r
			count++
		}
	}
	return found, count == 1
}

// TestRegionByArchetype_DuplicateArchetype_ReturnsNotOK is the Copilot
// review regression pin (PR #693): a naive first-match implementation
// would report ok=true even with two regions sharing an archetype,
// silently masking the exact "InitDungeon accidentally produced two boss
// regions" failure mode the crypt-spec tests above rely on this helper to
// catch.
func TestRegionByArchetype_DuplicateArchetype_ReturnsNotOK(t *testing.T) {
	space := &tkenc.SpaceData{
		Regions: []tkenc.RegionData{
			{ID: "boss-1", Archetype: tkenc.ArchetypeBoss},
			{ID: "boss-2", Archetype: tkenc.ArchetypeBoss},
		},
	}
	_, ok := regionByArchetype(space, tkenc.ArchetypeBoss)
	require.False(t, ok, "two regions sharing an archetype must not resolve as a single match")
}

func TestRegionByArchetype_NoMatch_ReturnsNotOK(t *testing.T) {
	space := &tkenc.SpaceData{Regions: []tkenc.RegionData{{ID: "entrance", Archetype: tkenc.ArchetypeEntrance}}}
	_, ok := regionByArchetype(space, tkenc.ArchetypeBoss)
	require.False(t, ok)
}

// regionParamsByArchetype is regionByArchetype's sibling over
// tkenc.DungeonParams.Regions (the toolkit CONSTRUCTOR's input params,
// pre-generation) rather than over a persisted tkenc.SpaceData.Regions
// (post-generation output) — used by rpg-api#694's tests to read expected
// per-region Width/Obstacles directly off a fresh tkenc.CryptDungeonParams
// call instead of hardcoding a number that could silently drift from the
// toolkit's own crypt template.
func regionParamsByArchetype(params tkenc.DungeonParams, archetype tkenc.RegionArchetype) (tkenc.DungeonRegionParams, bool) {
	var found tkenc.DungeonRegionParams
	count := 0
	for _, r := range params.Regions {
		if r.Archetype == archetype {
			found = r
			count++
		}
	}
	return found, count == 1
}

func TestRegionByArchetype_ExactlyOneMatch_ReturnsIt(t *testing.T) {
	boss := tkenc.RegionData{ID: "boss", Archetype: tkenc.ArchetypeBoss}
	space := &tkenc.SpaceData{Regions: []tkenc.RegionData{{ID: "entrance", Archetype: tkenc.ArchetypeEntrance}, boss}}
	got, ok := regionByArchetype(space, tkenc.ArchetypeBoss)
	require.True(t, ok)
	require.Equal(t, boss, got)
}

// regionArchetypeAt returns the RegionArchetype of the region containing
// hex, and whether one was found — the archetype-keyed sibling of
// SpaceData.RegionAt (which only returns the region ID), used throughout
// these tests so assertions key off the toolkit's fixed generic-role
// vocabulary instead of this package's own spec-specific region ID
// strings (cryptRegionIDEntrance etc. are unexported — these tests are
// black-box, package lobby_test).
func regionArchetypeAt(space *tkenc.SpaceData, hex core.Hex) (tkenc.RegionArchetype, bool) {
	for _, r := range space.Regions {
		if r.Hexes.Has(hex) {
			return r.Archetype, true
		}
	}
	return "", false
}

// regionBoundingWidth returns a region's offset-coordinate column span
// (max X - min X + 1) — the same "width" tkenc.DungeonRegionParams.Width
// configures, read back purely from the persisted hex membership rather
// than any rpg-api-side constant, so assertions using this pin the
// toolkit's OWN generated geometry, not a duplicated expectation.
func regionBoundingWidth(region tkenc.RegionData) int {
	minX, maxX := math.MaxInt, math.MinInt
	for h := range region.Hexes {
		x := int(h.ToPosition().X)
		if x < minX {
			minX = x
		}
		if x > maxX {
			maxX = x
		}
	}
	return maxX - minX + 1
}

// TestStartEncounter_CryptDungeon_ThreeRegionsArchetypesThemeScaleAndEntrance
// is the rpg-api#688 headline proof (updated by rpg-api#694 to read
// expected dimensions off the toolkit constructor itself, never a
// hardcoded literal): StartEncounter now builds the toolkit's generic
// InitDungeon N-region linear chain selected by the crypt key
// (rpg-toolkit#814's Approved Slice 3 corrections, rpg-toolkit#826's
// CryptDungeonParams) instead of the retired two-chamber constants —
// exactly 3 regions (entrance -> corridor -> boss), each carrying its own
// RegionArchetype, Space.Theme == "crypt" passed through opaque and
// unbranched, the boss region's scale invariant (primary playable axis >
// 6 hex steps, enforced by the toolkit's own validateDungeonParams at
// generation time — not eyeballed here), and the party spawning inside
// the entrance region at its designated anchor.
//
// wantParams is built by calling tkenc.CryptDungeonParams directly with
// the SAME seed StartEncounter is given below — the door IDs passed here
// are throwaway placeholders (rpg-api#694: Width/Height/Theme/Regions/
// Obstacles never depend on which door ID string a caller picks), used
// only so this black-box test never hardcodes a region width/height that
// could silently drift from the toolkit's own crypt template.
func (s *LobbySuite) TestStartEncounter_CryptDungeon_ThreeRegionsArchetypesThemeScaleAndEntrance() {
	const seed = 42
	wantParams := tkenc.CryptDungeonParams(seed, "want-door-1", "want-door-2")

	s.seedReadyLobby("lobby-crypt1", "alice", "bob")
	s.expectCharacter("char-alice", "alice", "Alice", 12, 12)
	s.expectCharacter("char-bob", "bob", "Bob", 10, 10)

	out, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-crypt1",
		DungeonKey: lobbyorch.DungeonKeyCrypt, RandomSeed: seed,
	})
	s.Require().NoError(err)

	encData, err := s.encRepo.Get(s.ctx, out.EncounterID)
	s.Require().NoError(err)
	s.Require().NotNil(encData.Space, "StartEncounter must create the encounter with an InitDungeon space")
	s.Require().Equal(wantParams.Theme, encData.Space.Theme, "the crypt spec's opaque theme must pass through verbatim")
	s.Require().Equal(wantParams.Height, encData.Space.Height)
	wantWidth := 0
	for _, r := range wantParams.Regions {
		wantWidth += r.Width + 1
	}
	wantWidth-- // no trailing boundary column after the last region
	s.Require().Equal(wantWidth, encData.Space.Width,
		"total width = sum(region width + 1 door-column) - 1 trailing column, entirely toolkit-derived")
	s.Require().Len(encData.Space.Regions, 3, "the crypt spec is a 3-region linear chain: entrance -> corridor -> boss")

	entrance, entranceOK := regionByArchetype(encData.Space, tkenc.ArchetypeEntrance)
	s.Require().True(entranceOK, "exactly one entrance-archetype region")
	corridor, corridorOK := regionByArchetype(encData.Space, tkenc.ArchetypeCorridor)
	s.Require().True(corridorOK, "exactly one corridor-archetype region")
	boss, bossOK := regionByArchetype(encData.Space, tkenc.ArchetypeBoss)
	s.Require().True(bossOK, "exactly one boss-archetype region")

	entranceParams, _ := regionParamsByArchetype(wantParams, tkenc.ArchetypeEntrance)
	corridorParams, _ := regionParamsByArchetype(wantParams, tkenc.ArchetypeCorridor)
	bossParams, _ := regionParamsByArchetype(wantParams, tkenc.ArchetypeBoss)
	s.Require().Equal(entranceParams.Width, regionBoundingWidth(entrance))
	s.Require().Equal(corridorParams.Width, regionBoundingWidth(corridor))
	s.Require().Equal(bossParams.Width, regionBoundingWidth(boss))

	// rpg-toolkit#814's Approved Slice 3 corrections' scale invariant: the
	// boss region's primary playable axis (min(width, shared height)) must
	// exceed 6 hex steps — enforced by the toolkit's own
	// validateDungeonParams at generation time, not eyeballed here.
	bossAxis := regionBoundingWidth(boss)
	if encData.Space.Height < bossAxis {
		bossAxis = encData.Space.Height
	}
	s.Require().Greater(bossAxis, 6, "boss chamber primary playable axis must exceed 6 hex steps")

	// Exactly 2 connector doors join the 3-region chain, closed and
	// unlocked by default — Interact/OpenDoor is what opens them;
	// StartEncounter never does so itself.
	s.Require().Len(encData.Doors, 2, "a 3-region chain has exactly 2 connectors")
	for id, door := range encData.Doors {
		s.Require().False(door.Open, "connector door %q must start closed", id)
		s.Require().False(door.Locked, "the crypt spec's connectors are plain, not locked")
	}

	// Entrance-anchored spawn (rpg-api#648/#676, preserved): every seated
	// member lands inside the entrance region, and the first member sits
	// exactly at the designated Space.Entrance.
	alice := encData.Players[core.PlayerID("alice")]
	bob := encData.Players[core.PlayerID("bob")]
	s.Require().NotNil(alice)
	s.Require().Equal(encData.Space.Entrance, alice.View.Position,
		"the first member must spawn exactly at the designated entrance")
	aliceArchetype, aliceOK := regionArchetypeAt(encData.Space, alice.View.Position)
	s.Require().True(aliceOK)
	s.Require().Equal(tkenc.ArchetypeEntrance, aliceArchetype)
	bobArchetype, bobOK := regionArchetypeAt(encData.Space, bob.View.Position)
	s.Require().True(bobOK)
	s.Require().Equal(tkenc.ArchetypeEntrance, bobArchetype, "every member must spawn inside the entrance region")

	s.Require().Equal(core.ModeFreeRoam, encData.Mode, "combat must not start at spawn")
}

// obstaclesByRef tallies a region's placed tkenc.ObstacleData by Ref, and
// records the (BlocksMovement, BlocksLoS) pair seen for each Ref — used by
// the obstacle tests below to assert exact counts and blocking flags
// without hardcoding a region's full obstacle list inline per test.
type obstacleBlocking struct {
	blocksMovement bool
	blocksLoS      bool
}

func obstaclesByRef(obstacles []tkenc.ObstacleData) (counts map[string]int, blocking map[string]obstacleBlocking) {
	counts = map[string]int{}
	blocking = map[string]obstacleBlocking{}
	for _, o := range obstacles {
		counts[o.Ref]++
		blocking[o.Ref] = obstacleBlocking{blocksMovement: o.BlocksMovement, blocksLoS: o.BlocksLoS}
	}
	return counts, blocking
}

// TestStartEncounter_CryptObstacles_ExactRefsCountsAndBlockingByRegion is
// rpg-api#694's headline proof: a REAL StartEncounter call persists a
// non-empty SpaceData.Obstacles list whose refs/counts/blocking flags are
// the EXACT canonical set rpg-toolkit#826's CryptDungeonParams places —
// entrance (1 obelisk + 2 pillars), corridor (1 pillar), boss (1 coffin +
// 1 altar + 1 statue-reaper + 1 statue-knight-hooded) — read off the
// toolkit's own exported Ref constants, never a string this package
// invents. rpg-api never computed this list itself; it is entirely the
// toolkit constructor's output, InitDungeon's placement, and this
// package's job is only to have asked for it via CryptDungeonParams.
func (s *LobbySuite) TestStartEncounter_CryptObstacles_ExactRefsCountsAndBlockingByRegion() {
	s.seedReadyLobby("lobby-crypt-obstacles1", "alice")
	s.expectCharacter("char-alice", "alice", "Alice", 12, 12)

	out, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-crypt-obstacles1",
		DungeonKey: lobbyorch.DungeonKeyCrypt, RandomSeed: 300,
	})
	s.Require().NoError(err)

	encData, err := s.encRepo.Get(s.ctx, out.EncounterID)
	s.Require().NoError(err)
	s.Require().NotEmpty(encData.Space.Obstacles, "a real StartEncounter call must persist a non-empty obstacle list")

	byRegion := map[tkenc.RegionArchetype][]tkenc.ObstacleData{}
	seenIDs := make(map[core.EntityID]bool, len(encData.Space.Obstacles))
	for _, o := range encData.Space.Obstacles {
		s.Require().False(seenIDs[o.ID], "obstacle IDs must be unique: %q", o.ID)
		seenIDs[o.ID] = true
		archetype, ok := regionArchetypeAt(encData.Space, o.Position)
		s.Require().True(ok, "obstacle %q must be placed inside a tagged region", o.ID)
		byRegion[archetype] = append(byRegion[archetype], o)
	}

	entranceCounts, entranceBlocking := obstaclesByRef(byRegion[tkenc.ArchetypeEntrance])
	s.Require().Equal(map[string]int{
		tkenc.CryptObstacleRefObelisk: 1,
		tkenc.CryptObstacleRefPillar:  2,
	}, entranceCounts, "entrance region: exactly 1 obelisk + 2 pillars")
	s.Require().Equal(obstacleBlocking{blocksMovement: true, blocksLoS: true}, entranceBlocking[tkenc.CryptObstacleRefObelisk])
	s.Require().Equal(obstacleBlocking{blocksMovement: true, blocksLoS: true}, entranceBlocking[tkenc.CryptObstacleRefPillar])

	corridorCounts, _ := obstaclesByRef(byRegion[tkenc.ArchetypeCorridor])
	s.Require().Equal(map[string]int{
		tkenc.CryptObstacleRefPillar: 1,
	}, corridorCounts, "corridor region: exactly 1 sparse pillar, no others")

	bossCounts, bossBlocking := obstaclesByRef(byRegion[tkenc.ArchetypeBoss])
	s.Require().Equal(map[string]int{
		tkenc.CryptObstacleRefCoffin:             1,
		tkenc.CryptObstacleRefAltar:              1,
		tkenc.CryptObstacleRefStatueReaper:       1,
		tkenc.CryptObstacleRefStatueKnightHooded: 1,
	}, bossCounts, "boss region: coffin + altar + one of each statue variant")
	s.Require().Equal(obstacleBlocking{blocksMovement: true, blocksLoS: false}, bossBlocking[tkenc.CryptObstacleRefCoffin],
		"the coffin/tomb blocks movement but not line of sight -- walk around, see over")
	s.Require().Equal(obstacleBlocking{blocksMovement: true, blocksLoS: true}, bossBlocking[tkenc.CryptObstacleRefAltar])
	s.Require().Equal(obstacleBlocking{blocksMovement: true, blocksLoS: true}, bossBlocking[tkenc.CryptObstacleRefStatueReaper])
	s.Require().Equal(obstacleBlocking{blocksMovement: true, blocksLoS: true},
		bossBlocking[tkenc.CryptObstacleRefStatueKnightHooded])
}

// TestStartEncounter_CryptObstacles_ExplicitSeed_DeterministicPositions
// proves obstacle PLACEMENT (not just refs/counts) is deterministic under
// an explicit seed, mirroring TestStartEncounter_ExplicitSeed_
// ReproducibleDungeonLayout's whole-Space proof but calling out obstacles
// specifically since they are this issue's headline addition.
func (s *LobbySuite) TestStartEncounter_CryptObstacles_ExplicitSeed_DeterministicPositions() {
	s.seedReadyLobby("lobby-crypt-obstacles2a", "alice")
	s.expectCharacter("char-alice", "alice", "Alice", 12, 12)
	out1, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-crypt-obstacles2a", RandomSeed: 271,
	})
	s.Require().NoError(err)
	data1, err := s.encRepo.Get(s.ctx, out1.EncounterID)
	s.Require().NoError(err)

	s.seedReadyLobby("lobby-crypt-obstacles2b", "bob")
	s.expectCharacter("char-bob", "bob", "Bob", 10, 10)
	out2, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "bob", LobbyID: "lobby-crypt-obstacles2b", RandomSeed: 271,
	})
	s.Require().NoError(err)
	data2, err := s.encRepo.Get(s.ctx, out2.EncounterID)
	s.Require().NoError(err)

	s.Require().NotEmpty(data1.Space.Obstacles)
	s.Require().Equal(data1.Space.Obstacles, data2.Space.Obstacles,
		"the same explicit RandomSeed must reproduce byte-identical obstacle placement")
}

// TestStartEncounter_CryptGoblins_SeededAtEntranceAndBossNotCorridor_CombatEntersByRule
// generalizes the pre-#688 "both occupied regions get monsters" rule
// (rpg-api#676) from two hardcoded chamber IDs to an N-region chain's two
// ENDPOINTS by chain POSITION — the entrance (index 0) and the terminal
// region (index len-1, here "boss") — with the interior corridor region
// getting none. This is deliberately NOT rpg-api#689's per-archetype
// seeding intelligence: every populated region still gets the exact same
// goblinsPerRegion count, keyed by chain position, not by Archetype value
// — a behavior-preserving generalization, not new seeding logic.
func (s *LobbySuite) TestStartEncounter_CryptGoblins_SeededAtEntranceAndBossNotCorridor_CombatEntersByRule() {
	s.seedReadyLobby("lobby-crypt2", "alice")
	s.expectCharacter("char-alice", "alice", "Alice", 12, 12)

	out, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-crypt2",
	})
	s.Require().NoError(err)

	encData, err := s.encRepo.Get(s.ctx, out.EncounterID)
	s.Require().NoError(err)

	s.Require().Len(encData.Monsters, 4, "2 goblins each in the entrance and boss regions, none in the corridor")
	archetypeCounts := map[tkenc.RegionArchetype]int{}
	seenPositions := make(map[core.Hex]bool, len(encData.Monsters))
	for id, m := range encData.Monsters {
		s.Require().Positive(m.HP, "goblin %q must have positive HP", id)
		s.Require().NotEmpty(m.DataJSON, "goblin %q must carry DataJSON (monster.NewGoblin, not a hand-rolled stub)", id)
		archetype, ok := regionArchetypeAt(encData.Space, m.Position)
		s.Require().True(ok, "goblin %q must be placed inside a tagged region", id)
		archetypeCounts[archetype]++
		// Distinct positions: monster.Monster doesn't implement
		// spatial.Placeable, so the spawn engine's baseline CanPlaceEntity
		// occupancy check silently no-ops for goblin-on-goblin placement.
		// seedGoblins' PositionOracle explicitly rejects already-occupied
		// cells to restore the distinctness guarantee (gate finding on
		// rpg-api#650's PR).
		s.Require().False(seenPositions[m.Position], "goblins must not be placed on the same hex")
		seenPositions[m.Position] = true
	}
	s.Require().Equal(2, archetypeCounts[tkenc.ArchetypeEntrance], "the entrance region must get exactly 2 goblins")
	s.Require().Equal(2, archetypeCounts[tkenc.ArchetypeBoss], "the boss (terminal) region must get exactly 2 goblins")
	s.Require().Zero(archetypeCounts[tkenc.ArchetypeCorridor], "the interior corridor region must get no goblins")

	// Combat must NOT have started at spawn: every goblin was placed
	// outside alice's initial sight, so AddMonster's inline combat-entry
	// check (rpg-toolkit#759) must not have fired.
	s.Require().Equal(core.ModeFreeRoam, encData.Mode, "combat must not start at spawn — goblins are placed outside initial LOS")

	// Rehydrate the live encounter and move alice directly onto an
	// ENTRANCE-region goblin's hex (reachable without opening any door,
	// exactly like the pre-#688 chamber-1 proof) — a Move that forms sight.
	// This must flip the encounter to TURN_BASED BY RULE (the toolkit's own
	// inline checkCombatEntry), not via any rpg-api-side trigger.
	enc, err := tkenc.LoadFromData(s.ctx, encData, s.encBroker)
	s.Require().NoError(err)
	s.Require().Equal(core.ModeFreeRoam, enc.Mode(), "rehydration must not itself flip mode")

	var targetGoblin core.Hex
	var foundEntranceGoblin bool
	for _, m := range encData.Monsters {
		if archetype, _ := regionArchetypeAt(encData.Space, m.Position); archetype == tkenc.ArchetypeEntrance {
			targetGoblin = m.Position
			foundEntranceGoblin = true
			break
		}
	}
	s.Require().True(foundEntranceGoblin, "must find at least one entrance-region goblin to move onto")
	s.Require().NoError(enc.Move("alice", []core.Hex{targetGoblin}),
		"moving onto a pre-vetted (non-wall) entrance-region goblin hex must not be blocked")
	s.Require().Equal(core.ModeTurnBased, enc.Mode(),
		"a Move that brings an entrance-region goblin into sight must flip the encounter to TURN_BASED by rule")
}

// TestStartEncounter_DefaultDungeonKey_MatchesExplicitCryptKey proves
// rpg-api#688's Scope-section default: StartEncounterInput.DungeonKey's
// zero value must resolve to the exact same spec as explicitly passing
// DungeonKeyCrypt — not merely "some dungeon," the same literal geometry
// given the same seed.
func (s *LobbySuite) TestStartEncounter_DefaultDungeonKey_MatchesExplicitCryptKey() {
	s.seedReadyLobby("lobby-crypt3a", "alice")
	s.expectCharacter("char-alice", "alice", "Alice", 12, 12)
	outDefault, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-crypt3a", RandomSeed: 7,
	})
	s.Require().NoError(err)
	defaultData, err := s.encRepo.Get(s.ctx, outDefault.EncounterID)
	s.Require().NoError(err)

	s.seedReadyLobby("lobby-crypt3b", "carol")
	s.expectCharacter("char-carol", "carol", "Carol", 12, 12)
	outExplicit, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "carol", LobbyID: "lobby-crypt3b", RandomSeed: 7, DungeonKey: lobbyorch.DungeonKeyCrypt,
	})
	s.Require().NoError(err)
	explicitData, err := s.encRepo.Get(s.ctx, outExplicit.EncounterID)
	s.Require().NoError(err)

	s.Require().Equal(defaultData.Space, explicitData.Space,
		"the zero-value DungeonKey must resolve to the same spec as an explicit DungeonKeyCrypt")
}

// TestStartEncounter_UnknownDungeonKey_ErrorsAndLobbyStaysWaiting proves
// rpg-api#688's boundary: an unrecognized key fails LOUDLY — rpg-api never
// invents geometry for a key it doesn't recognize — and fails BEFORE any
// state transition (no encounter persisted, lobby stays WAITING).
func (s *LobbySuite) TestStartEncounter_UnknownDungeonKey_ErrorsAndLobbyStaysWaiting() {
	s.seedReadyLobby("lobby-crypt4", "alice")
	// No expectCharacter armed: resolving the dungeon key fails before
	// StartEncounter ever reaches character-snapshot seeding — gomock would
	// fail this test outright if that assumption were wrong.

	_, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-crypt4", DungeonKey: lobbyorch.DungeonKey("no-such-key"),
	})
	s.Require().Error(err)
	s.Require().ErrorIs(err, lobbyorch.ErrUnknownDungeonKey)

	lobbyData, err := s.lobbyRepo.Get(s.ctx, "lobby-crypt4")
	s.Require().NoError(err)
	s.Require().Equal(lobbyrepo.StatusWaiting, lobbyData.Status, "an unknown dungeon key must fail before any state transition")
	s.Require().Empty(lobbyData.EncounterID)
}

// TestStartEncounter_ExplicitSeed_ReproducibleDungeonLayout proves seed
// semantics survive the InitDungeon migration: the same explicit
// RandomSeed passed through tkenc.DungeonParams.RandomSeed must reproduce
// byte-identical dungeon geometry across two entirely independent
// encounters.
func (s *LobbySuite) TestStartEncounter_ExplicitSeed_ReproducibleDungeonLayout() {
	s.seedReadyLobby("lobby-crypt5a", "alice")
	s.expectCharacter("char-alice", "alice", "Alice", 12, 12)
	out1, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-crypt5a", RandomSeed: 999,
	})
	s.Require().NoError(err)
	data1, err := s.encRepo.Get(s.ctx, out1.EncounterID)
	s.Require().NoError(err)

	s.seedReadyLobby("lobby-crypt5b", "bob")
	s.expectCharacter("char-bob", "bob", "Bob", 10, 10)
	out2, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "bob", LobbyID: "lobby-crypt5b", RandomSeed: 999,
	})
	s.Require().NoError(err)
	data2, err := s.encRepo.Get(s.ctx, out2.EncounterID)
	s.Require().NoError(err)

	s.Require().Equal(data1.Space, data2.Space, "the same explicit RandomSeed must reproduce the identical dungeon layout")
}

// TestStartEncounter_DungeonGeometryIndependentOfPartySize is the API-
// boundary proof: given the SAME key+seed, dungeon geometry (Space and
// connector door positions) must be IDENTICAL regardless of party size —
// InitDungeon runs before any AddPlayer call and takes no party
// information as input, so a runtime request shape difference (1 member
// vs 2) can never leak into rpg-api-side geometry derivation. Monster
// placement is deliberately NOT compared here — it legitimately depends
// on which players are seated (out-of-sight-of-every-player-view), a
// separate, pre-existing mechanism this issue doesn't touch.
func (s *LobbySuite) TestStartEncounter_DungeonGeometryIndependentOfPartySize() {
	s.seedReadyLobby("lobby-crypt6a", "alice")
	s.expectCharacter("char-alice", "alice", "Alice", 12, 12)
	out1, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "alice", LobbyID: "lobby-crypt6a", RandomSeed: 555,
	})
	s.Require().NoError(err)
	data1, err := s.encRepo.Get(s.ctx, out1.EncounterID)
	s.Require().NoError(err)

	s.seedReadyLobby("lobby-crypt6b", "carol", "dave")
	s.expectCharacter("char-carol", "carol", "Carol", 14, 14)
	s.expectCharacter("char-dave", "dave", "Dave", 11, 11)
	out2, err := s.orch.StartEncounter(s.ctx, &lobbyorch.StartEncounterInput{
		PlayerID: "carol", LobbyID: "lobby-crypt6b", RandomSeed: 555,
	})
	s.Require().NoError(err)
	data2, err := s.encRepo.Get(s.ctx, out2.EncounterID)
	s.Require().NoError(err)

	s.Require().Equal(data1.Space, data2.Space,
		"dungeon geometry must be identical regardless of party size — rpg-api never derives geometry "+
			"from runtime request shape, only the (key, seed) pair passed verbatim to the toolkit")

	doorPositions := func(d *tkenc.Data) map[core.EntityID]core.Hex {
		out := make(map[core.EntityID]core.Hex, len(d.Doors))
		for id, door := range d.Doors {
			out[id] = door.Position
		}
		return out
	}
	s.Require().Equal(doorPositions(data1), doorPositions(data2), "connector door positions are geometry too")
}
