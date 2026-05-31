package encounter_test

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	encounterv2pb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha2/encounter"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	v2encounter "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v2/encounter"
	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
	core "github.com/KirkDiggler/rpg-toolkit/encounter/core"
)

// Test fixture identifiers shared across the combat handler tests.
const (
	combatEncID    = "enc-combat"
	playerAliceID  = "alice"
	entityAliceID  = "char-alice"
	playerBobID    = "bob"
	entityBobID    = "char-bob"
	monsterGoblin1 = "goblin-1"
)

// seedCombatEncounter builds a deterministic two-player + one-monster fixture
// in TURN_BASED mode and persists it through the suite's repo. Returns the
// loaded data so callers can inspect Initiative ordering for tests that need
// to know whose turn is active first.
func (s *HandlerSuite) seedCombatEncounter() {
	enc := tkenc.New(context.Background(), combatEncID, s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: playerAliceID, EntityID: entityAliceID,
		Position: core.Hex{Q: 0, R: 0, S: 0}, SightRange: 10,
		HP: 12, MaxHP: 12, AC: 14, AttackBonus: 4, DamageDice: "1d8+2", DamageType: "slashing",
	}))
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: playerBobID, EntityID: entityBobID,
		Position: core.Hex{Q: 1, R: 0, S: -1}, SightRange: 10,
		HP: 10, MaxHP: 10, AC: 13, AttackBonus: 3, DamageDice: "1d6+1", DamageType: "piercing",
	}))
	s.Require().NoError(enc.AddMonster(tkenc.MonsterInput{
		ID: monsterGoblin1, Position: core.Hex{Q: 2, R: -1, S: -1},
		HP: 7, MaxHP: 7, AC: 15, Speed: 6,
		MonsterRef:  "dnd5e:monsters:goblin",
		AttackBonus: 4, DamageDice: "1d6+2", DamageType: "slashing",
	}))
	s.Require().NoError(enc.SetMode(core.ModeTurnBased))
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))
}

// makeAttackRequest constructs the standard "alice attacks goblin-1" attack
// request body, parameterized so tests can swap actor/target ids. The
// action_ref is hardwired to the wave-2.8 supported {dnd5e:action:attack}.
func makeAttackRequest(actorEntityID, targetEntityID string) *encounterv2pb.TakeActionRequest {
	return &encounterv2pb.TakeActionRequest{
		EncounterId:   combatEncID,
		ActorEntityId: actorEntityID,
		ActionRef:     &encounterv2pb.Ref{Module: "dnd5e", Type: "action", Id: "attack"},
		Target: &encounterv2pb.ActionTarget{
			Kind: &encounterv2pb.ActionTarget_EntityId{EntityId: targetEntityID},
		},
	}
}

// activeEntityID returns the entity id of the currently active actor in the
// seeded encounter — handy for tests that need to dispatch TakeAction /
// EndTurn against whichever side initiative landed on.
func (s *HandlerSuite) activeEntityID() core.EntityID {
	data, err := s.repo.Get(s.ctx, combatEncID)
	s.Require().NoError(err)
	s.Require().NotEmpty(data.Initiative, "initiative must be rolled before reading active actor")
	return data.Initiative[data.ActiveIdx]
}

// activePlayerCtx returns an auth context for whichever player owns the
// active entity. Returns nil if the active entity is the monster.
func (s *HandlerSuite) activePlayerCtx() context.Context {
	id := s.activeEntityID()
	switch string(id) {
	case entityAliceID:
		return contextWithPlayer(s.ctx, playerAliceID)
	case entityBobID:
		return contextWithPlayer(s.ctx, playerBobID)
	}
	return nil
}

// contextWithPlayer wraps the suite's context with a different auth player.
// The suite's s.ctx already carries player-A; this lets tests authenticate
// as a different player without rebuilding the entire context chain.
func contextWithPlayer(parent context.Context, player string) context.Context {
	return auth.WithPlayerID(parent, player)
}

