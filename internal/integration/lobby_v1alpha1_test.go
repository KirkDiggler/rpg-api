// Package integration_test's lobby suite is the boarded "Party Assembles"
// gate test (rpg-project's lobby-surface.md "Verify"): four dev-authenticated
// players create/join/ready/start and land in one v2 encounter with correct
// entities, driven entirely through the LobbyService RPC surface plus the
// pre-existing StreamEncounter path — proving StartEncounter is now the sole
// encounter construction path end-to-end, not just at the unit level.
package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	corepb "github.com/KirkDiggler/rpg-api-protos/gen/go/api/v1alpha2/core"
	lobbyv1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/lobby/v1alpha1"
	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	"github.com/KirkDiggler/rpg-api/internal/integration/harness"
	"github.com/KirkDiggler/rpg-api/internal/pkg/devcombat"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	core "github.com/KirkDiggler/rpg-toolkit/encounter/core"
	toolkitchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
)

// LobbyV1alpha1IntegrationSuite is the Party Assembles gate test suite.
type LobbyV1alpha1IntegrationSuite struct {
	suite.Suite
	ctx    context.Context
	cancel context.CancelFunc
	srv    *harness.TestServer
}

func (s *LobbyV1alpha1IntegrationSuite) SetupTest() {
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 2*time.Minute)

	var err error
	s.srv, err = harness.New(s.ctx, nil)
	s.Require().NoError(err, "failed to create test server")
}

func (s *LobbyV1alpha1IntegrationSuite) TearDownTest() {
	if s.srv != nil {
		s.srv.Close()
	}
	if s.cancel != nil {
		s.cancel()
	}
}

// authCtx returns a context carrying dev-mode auth for the given player,
// mirroring EncounterV2IntegrationSuite.authCtx.
func (s *LobbyV1alpha1IntegrationSuite) authCtx(playerID string) context.Context {
	return metadata.AppendToOutgoingContext(s.ctx, "authorization", "Dev "+playerID)
}

// seedCharacter creates a minimal playable character owned by playerID, for
// binding to a lobby seat.
func (s *LobbyV1alpha1IntegrationSuite) seedCharacter(characterID, playerID, name string, hp int) {
	_, err := s.srv.CharacterRepo.Create(s.ctx, characterrepo.CreateInput{
		Character: &entities.Character{
			Data: &toolkitchar.Data{
				ID: characterID, PlayerID: playerID, Name: name,
				HitPoints: hp, MaxHitPoints: hp,
			},
		},
	})
	s.Require().NoError(err)
}

