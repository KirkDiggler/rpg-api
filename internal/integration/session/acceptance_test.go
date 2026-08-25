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
	rosterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/roster"
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

// pointy is the one orientation this fixture is authored in. Every [col,row]
// below is an absolute offset pair under it; at(col,row) turns one into the
// dungeon-absolute axial cell the session verbs speak, by asking the toolkit
// (encounter.HexCellAt -- the one conversion, never reimplemented here).
var pointy = tkencounter.HexesArePointyTop()

func at(col, row int) spatial.Position { return tkencounter.HexCellAt(pointy, col, row) }

func pbAt(col, row int) *sessionpb.Position {
	p := at(col, row)
	return &sessionpb.Position{X: p.X, Y: p.Y}
}

// rect paints a width x height block of absolute offset cells whose top-left
// is [x0,y0].
func rect(x0, y0, width, height int) []spatial.Position {
	out := make([]spatial.Position, 0, width*height)
	for row := y0; row < y0+height; row++ {
		for col := x0; col < x0+width; col++ {
			out = append(out, spatial.Position{X: float64(col), Y: float64(row)})
		}
	}
	return out
}

// seamWall is the wall between two side-by-side regions: every edge between
// column west and column west+1, except the row the doorway is on. Absolute
// offset, like everything authored (rpg-project#256: walls are declared
// edges between adjacent floor cells; nothing is generated).
//
// THIS IS THE MIGRATION HAZARD OF rpg-toolkit#1130, and it is the reason this
// fixture grew rather than merely being renamed. When every chamber owned its
// own grid, nothing could cross between them except through a declared
// doorway -- the walls were implied by the room structure. On ONE CANVAS two
// regions side by side are simply ONE OPEN SPACE, and this list is the whole
// answer to what a member cannot walk through or see past. Without it the tomb
// is not three rooms joined by two doors; it is one 22-wide hall with some
// furniture in it.
func seamWall(west, rows, openRow int) []tkencounter.WallInput {
	out := make([]tkencounter.WallInput, 0, rows)
	for row := 0; row < rows; row++ {
		if row == openRow {
			continue // the doorway itself
		}
		out = append(out, tkencounter.WallInput{Boundary: spatial.Boundary{
			From:              spatial.Position{X: float64(west), Y: float64(row)},
			To:                spatial.Position{X: float64(west + 1), Y: float64(row)},
			BlocksMovement:    true,
			BlocksLineOfSight: true,
		}})
	}

	return out
}

// seamDoor is the doorway seamWall left open: one open door on the one edge.
// A DoorEdge is ABSOLUTE AXIAL, unlike a wall (the toolkit's own asymmetry,
// documented on DoorEdge), so the same [col,row] pair goes through at().
func seamDoor(id tkencounter.DoorID, west, row int) tkencounter.DoorInput {
	return tkencounter.DoorInput{
		ID:    id,
		Edges: []tkencounter.DoorEdge{{From: at(west, row), To: at(west+1, row)}},
		State: tkencounter.DoorIsOpen(),
	}
}

// testDice rolls the crypto roller would: this suite is not testing whether
// a d20 lands, only that the seam carries a swing through to damage, so a
// fixed non-zero source is enough -- Attack requires a Dice source at
// construction regardless (toolkit law: capabilities are supplied, never
// defaulted).
type testDice struct{}

func (testDice) Roll(_ context.Context, size int) (int, error) { return size, nil } // always the max face

// tombRoute is the whole walk from the entrance to the skeleton, in ONE
// Move call: along row 1 through the entrance-hall doorway, down to row 3
// early in the hall (so the first sightline down the hall-tomb doorway's row
// forms the fight with most of the hall still to cross), then along row 3
// through the second doorway and the pillar gap. The walk stops on its own
// once sight forms the fight (Move's own contract: `len(Steps) < len(Path)`
// is not an error), so the tail of this path is a safety margin, not an
// assumption that every requested cell gets walked; the fight's first turn
// continues along the SAME route from wherever it stopped.
func tombRoute() []*sessionpb.Position {
	return []*sessionpb.Position{
		pbAt(2, 1), pbAt(3, 1), pbAt(4, 1), pbAt(5, 1), pbAt(6, 1), // cross into the hall
		pbAt(7, 1), pbAt(7, 2), pbAt(7, 3), // down to the doorway's row
		pbAt(8, 3), pbAt(9, 3), pbAt(10, 3), pbAt(11, 3), pbAt(12, 3), pbAt(13, 3), pbAt(14, 3), // cross into the tomb
		pbAt(15, 3), pbAt(16, 3), pbAt(17, 3), pbAt(18, 3), // through the gap, toward the skeleton
	}
}

