// Package session_test proves the session-service acceptance loop (design
// rpg-project/ideas/session-api/design.md §6.1) through the real, wired
// stack: a real session.Manager over a real (miniredis-backed) Redis
// SessionRepository/EncounterRepository, a real CharacterRepository adapter
// over a real character store, and the real sessionv1alpha1.Handler calling
// into it -- proto request in, proto response out, exactly the production
// path minus the network hop.
package session_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/abilities"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/classes"
	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/proficiencies"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/races"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/shared"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/weapons"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	sessionhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

// requireGRPCCode asserts the gRPC status code of a handler error.
func requireGRPCCode(t *testing.T, err error, want codes.Code) {
	t.Helper()
	require.Error(t, err)
	st, ok := status.FromError(err)
	require.True(t, ok, "expected a gRPC status error, got %T", err)
	require.Equal(t, want, st.Code(), "unexpected code for: %v", err)
}

// orderAsGiven is the trivial InitiativeRoller every fixture in this test
// needs (encounter.SetupInput requires one): the members already arrive in
// the order the composition detected them, so there is nothing to decide.
type orderAsGiven struct{}

func (orderAsGiven) RollInitiative(members []tkencounter.MemberID) ([]tkencounter.MemberID, error) {
	return members, nil
}

// allStanding and allSeeing are the Standing and Sight capabilities
// encounter.SetupInput has required since rpg-toolkit#1033: supplied, never
// defaulted, and refused at construction (ErrNoStanding / ErrNoSight) rather
// than given a default, because "everybody can see" is a decision about a world
// and not a fact the composition may assume.
//
// Trivial on purpose. These are used only to VALIDATE construction here; they
// are not persisted in the EncounterData this function returns, and the session
// package supplies its own when it loads the world (including the sight RANGE
// that decides who is in contact). A fixture that made these clever would be
// testing the fixture.
type allStanding struct{}

// Standing returns who is DOWN, not who is up -- the interface's own parameter
// name is `down`, and reading it the other way round would report a healthy
// party as a wiped one.
func (allStanding) Standing(_ []tkencounter.MemberID) ([]tkencounter.MemberID, error) {
	return nil, nil
}

type allSeeing struct{}

func (allSeeing) Sight(members []tkencounter.MemberID) (map[tkencounter.MemberID]int, error) {
	out := make(map[tkencounter.MemberID]int, len(members))
	for _, id := range members {
		out[id] = 1_000_000
	}

	return out, nil
}

// seamWall is the wall between two side-by-side chambers, with one row left
// open for the doorway. Room-local to the WEST chamber, where column width-1 is
// its last and column width is the east chamber's first.
//
// THIS IS THE MIGRATION HAZARD OF rpg-toolkit#1130, and it is the reason this
// fixture grew rather than merely being renamed. When every chamber owned its
// own grid, nothing could cross between them except through a declared
// doorway -- the walls were implied by the room structure. On ONE CANVAS two
// chambers side by side are simply ONE OPEN SPACE, and this list is the whole
// answer to what a member cannot walk through or see past. Without it the tomb
// is not three rooms joined by two doors; it is one 22-wide hall with some
// furniture in it.
func seamWall(width, rows, openRow int) []spatial.Boundary {
	out := make([]spatial.Boundary, 0, rows)
	for row := 0; row < rows; row++ {
		if row == openRow {
			continue // the doorway itself
		}
		out = append(out, spatial.Boundary{
			From:              spatial.Position{X: float64(width - 1), Y: float64(row)},
			To:                spatial.Position{X: float64(width), Y: float64(row)},
			BlocksMovement:    true,
			BlocksLineOfSight: true,
		})
	}

	return out
}

// testDice rolls the crypto roller would: this suite is not testing whether
// a d20 lands, only that the seam carries a swing through to damage, so a
// fixed non-zero source is enough -- Attack requires a Dice source at
// construction regardless (toolkit law: capabilities are supplied, never
// defaulted).
type testDice struct{}

func (testDice) Roll(_ context.Context, size int) (int, error) { return size, nil } // always the max face

