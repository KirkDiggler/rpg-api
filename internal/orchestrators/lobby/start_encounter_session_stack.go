package lobby

import (
	"context"
	"errors"
	"fmt"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/npcs"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
)

// DungeonKey selects a registered dungeon by the key its file names itself
// with (the `key:` line; internal/dungeons). Empty means
// dungeons.DefaultKey.
type DungeonKey string

// StartEncounterInput carries the entity-typed StartEncounter request.
type StartEncounterInput struct {
	// PlayerID is the authenticated caller. Must be the lobby's host.
	PlayerID string
	LobbyID  string

	// DungeonKey is the proto's dungeon_key field (rpg-api#688). Empty
	// plays dungeons.DefaultKey (the reference tomb); a key the registry
	// does not have is ErrDungeonNotFound, never a silent fallback
	// (design.md §3c, rpg-project#256).
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
// # The party plays the dungeon the host picked
//
// The world comes from the content registry (internal/dungeons): every file
// under RPG_CONTENT_DIR, compiled once at boot through internal/sessionworld
// on rpg-toolkit's rulebooks/dnd5e/encounter/dungeonspec, plus whatever the
// AuthoringService has Put since. dungeon_key picks one; empty picks the
// reference tomb; unknown is refused. ListDungeons reads the same registry,
// which is how the picker and this call can never disagree about what
// exists (rpg-api#806, rpg-project#256).
//
// # What is still narrow here, stated rather than implied
//
//   - NO MONSTER BEHAVIOR. session.Spawn takes no decider, by its own
//     design ("behavior arrives with the wave that brings it"), so the
//     garrison is placed, perceived and remembered correctly and does
//     not act.
//   - The authored endings are BOTH declared now (rpg-project#268): the
//     party withdrawing (sessionworld.EndingWithdrawn, external) and the
//     boss going down (sessionworld.EndingBossDown, TriggerMemberDown over
//     the member ID this launch spawns the flagged placement under).
//   - FIRST ADMISSION RECOVERY is provider-owned. StartEncounter supplies
//     member IDs and authored seats to sdk.Manager.Join; the Session SDK uses
//     its persisted EverMembers admission record and saves the normal-rested
//     character before projection and placement. This host neither loads a
//     runtime D&D character nor authors lifecycle policy. Stale ACTION-ECONOMY
//     is likewise mostly the SDK's own problem now: since session v0.24.1 it
//     clears a member's economy when their fight dissolves (rpg-toolkit#1222);
//     only sessions that end while a fight is still running — Manager.End /
//     Manager.Exit — can still leak one into a later encounter
//     (rpg-toolkit#1223).
//
// # One thing that is NOT a shortcut any more
//
// Nothing in this file supplies a capability: the construction-time ones
// live in internal/sessionworld, and the session package supplies its own —
// including the sight range that decides who is in contact — when it loads
// the world.
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

	key := string(in.DungeonKey)
	if key == "" {
		key = dungeons.DefaultKey
	}
	entry, err := o.dungeons.Get(ctx, key)
	if err != nil {
		if errors.Is(err, dungeons.ErrNotFound) {
			return nil, fmt.Errorf("dungeon %q: %w", key, ErrDungeonNotFound)
		}
		return nil, fmt.Errorf("load dungeon %q: %w", key, err)
	}
	dungeon := entry.Dungeon
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
		_, err := o.sessionManager.Spawn(ctx, &sdk.SpawnInput{
			Session: encID, ID: monster.MemberID, Ref: monster.Ref, Position: monster.At,
		})
		if err != nil {
			return nil, fmt.Errorf("spawn %q into session %q on new stack: %w", monster.MemberID, encID, err)
		}
	}

	// TEMPORARY (rpg-api#903 Phase 1): one hardcoded demo vendor, placed one
	// hex step from the party's own entry seat so a player can Interact with
	// it at Range 0 (adjacent, the default) right after joining. This is a
	// placement GATE, not the real thing — proving PlaceNPC/Interact work
	// end to end before any authoring format exists. It is headed for the
	// dungeon's own `place:` list (a routed `npcs` ref type, rpg-api#903
	// Phase 2) and should be removed, not built on top of, once that lands.
	//
	// The cell is a genuine axial hex-neighbor of dungeon.PartySeats[0]
	// (verified directly against the compiled tomb, not derived from its
	// authored offset coordinates -- offset-to-axial is a sheared
	// conversion, and two offset cells that look adjacent on the page are
	// not reliably adjacent on the hex grid). Gated to the reference tomb
	// specifically, by key -- not "every dungeon": this cell is only
	// known-floor there. Other dungeons (including the small synthetic
	// fixtures dungeonstest builds for unrelated tests) have no reason to
	// share that geometry.
	if key == dungeons.DefaultKey {
		demoVendor, err := npcs.NewMerchant(nil)
		if err != nil {
			return nil, fmt.Errorf("build demo vendor for session %q: %w", encID, err)
		}
		demoVendorPosition := spatial.Position{X: dungeon.PartySeats[0].X + 1, Y: dungeon.PartySeats[0].Y}
		if _, err := o.sessionManager.PlaceNPC(ctx, &sdk.PlaceNPCInput{
			Session: encID, Member: "demo-merchant-1", Position: demoVendorPosition,
			NPC: demoVendor.NPC().ToData(),
		}); err != nil {
			return nil, fmt.Errorf("place demo vendor into session %q on new stack: %w", encID, err)
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