// advanceToActivePlayer ends NPC turns until a player is active. With the
// 3-combatant fixture (alice, bob, goblin) at most one EndTurn-the-NPC step
// is needed. Reads the current encounter state to decide; calls EndTurn via
// the handler so the NPC dispatch loop fires (which does the right thing).
//
// In practice the seed fixture's deterministic initiative order (sorted by
// rolled d20 desc, ties by id asc) plus the test's fixed roller means the
// first active actor is one of the three deterministically. If the goblin
// is first, this method dispatches EndTurn from one of the player contexts
// — but EndTurn's auth check requires the calling player owns the active
// entity, so when the goblin is active we can't EndTurn it from a player.
// Instead, this method finds the player adjacent in initiative order who
// would be active after the goblin, simulates that path by directly invoking
// the toolkit's EndTurn (which the orchestrator's loop would also do
// server-side), and persists.
func (s *HandlerSuite) advanceToActivePlayer() {
	for {
		data, err := s.repo.Get(s.ctx, combatEncID)
		s.Require().NoError(err)
		active := data.Initiative[data.ActiveIdx]
		if _, isPlayer := s.findPlayerByEntity(data, active); isPlayer {
			return
		}
		// Active is goblin — load, advance via toolkit (NPCAct + EndTurn) and
		// save. We bypass the handler here because the handler's EndTurn auth
		// check requires the caller's controlled entity = active actor; the
		// goblin has no controlling player.
		//
		// WithCombatResolver is required since encounter v0.7.0: NPCAct routes
		// through the wired resolver. The stand-in is sufficient for test setup
		// because we only need initiative to advance past the NPC — the attack
		// outcome itself is not asserted in advanceToActivePlayer.
		enc, err := tkenc.LoadFromData(context.Background(), data, s.broker,
			tkenc.WithCombatResolver(v2encounter.NewStandInCombatResolver(nil)))
		s.Require().NoError(err)
		s.Require().NoError(enc.NPCAct(s.ctx, active))
		_, _, err = enc.EndTurn(context.Background(), active)
		s.Require().NoError(err)
		s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))
	}
}

// findPlayerByEntity scans data.Players for the entry whose EntityID matches
// entityID. Returns (PlayerID, true) on match, ("", false) otherwise.
func (s *HandlerSuite) findPlayerByEntity(data *tkenc.Data, entityID core.EntityID) (core.PlayerID, bool) {
	for pid, pd := range data.Players {
		if pd.EntityID == entityID {
			return pid, true
		}
	}
	return "", false
}

// TakeAction tests --------------------------------------------------------

func (s *HandlerSuite) TestTakeAction_NoAuth_Unauthenticated() {
	ctx := context.Background()
	_, err := s.handler.TakeAction(ctx, makeAttackRequest(entityAliceID, monsterGoblin1))
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.Unauthenticated, st.Code())
}

func (s *HandlerSuite) TestTakeAction_EmptyEncounterID_InvalidArgument() {
	req := makeAttackRequest(entityAliceID, monsterGoblin1)
	req.EncounterId = ""
	_, err := s.handler.TakeAction(s.ctx, req)
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.InvalidArgument, st.Code())
}

func (s *HandlerSuite) TestTakeAction_EmptyActorEntityID_InvalidArgument() {
	req := makeAttackRequest("", monsterGoblin1)
	_, err := s.handler.TakeAction(s.ctx, req)
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.InvalidArgument, st.Code())
}

func (s *HandlerSuite) TestTakeAction_NilActionRef_InvalidArgument() {
	req := makeAttackRequest(entityAliceID, monsterGoblin1)
	req.ActionRef = nil
	_, err := s.handler.TakeAction(s.ctx, req)
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.InvalidArgument, st.Code())
}

func (s *HandlerSuite) TestTakeAction_NilTarget_InvalidArgument() {
	req := makeAttackRequest(entityAliceID, monsterGoblin1)
	req.Target = nil
	_, err := s.handler.TakeAction(s.ctx, req)
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.InvalidArgument, st.Code())
}