// buildThreeRoomTomb is entrance -> hall -> tomb, matching design §6.1's
// acceptance shape: a party walks two doorway crossings and meets a monster
// on the far side. Authored the way dungeonspec version 2 authors
// (rpg-project#256): three regions painted as absolute offset cells on one
// pointy-top canvas, the two seam walls as declared edges, the two doorways
// as open doors on the edges the walls leave out, and a column of pillars
// in the tomb (absolute col 17, a gap at row 3) so a sightline reaches the
// skeleton only along that one row -- the toolkit's own proven "sight forms
// a fight" geometry, transplanted into a region reached by crossing two
// doorways. Only the SESSION-FACING verbs this test drives against (Join,
// Move, Attack, ...) speak axial cells, and every one of those is at(col,row).
func buildThreeRoomTomb(t *testing.T) *tkencounter.EncounterData {
	return buildTomb(t, nil)
}

// buildTomb is buildThreeRoomTomb with a mutation hook: the run-ending suite
// locks the second door and declares the doom, everything else identical, so
// the two scenarios cannot drift apart in geometry.
func buildTomb(t *testing.T, mutate func(*tkencounter.SetupInput)) *tkencounter.EncounterData {
	t.Helper()

	// The tomb's column: a solid line of pillars at absolute col 17 with a
	// gap at row 3. Props answer both blocking questions independently
	// (rpg-toolkit#1130), and these say what the old field could only imply:
	// they stop SIGHT and not MOVEMENT. This scene is about who can see whom,
	// never about who can walk where.
	blocksSight, passable := true, false
	pillars := make([]tkencounter.PropInput, 0, 7)
	for y := 0; y < 8; y++ {
		if y == 3 {
			continue // the gap
		}
		pillars = append(pillars, tkencounter.PropInput{
			Ref:               "pillar",
			At:                spatial.Position{X: 17, Y: float64(y)},
			BlocksMovement:    &passable,
			BlocksLineOfSight: &blocksSight,
		})
	}
	lit := &tkencounter.Lighting{Intensity: 1}

	in := &tkencounter.SetupInput{
		Initiative: orderAsGiven{},
		Retention:  tkencounter.RetentionUnbounded,
		Standing:   allStanding{},
		Sight:      allSeeing{},
		// tkencounter.PassDriver{}/RefusingStriker{} are the toolkit's own
		// trivial, exported stand-ins (rpg-toolkit#1167 closed) --
		// construction-time only, same as allStanding/allSeeing above: this
		// fixture builds the seed EncounterData via NewEncounter and the real,
		// played session supplies its own TurnDriver (sdk.Behavior() today)
		// and Striker when it loads and plays this world, so what a monster's
		// turn does here never matters.
		TurnDriver: tkencounter.PassDriver{},
		Striker:    tkencounter.RefusingStriker{},
		Field: tkencounter.FieldInput{
			Canvas: tkencounter.CanvasInput{
				// A tomb is cut from stone: you cannot see across the space
				// between the chambers, or walk it. Required rather than
				// defaulted (rpg-toolkit#1116) -- there is no correct default.
				Void:        tkencounter.VoidIsOpaque(),
				Orientation: pointy,
			},
			Regions: []tkencounter.RegionInput{
				{ID: "entrance", Name: "Entrance", Archetype: "crypt", Lighting: lit, Cells: rect(0, 0, 6, 6)},
				{ID: "hall", Name: "Hall", Archetype: "crypt", Lighting: lit, Cells: rect(6, 0, 8, 6)},
				{ID: "tomb", Name: "Tomb", Archetype: "crypt", Lighting: lit, Cells: rect(14, 0, 8, 8)},
			},
			// Each seam is drawn, leaving the doorway row open. Without these
			// the party could walk from the entrance into the tomb at any
			// row, and see all the way down.
			Walls: append(
				seamWall(5, 6, 1),      // the entrance-hall door is on row 1
				seamWall(13, 6, 3)...), // the hall-tomb door is on row 3
			Doors: []tkencounter.DoorInput{
				seamDoor("entrance-hall", 5, 1),
				seamDoor("hall-tomb", 13, 3),
			},
			Props: pillars,
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
	}
	if mutate != nil {
		mutate(in)
	}
	enc, err := tkencounter.NewEncounter(in)
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
	handler    *sessionhandler.Handler
	charRepo   characterrepo.Repository
	manager    *sessionorch.Orchestrator
	rosterRepo rosterrepo.Repository
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

	rosterRepo := rosterrepo.NewInMemory()
	h, err := sessionhandler.New(&sessionhandler.HandlerConfig{
		Manager: orch.Manager, Broker: orch.Broker, Characters: charRepo, Roster: rosterRepo,
	})
	require.NoError(t, err)

	return &acceptanceHarness{handler: h, charRepo: charRepo, manager: orch, rosterRepo: rosterRepo}
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
		Position: at(19, 3),
	})
	require.NoError(t, err)

	// -- join, at the entrance's absolute position (its Origin is (0,0), so
	// local and absolute coincide here) --
	joinResp, err := h.handler.Join(ctx, &sessionpb.JoinRequest{
		Session: "acceptance-run", Member: "alice",
		Position: pbAt(1, 1),
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
	route := tombRoute()
	moveResp, err := h.handler.Move(ctx, &sessionpb.MoveRequest{
		Session: "acceptance-run", Member: "alice", Path: route,
	})
	require.NoError(t, err)
	require.NotEmpty(t, moveResp.GetSteps(), "the walk must have taken at least one step")
	walked := len(moveResp.GetSteps())
	lastStep := moveResp.GetSteps()[walked-1]
	t.Logf("walk stopped after %d/%d steps, last at (%v,%v)",
		walked, len(route), lastStep.GetPosition().GetX(), lastStep.GetPosition().GetY())
	require.Less(t, walked+6, len(route)-1,
		"sight must form the fight with more than one turn of walking still between alice and the skeleton")

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
		DeclarationId: currentDeclarationID(ctx, t, h.handler, "acceptance-run", "alice", sessionpb.Verb_VERB_MOVE),
		Path:          route[walked : walked+6],
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
	require.Equal(t, at(19, 3).X, skeletonSighting.GetSeen().GetPosition().GetX())
	require.Equal(t, at(19, 3).Y, skeletonSighting.GetSeen().GetPosition().GetY())

	// -- end her own turn: she has no more movement and is not yet in reach,
	// so passing it is the honest move -- and the skeleton's WHOLE driven
	// turn (struck, turn-ended) runs synchronously inside this one EndTurn
	// call (session.Behavior(), rpg-project#254). Since dnd5e v0.99.0's
	// inert action definitions (rpg-toolkit#1198) the skeleton carries an
	// authored shortbow, so its driven turn strikes from where it stands
	// instead of walking into melee. See TestSkeletonsDrivenTurnStrikesFromRange
	// for the dedicated proof of that sequence; this leg only needs the fact
	// that it hands cleanly back.
	endResp, err := h.handler.EndTurn(ctx, &sessionpb.EndTurnRequest{
		Session: "acceptance-run", Member: "alice",
		DeclarationId: currentDeclarationID(ctx, t, h.handler, "acceptance-run", "alice", sessionpb.Verb_VERB_END_TURN),
	})
	require.NoError(t, err)
	require.Equal(t, "alice", endResp.GetNext(),
		"a two-member fight wraps straight back to whoever led it once the skeleton's own turn ends")
	require.True(t, endResp.GetRoundWrapped())

	// -- close the REST of the gap on her own fresh turn: the skeleton no
	// longer approaches (its shortbow needed no move), so the walk to melee
	// is all hers now. The declared route's remaining tail is exactly one
	// more turn's budget and ends adjacent to the skeleton's spawn cell --
	// the safety margin above becoming load-bearing.
	rest := route[walked+6:]
	require.Len(t, rest, 6,
		"geometry gate: the walk must have stopped where it always does, leaving the declared route's tail exactly one turn's movement -- if the fight formed later than that, the closing arithmetic below is meaningless")
	close2Resp, err := h.handler.Move(ctx, &sessionpb.MoveRequest{
		Session: "acceptance-run", Member: "alice",
		DeclarationId: currentDeclarationID(ctx, t, h.handler, "acceptance-run", "alice", sessionpb.Verb_VERB_MOVE),
		Path:          rest,
	})
	require.NoError(t, err)
	require.Len(t, close2Resp.GetSteps(), 6,
		"the route's remaining tail is one more turn's whole 30 ft, ending in melee reach")

	// -- attack, now in reach after closing the rest of the gap herself --
	// and damage applies --
	attackResp, err := h.handler.Attack(ctx, &sessionpb.AttackRequest{
		Session: "acceptance-run", Attacker: "alice", Target: "skel-1",
		DeclarationId: currentDeclarationID(ctx, t, h.handler, "acceptance-run", "alice", sessionpb.Verb_VERB_ATTACK),
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
	closeLastStep := close2Resp.GetSteps()[len(close2Resp.GetSteps())-1]
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

// TestGetRoster_ServesTheLaunchWrittenRow drives the read side of
// rpg-project#264 against the real handler and the real character store: a
// roster row shaped exactly as launch writes it (see the lobby stack suite's
// TestStartEncounter_WritesTheRosterRow for the write side) serves alice her
// party's public identity — her own class/race refs read FRESH from the
// character record, the skeleton's authored ref and name from the row — and
// nothing else.
func TestGetRoster_ServesTheLaunchWrittenRow(t *testing.T) {
	h := newAcceptanceHarness(t)
	ctx := auth.WithPlayerID(context.Background(), "player-alice")

	_, err := h.charRepo.Create(context.Background(), characterrepo.CreateInput{
		Character: &entities.Character{Data: armedFighter("alice", "player-alice")},
	})
	require.NoError(t, err)

	require.NoError(t, h.rosterRepo.Save(context.Background(), &rosterrepo.Data{
		EncounterID: "roster-run",
		Members: []rosterrepo.Member{
			{ID: "alice", Kind: rosterrepo.KindPlayer},
			{ID: "skeleton-1", Kind: rosterrepo.KindMonster, Ref: "dnd5e:monsters:skeleton", Name: "Skeleton"},
		},
	}))

	resp, err := h.handler.GetRoster(ctx, &sessionpb.GetRosterRequest{Session: "roster-run"})
	require.NoError(t, err)
	require.Len(t, resp.GetMembers(), 2)

	alice := resp.GetMembers()[0]
	require.Equal(t, sessionpb.MemberKind_MEMBER_KIND_PLAYER, alice.GetKind())
	require.Equal(t, string(classes.Fighter), alice.GetClassRef(),
		"class ref must be the character record's own word — the one the local-player render path already maps")
	require.Equal(t, string(races.Human), alice.GetRaceRef())
	require.NotNil(t, alice.GetCustomization(), "the shelf is always set")

	skel := resp.GetMembers()[1]
	require.Equal(t, sessionpb.MemberKind_MEMBER_KIND_MONSTER, skel.GetKind())
	require.Equal(t, "dnd5e:monsters:skeleton", skel.GetMonsterRef())
	require.Equal(t, "Skeleton", skel.GetName())

	// The seated gate, through the real stack: a player with no seat in this
	// roster is refused the read.
	strangerCtx := auth.WithPlayerID(context.Background(), "player-nobody")
	_, err = h.handler.GetRoster(strangerCtx, &sessionpb.GetRosterRequest{Session: "roster-run"})
	requireGRPCCode(t, err, codes.PermissionDenied)
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
		Position: at(19, 3),
	})
	require.NoError(t, err)

	_, err = h.handler.Join(ctx, &sessionpb.JoinRequest{
		Session: "decision-run", Member: "alice",
		Position: pbAt(1, 1),
	})
	require.NoError(t, err)

	moveResp, err := h.handler.Move(ctx, &sessionpb.MoveRequest{
		Session: "decision-run", Member: "alice", Path: tombRoute(),
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
		Session: "wall-run", Member: "alice", Position: pbAt(1, 2),
	})
	require.NoError(t, err)

	// Walk the entrance freely along row 2, then try to keep going into the
	// hall where there is no door.
	resp, err := h.handler.Move(ctx, &sessionpb.MoveRequest{
		Session: "wall-run", Member: "alice",
		Path: []*sessionpb.Position{pbAt(2, 2), pbAt(3, 2), pbAt(4, 2), pbAt(5, 2), pbAt(6, 2)},
	})
	requireGRPCCode(t, err, codes.InvalidArgument)
	require.Empty(t, resp.GetSteps(), "a path whose author was wrong about the map is refused whole, not walked partway")

	// And she has not moved: the refusal is not a half-applied walk.
	whereResp, err := h.handler.GetWhere(ctx, &sessionpb.GetWhereRequest{
		Session: "wall-run", Member: "alice",
	})
	require.NoError(t, err)
	require.Equal(t, at(1, 2).X, whereResp.GetPosition().GetX())
	require.Equal(t, at(1, 2).Y, whereResp.GetPosition().GetY())
}

// storyBeats reads a session's whole story for member and returns just the
// "beat" name from each entry's JSON payload, in order -- the same
// projection the toolkit's own MonsterTurnTestSuite uses (session's own
// monster_turn_test.go).
//
// GetStory carries a typed EventKind now too (session/v0.23.0,
// rpg-toolkit#1213, rpg-api-protos#239 -- see story_events_test.go for the
// acceptance proof that it is byte-equal to what the live stream sends), but
// this helper still reads the composition's own literal beat STRING
// ("moved", "turn-ended", ...) off Payload rather than the wire enum -- the
// two vocabularies are not one-to-one everywhere (kindFor's "down" ->
// EventDowned pairs with THIS package's own "downed" reserved word,
// design rule 4's payload-is-passthrough corollary), and this file's own
// assertions below are written against the composition's words.
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

// TestSkeletonsDrivenTurnStrikesFromRange is rpg-project#254's own acceptance
// criterion made executable at the handler level: with session.Behavior()
// wired as the SessionService's TurnDriver (internal/orchestrators/session),
// a player's EndTurn on a fight with an unplayed monster drives that
// monster's WHOLE turn synchronously -- entirely inside the one EndTurn call
// -- and hands the fight cleanly back to whoever is left playing. Since
// dnd5e v0.99.0's inert action definitions (rpg-toolkit#1198) the skeleton
// carries an authored shortbow (range 80/320), so three cells out is already
// in range: its driven turn strikes from where it stands without inventing
// an approach, the same shape the toolkit's own
// TestSkeletonAttacksFromRange pins.
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
func TestSkeletonsDrivenTurnStrikesFromRange(t *testing.T) {
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
		Position: at(19, 3),
	})
	require.NoError(t, err)

	joinResp, err := h.handler.Join(ctx, &sessionpb.JoinRequest{
		Session: "monster-turn-run", Member: "alice",
		Position: pbAt(16, 3), // inside the tomb, on the pillar gap row, three cells out
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
		DeclarationId: currentDeclarationID(ctx, t, h.handler, "monster-turn-run", "alice", sessionpb.Verb_VERB_END_TURN),
	})
	require.NoError(t, err, "the skeleton's whole turn -- strike, end -- drives inside this one call")

	// Isolated to what THIS EndTurn call itself produced: alice's own
	// end-turn beat, then the skeleton's swing. Exactly three beats: the
	// shortbow attack needs no approach, so no moved beat is authored.
	beats := storyBeats(ctx, t, h.handler, "monster-turn-run", "alice")[before:]
	require.Len(t, beats, 3,
		"expected alice's turn-ended, the shortbow swing, and the skeleton's turn-ended; got %v", beats)
	require.Equal(t, "turn-ended", beats[0], "alice's own end-turn beat comes first")
	require.Contains(t, []string{"struck", "missed"}, beats[1], "the swing, with no approach before it")
	require.Equal(t, "turn-ended", beats[2], "the skeleton's own turn closes back to alice")

	require.Equal(t, "alice", endResp.GetNext(), "turn_ended{next: player} -- the fight hands cleanly back")
	require.True(t, endResp.GetRoundWrapped(), "a two-member fight wraps straight back to whoever led it")
}