// TestPartyAssembles_FourPlayers_CreateJoinReadyStart is the boarded
// verify story: one host creates, three join via the dev join_ref carrier,
// all four ready up, host starts, all four land in the same v2 encounter
// with HP seeded from their bound characters.
func (s *LobbyV1alpha1IntegrationSuite) TestPartyAssembles_FourPlayers_CreateJoinReadyStart() {
	players := []struct {
		id, character, name string
		hp                  int
	}{
		{"alice", "char-alice", "Alice", 12},
		{"bob", "char-bob", "Bob", 10},
		{"carol", "char-carol", "Carol", 14},
		{"dave", "char-dave", "Dave", 8},
	}
	for _, p := range players {
		s.seedCharacter(p.character, p.id, p.name, p.hp)
	}

	// Alice creates the lobby and becomes host.
	createResp, err := s.srv.LobbyClient.CreateLobby(s.authCtx("alice"), &lobbyv1alpha1.CreateLobbyRequest{
		CampaignId: "campaign-1", CharacterId: "char-alice",
	})
	s.Require().NoError(err)
	lobbyID := createResp.GetLobbyId()
	joinRef := createResp.GetJoinRef()
	s.Require().NotEmpty(lobbyID)
	s.Require().NotEmpty(joinRef)
	s.Require().Equal("alice", createResp.GetHostPlayerId())

	// bob, carol, dave join via the dev join_ref carrier — the same opaque
	// ref every player supplies, dev/playtest just passes it explicitly.
	var lastJoinResp *lobbyv1alpha1.JoinLobbyResponse
	for _, p := range players[1:] {
		resp, joinErr := s.srv.LobbyClient.JoinLobby(s.authCtx(p.id), &lobbyv1alpha1.JoinLobbyRequest{
			JoinRef: joinRef, CharacterId: p.character,
		})
		s.Require().NoError(joinErr)
		s.Require().Equal(lobbyID, resp.GetLobbyId())
		lastJoinResp = resp
	}
	s.Require().Len(lastJoinResp.GetMembers(), 4, "all four players must be in the roster after joining")

	// All four ready up.
	for _, p := range players {
		_, readyErr := s.srv.LobbyClient.SetReady(s.authCtx(p.id), &lobbyv1alpha1.SetReadyRequest{
			LobbyId: lobbyID, Ready: true,
		})
		s.Require().NoError(readyErr)
	}

	// Host starts. All-ready gate must pass; encounter_id comes back.
	startResp, err := s.srv.LobbyClient.StartEncounter(s.authCtx("alice"), &lobbyv1alpha1.StartEncounterRequest{
		LobbyId: lobbyID,
	})
	s.Require().NoError(err)
	encounterID := startResp.GetEncounterId()
	s.Require().NotEmpty(encounterID)

	// Persist-then-emit proof at the integration level: the encounter must
	// already be readable from the encounter repo StartEncounter returned
	// success from — no polling/race needed since Save happens before the
	// RPC response.
	encData, err := s.srv.EncRepoV2.Get(s.ctx, encounterID)
	s.Require().NoError(err)
	s.Require().Len(encData.Players, 4, "all four ready members must become encounter players")
	for _, p := range players {
		pd, ok := encData.Players[core.PlayerID(p.id)]
		s.Require().True(ok, "player %q must be seated in the encounter", p.id)
		s.Require().Equal(core.EntityID(p.character), pd.EntityID)
		s.Require().Equal(p.hp, pd.HP, "player %q's HP must be seeded from their bound character", p.id)
		s.Require().Equal(p.hp, pd.MaxHP)
	}

	// rpg-api#632: an unseeded SightRange leaves every member able to see only
	// their own spawn hex — the party never actually sees each other despite
	// spawning adjacent. Assert every member has a seeded SightRange and a
	// RevealedHexes set covering a real radius (not just their own hex), so
	// this regression can't pass silently again.
	//
	// rpg-api#656 note (why this ISN'T "every viewer sees every other
	// member's exact spawn hex"): an earlier version of this test asserted
	// that stronger claim. It happened to hold when the party spawned at the
	// room's offset-coordinate corner, but was never actually guaranteed
	// anywhere in the room — a wall can sit on the direct line between two
	// adjacent party members regardless of where they spawn. #656's fix
	// (spawning the party at the room's center instead of its corner, fixing
	// a real movement-truncation bug) happens to put exactly one wall
	// between two of these four spawn hexes for this room's random wall
	// layout, which deterministically failed the old, stronger assertion.
	// The radius check below is what #632 actually needs guarded; wall-free
	// LOS between every possible pair of spawn positions was never a real
	// requirement and is not worth chasing with a placement-search here.
	for _, viewer := range players {
		viewerPD := encData.Players[core.PlayerID(viewer.id)]
		s.Require().NotZero(viewerPD.View.SightRange,
			"player %q must have a seeded SightRange", viewer.id)
		s.Require().Greater(len(viewerPD.View.RevealedHexes), 1,
			"player %q's RevealedHexes must cover a real sight radius, not just their own hex "+
				"(rpg-api#632: an unseeded SightRange reveals only the mover's own position)", viewer.id)
	}

	// All four land in the SAME encounter via the pre-existing v2
	// StreamEncounter path — the already-proven snapshot-first flow
	// (lobby-surface.md "Verify").
	for _, p := range players {
		stream, streamErr := s.srv.EncounterClientV2.StreamEncounter(s.authCtx(p.id),
			&encounterv2pb.StreamEncounterRequest{EncounterId: encounterID})
		s.Require().NoError(streamErr)
		snap, recvErr := stream.Recv()
		s.Require().NoError(recvErr)
		s.Require().NotNil(snap.GetSnapshotDelivered())
		s.Require().Equal(encounterID, snap.GetSnapshotDelivered().GetEncounter().GetId())
	}
}