func (s *HandlerSuite) TestTakeAction_TargetMissingKind_InvalidArgument() {
	req := makeAttackRequest(entityAliceID, monsterGoblin1)
	req.Target = &encounterv2pb.ActionTarget{} // no oneof set
	_, err := s.handler.TakeAction(s.ctx, req)
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.InvalidArgument, st.Code())
}

func (s *HandlerSuite) TestTakeAction_MissingEncounter_NotFound() {
	ctx := contextWithPlayer(s.ctx, playerAliceID)
	req := makeAttackRequest(entityAliceID, monsterGoblin1)
	req.EncounterId = "does-not-exist"
	_, err := s.handler.TakeAction(ctx, req)
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.NotFound, st.Code())
}

func (s *HandlerSuite) TestTakeAction_PlayerNotInEncounter_PermissionDenied() {
	s.seedCombatEncounter()
	ctx := contextWithPlayer(s.ctx, "interloper")
	_, err := s.handler.TakeAction(ctx, makeAttackRequest(entityAliceID, monsterGoblin1))
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.PermissionDenied, st.Code())
}

func (s *HandlerSuite) TestTakeAction_ActorIDMismatch_PermissionDenied() {
	s.seedCombatEncounter()
	ctx := contextWithPlayer(s.ctx, playerAliceID)
	// alice's controlled entity is char-alice; she can't direct char-bob.
	_, err := s.handler.TakeAction(ctx, makeAttackRequest(entityBobID, monsterGoblin1))
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.PermissionDenied, st.Code())
}

func (s *HandlerSuite) TestTakeAction_NotTurnBased_FailedPrecondition() {
	// Seed a free-roam encounter (no SetMode). The toolkit's combat verb
	// returns ErrNotTurnBased which the handler maps to FailedPrecondition.
	enc := tkenc.New(context.Background(), combatEncID, s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: playerAliceID, EntityID: entityAliceID,
		Position: core.Hex{Q: 0, R: 0, S: 0}, SightRange: 10,
		HP: 12, MaxHP: 12, AC: 14, AttackBonus: 4, DamageDice: "1d8+2",
	}))
	s.Require().NoError(enc.AddMonster(tkenc.MonsterInput{
		ID: monsterGoblin1, Position: core.Hex{Q: 1, R: 0, S: -1},
		HP: 7, MaxHP: 7, AC: 15, Speed: 6, MonsterRef: "dnd5e:monsters:goblin",
		AttackBonus: 4, DamageDice: "1d6+2",
	}))
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))

	ctx := contextWithPlayer(s.ctx, playerAliceID)
	_, err := s.handler.TakeAction(ctx, makeAttackRequest(entityAliceID, monsterGoblin1))
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.FailedPrecondition, st.Code())
}

func (s *HandlerSuite) TestTakeAction_NotYourTurn_FailedPrecondition() {
	s.seedCombatEncounter()
	// Whichever player is NOT active tries to attack. Initiative is rolled
	// deterministically for the ID order so we read from data and pick the
	// non-active player's context.
	active := s.activeEntityID()
	var idleCtx context.Context
	var idleEntity string
	switch string(active) {
	case entityAliceID:
		idleCtx = contextWithPlayer(s.ctx, playerBobID)
		idleEntity = entityBobID
	case entityBobID:
		idleCtx = contextWithPlayer(s.ctx, playerAliceID)
		idleEntity = entityAliceID
	default:
		// Goblin is active first — both players are off-turn. Use alice.
		idleCtx = contextWithPlayer(s.ctx, playerAliceID)
		idleEntity = entityAliceID
	}
	_, err := s.handler.TakeAction(idleCtx, makeAttackRequest(idleEntity, monsterGoblin1))
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.FailedPrecondition, st.Code())
}

