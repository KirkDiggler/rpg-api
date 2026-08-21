package lobby

import (
	"context"
	"errors"
	"fmt"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
)

// DungeonKey selects a named dungeon specification. Carried on
// StartEncounterInput for parity with the proto's dungeon_key field; this
// package's sole StartEncounter implementation does not consult it (see
// its own doc comment below).
type DungeonKey string

// StartEncounterInput carries the entity-typed StartEncounter request.
type StartEncounterInput struct {
	// PlayerID is the authenticated caller. Must be the lobby's host.
	PlayerID string
	LobbyID  string

	// DungeonKey mirrors the proto's dungeon_key field (rpg-api#688,
	// rpg-api-protos v0.1.115). Unused today: this, the session stack's
	// sole remaining StartEncounter implementation, always plays the one
	// embedded reference-tomb dungeon regardless of what's supplied here
	// — see this file's package-level doc for the content-authoring gap
	// that leaves open. Kept on the struct rather than dropped so the
	// handler's proto-to-input translation stays a straight field copy,
	// and so a future content-key-aware session-stack path has somewhere
	// to land without a handler change.
	DungeonKey DungeonKey
}

// StartEncounterOutput carries the freshly constructed encounter's ID.
// Clients drop the lobby stream and subscribe to the session stream on
// receipt of the parallel EncounterStarted broadcast.
type StartEncounterOutput struct {
	EncounterID string
}

// StartEncounter is the lobby -> encounter seam (design rpg-project/ideas/
// session-api/design.md §3), and now the session stack's ONLY
// implementation — the old encounter stack (github.com/KirkDiggler/
// rpg-toolkit/encounter) was removed in rpg-project#227, so there is no
// second branch to coexist with any more. Host-only, all-ready gated,
// atomic member-set snapshot (guarded by the per-lobby lock so a racing
// LeaveLobby lands either before this snapshot — member excluded — or
// after — FailedPrecondition, lobby-surface.md "Start/leave atomicity").
//
// # The party plays the reference tomb
//
// This path used to seed every session from a single hardcoded room,
// because there was no authored-content compiler for the new stack and
// building one here would have been rpg-api inventing dungeon geometry.
// That gap CLOSED: rpg-toolkit#1133 shipped rulebooks/dnd5e/encounter/
// dungeonspec, whose own forcing case is the sentence this path
// implements — reference-tomb.yaml compiles into a runnable world and a
// player walks entrance -> hall -> tomb. internal/sessionworld holds the
// compile and the one seam the compiler leaves to a host; see its package
// comment for what that seam is and why it borrows the projection
// instead of doing arithmetic.
//
// So a session now opens in three chambers with walls between them, a
// garrison of skeletons holding the hall, and a captain behind a door
// locked at DC 12 — a fixed, authored dungeon compiled fresh per call.
//
// # What is still narrow here, stated rather than implied
//
//   - ONE DUNGEON. StartEncounterInput.DungeonKey is carried on the
//     struct (proto parity) but ignored — every call plays the tomb. The
//     old-dialect authoring path (PutDungeon, internal/dungeonregistry,
//     internal/orchestrators/authoring) and the old-dialect ListDungeons
//     RPC were deleted alongside the rest of the old encounter stack
//     (rpg-project#227) rather than kept alive for a compiler nothing
//     else uses any more — a content-key-aware session-stack path (and
//     whatever replaces ListDungeons/PutDungeon against the NEW
//     rulebooks/dnd5e/encounter/dungeonspec compiler) is the next
//     content-side piece of work, not something this rip-out papers
//     over. LobbyService.ListDungeons is Unimplemented until then (its
//     embedded UnimplementedLobbyServiceServer covers it — no orchestrator
//     method exists for it any more).
//   - NO MONSTER BEHAVIOR. session.Spawn takes no decider, by its own
//     design ("behavior arrives with the wave that brings it"), so the
//     garrison is placed, perceived and remembered correctly and does
//     not act.
//   - NO AUTHORED ENDING BEYOND WITHDRAWAL. See sessionworld.EndingWithdrawn:
//     the composition has no "the boss died" trigger to declare, so the
//     boss flag is carried and unused.
//   - NO ARCADE RECOVERY / STALE-ACTION-ECONOMY RESET. The old stack's
//     StartEncounter called tkcharacter.RestoreForNewEncounter and cleared
//     a stale in-combat action economy before seating each member
//     (character.go's since-removed seedMemberCombatSnapshot); nothing in
//     this path or in sdk.Manager.Join's own documented contract performs
//     an equivalent today, so a character who died (or was mid-turn) in a
//     PRIOR encounter joins a fresh one exactly as their stored record
//     left them. Not ported here — deciding where this belongs (the
//     toolkit's Join, or an explicit rpg-api call before it) is a design
//     question, not a mechanical port; flagged, not silently dropped.
//
// # One thing that is NOT a shortcut any more
//
// The old placeholder needed an InitiativeRoller and could only supply the
// toolkit's own test fake, which meant a fight in it resolved turn order
// party-then-monster. Nothing in this file supplies one: the capabilities
// are construction-time only and live in internal/sessionworld, and the
// session package supplies its own — including the sight range that
// decides who is in contact — when it loads the world.
func (o *Orchestrator) StartEncounter(ctx context.Context, in *StartEncounterInput) (*StartEncounterOutput, error) {
	if in == nil {
		return nil, errors.New("lobby orchestrator: StartEncounterInput is required")
	}

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