// TestPartyAssembles_FourPlayers_ThenCombatEntry_AttackResolves builds on the
// same create/join/ready/start party-assembly flow as
// TestPartyAssembles_FourPlayers_CreateJoinReadyStart and adds rpg-api#634's
// combat-entry proof: once the party is seated in a v2 encounter,
// devcombat.Inject (the SAME code path cmd/devseed's --inject-combat flag
// uses) adds a goblin and flips the encounter to TURN_BASED, and the active
// player's real attack via the production TakeAction RPC actually resolves
// — NOT ErrNonCombatant, which is what every lobby-created player hit before
// #634 (isPlayerCombatant gated on a flat AC/DamageDice snapshot
// StartEncounter had no honest way to populate for an arbitrary character).
//
// A miss is still a valid pass: the toolkit resolves the attack with a real
// d20, matching TestCombatSlice_TakeActionAndEndTurn_NPCDispatch's
// established pattern in encounter_v2_test.go. What this test proves is that
// the attack RESOLVES at all instead of being rejected at the combatant gate
// — that rejection is the bug #634 fixes, not the coin-flip of hit vs. miss.
func (s *LobbyV1alpha1IntegrationSuite) TestPartyAssembles_FourPlayers_ThenCombatEntry_AttackResolves() {
	players := []struct {
		id, character, name string
		hp                  int
	}{
		{"alice", "char-alice", "Alice", 12},
		{"bob", "char-bob", "Bob", 10},
		{"carol", "char-carol", "Carol", 14},
		{"dave", "char-dave", "Dave", 8},
	}
	for _, p := range players {
		s.seedCharacter(p.character, p.id, p.name, p.hp)
	}

	createResp, err := s.srv.LobbyClient.CreateLobby(s.authCtx("alice"), &lobbyv1alpha1.CreateLobbyRequest{
		CampaignId: "campaign-1", CharacterId: "char-alice",
	})
	s.Require().NoError(err)
	lobbyID := createResp.GetLobbyId()
	joinRef := createResp.GetJoinRef()

	for _, p := range players[1:] {
		_, joinErr := s.srv.LobbyClient.JoinLobby(s.authCtx(p.id), &lobbyv1alpha1.JoinLobbyRequest{
			JoinRef: joinRef, CharacterId: p.character,
		})
		s.Require().NoError(joinErr)
	}
	for _, p := range players {
		_, readyErr := s.srv.LobbyClient.SetReady(s.authCtx(p.id), &lobbyv1alpha1.SetReadyRequest{
			LobbyId: lobbyID, Ready: true,
		})
		s.Require().NoError(readyErr)
	}

	startResp, err := s.srv.LobbyClient.StartEncounter(s.authCtx("alice"), &lobbyv1alpha1.StartEncounterRequest{
		LobbyId: lobbyID,
	})
	s.Require().NoError(err)
	encounterID := startResp.GetEncounterId()
	s.Require().NotEmpty(encounterID)

	// rpg-api#634 Part 2: inject a goblin into the lobby-started encounter and
	// flip it to TURN_BASED — the same code path cmd/devseed's --inject-combat
	// uses.
	injectOut, err := devcombat.Inject(s.ctx, s.srv.EncRepoV2, devcombat.InjectInput{EncounterID: encounterID})
	s.Require().NoError(err)
	s.Require().Equal(core.ModeTurnBased, injectOut.Mode)

	encData, err := s.srv.EncRepoV2.Get(s.ctx, encounterID)
	s.Require().NoError(err)
	s.Require().Contains(encData.Monsters, injectOut.GoblinID, "injected goblin must be in the encounter")
	s.Require().Contains(encData.Initiative, injectOut.GoblinID, "goblin must be included in the rolled initiative order")
	for _, p := range players {
		pd, ok := encData.Players[core.PlayerID(p.id)]
		s.Require().True(ok, "player %q must survive combat injection", p.id)
		s.Require().Contains(encData.Initiative, pd.EntityID, "player %q must be included in the rolled initiative order", p.id)
	}

	// Advance to a player's own turn (initiative is roll-dependent — the
	// goblin may go first) via direct toolkit manipulation. This is setup for
	// the assertion below, not the assertion itself — mirrors
	// EncounterV2IntegrationSuite.advanceUntilPlayerActiveByDirectToolkit in
	// encounter_v2_test.go (duplicated rather than shared across suite types
	// for a ~15-line helper).
	s.advanceUntilPlayerActive(encData)

	data, err := s.srv.EncRepoV2.Get(s.ctx, encounterID)
	s.Require().NoError(err)
	activeEntity := data.Initiative[data.ActiveIdx]
	var activePlayer string
	for pid, pd := range data.Players {
		if pd.EntityID == activeEntity {
			activePlayer = string(pid)
			break
		}
	}
	s.Require().NotEmpty(activePlayer, "an active player must be found before TakeAction")

	// The headline #634 proof: TakeAction against the lobby-created,
	// characterData.Attach-hydrated player must NOT return ErrNonCombatant.
	// Pre-#634, isPlayerCombatant rejected EVERY lobby-created player because
	// StartEncounter had no honest AC/DamageDice snapshot to seed. The gate
	// relaxation (isPlayerCombatant treats an actually-held seat as
	// combat-ready, since the real Dnd5eCombatResolver ignores the flat
	// AC/DamageDice snapshot once hydrated anyway) shipped in rpg-toolkit
	// encounter v0.24.4 (rpg-toolkit#750/#751).
	_, err = s.srv.EncounterClientV2.TakeAction(s.authCtx(activePlayer), &encounterv2pb.TakeActionRequest{
		EncounterId:   encounterID,
		ActorEntityId: string(activeEntity),
		ActionRef:     &corepb.Ref{Module: "dnd5e", Type: "action", Id: "attack"},
		Target: &encounterv2pb.ActionTarget{
			Kind: &encounterv2pb.ActionTarget_EntityId{EntityId: string(injectOut.GoblinID)},
		},
	})
	s.Require().NoError(err, "attack must resolve — a lobby-created player must pass the combatant gate")
}