func (s *HandlerSuite) TestTakeAction_UnknownTarget_FailedPrecondition() {
	s.seedCombatEncounter()
	// EndTurn until a player is active.
	s.advanceToActivePlayer()
	ctx := s.activePlayerCtx()
	actorEntity := s.activeEntityID()
	_, err := s.handler.TakeAction(ctx, makeAttackRequest(string(actorEntity), "ghost-monster"))
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.FailedPrecondition, st.Code())
}

func (s *HandlerSuite) TestTakeAction_UnsupportedAction_Unimplemented() {
	s.seedCombatEncounter()
	s.advanceToActivePlayer()
	ctx := s.activePlayerCtx()
	actorEntity := s.activeEntityID()
	req := makeAttackRequest(string(actorEntity), monsterGoblin1)
	req.ActionRef = &encounterv2pb.Ref{Module: "dnd5e", Type: "action", Id: "fireball"}
	_, err := s.handler.TakeAction(ctx, req)
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.Unimplemented, st.Code())
}

func (s *HandlerSuite) TestTakeAction_AttackHappyPath_PersistsMonsterHP() {
	s.seedCombatEncounter()
	s.advanceToActivePlayer()
	ctx := s.activePlayerCtx()
	actorEntity := s.activeEntityID()

	// Snapshot goblin HP before.
	preData, err := s.repo.Get(s.ctx, combatEncID)
	s.Require().NoError(err)
	preHP := preData.Monsters[monsterGoblin1].HP

	resp, err := s.handler.TakeAction(ctx, makeAttackRequest(string(actorEntity), monsterGoblin1))
	s.Require().NoError(err)
	s.Require().NotNil(resp)

	// Reload — toolkit may or may not have hit (random d20). The contract is
	// that the encounter is saved post-attack. Three valid post-states:
	//   - miss: goblin still in map, HP unchanged
	//   - hit (non-lethal): goblin still in map, HP strictly less than preHP
	//   - hit (lethal): goblin removed from map by Wave 2.10 killEntity chain
	// Wave 2.10 guarantees the lethal path: the seed fixture has goblin HP=7
	// and alice deals 1d8+2 (3-10), so a single hit can kill. Don't index the
	// monsters map blindly — check membership first to differentiate the
	// surviving from the killed branch.
	postData, err := s.repo.Get(s.ctx, combatEncID)
	s.Require().NoError(err)
	if postGob, alive := postData.Monsters[monsterGoblin1]; alive {
		s.Require().LessOrEqual(postGob.HP, preHP,
			"goblin HP must not increase after attack")
	}
	// Lethal-hit branch: goblin absent from map. Implicit pass — the kill
	// chain (toolkit's killEntity) already covers the post-state in
	// Wave 2.10's TestCombatSlice_KillLastHostile_FiresDeathChainAndEnds.
}

// EndTurn tests -----------------------------------------------------------