// buildThreeRoomTomb is entrance -> hall -> tomb, matching design §6.1's
// acceptance shape: a party walks two doorway crossings and meets a monster
// on the far side. World AUTHORING still speaks rooms (RoomInput/
// ConnectionInput are rulebooks/dnd5e/encounter's own vocabulary, unaffected
// by the session SDK's one-map convergence -- "authoring is construction
// data, and the one-map rule governs what a session SEES while it plays,"
// per Atlas's own doc); only the SESSION-FACING verbs this test drives
// against (Join, Move, Attack, ...) speak absolute positions with no room
// concept at all. The tomb room's occluder wall (a solid column at local
// x=3 with a gap at y=3) and the skeleton's position mirror the toolkit's
// own proven "sight forms a fight" fixture (rulebooks/dnd5e/session's
// fight_starts_test.go buildAmbush) -- the same geometry that is already
// pinned to work, transplanted into a room reached by crossing two doorways.
func buildThreeRoomTomb(t *testing.T) *tkencounter.EncounterData {
	t.Helper()

	// The tomb's column: a solid line of pillars at local x=3 with a gap at
	// y=3, so a sightline reaches the skeleton only along that one row.
	//
	// This was `Occluders []spatial.Position` until rpg-toolkit#1130. Props
	// answer both blocking questions independently, and these say what the old
	// field could only imply: they stop SIGHT and not MOVEMENT. That was always
	// what the word occluder meant here -- this scene is about who can see whom,
	// never about who can walk where -- and a fixture that blocked movement too
	// would change what the test asks while appearing only to rename a field.
	blocksSight, passable := true, false
	pillars := make([]tkencounter.PropInput, 0, 7)
	for y := 0; y < 8; y++ {
		if y == 3 {
			continue // the gap
		}
		pillars = append(pillars, tkencounter.PropInput{
			Ref:               "pillar",
			At:                spatial.Position{X: 3, Y: float64(y)},
			BlocksMovement:    &passable,
			BlocksLineOfSight: &blocksSight,
		})
	}

	enc, err := tkencounter.NewEncounter(&tkencounter.SetupInput{
		Initiative: orderAsGiven{},
		Retention:  tkencounter.RetentionUnbounded,
		Standing:   allStanding{},
		Sight:      allSeeing{},
		// tkencounter.PassDriver{}/RefusingStriker{} are the toolkit's own
		// trivial, exported stand-ins (encounter v0.30.6, rpg-toolkit#1167
		// closed) -- construction-time only, same as allStanding/allSeeing
		// above: this fixture builds the seed EncounterData via NewEncounter
		// and the real, played session supplies its own TurnDriver
		// (sdk.Behavior() today) and Striker when it loads and plays this
		// world, so what a monster's turn does here never matters.
		TurnDriver: tkencounter.PassDriver{},
		Striker:    tkencounter.RefusingStriker{},
		Field: tkencounter.FieldInput{
			Canvas: tkencounter.CanvasInput{
				// A tomb is cut from stone: you cannot see across the space
				// between the chambers, or walk it. Required rather than
				// defaulted (rpg-toolkit#1116) -- there is no correct default,
				// since an open-air ruin's or a ship's deck answer is the
				// opposite, and a default would be the composition deciding
				// what a dungeon is made of in a field the author never wrote.
				//
				// No Orientation: required for a hex field and REFUSED for a
				// square one, which this is.
				Void: tkencounter.VoidIsOpaque(),
			},
			Rooms: []tkencounter.RoomInput{
				// Each chamber draws its own east wall, leaving the doorway row
				// open. See seamWall: on one canvas these three rectangles are
				// contiguous, so without these the party could walk from the
				// entrance into the tomb at any row, and see all the way down.
				{
					ID: "entrance", Width: 6, Height: 6, Origin: spatial.Position{X: 0, Y: 0},
					Boundaries: seamWall(6, 6, 1), // the entrance-hall door is on row 1
				},
				{
					ID: "hall", Width: 8, Height: 6, Origin: spatial.Position{X: 6, Y: 0},
					Boundaries: seamWall(8, 6, 3), // the hall-tomb door is on row 3
				},
				{ID: "tomb", Width: 8, Height: 8, Origin: spatial.Position{X: 14, Y: 0}, Props: pillars},
			},
			Connections: []tkencounter.ConnectionInput{
				{
					ID: "entrance-hall", From: "entrance", To: "hall",
					FromPosition: spatial.Position{X: 5, Y: 1}, ToPosition: spatial.Position{X: 0, Y: 1},
				},
				{
					ID: "hall-tomb", From: "hall", To: "tomb",
					FromPosition: spatial.Position{X: 7, Y: 3}, ToPosition: spatial.Position{X: 0, Y: 3},
				},
			},
		},
		// Members is deliberately empty: alice enters through Join (the
		// verb this suite is proving), not pre-authored into the world --
		// authoring her here too would have Join placing an ID the
		// composition already holds, which is not what "a player joins" means.
		// SetupInput requires at least one declared ending; this suite never
		// fires it (the acceptance loop dissolves the fight by decision, not
		// via an ending), so a never-triggered external declaration satisfies
		// the requirement without shaping the scenario.
		Endings: []tkencounter.EndingInput{{Key: "unused", Trigger: tkencounter.TriggerExternal{}}},
	})
	require.NoError(t, err, "building the three-room tomb")
	data := enc.ToData()
	return &data
}