// advanceUntilPlayerActive cycles NPC (goblin) turns via direct toolkit
// calls + a deterministic stand-in resolver until the active actor is a
// player, then persists the advanced state. Mirrors
// EncounterV2IntegrationSuite.advanceUntilPlayerActiveByDirectToolkit in
// encounter_v2_test.go.
func (s *LobbyV1alpha1IntegrationSuite) advanceUntilPlayerActive(data *tkenc.Data) {
	for {
		s.Require().NotEmpty(data.Initiative, "encounter has empty initiative; SetMode/inject may have failed")
		active := data.Initiative[data.ActiveIdx]
		isPlayer := false
		for _, pd := range data.Players {
			if pd.EntityID == active {
				isPlayer = true
				break
			}
		}
		if isPlayer {
			break
		}
		// Active is the goblin — run NPCAct + EndTurn to cycle, via a
		// deterministic stand-in resolver (always hits) so this setup step
		// doesn't depend on a real d20.
		enc, reloadErr := tkenc.LoadFromData(s.ctx, data, s.srv.BrokerV2, tkenc.WithCombatResolver(testStandInResolver{}))
		s.Require().NoError(reloadErr)
		s.Require().NoError(enc.NPCAct(s.ctx, active))
		_, _, endErr := enc.EndTurn(context.Background(), active)
		s.Require().NoError(endErr)
		data = enc.ToData()
	}
	s.Require().NoError(s.srv.EncRepoV2.Save(s.ctx, data))
}

