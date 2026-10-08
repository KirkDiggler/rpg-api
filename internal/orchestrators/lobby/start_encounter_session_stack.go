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

// demoVendorMemberID is the member id of the TEMPORARY demo vendor placed on
// the reference tomb (see the block in StartEncounter). Named so the
// launch's one-id-one-member check can count it.
const demoVendorMemberID = "demo-merchant-1"

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
// # The launch is one SDK verb
//
// session.Manager.Launch takes the compiled dungeon (dungeonspec's own
// output, monster member ids minted by the compile) and the party in seat
// order, and places the whole board in one load-act-save (rpg-project#542):
// the first-admission long rest, the seats, every authored monster with its
// faction, holdings, arms, social prices, table and arrival, the party on the
// dungeon's seats, the endings (withdrawn, boss-down, every bound scenario's
// and the authored ones), and then ONE look that forms the fight with
// everybody in it. This host re-projects none of that, and checks none of
// it: a party too big for the seats, an id claimed twice and an unresolvable
// monster are Launch's refusals, made before anything is written.
//
// The demo vendor is the one thing still placed here, as its own PlaceNPC
// call after the launch (R11: authored world NPCs are deferred).
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

	party := make([]string, len(members))
	for i, m := range members {
		party[i] = m.CharacterID
	}
	encID := o.encounterIDGen.Generate()

	// ONE CALL PLACES THE WHOLE BOARD (rpg-project#542, R7). Launch is one
	// load-act-save under the session's guard and each party character's
	// guard: it refuses before anything is written when the party outnumbers
	// the seats, an id is claimed twice, or a sheet, monster or faction cannot
	// be resolved; it rests and seats every party member, places the garrison
	// and then the party, and lets the fight form once over the finished
	// board. The ordering, id and ending rules this function used to carry
	// (garrison first, one id one member, withdrawn/boss-down) are the SDK's,
	// and the monster member ids are the compile's own.
	if _, err := o.sessionManager.Launch(ctx, &sdk.LaunchInput{
		Session: encID,
		// The RESOLVED key, after the default fallback above -- what this
		// launch actually loaded (rpg-project#479). It reaches a client on
		// GetAtlasResponse.dungeon_key, which is how a play view fetches the
		// room's appearance from the same registry entry.
		DungeonKey: key,
		Dungeon:    dungeon.Spec,
		Party:      party,
	}); err != nil {
		return nil, fmt.Errorf("launch session %q: %w", encID, err)
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
			Session: encID, Member: demoVendorMemberID, Position: demoVendorPosition,
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
