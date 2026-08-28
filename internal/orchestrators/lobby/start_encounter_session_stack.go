package lobby

import (
	"context"
	"errors"
	"fmt"

	tkchar "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	lobbyrepo "github.com/KirkDiggler/rpg-api/internal/repositories/lobby"
	rosterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/roster"
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
//   - ARCADE RECOVERY IS BACK, and it lives HERE (rpg-api#828 closed the
//     design question the rip-out left open): StartEncounter calls
//     tkcharacter.RestoreForLaunch on every member before seating —
//     the toolkit owns the mechanism, this host path owns the policy, and
//     sdk.Manager.Join stays restoration-free by contract. Stale
//     ACTION-ECONOMY is likewise mostly the SDK's own problem now: since
//     session v0.24.1 it clears a member's economy when their fight
//     dissolves (rpg-toolkit#1222); only sessions that end while a fight
//     is still running — Manager.End / Manager.Exit — can still leak one
//     into a later encounter (rpg-toolkit#1223).
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

	// Launch is an arcade run start (Kirk's ruling, rpg-project#253 /
	// rpg-api#828): every member is seated fully restored — max HP, no
	// death-save state, no lingering Unconscious, full resource pools —
	// via the toolkit's own launch-only mechanism. Done BEFORE the session
	// exists so a storage failure refuses the launch cleanly, and before
	// Join so the SDK seats the restored record. Mid-run reloads must
	// never do this; see RestoreForLaunch's contract.
	// Two phases: load-and-restore every record in memory FIRST (so a
	// missing or malformed character refuses the launch before anything is
	// written), then persist. A failure mid-persist still leaves earlier
	// members durably restored with no session started — defined behavior,
	// not a bug: RestoreForLaunch is idempotent and only ever moves a
	// record toward the exact state the next successful launch wants, so a
	// retry (or a launch of a different lobby) re-runs it as a no-op.
	restored := make([]*entities.Character, 0, len(members))
	for _, m := range members {
		got, err := o.characterRepo.Get(ctx, characterrepo.GetInput{ID: m.CharacterID})
		if err != nil {
			return nil, fmt.Errorf("load character %q for launch restore: %w", m.CharacterID, err)
		}
		if got == nil || got.Character == nil || got.Character.Data == nil {
			return nil, fmt.Errorf("character %q has no data to restore at launch", m.CharacterID)
		}
		if tkchar.RestoreForLaunch(got.Character.Data) {
			restored = append(restored, got.Character)
		}
	}
	for _, ch := range restored {
		if _, err := o.characterRepo.Update(ctx, characterrepo.UpdateInput{Character: ch}); err != nil {
			return nil, fmt.Errorf("persist launch restore for character %q: %w", ch.Data.ID, err)
		}
	}

	encID := o.encounterIDGen.Generate()

	if _, err := o.sessionManager.StartSession(ctx, &sdk.StartSessionInput{
		Session: encID, Encounter: encID, World: dungeon.World,
	}); err != nil {
		return nil, fmt.Errorf("start session %q on new stack: %w", encID, err)
	}

	// The roster row is built as launch does its work — this is the only
	// moment that knows every member and every authored spawn at once
	// (rpg-project#264, ideas/characters/presentation). Identity facts only:
	// a player row is just the id (their name and refs are read fresh from
	// the character record at GetRoster time); a monster row carries the
	// authored ref and name exactly as the spawn reports them.
	rosterRows := make([]rosterrepo.Member, 0, len(members)+len(dungeon.Monsters))

	for i, m := range members {
		if _, err := o.sessionManager.Join(ctx, &sdk.JoinInput{
			Session: encID, Member: m.CharacterID, Position: dungeon.PartySeats[i],
		}); err != nil {
			return nil, fmt.Errorf("join %q to session %q on new stack: %w", m.CharacterID, encID, err)
		}
		rosterRows = append(rosterRows, rosterrepo.Member{
			ID: m.CharacterID, Kind: rosterrepo.KindPlayer,
		})
	}

	for _, monster := range dungeon.Monsters {
		spawned, err := o.sessionManager.Spawn(ctx, &sdk.SpawnInput{
			Session: encID, ID: monster.MemberID, Ref: monster.Ref, Position: monster.At,
		})
		if err != nil {
			return nil, fmt.Errorf("spawn %q into session %q on new stack: %w", monster.MemberID, encID, err)
		}
		// The authored display name, from the spawn's own report rather than
		// re-derived here — no drift with what sightings will call it. A
		// successful Spawn that reports no NPC state would break that
		// contract, so it fails the launch loudly instead of seating an
		// unnamed monster (Copilot, PR #838).
		if spawned == nil || spawned.NPC == nil {
			return nil, fmt.Errorf("spawn %q into session %q reported no NPC state", monster.MemberID, encID)
		}
		rosterRows = append(rosterRows, rosterrepo.Member{
			ID: monster.MemberID, Kind: rosterrepo.KindMonster, Ref: monster.Ref,
			Name: spawned.NPC.Name,
		})
	}

	if err := o.rosterRepo.Save(ctx, &rosterrepo.Data{
		EncounterID: encID, Members: rosterRows,
	}); err != nil {
		return nil, fmt.Errorf("save roster for session %q: %w", encID, err)
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