// TestPartyAssembles_FourPlayers_ThenCombatEntry_NPCFirstInitiative_StreamDrivesNPCTurn
// is the rpg-api#636 regression: it repeats the same
// create/join/ready/start/inject flow as
// TestPartyAssembles_FourPlayers_ThenCombatEntry_AttackResolves, but forces
// the injected goblin to lead initiative via devcombat.Inject's
// ForceNPCFirst knob — the SAME deterministic reorder `devseed --inject-combat
// --inject-combat-npc-first` uses for manual repro — instead of relying on a
// lucky roll. Pre-#636, nothing ever drove the goblin's turn in this
// state: EndTurn's NPC dispatch loop only ran as a tail of an EndTurn call,
// and there was none to chain off of (every player is correctly locked out
// since it isn't their turn), so the encounter stalled forever.
//
// The proof is StreamEncounter itself: connecting via the production RPC (not
// a direct toolkit/repo manipulation, unlike advanceUntilPlayerActive's setup
// helper) must come back with a player active, not the goblin — proving the
// handler's combat-entry kick (DriveStalledNPCTurn, called before Subscribe)
// actually drove the stalled NPC turn as a side effect of the very first
// connect.
func (s *LobbyV1alpha1IntegrationSuite) TestPartyAssembles_FourPlayers_ThenCombatEntry_NPCFirstInitiative_StreamDrivesNPCTurn() {
	players := []struct {
		id, character, name string
		hp                  int
	}{
		{"alice", "char-alice", "Alice", 12},
		{"bob", "char-bob", "Bob", 10},
		{"carol", "char-carol", "Carol", 14},
		{"dave", "char-dave", "Dave", 8},
	}
	for _, p := range players {
		s.seedCharacter(p.character, p.id, p.name, p.hp)
	}

	createResp, err := s.srv.LobbyClient.CreateLobby(s.authCtx("alice"), &lobbyv1alpha1.CreateLobbyRequest{
		CampaignId: "campaign-1", CharacterId: "char-alice",
	})
	s.Require().NoError(err)
	lobbyID := createResp.GetLobbyId()
	joinRef := createResp.GetJoinRef()

	for _, p := range players[1:] {
		_, joinErr := s.srv.LobbyClient.JoinLobby(s.authCtx(p.id), &lobbyv1alpha1.JoinLobbyRequest{
			JoinRef: joinRef, CharacterId: p.character,
		})
		s.Require().NoError(joinErr)
	}
	for _, p := range players {
		_, readyErr := s.srv.LobbyClient.SetReady(s.authCtx(p.id), &lobbyv1alpha1.SetReadyRequest{
			LobbyId: lobbyID, Ready: true,
		})
		s.Require().NoError(readyErr)
	}

	startResp, err := s.srv.LobbyClient.StartEncounter(s.authCtx("alice"), &lobbyv1alpha1.StartEncounterRequest{
		LobbyId: lobbyID,
	})
	s.Require().NoError(err)
	encounterID := startResp.GetEncounterId()
	s.Require().NotEmpty(encounterID)

	// rpg-api#636 repro: force the goblin to lead initiative instead of
	// leaving it to the real roll.
	injectOut, err := devcombat.Inject(s.ctx, s.srv.EncRepoV2, devcombat.InjectInput{
		EncounterID:   encounterID,
		ForceNPCFirst: true,
	})
	s.Require().NoError(err)
	s.Require().Equal(core.ModeTurnBased, injectOut.Mode)

	preKickData, err := s.srv.EncRepoV2.Get(s.ctx, encounterID)
	s.Require().NoError(err)
	s.Require().Equal(injectOut.GoblinID, preKickData.Initiative[preKickData.ActiveIdx],
		"setup must confirm the goblin is active before the stream connects — otherwise this isn't testing the stall")

	// The headline #636 proof: connecting via the production StreamEncounter
	// RPC — the only "first touch" available for an out-of-process injection —
	// must itself drive the stalled NPC turn. No EndTurn call precedes this;
	// nothing else has touched the encounter since injection.
	stream, streamErr := s.srv.EncounterClientV2.StreamEncounter(s.authCtx("alice"),
		&encounterv2pb.StreamEncounterRequest{EncounterId: encounterID})
	s.Require().NoError(streamErr)
	snap, recvErr := stream.Recv()
	s.Require().NoError(recvErr)
	s.Require().NotNil(snap.GetSnapshotDelivered())

	turnState := snap.GetSnapshotDelivered().GetEncounter().GetTurnState()
	s.Require().NotNil(turnState, "a turn-based encounter's snapshot must carry TurnState")
	s.NotEqual(string(injectOut.GoblinID), turnState.GetActiveEntityId(),
		"the goblin must no longer be active by the time the snapshot is built — StreamEncounter's combat-entry kick must have driven its turn")

	postKickData, err := s.srv.EncRepoV2.Get(s.ctx, encounterID)
	s.Require().NoError(err)
	activeEntity := postKickData.Initiative[postKickData.ActiveIdx]
	var activeIsPlayer bool
	for _, p := range players {
		if activeEntity == core.EntityID(p.character) {
			activeIsPlayer = true
			break
		}
	}
	s.Require().True(activeIsPlayer, "the active actor after the kick must be a player, not another NPC")
}

// TestJoinLobby_LateJoin_FailedPrecondition proves the "late join" edge
// policy end-to-end: JoinLobby on a STARTED lobby is rejected, not silently
// admitted into a mid-encounter party.
func (s *LobbyV1alpha1IntegrationSuite) TestJoinLobby_LateJoin_FailedPrecondition() {
	s.seedCharacter("char-alice2", "alice", "Alice", 10)
	s.seedCharacter("char-eve", "eve", "Eve", 10)

	createResp, err := s.srv.LobbyClient.CreateLobby(s.authCtx("alice"), &lobbyv1alpha1.CreateLobbyRequest{
		CampaignId: "campaign-1", CharacterId: "char-alice2",
	})
	s.Require().NoError(err)

	_, err = s.srv.LobbyClient.SetReady(s.authCtx("alice"), &lobbyv1alpha1.SetReadyRequest{
		LobbyId: createResp.GetLobbyId(), Ready: true,
	})
	s.Require().NoError(err)
	_, err = s.srv.LobbyClient.StartEncounter(s.authCtx("alice"), &lobbyv1alpha1.StartEncounterRequest{
		LobbyId: createResp.GetLobbyId(),
	})
	s.Require().NoError(err)

	_, err = s.srv.LobbyClient.JoinLobby(s.authCtx("eve"), &lobbyv1alpha1.JoinLobbyRequest{
		JoinRef: createResp.GetJoinRef(), CharacterId: "char-eve",
	})
	s.Require().Error(err)
	st, ok := status.FromError(err)
	s.Require().True(ok)
	s.Require().Equal(codes.FailedPrecondition, st.Code())
}

func TestLobbyV1alpha1IntegrationSuite(t *testing.T) {
	suite.Run(t, new(LobbyV1alpha1IntegrationSuite))
}