// TestTheRunEndsWhenTheBossFalls is the journey's Done-when as one test
// (rpg-project#253, slice rpg-project#268): the locked door refuses the walk
// BY NAME, the lock is beaten server-side and the attempt narrated with its
// numbers, what the door revealed starts the fight, and the boss going down
// closes the encounter with a recorded outcome — on the world clock, with
// the beat order Kirk ruled (rpg-project#269 §6.6): down, fight over, ended.
func TestTheRunEndsWhenTheBossFalls(t *testing.T) {
	h := newAcceptanceHarness(t)
	ctx := auth.WithPlayerID(context.Background(), "player-alice")

	_, err := h.charRepo.Create(context.Background(), characterrepo.CreateInput{
		Character: &entities.Character{Data: armedFighter("alice", "player-alice")},
	})
	require.NoError(t, err)

	// The same tomb, with the second door locked at the reference tomb's DC
	// and the doom declared over the monster the launch is about to spawn —
	// an ending may name a member that joins later.
	world := buildTomb(t, func(in *tkencounter.SetupInput) {
		in.Field.Doors[1].State = tkencounter.DoorIsLocked(tkencounter.Lock{DC: 12, Ability: "dex"})
		in.Endings = []tkencounter.EndingInput{
			{Key: "withdrawn", Trigger: tkencounter.TriggerExternal{}},
			{Key: "boss-down", Trigger: tkencounter.TriggerMemberDown{Member: "skel-1"}},
		}
	})
	_, err = h.manager.Manager.StartSession(context.Background(), &sdk.StartSessionInput{
		Session: "doom-run", Encounter: "tomb-encounter", World: world,
	})
	require.NoError(t, err)
	_, err = h.manager.Manager.Spawn(context.Background(), &sdk.SpawnInput{
		Session: "doom-run", ID: "skel-1", Ref: refs.Monsters.Skeleton().String(),
		Position: at(19, 3),
	})
	require.NoError(t, err)

	// The launch also writes the roster row GetDoors's seated gate reads.
	require.NoError(t, h.rosterRepo.Save(context.Background(), &rosterrepo.Data{
		EncounterID: "doom-run",
		Members:     []rosterrepo.Member{{ID: "alice", Kind: rosterrepo.KindPlayer}},
	}))

	// Join in the hall, on the locked door's row, with the door shut between
	// alice and the skeleton: no fight forms — the lock blocks sight.
	joinResp, err := h.handler.Join(ctx, &sessionpb.JoinRequest{
		Session: "doom-run", Member: "alice", Position: pbAt(12, 3),
	})
	require.NoError(t, err)
	require.Nil(t, joinResp.GetFormed(), "the locked door is dark: nothing on the far side is in sight")

	// -- GetDoors: the live half of the atlas's doorways --
	doorsResp, err := h.handler.GetDoors(ctx, &sessionpb.GetDoorsRequest{Session: "doom-run"})
	require.NoError(t, err)
	require.Len(t, doorsResp.GetDoors(), 2)
	byID := map[string]*sessionpb.DoorInfo{}
	for _, d := range doorsResp.GetDoors() {
		byID[d.GetDoor()] = d
	}
	require.Equal(t, sessionpb.DoorState_DOOR_STATE_OPEN, byID["entrance-hall"].GetState())
	require.Nil(t, byID["entrance-hall"].GetLock(), "an open door carries no lock")
	require.Equal(t, sessionpb.DoorState_DOOR_STATE_LOCKED, byID["hall-tomb"].GetState())
	require.Equal(t, int32(12), byID["hall-tomb"].GetLock().GetDc(), "the DC is public — full data until v1.0")

	// -- the walk is refused as FICTION, not as a bad cell (rpg-toolkit#1135) --
	_, err = h.handler.Move(ctx, &sessionpb.MoveRequest{
		Session: "doom-run", Member: "alice", Path: []*sessionpb.Position{pbAt(13, 3), pbAt(14, 3)},
	})
	requireGRPCCode(t, err, codes.FailedPrecondition)
	require.Contains(t, err.Error(), "hall-tomb", "the refusal names the door that stopped her")
	require.Contains(t, err.Error(), "DC 12", "and what it would take")

	// She walked one step before the door refused the second — nothing is
	// saved on a mid-walk rejection, so she still stands at (12,3). Step to
	// the door's own cell first, then try the lock.
	_, err = h.handler.Move(ctx, &sessionpb.MoveRequest{
		Session: "doom-run", Member: "alice", Path: []*sessionpb.Position{pbAt(13, 3)},
	})
	require.NoError(t, err)

	// -- the check rolls server-side: d20 max face 20 + dex 2 beats DC 12,
	// and a beaten lock OPENS the door --
	unlockResp, err := h.handler.Unlock(ctx, &sessionpb.UnlockRequest{
		Session: "doom-run", Member: "alice", Door: "hall-tomb",
	})
	require.NoError(t, err)
	require.True(t, unlockResp.GetBeaten())
	require.Equal(t, int32(22), unlockResp.GetTotal(), "the roll is public down to the number")
	require.Equal(t, int32(12), unlockResp.GetDc())
	require.Equal(t, sessionpb.DoorState_DOOR_STATE_OPEN, unlockResp.GetDoor().GetState())

	// The attempt is narrated with its author and its numbers, typed.
	events, err := h.handler.GetStory(ctx, &sessionpb.GetStoryRequest{Session: "doom-run", Member: "alice"})
	require.NoError(t, err)
	var door *sessionpb.DoorChanged
	for _, e := range events.GetEntries() {
		if e.GetKind() == sessionpb.EventKind_EVENT_KIND_DOOR {
			door = e.GetDoor()
		}
	}
	require.NotNil(t, door, "the door beat reaches the wire typed")
	require.Equal(t, "hall-tomb", door.GetDoor())
	require.Equal(t, sessionpb.DoorState_DOOR_STATE_OPEN, door.GetState())
	require.Equal(t, "alice", door.GetActor(), "the story says whose hands")
	require.Equal(t, int32(12), door.GetDc())
	require.Equal(t, int32(22), door.GetTotal())
	require.True(t, door.GetBeaten())

	// -- what the door revealed started the fight: the sightline down row 3
	// reaches the skeleton through the pillar gap --
	turnResp, err := h.handler.Turn(ctx, &sessionpb.TurnRequest{Session: "doom-run", Member: "alice"})
	require.NoError(t, err)
	require.Equal(t, sessionpb.ClockKind_CLOCK_KIND_TURN, turnResp.GetClock(), "opening the door started the fight")

	// -- her turn: close to reach (five cells, within a fighter's 30 ft)
	// and put the boss down. The max-face roller crits: 2d8 maxed plus
	// strength is past a skeleton's whole pool in one swing.
	_, err = h.handler.Move(ctx, &sessionpb.MoveRequest{
		Session: "doom-run", Member: "alice",
		DeclarationId: currentDeclarationID(ctx, t, h.handler, "doom-run", "alice", sessionpb.Verb_VERB_MOVE),
		Path:          []*sessionpb.Position{pbAt(14, 3), pbAt(15, 3), pbAt(16, 3), pbAt(17, 3), pbAt(18, 3)},
	})
	require.NoError(t, err)

	attackResp, err := h.handler.Attack(ctx, &sessionpb.AttackRequest{
		Session: "doom-run", Attacker: "alice", Target: "skel-1",
		DeclarationId: currentDeclarationID(ctx, t, h.handler, "doom-run", "alice", sessionpb.Verb_VERB_ATTACK),
	})
	require.NoError(t, err)
	require.NotNil(t, attackResp.GetAttack(), "the swing resolved")

	// -- the run is over: the boss fell, the fight dissolved FIRST, and the
	// encounter closed on the world clock (ruling §6.6), with the outcome
	// recorded and readable forever --
	statusResp, err := h.handler.GetStatus(ctx, &sessionpb.GetStatusRequest{Session: "doom-run"})
	require.NoError(t, err)
	require.False(t, statusResp.GetOpen(), "the run is over and nobody had to say so")
	require.Equal(t, "boss-down", statusResp.GetOutcome().GetEnding())

	beats := storyBeats(ctx, t, h.handler, "doom-run", "alice")
	require.GreaterOrEqual(t, len(beats), 4)
	tail := beats[len(beats)-3:]
	require.Equal(t, []string{"down", "bubble-dissolved", "ended"}, tail,
		"the ruled order: the body is news, the fight ends, and only then the run")

	// The typed ended body carries the key a client maps to its sentence.
	events, err = h.handler.GetStory(ctx, &sessionpb.GetStoryRequest{Session: "doom-run", Member: "alice"})
	require.NoError(t, err)
	last := events.GetEntries()[len(events.GetEntries())-1]
	require.Equal(t, sessionpb.EventKind_EVENT_KIND_ENDED, last.GetKind())
	require.Equal(t, "boss-down", last.GetEnded().GetEnding(),
		"a client following the stream finally hears HOW the run ended")

	// A closed run refuses verbs in FAILED_PRECONDITION's vocabulary.
	_, err = h.handler.Move(ctx, &sessionpb.MoveRequest{
		Session: "doom-run", Member: "alice", Path: []*sessionpb.Position{pbAt(17, 3)},
	})
	requireGRPCCode(t, err, codes.FailedPrecondition)
}
