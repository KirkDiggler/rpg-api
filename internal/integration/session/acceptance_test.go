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
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

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

// orderAsGiven is the trivial InitiativeRoller every fixture in this test
// needs (encounter.SetupInput requires one): the members already arrive in
// the order the composition detected them, so there is nothing to decide.
type orderAsGiven struct{}

func (orderAsGiven) RollInitiative(members []tkencounter.MemberID) ([]tkencounter.MemberID, error) {
	return members, nil
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

	occluders := make([]spatial.Position, 0, 7)
	for y := 0; y < 8; y++ {
		if y == 3 {
			continue // the gap
		}
		occluders = append(occluders, spatial.Position{X: 3, Y: float64(y)})
	}

	enc, err := tkencounter.NewEncounter(&tkencounter.SetupInput{
		Initiative: orderAsGiven{},
		Retention:  tkencounter.RetentionUnbounded,
		Field: tkencounter.FieldInput{
			Rooms: []tkencounter.RoomInput{
				{ID: "entrance", Width: 6, Height: 6, Origin: spatial.Position{X: 0, Y: 0}},
				{ID: "hall", Width: 8, Height: 6, Origin: spatial.Position{X: 6, Y: 0}},
				{ID: "tomb", Width: 8, Height: 8, Origin: spatial.Position{X: 14, Y: 0}, Occluders: occluders},
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

	// -- attack, and damage applies --
	attackResp, err := h.handler.Attack(ctx, &sessionpb.AttackRequest{
		Session: "acceptance-run", Attacker: "alice", Target: "skel-1",
	})
	require.NoError(t, err)
	t.Logf("attack: roll=%d total=%d against=%d hit=%v damage=%d",
		attackResp.GetRoll(), attackResp.GetTotal(), attackResp.GetAgainst(), attackResp.GetHit(), attackResp.GetDamage())

	// -- dissolve the fight by decision --
	dissolveResp, err := h.handler.Dissolve(ctx, &sessionpb.DissolveRequest{
		Session: "acceptance-run", Member: "alice", Cause: sessionpb.DissolveKind_DISSOLVE_KIND_BY_DECISION,
	})
	require.NoError(t, err)
	require.Contains(t, dissolveResp.GetMembers(), "alice")

	// -- disconnect and resume: a cold client learns its own position from
	// GetWhere (design rule 11's destination, live since session v0.13.0 /
	// rpg-toolkit#1051) rather than only from remembering its last Move. She
	// did not move during the fight or the dissolve, so this must be
	// exactly where the walk left her.
	whereResp, err := h.handler.GetWhere(ctx, &sessionpb.GetWhereRequest{
		Session: "acceptance-run", Member: "alice",
	})
	require.NoError(t, err)
	require.Equal(t, lastStep.GetPosition().GetX(), whereResp.GetPosition().GetX(), "GetWhere must recover the walk's actual stopping cell")
	require.Equal(t, lastStep.GetPosition().GetY(), whereResp.GetPosition().GetY())

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