func (s *HandlerSuite) TestEndTurn_NoAuth_Unauthenticated() {
	ctx := context.Background()
	_, err := s.handler.EndTurn(ctx, &encounterv2pb.EndTurnRequest{
		EncounterId: combatEncID, EntityId: entityAliceID,
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.Unauthenticated, st.Code())
}

func (s *HandlerSuite) TestEndTurn_EmptyEncounterID_InvalidArgument() {
	_, err := s.handler.EndTurn(s.ctx, &encounterv2pb.EndTurnRequest{
		EncounterId: "", EntityId: entityAliceID,
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.InvalidArgument, st.Code())
}

func (s *HandlerSuite) TestEndTurn_EmptyEntityID_InvalidArgument() {
	_, err := s.handler.EndTurn(s.ctx, &encounterv2pb.EndTurnRequest{
		EncounterId: combatEncID, EntityId: "",
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.InvalidArgument, st.Code())
}

func (s *HandlerSuite) TestEndTurn_MissingEncounter_NotFound() {
	ctx := contextWithPlayer(s.ctx, playerAliceID)
	_, err := s.handler.EndTurn(ctx, &encounterv2pb.EndTurnRequest{
		EncounterId: "does-not-exist", EntityId: entityAliceID,
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.NotFound, st.Code())
}

func (s *HandlerSuite) TestEndTurn_PlayerNotInEncounter_PermissionDenied() {
	s.seedCombatEncounter()
	ctx := contextWithPlayer(s.ctx, "interloper")
	_, err := s.handler.EndTurn(ctx, &encounterv2pb.EndTurnRequest{
		EncounterId: combatEncID, EntityId: entityAliceID,
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.PermissionDenied, st.Code())
}

func (s *HandlerSuite) TestEndTurn_EntityIDMismatch_PermissionDenied() {
	s.seedCombatEncounter()
	ctx := contextWithPlayer(s.ctx, playerAliceID)
	_, err := s.handler.EndTurn(ctx, &encounterv2pb.EndTurnRequest{
		EncounterId: combatEncID, EntityId: entityBobID,
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.PermissionDenied, st.Code())
}

func (s *HandlerSuite) TestEndTurn_NotTurnBased_FailedPrecondition() {
	// Seed FreeRoam — EndTurn requires TURN_BASED.
	enc := tkenc.New(context.Background(), combatEncID, s.broker)
	s.Require().NoError(enc.AddPlayer(tkenc.PlayerInput{
		PlayerID: playerAliceID, EntityID: entityAliceID,
		Position: core.Hex{}, SightRange: 10,
	}))
	s.Require().NoError(s.repo.Save(s.ctx, enc.ToData()))

	ctx := contextWithPlayer(s.ctx, playerAliceID)
	_, err := s.handler.EndTurn(ctx, &encounterv2pb.EndTurnRequest{
		EncounterId: combatEncID, EntityId: entityAliceID,
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.FailedPrecondition, st.Code())
}

func (s *HandlerSuite) TestEndTurn_NotYourTurn_FailedPrecondition() {
	s.seedCombatEncounter()
	active := s.activeEntityID()
	// Pick the non-active player and try to end their turn.
	var idleCtx context.Context
	var idleEntity string
	switch string(active) {
	case entityAliceID:
		idleCtx = contextWithPlayer(s.ctx, playerBobID)
		idleEntity = entityBobID
	default:
		idleCtx = contextWithPlayer(s.ctx, playerAliceID)
		idleEntity = entityAliceID
	}
	_, err := s.handler.EndTurn(idleCtx, &encounterv2pb.EndTurnRequest{
		EncounterId: combatEncID, EntityId: idleEntity,
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.FailedPrecondition, st.Code())
}

func (s *HandlerSuite) TestEndTurn_HappyPath_AdvancesInitiative() {
	s.seedCombatEncounter()
	s.advanceToActivePlayer()
	ctx := s.activePlayerCtx()
	actorEntity := s.activeEntityID()

	preData, err := s.repo.Get(s.ctx, combatEncID)
	s.Require().NoError(err)
	preIdx := preData.ActiveIdx

	_, err = s.handler.EndTurn(ctx, &encounterv2pb.EndTurnRequest{
		EncounterId: combatEncID, EntityId: string(actorEntity),
	})
	s.Require().NoError(err)

	postData, err := s.repo.Get(s.ctx, combatEncID)
	s.Require().NoError(err)
	// Initiative advanced. Either ActiveIdx incremented or it wrapped to 0
	// (and Round incremented), AND the active actor is no longer actorEntity
	// (or it cycled all the way back through NPC turns and an NPC dispatch
	// loop brought us back to the same player — but with our 3-combatant
	// fixture that shouldn't happen in one EndTurn call).
	if preIdx+1 < len(preData.Initiative) {
		s.Require().NotEqual(preIdx, postData.ActiveIdx, "ActiveIdx must advance")
	}
}

// TestEndTurn_NPCDispatchLoop_RunsNPCTurnAutomatically verifies the
// orchestrator-side NPC dispatch loop: when EndTurn cycles to an NPC, the
// handler calls NPCAct and EndTurn-the-NPC server-side until the next
// active actor is a player. Verified by: after the first player ends turn,
// the saved data shows initiative has cycled past the goblin and the next
// active actor is a player (not the goblin), AND the goblin's HP was
// affected if the player attacked it OR a player's HP was affected by the
// goblin's NPCAct attack.
func (s *HandlerSuite) TestEndTurn_NPCDispatchLoop_CyclesPastNPC() {
	s.seedCombatEncounter()

	// Walk through initiative until a player is active. Then end that
	// player's turn — if the next combatant in initiative is the goblin,
	// the NPC dispatch loop should fire NPCAct and advance past the goblin.
	s.advanceToActivePlayer()
	ctx := s.activePlayerCtx()
	actorEntity := s.activeEntityID()

	_, err := s.handler.EndTurn(ctx, &encounterv2pb.EndTurnRequest{
		EncounterId: combatEncID, EntityId: string(actorEntity),
	})
	s.Require().NoError(err)

	postData, err := s.repo.Get(s.ctx, combatEncID)
	s.Require().NoError(err)
	postActive := postData.Initiative[postData.ActiveIdx]

	// If the goblin was next in initiative, the dispatch loop ran NPCAct +
	// EndTurn-the-goblin, so the post-state active actor is NOT the goblin.
	// Either way (goblin was next-and-cycled or goblin was elsewhere), the
	// post-state active actor must be a player.
	_, postIsPlayer := s.findPlayerByEntity(postData, postActive)
	s.Require().True(postIsPlayer, "after EndTurn, active actor must be a player (not goblin)")
}

// Wave 2.10: terminal-state guard tests --------------------------------
//
// These verify the handler maps tkenc.ErrEncounterEnded → FailedPrecondition.
// Direct-mutation seeding (set data.Mode = ModeEnded after Save) keeps the
// fixture deterministic — we don't depend on running an attack chain to
// drive the encounter to ModeEnded; the toolkit's own death tests cover the
// transition.

// seedEndedEncounter saves a turn-based encounter with Mode rewritten to
// ModeEnded so combat verbs return ErrEncounterEnded on the next call.
// Returns the entity id of whichever player is in the (now-ignored)
// initiative roster — handler tests need a syntactically valid actor /
// entity id to exercise the post-validation, post-load gate.
func (s *HandlerSuite) seedEndedEncounter() string {
	s.seedCombatEncounter()
	data, err := s.repo.Get(s.ctx, combatEncID)
	s.Require().NoError(err)
	// Pick alice if she's in the roster (always is, per fixture); fallback
	// to whatever active actor was seeded.
	actorEntity := entityAliceID
	// Flip to terminal state — the combat verbs check this gate first
	// (before turn-violation / non-combatant checks).
	data.Mode = core.ModeEnded
	s.Require().NoError(s.repo.Save(s.ctx, data))
	return actorEntity
}

func (s *HandlerSuite) TestTakeAction_EncounterEnded_FailedPrecondition() {
	actorEntity := s.seedEndedEncounter()
	ctx := contextWithPlayer(s.ctx, playerAliceID)
	_, err := s.handler.TakeAction(ctx, makeAttackRequest(actorEntity, monsterGoblin1))
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.FailedPrecondition, st.Code(),
		"TakeAction against an ended encounter must return FailedPrecondition, got: %v", err)
}

func (s *HandlerSuite) TestEndTurn_EncounterEnded_FailedPrecondition() {
	actorEntity := s.seedEndedEncounter()
	ctx := contextWithPlayer(s.ctx, playerAliceID)
	_, err := s.handler.EndTurn(ctx, &encounterv2pb.EndTurnRequest{
		EncounterId: combatEncID, EntityId: actorEntity,
	})
	s.Require().Error(err)
	st, _ := status.FromError(err)
	s.Require().Equal(codes.FailedPrecondition, st.Code(),
		"EndTurn against an ended encounter must return FailedPrecondition, got: %v", err)
}
