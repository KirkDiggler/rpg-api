package lobby

import (
	"context"
	"errors"
	"fmt"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
)

// StartEncounter's new-session-stack path (design rpg-project/ideas/session-
// api/design.md §3 "coexistence"): server configuration selects EXACTLY ONE
// stack per StartEncounter call, never both. This file is the new stack's
// half; start_encounter.go's existing body is the old stack's, completely
// untouched by anything here.
//
// # The party plays the reference tomb
//
// This path used to seed every session from a single hardcoded room, because
// there was no authored-content compiler for the new stack and building one
// here would have been rpg-api inventing dungeon geometry. That gap CLOSED:
// rpg-toolkit#1133 shipped rulebooks/dnd5e/encounter/dungeonspec, whose own
// forcing case is the sentence this path now implements -- reference-tomb.yaml
// compiles into a runnable new-stack world and a player walks entrance → hall →
// tomb. internal/sessionworld holds the compile and the one seam the compiler
// leaves to a host; see its package comment for what that seam is and why it
// borrows the projection instead of doing arithmetic.
//
// So a new-stack session now opens in three chambers with walls between them, a
// garrison of skeletons holding the hall, and a captain behind a door locked at
// DC 12 -- the same dungeon the old stack has always served, compiled by the new
// one.
//
// # What is still narrow here, stated rather than implied
//
//   - ONE DUNGEON. StartEncounterInput.DungeonKey selects content on the old
//     path; this one ignores it and always plays the tomb. That is a smaller
//     narrowing than it looks -- no proto field carries a key, so every real
//     call leaves it at the zero value and the old path serves exactly one
//     dungeon too -- but it is a narrowing, and the authoring path
//     (PutDungeon → dungeonregistry) still writes the OLD dialect, so the
//     dungeon builder cannot yet author for this stack. That is the next
//     content-side piece of work, not something to paper over here.
//   - NO MONSTER BEHAVIOR. session.Spawn takes no decider, by its own design
//     ("behavior arrives with the wave that brings it"), so the garrison is
//     placed, perceived and remembered correctly and does not act.
//   - NO AUTHORED ENDING BEYOND WITHDRAWAL. See sessionworld.EndingWithdrawn:
//     the composition has no "the boss died" trigger to declare, so the boss
//     flag is carried and unused.
//
// # One thing that is NOT a shortcut any more
//
// The old placeholder needed an InitiativeRoller and could only supply the
// toolkit's own test fake, which meant a fight in it resolved turn order
// party-then-monster. Nothing in THIS file supplies one: the capabilities are
// construction-time only and live in internal/sessionworld, and the session
// package supplies its own -- including the sight range that decides who is in
// contact -- when it loads the world. The honest remaining gap is upstream, and
// unchanged: the toolkit still exposes no injectable real (dice + ability
// score) InitiativeRoller, only the interface.

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

	dungeon, err := sessionworld.ReferenceTomb()
	if err != nil {
		return nil, err
	}
	// Checked BEFORE anything is written, so a party too big for the dungeon's
	// entrance is refused rather than half-seated: the alternative is a session
	// that exists with some members in it and an error returned, which is the
	// one outcome a caller cannot recover from.
	if len(members) > len(dungeon.PartySeats) {
		return nil, fmt.Errorf("lobby %q has %d members and the dungeon seats %d",
			in.LobbyID, len(members), len(dungeon.PartySeats))
	}

	encID := o.encounterIDGen.Generate()

	if _, err := o.sessionManager.StartSession(ctx, &sdk.StartSessionInput{
		Session: encID, Encounter: encID, World: dungeon.World,
	}); err != nil {
		return nil, fmt.Errorf("start session %q on new stack: %w", encID, err)
	}

	for i, m := range members {
		if _, err := o.sessionManager.Join(ctx, &sdk.JoinInput{
			Session: encID, Member: m.CharacterID, Position: dungeon.PartySeats[i],
		}); err != nil {
			return nil, fmt.Errorf("join %q to session %q on new stack: %w", m.CharacterID, encID, err)
		}
	}

	for _, monster := range dungeon.Monsters {
		if _, err := o.sessionManager.Spawn(ctx, &sdk.SpawnInput{
			Session: encID, ID: monster.MemberID, Ref: monster.Ref, Position: monster.At,
		}); err != nil {
			return nil, fmt.Errorf("spawn %q into session %q on new stack: %w", monster.MemberID, encID, err)
		}
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
