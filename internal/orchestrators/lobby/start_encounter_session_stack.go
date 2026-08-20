package lobby

import (
	"context"
	"errors"
	"fmt"

	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/refs"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
)

// StartEncounter's new-session-stack path (design rpg-project/ideas/session-
// api/design.md §3 "coexistence"): server configuration selects EXACTLY ONE
// stack per StartEncounter call, never both. This file is the new stack's
// half; start_encounter.go's existing body is the old stack's, completely
// untouched by anything here.
//
// KNOWN GAP, reported rather than worked around (per the brief this shipped
// under): there is no authored-dungeon-YAML -> new-stack EncounterData
// compiler in the toolkit yet. The old path's resolveDungeonSpec /
// dungeonspec.Load pipeline produces tkenc.DungeonParams for the OLD
// encounter module (rpg-toolkit/encounter) -- a different module with a
// different (hex-cube) coordinate system from the new one-map, absolute-
// Position world rulebooks/dnd5e/session.StartSession consumes. Building
// that compiler is real toolkit-side work, not something to improvise here
// (design rule 1: rpg-api invents no vocabulary; a content compiler is
// exactly the kind of thing that belongs upstream, not guessed at in the
// host). Until it exists, this path seeds every session from the single
// fixed builtInSessionWorld below -- good enough to prove the new stack is
// live and walkable in local dev, not a second content pipeline.
//
// A second, smaller gap rides along: encounter.SetupInput requires an
// InitiativeRoller, and the toolkit exposes only the INTERFACE publicly, no
// injectable real (dice + ability-score) implementation -- only its own
// test suites' trivial "order as given" fakes satisfy it today. sessionOrderAsGiven
// below is that same shortcut, not a rules decision made here: a fight in
// this minimal world resolves turn order as party-then-monster, which is
// honest about being a placeholder, not a hidden ruling.

// builtInSessionRoomID names the one room every new-stack session plays in
// until real authored content exists.
const builtInSessionRoomID = "hall"

// sessionOrderAsGiven is the toolkit's own test pattern (rulebooks/dnd5e/
// session's fight_starts_test.go encOrderAsGiven), duplicated here rather
// than imported because it is a test type unexported from that package. See
// this file's header comment for why "order as given" is the honest choice
// for a placeholder world rather than an invented dice-and-DEX ranking.
type sessionOrderAsGiven struct{}

func (sessionOrderAsGiven) RollInitiative(members []tkencounter.MemberID) ([]tkencounter.MemberID, error) {
	return members, nil
}

// sessionAllStanding and sessionAllSeeing are the Standing and Sight
// capabilities encounter.SetupInput has required since rpg-toolkit#1033:
// supplied, never defaulted, refused at construction rather than assumed,
// because "everybody can see" is a decision about a world and not something the
// composition may take for granted.
//
// They are CONSTRUCTION-TIME ONLY and that is what makes trivial versions
// honest here rather than a hidden ruling. Capabilities are not persisted into
// the EncounterData this returns; the session package supplies its own when it
// loads a world, including the sight RANGE that decides who is in contact. What
// these two do is satisfy NewEncounter's validation for a world that is empty
// at the moment it is built -- there is nobody to see or to be standing yet.
type sessionAllStanding struct{}

// Standing reports who is DOWN, not who is up: the interface's own parameter is
// named down, and reading it backwards would report a healthy party as a wiped
// one.
func (sessionAllStanding) Standing(_ []tkencounter.MemberID) ([]tkencounter.MemberID, error) {
	return nil, nil
}

type sessionAllSeeing struct{}

func (sessionAllSeeing) Sight(members []tkencounter.MemberID) (map[tkencounter.MemberID]int, error) {
	out := make(map[tkencounter.MemberID]int, len(members))
	for _, id := range members {
		out[id] = 1_000_000
	}

	return out, nil
}