// armedFighter is a sheet that can actually swing (mirrors the toolkit's own
// attack_test.go fixture of the same name): a longsword in the main hand and
// the proficiency to use it. The shared "empty hand" fixture other suites use
// is correct everywhere else and useless here.
func armedFighter(id, playerID string) *tkcharacter.Data {
	return &tkcharacter.Data{
		ID:       id,
		PlayerID: playerID,
		Name:     id,
		Level:    3,
		ClassID:  classes.Fighter,
		RaceID:   races.Human,
		AbilityScores: shared.AbilityScores{
			abilities.STR: 16, abilities.DEX: 14, abilities.CON: 14,
			abilities.INT: 10, abilities.WIS: 12, abilities.CHA: 8,
		},
		HitPoints:           24,
		MaxHitPoints:        28,
		ArmorClass:          16,
		ProficiencyBonus:    2,
		WeaponProficiencies: []proficiencies.Weapon{proficiencies.WeaponMartial},
		Inventory: []tkcharacter.InventoryItemData{
			{Type: shared.EquipmentTypeWeapon, ID: weapons.Longsword, Quantity: 1},
		},
		EquipmentSlots: tkcharacter.EquipmentSlots{
			tkcharacter.SlotMainHand: weapons.Longsword,
		},
	}
}

// acceptanceHarness is the design §6.1 acceptance criterion made executable:
// a party can, entirely through SessionService against the local stack,
// walk a multi-room world, meet a monster by sight, fight it, disengage,
// and recover its own position and the full story after the fact.
type acceptanceHarness struct {
	handler  *sessionhandler.Handler
	charRepo characterrepo.Repository
	manager  *sessionorch.Orchestrator
}

func newAcceptanceHarness(t *testing.T) *acceptanceHarness {
	t.Helper()

	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	charRepo, err := characterrepo.NewRedis(&characterrepo.RedisConfig{Client: client})
	require.NoError(t, err)

	// A fixed, always-maximum roller: this suite is proving the seam carries
	// a swing through to applied damage, not testing whether a d20 lands, so
	// a deterministic hit is the right choice over a ~1-in-3 chance of a
	// flaky "miss" run with real randomness (session.Roller's own doc:
	// "a test wires a fixed one and gets a reproducible fight").
	orch, err := sessionorch.New(sessionorch.Config{
		Redis: client, Characters: charRepo, TTL: 24 * time.Hour, Dice: testDice{},
	})
	require.NoError(t, err)

	h, err := sessionhandler.New(&sessionhandler.HandlerConfig{
		Manager: orch.Manager, Broker: orch.Broker, Characters: charRepo,
	})
	require.NoError(t, err)

	return &acceptanceHarness{handler: h, charRepo: charRepo, manager: orch}
}