// builtInSessionWorld is the fixed placeholder world every new-stack session
// starts in: one 12x10 square room, empty until StartEncounterOnSessionStack
// joins the party and spawns a monster into it.
//
// THE PARTY NOW GETS ROOM TO EXPLORE, and not because this world earned it.
// Until session/v0.18.0 this placeholder had a known consequence: with nothing
// to hide behind, the party and the monster saw each other and the fight
// started itself the moment both were placed. That stopped being true when
// session gave sight a RANGE (four cells, a torch's twenty feet) instead of
// letting geometry alone decide. The party enters at x=1 and the skeleton
// stands at (9,5) -- eight cells apart -- so everyone starts on the world clock
// and somebody has to walk toward the fight.
//
// Recorded rather than celebrated: the improvement is a side effect of a policy
// that lives in the session package, not a property of this world, and it would
// evaporate if that constant changed or if this placeholder ever placed things
// closer together. Real authored content should give a party that room on
// purpose. Adding hand-tuned sight-blocking geometry here would be exactly the
// content authoring this file's header explains rpg-api should not improvise.
//
// ONE room, so there are no seams to draw. Worth stating explicitly, because a
// multi-room version of this function would need them: since rpg-toolkit#1130
// rooms sit on one canvas and do NOT imply walls, so two chambers side by side
// are one open space until a Boundaries list says otherwise. A future authored
// world that grows this into several rooms and forgets that would ship a
// dungeon with no interior walls and no error to say so.
// Verified directly (start_encounter_session_stack_test.go): Turn on the
// spawned monster reports ClockTurn, not ClockWorld, immediately after
// StartEncounter returns.
func builtInSessionWorld() (*tkencounter.EncounterData, error) {
	enc, err := tkencounter.NewEncounter(&tkencounter.SetupInput{
		Initiative: sessionOrderAsGiven{},
		Standing:   sessionAllStanding{},
		Sight:      sessionAllSeeing{},
		Retention:  tkencounter.RetentionUnbounded,
		Field: tkencounter.FieldInput{
			Canvas: tkencounter.CanvasInput{
				// Required rather than defaulted (rpg-toolkit#1116): what the
				// space between chambers does to a sightline is a fact about a
				// world, and there is no correct default -- a tomb's answer is
				// the opposite of an open-air ruin's. Opaque is the reference
				// dungeon's answer and the conservative one for a placeholder;
				// with a single room there is no interior void for it to govern
				// anyway.
				//
				// No Orientation: it is required for a hex field and REFUSED
				// for a square one, which this is.
				Void: tkencounter.VoidIsOpaque(),
			},
			Rooms: []tkencounter.RoomInput{
				{ID: builtInSessionRoomID, Width: 12, Height: 10},
			},
		},
		// SetupInput requires at least one declared ending; this placeholder
		// world has nothing to end it with yet (no authored win/loss
		// conditions -- another facet of the content-compile gap above), so
		// a never-fired external declaration satisfies construction without
		// inventing a real one.
		Endings: []tkencounter.EndingInput{{Key: "external", Trigger: tkencounter.TriggerExternal{}}},
	})
	if err != nil {
		return nil, fmt.Errorf("build built-in session world: %w", err)
	}
	data := enc.ToData()
	return &data, nil
}

// builtInPartyPositions returns up to n fixed, non-overlapping join
// positions along the room's west wall, one per ready member in roster
// order.
func builtInPartyPositions(n int) []spatial.Position {
	positions := make([]spatial.Position, n)
	for i := range positions {
		positions[i] = spatial.Position{X: 1, Y: float64(1 + i)}
	}
	return positions
}

// builtInMonsterID and builtInMonsterPosition place the one placeholder
// monster every new-stack session's world is seeded with, away from the
// party's entry wall so joining does not itself start the fight.
const builtInMonsterID = "skeleton-1"

var builtInMonsterPosition = spatial.Position{X: 9, Y: 5}

// startEncounterOnSessionStack is StartEncounter's new-stack branch,
// self-contained on purpose (its own lock/load/validate, not shared with
// the old path's) so a change to one can never silently affect the other --
// the same separation design rule 9 requires between SessionService and the
// old encounter stack, applied here to the two STACKS rather than the two
// SERVICES.
func (o *Orchestrator) startEncounterOnSessionStack(ctx context.Context, in *StartEncounterInput) (*StartEncounterOutput, error) {
	unlock := o.locks.Lock(in.LobbyID)
	defer unlock()

	data, err := o.lobbyRepo.Get(ctx, in.LobbyID)
	if err != nil {
		if errors.Is(err, lobbyrepo.ErrNotFound) {
			return nil, ErrLobbyNotFound
		}
		return nil, fmt.Errorf("load lobby %q: %w", in.LobbyID, err)
	}
	if data.Status == lobbyrepo.StatusStarted {
		return nil, ErrLobbyAlreadyStarted
	}
	if data.HostPlayerID != in.PlayerID {
		return nil, ErrNotHost
	}
	members := orderedMembers(data)
	for _, m := range members {
		if !m.IsReady {
			return nil, ErrNotAllReady
		}
	}

	encID := o.encounterIDGen.Generate()

	world, err := builtInSessionWorld()
	if err != nil {
		return nil, err
	}
	if _, err := o.sessionManager.StartSession(ctx, &sdk.StartSessionInput{
		Session: encID, Encounter: encID, World: world,
	}); err != nil {
		return nil, fmt.Errorf("start session %q on new stack: %w", encID, err)
	}

	positions := builtInPartyPositions(len(members))
	for i, m := range members {
		if _, err := o.sessionManager.Join(ctx, &sdk.JoinInput{
			Session: encID, Member: m.CharacterID, Position: positions[i],
		}); err != nil {
			return nil, fmt.Errorf("join %q to session %q on new stack: %w", m.CharacterID, encID, err)
		}
	}

	if _, err := o.sessionManager.Spawn(ctx, &sdk.SpawnInput{
		Session: encID, ID: builtInMonsterID, Ref: refs.Monsters.Skeleton().String(),
		Position: builtInMonsterPosition,
	}); err != nil {
		return nil, fmt.Errorf("spawn monster into session %q on new stack: %w", encID, err)
	}

	data.Status = lobbyrepo.StatusStarted
	data.EncounterID = encID
	if err := o.lobbyRepo.Save(ctx, data); err != nil {
		return nil, fmt.Errorf("save lobby %q: %w", in.LobbyID, err)
	}

	o.lobbyBroker.Publish(in.LobbyID, &Event{
		Kind:             EventKindEncounterStarted,
		EncounterStarted: &EncounterStartedPayload{EncounterID: encID},
	})

	return &StartEncounterOutput{EncounterID: encID}, nil
}