func TestAcceptanceLoop_WalkFightDissolveResync(t *testing.T) {
	h := newAcceptanceHarness(t)
	ctx := auth.WithPlayerID(context.Background(), "player-alice")

	// Seed alice's character so Join can load her and Attack can swing.
	_, err := h.charRepo.Create(context.Background(), characterrepo.CreateInput{
		Character: &entities.Character{Data: armedFighter("alice", "player-alice")},
	})
	require.NoError(t, err)

	// The lobby's job, in-process: build the world, start the session.
	// (StartSession/Spawn are deliberately absent from SessionService --
	// design rule 5, "creation is the lobby's" -- so this test plays that
	// role directly against the manager, exactly as StartEncounter's
	// re-point will.)
	world := buildThreeRoomTomb(t)
	_, err = h.manager.Manager.StartSession(context.Background(), &sdk.StartSessionInput{
		Session: "acceptance-run", Encounter: "tomb-encounter", World: world,
	})
	require.NoError(t, err)
	_, err = h.manager.Manager.Spawn(context.Background(), &sdk.SpawnInput{
		Session: "acceptance-run", ID: "skel-1", Ref: refs.Monsters.Skeleton().String(),
		// tomb's Origin is (14,0); local (5,3) (matching the toolkit's own
		// proven ambush geometry) is absolute (19,3). Spawn speaks absolute
		// positions only -- there is no Room field any more (session
		// v0.12.0/rpg-toolkit#1048, one map complete).
		Position: spatial.Position{X: 19, Y: 3},
	})
	require.NoError(t, err)

	// -- join, at the entrance's absolute position (its Origin is (0,0), so
	// local and absolute coincide here) --
	joinResp, err := h.handler.Join(ctx, &sessionpb.JoinRequest{
		Session: "acceptance-run", Member: "alice",
		Position: &sessionpb.Position{X: 1, Y: 1},
	})
	require.NoError(t, err)
	require.Equal(t, "alice", joinResp.GetMember().GetId())

	// -- walk the whole route in ONE Move call: entrance -> hall -> tomb,
	// crossing both doorways as ordinary steps (design §0's destination
	// state, live since session v0.12.0/rpg-toolkit#1048 -- Traverse is
	// retired, there is no separate crossing verb any more). Absolute
	// coordinates: the entrance-hall door sits at (5,1)/(6,1); the
	// hall-tomb door at (13,3)/(14,3) (hall's Origin is (6,0), tomb's is
	// (14,0)). The walk stops on its own once sight forms the fight (Move's
	// own contract: `len(Steps) < len(Path)` is not an error), so the tail
	// of this path past the tomb doorway is a safety margin, not an
	// assumption that every requested cell gets walked.
	moveResp, err := h.handler.Move(ctx, &sessionpb.MoveRequest{
		Session: "acceptance-run", Member: "alice",
		Path: []*sessionpb.Position{
			{X: 2, Y: 1}, {X: 3, Y: 1}, {X: 4, Y: 1}, {X: 5, Y: 1}, {X: 6, Y: 1}, // cross into hall
			{X: 7, Y: 1}, {X: 8, Y: 1}, {X: 9, Y: 1}, {X: 10, Y: 1}, {X: 11, Y: 1}, {X: 12, Y: 1}, {X: 13, Y: 1},
			{X: 13, Y: 2}, {X: 13, Y: 3}, {X: 14, Y: 3}, // cross into tomb, onto the gap row
			{X: 15, Y: 3}, {X: 16, Y: 3}, {X: 17, Y: 3}, {X: 18, Y: 3}, // safety margin toward the skeleton
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, moveResp.GetSteps(), "the walk must have taken at least one step")
	lastStep := moveResp.GetSteps()[len(moveResp.GetSteps())-1]
	t.Logf("walk stopped after %d/19 steps, last at (%v,%v)",
		len(moveResp.GetSteps()), lastStep.GetPosition().GetX(), lastStep.GetPosition().GetY())

	formed := moveResp.GetFormed()
	require.NotNil(t, formed, "sight must have formed a fight during the walk into the tomb")
	require.Contains(t, formed.GetOrder(), "alice")
	require.Contains(t, formed.GetOrder(), "skel-1")

	// -- close as much of the reach gap as her own turn's movement allows --
	//
	// Sight forming the fight is not the same question as being within
	// melee reach of what it revealed (rpg-toolkit#1010): the walk above
	// stopped the moment the skeleton came into view, and a swing from
	// there is correctly refused rather than silently allowed the way it
	// was before this gate existed. This mirrors the toolkit's own
	// Example_theFightThatStartsItself: a second Move call, now on the
	// turn clock, still lets her walk (a fight does not freeze the mover,
	// it prices the mover -- rpg-toolkit#1169) using the remainder of the
	// very path already declared above as safety margin -- but a turn's
	// movement is BUDGETED (rpg-project#254's monster turn ships the same
	// economy a player's own turn already had): six cells is the whole 30
	// ft a fighter has this turn, and it is not enough by itself to close
	// what sight revealed early. Correct: she does not finish this leg
	// alone any more, and that is the point of the wave this suite is
	// pinned to.
	closeResp, err := h.handler.Move(ctx, &sessionpb.MoveRequest{
		Session: "acceptance-run", Member: "alice",
		Path: []*sessionpb.Position{
			{X: 13, Y: 1}, {X: 13, Y: 2}, {X: 13, Y: 3}, {X: 14, Y: 3},
			{X: 15, Y: 3}, {X: 16, Y: 3},
		},
	})
	require.NoError(t, err)
	require.Len(t, closeResp.GetSteps(), 6, "the whole of one turn's movement, and no more -- a fighter's 30 ft")

	// -- GetView reports the skeleton with its typed Seen position (ADR-0041,
	// rpg-toolkit#1157, session v0.21.2) -- through the real handler's
	// sightingToProto, not a mock. Sight is what just formed the fight above,
	// so this is the ordinary case Seen exists for: a live sighting produced
	// by the sight channel whose payload decoded. The position asserted is
	// skel-1's Spawn position (X:19, Y:3) -- it has not moved YET: nothing has
	// ended a turn to give its own driver the chance (below is where that
	// happens now that one is wired -- rpg-project#254 -- rather than never).
	viewResp, err := h.handler.GetView(ctx, &sessionpb.GetViewRequest{
		Session: "acceptance-run", Member: "alice",
	})
	require.NoError(t, err)
	var skeletonSighting *sessionpb.Sighting
	for _, sighting := range viewResp.GetSightings() {
		if sighting.GetSubject() == "skel-1" {
			skeletonSighting = sighting
			break
		}
	}
	require.NotNil(t, skeletonSighting, "alice's view must include the skeleton she just formed a fight with")
	require.NotNil(t, skeletonSighting.GetSeen(), "a live sight-channel sighting must carry its typed Seen position")
	require.Equal(t, 19.0, skeletonSighting.GetSeen().GetPosition().GetX())
	require.Equal(t, 3.0, skeletonSighting.GetSeen().GetPosition().GetY())

	// -- end her own turn: she has no more movement and is not yet in reach,
	// so passing it is the honest move -- and the skeleton's WHOLE driven
	// turn (moved, moved, struck, turn-ended) runs synchronously inside this
	// one EndTurn call (session.Behavior(), rpg-project#254). See
	// TestSkeletonsDrivenTurnMovesAndStrikes for the dedicated proof of that
	// sequence; this leg only needs the fact that it hands cleanly back.
	endResp, err := h.handler.EndTurn(ctx, &sessionpb.EndTurnRequest{
		Session: "acceptance-run", Member: "alice",
	})
	require.NoError(t, err)
	require.Equal(t, "alice", endResp.GetNext(),
		"a two-member fight wraps straight back to whoever led it once the skeleton's own turn ends")
	require.True(t, endResp.GetRoundWrapped())

	// -- attack, now in reach after the skeleton closed the rest of the gap
	// to swing at her -- and damage applies --
	attackResp, err := h.handler.Attack(ctx, &sessionpb.AttackRequest{
		Session: "acceptance-run", Attacker: "alice", Target: "skel-1",
	})
	require.NoError(t, err)
	t.Logf("attack: roll=%d total=%d against=%d hit=%v damage=%d",
		attackResp.GetRoll(), attackResp.GetTotal(), attackResp.GetAgainst(), attackResp.GetHit(), attackResp.GetDamage())

	// -- the fight ends ITSELF, because the skeleton stopped standing --
	//
	// This leg used to dissolve the fight BY DECISION. It cannot any more, and
	// the reason is a capability arriving rather than a regression: since
	// session/v0.15.0 a bubble left with nobody upright on one side dissolves
	// itself, with cause BY_DEFEAT, in the same sight refresh that notices it
	// (rpg-toolkit#1078). This suite's dice are rigged to the maximum face, so
	// the swing above is always a natural 20 -- a crit for 19 against a
	// skeleton's 13 hit points. There is no fight left to walk away from.
	//
	// Nobody announced it and no verb reported it: the world noticed. So the
	// observable proof is that alice is back on the WORLD clock without having
	// asked to be.
	require.True(t, attackResp.GetHit(), "the rigged dice must land this swing for the rest of this leg to mean anything")

	turnResp, err := h.handler.Turn(ctx, &sessionpb.TurnRequest{
		Session: "acceptance-run", Member: "alice",
	})
	require.NoError(t, err)
	require.Equal(t, sessionpb.ClockKind_CLOCK_KIND_WORLD, turnResp.GetClock(),
		"the killing blow must have ended the fight on its own and returned alice to free roam")

	// And Dissolve now refuses, which is the same fact stated from the other
	// side. Asserted rather than skipped: a client that offers a "disengage"
	// button needs to know this is the answer once the last enemy drops.
	_, err = h.handler.Dissolve(ctx, &sessionpb.DissolveRequest{
		Session: "acceptance-run", Member: "alice", Cause: sessionpb.DissolveKind_DISSOLVE_KIND_BY_DECISION,
	})
	requireGRPCCode(t, err, codes.FailedPrecondition)

	// -- disconnect and resume: a cold client learns its own position from
	// GetWhere (design rule 11's destination, live since session v0.13.0 /
	// rpg-toolkit#1051) rather than only from remembering its last Move. She
	// did not move during the dissolve, but she DID move during the fight --
	// closing the reach gap above -- so this must be exactly where THAT walk
	// left her, not where the first one stopped.
	closeLastStep := closeResp.GetSteps()[len(closeResp.GetSteps())-1]
	whereResp, err := h.handler.GetWhere(ctx, &sessionpb.GetWhereRequest{
		Session: "acceptance-run", Member: "alice",
	})
	require.NoError(t, err)
	require.Equal(t, closeLastStep.GetPosition().GetX(), whereResp.GetPosition().GetX(), "GetWhere must recover the walk's actual stopping cell")
	require.Equal(t, closeLastStep.GetPosition().GetY(), whereResp.GetPosition().GetY())

	// -- resync from zero and see the whole story --
	storyResp, err := h.handler.GetStory(ctx, &sessionpb.GetStoryRequest{
		Session: "acceptance-run", Member: "alice", FromSeq: 0,
	})
	require.NoError(t, err)
	require.NotEmpty(t, storyResp.GetEntries())
	t.Logf("story replay: %d beats from seq 0", len(storyResp.GetEntries()))

	seqs := make([]uint64, len(storyResp.GetEntries()))
	for i, e := range storyResp.GetEntries() {
		seqs[i] = e.GetSeq()
	}
	for i := 1; i < len(seqs); i++ {
		require.Greater(t, seqs[i], seqs[i-1], "story sequence must be strictly increasing (monotonic, gapless)")
	}
}

// TestFightEndsByDecisionWhenThePartyWalksAway covers the OTHER half of ending
// a fight, which the main acceptance loop can no longer reach.
//
// Since session/v0.15.0 the loop above ends by DEFEAT -- its rigged maximum-face
// dice always crit, and a skeleton does not survive that. Dissolve-by-decision
// is still a real verb and the only cause a caller can honestly declare, so it
// gets its own scene: the party sees the skeleton, decides it wants no part of
// this, and breaks off WITHOUT swinging.
//
// The two tests together are the whole vocabulary of DissolveKind: one cause
// nobody declares and one nobody else can.
func TestFightEndsByDecisionWhenThePartyWalksAway(t *testing.T) {
	h := newAcceptanceHarness(t)
	ctx := auth.WithPlayerID(context.Background(), "player-alice")

	_, err := h.charRepo.Create(context.Background(), characterrepo.CreateInput{
		Character: &entities.Character{Data: armedFighter("alice", "player-alice")},
	})
	require.NoError(t, err)

	world := buildThreeRoomTomb(t)
	_, err = h.manager.Manager.StartSession(context.Background(), &sdk.StartSessionInput{
		Session: "decision-run", Encounter: "tomb-encounter", World: world,
	})
	require.NoError(t, err)
	_, err = h.manager.Manager.Spawn(context.Background(), &sdk.SpawnInput{
		Session: "decision-run", ID: "skel-1", Ref: refs.Monsters.Skeleton().String(),
		Position: spatial.Position{X: 19, Y: 3},
	})
	require.NoError(t, err)

	_, err = h.handler.Join(ctx, &sessionpb.JoinRequest{
		Session: "decision-run", Member: "alice",
		Position: &sessionpb.Position{X: 1, Y: 1},
	})
	require.NoError(t, err)

	moveResp, err := h.handler.Move(ctx, &sessionpb.MoveRequest{
		Session: "decision-run", Member: "alice",
		Path: []*sessionpb.Position{
			{X: 2, Y: 1}, {X: 3, Y: 1}, {X: 4, Y: 1}, {X: 5, Y: 1}, {X: 6, Y: 1},
			{X: 7, Y: 1}, {X: 8, Y: 1}, {X: 9, Y: 1}, {X: 10, Y: 1}, {X: 11, Y: 1}, {X: 12, Y: 1}, {X: 13, Y: 1},
			{X: 13, Y: 2}, {X: 13, Y: 3}, {X: 14, Y: 3},
			{X: 15, Y: 3}, {X: 16, Y: 3}, {X: 17, Y: 3}, {X: 18, Y: 3},
		},
	})
	require.NoError(t, err)
	require.NotNil(t, moveResp.GetFormed(), "sight must have formed a fight during the walk into the tomb")

	// Break off without swinging. Nobody is defeated, so the only honest cause
	// is the one this verb IS.
	dissolveResp, err := h.handler.Dissolve(ctx, &sessionpb.DissolveRequest{
		Session: "decision-run", Member: "alice", Cause: sessionpb.DissolveKind_DISSOLVE_KIND_BY_DECISION,
	})
	require.NoError(t, err)
	require.Contains(t, dissolveResp.GetMembers(), "alice")
	require.Equal(t, sessionpb.DissolveKind_DISSOLVE_KIND_BY_DECISION, dissolveResp.GetCause(),
		"a fight the party chose to leave must be reported as such, not as a defeat")

	// Back on the world clock, with the skeleton still standing behind her.
	turnResp, err := h.handler.Turn(ctx, &sessionpb.TurnRequest{
		Session: "decision-run", Member: "alice",
	})
	require.NoError(t, err)
	require.Equal(t, sessionpb.ClockKind_CLOCK_KIND_WORLD, turnResp.GetClock())
}

// TestAWalkCannotCrossAWallWhereThereIsNoDoorway pins the seams that
// buildThreeRoomTomb draws, and it exists because nothing else here would
// notice if they vanished.
//
// This is the migration hazard of rpg-toolkit#1130 stated as a test. On one
// canvas, "entrance" and "hall" are two rectangles that happen to touch: their
// separation is NOT implied by being different rooms, it is only the boundary
// list. Delete those boundaries and every other test in this file still passes,
// because they all walk through the doorway anyway -- the party would simply
// also be able to stroll through the stone on any other row, and nobody would
// be told.
//
// Row 2 is the assertion. The entrance-hall door is on row 1, so (5,2)->(6,2)
// is a pair of adjacent cells with a wall between them: adjacency is not
// permission. The whole Move is refused rather than partially walked, which is
// the right answer for a path whose author was wrong about the map.
func TestAWalkCannotCrossAWallWhereThereIsNoDoorway(t *testing.T) {
	h := newAcceptanceHarness(t)
	ctx := auth.WithPlayerID(context.Background(), "player-alice")

	_, err := h.charRepo.Create(context.Background(), characterrepo.CreateInput{
		Character: &entities.Character{Data: armedFighter("alice", "player-alice")},
	})
	require.NoError(t, err)

	world := buildThreeRoomTomb(t)
	_, err = h.manager.Manager.StartSession(context.Background(), &sdk.StartSessionInput{
		Session: "wall-run", Encounter: "tomb-encounter", World: world,
	})
	require.NoError(t, err)

	_, err = h.handler.Join(ctx, &sessionpb.JoinRequest{
		Session: "wall-run", Member: "alice", Position: &sessionpb.Position{X: 1, Y: 2},
	})
	require.NoError(t, err)

	// Walk the entrance freely along row 2, then try to keep going into the
	// hall where there is no door.
	resp, err := h.handler.Move(ctx, &sessionpb.MoveRequest{
		Session: "wall-run", Member: "alice",
		Path: []*sessionpb.Position{{X: 2, Y: 2}, {X: 3, Y: 2}, {X: 4, Y: 2}, {X: 5, Y: 2}, {X: 6, Y: 2}},
	})
	requireGRPCCode(t, err, codes.InvalidArgument)
	require.Empty(t, resp.GetSteps(), "a path whose author was wrong about the map is refused whole, not walked partway")

	// And she has not moved: the refusal is not a half-applied walk.
	whereResp, err := h.handler.GetWhere(ctx, &sessionpb.GetWhereRequest{
		Session: "wall-run", Member: "alice",
	})
	require.NoError(t, err)
	require.Equal(t, 1.0, whereResp.GetPosition().GetX())
	require.Equal(t, 2.0, whereResp.GetPosition().GetY())
}

// storyBeats reads a session's whole story for member and returns just the
// "beat" name from each entry's JSON payload, in order -- the same
// projection the toolkit's own MonsterTurnTestSuite uses (session's own
// monster_turn_test.go), because StoryEntry carries no typed Kind (only
// Event, on the stream, does -- design rule 4's payload-is-passthrough
// corollary means a beat NAME still lives only in the payload here).
func storyBeats(ctx context.Context, t *testing.T, h *sessionhandler.Handler, session, member string) []string {
	t.Helper()
	resp, err := h.GetStory(ctx, &sessionpb.GetStoryRequest{Session: session, Member: member, FromSeq: 0})
	require.NoError(t, err)

	beats := make([]string, len(resp.GetEntries()))
	for i, e := range resp.GetEntries() {
		var body struct {
			Beat string `json:"beat"`
		}
		require.NoError(t, json.Unmarshal(e.GetPayload(), &body))
		beats[i] = body.Beat
	}
	return beats
}

// TestSkeletonsDrivenTurnMovesAndStrikes is rpg-project#254's own acceptance
// criterion made executable at the handler level: with session.Behavior()
// wired as the SessionService's TurnDriver (internal/orchestrators/session),
// a player's EndTurn on a fight with an unplayed monster drives that
// monster's WHOLE turn synchronously -- closing the distance sight already
// revealed, then swinging -- entirely inside the one EndTurn call, and hands
// the fight cleanly back to whoever is left playing.
//
// Alice joins already inside the tomb room, three cells from the skeleton
// and in its line of sight along the pillar gap row (buildThreeRoomTomb's
// own geometry -- see its doc comment) -- Join's own first-light contact
// check forms the fight on the spot, the same rule NewEncounter's does
// (encounter.SetupInput's TurnDriver doc). The fixed maximum-face dice break
// the tied initiative roll to alice (lexicographically first, the same
// tie-break the toolkit's own tombManager tests pin), so it is HER turn
// first: with no movement spent and no reach without it, ending immediately
// is the honest move, and it is what puts the skeleton's own turn under
// test.
func TestSkeletonsDrivenTurnMovesAndStrikes(t *testing.T) {
	h := newAcceptanceHarness(t)
	ctx := auth.WithPlayerID(context.Background(), "player-alice")

	_, err := h.charRepo.Create(context.Background(), characterrepo.CreateInput{
		Character: &entities.Character{Data: armedFighter("alice", "player-alice")},
	})
	require.NoError(t, err)

	world := buildThreeRoomTomb(t)
	_, err = h.manager.Manager.StartSession(context.Background(), &sdk.StartSessionInput{
		Session: "monster-turn-run", Encounter: "tomb-encounter", World: world,
	})
	require.NoError(t, err)
	_, err = h.manager.Manager.Spawn(context.Background(), &sdk.SpawnInput{
		Session: "monster-turn-run", ID: "skel-1", Ref: refs.Monsters.Skeleton().String(),
		Position: spatial.Position{X: 19, Y: 3},
	})
	require.NoError(t, err)

	joinResp, err := h.handler.Join(ctx, &sessionpb.JoinRequest{
		Session: "monster-turn-run", Member: "alice",
		Position: &sessionpb.Position{X: 16, Y: 3}, // inside the tomb, on the pillar gap row, three cells out
	})
	require.NoError(t, err)
	require.NotNil(t, joinResp.GetFormed(), "in sight and in range at first light: the fight forms on Join")
	require.Equal(t, []string{"alice", "skel-1"}, joinResp.GetFormed().GetOrder(),
		"the tied roll breaks to alice -- it must be her turn first, not the skeleton's")

	before := len(storyBeats(ctx, t, h.handler, "monster-turn-run", "alice"))

	// She has nothing to do this turn -- three cells out with no movement
	// spent, and no reach without it -- so passing is the honest move, and
	// what happens next belongs entirely to the skeleton's own driven turn.
	endResp, err := h.handler.EndTurn(ctx, &sessionpb.EndTurnRequest{
		Session: "monster-turn-run", Member: "alice",
	})
	require.NoError(t, err, "the skeleton's whole turn -- move, strike, end -- drives inside this one call")

	// Isolated to what THIS EndTurn call itself produced: alice's own
	// end-turn beat, then the skeleton's approach and swing.
	beats := storyBeats(ctx, t, h.handler, "monster-turn-run", "alice")[before:]
	// At least four beats or the slicing below is meaningless (and, for
	// fewer than two, out of bounds): alice's own turn-ended, one moved
	// closing the three-cell gap, the swing, and the skeleton's own
	// turn-ended. Asserted up front, with the actual trace on failure,
	// rather than letting a short trace panic on the slice or pass a
	// vacuous loop silently (Copilot, PR #817).
	require.GreaterOrEqual(t, len(beats), 4,
		"expected alice's turn-ended, >=1 moved, the swing, and the skeleton's turn-ended; got %v", beats)
	require.Equal(t, "turn-ended", beats[0], "alice's own end-turn beat comes first")

	// At least one moved beat (closing the three-cell gap), exactly one
	// struck-or-missed beat (the swing), and the skeleton's own turn ending
	// last, handing back to alice -- "moved beats to adjacent, then
	// struck/missed, then turn_ended{next: player}", the brief's own words.
	middle := beats[1 : len(beats)-1]
	require.NotEmpty(t, middle, "the skeleton had three cells to close before it could swing")
	for _, b := range middle[:len(middle)-1] {
		require.Equal(t, "moved", b, "everything before the swing is the approach")
	}
	swing := middle[len(middle)-1]
	require.Contains(t, []string{"struck", "missed"}, swing)
	require.Equal(t, "turn-ended", beats[len(beats)-1], "the skeleton's own turn closes back to alice")

	require.Equal(t, "alice", endResp.GetNext(), "turn_ended{next: player} -- the fight hands cleanly back")
	require.True(t, endResp.GetRoundWrapped(), "a two-member fight wraps straight back to whoever led it")
}
