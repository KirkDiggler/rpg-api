// Package sessionv1alpha1 is the wire form of the toolkit's
// rulebooks/dnd5e/session SDK: pure proto <-> SDK translation, per
// rpg-project/ideas/session-api/design.md §3. No rule lives here (design
// rule 8, the Boundary Rule) -- every handler loads nothing, decides
// nothing, and calls exactly one Manager verb.
package sessionv1alpha1

import (
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

// statusError is the ONE tested error-translation table design rule 7
// requires: every exported sentinel in rulebooks/dnd5e/session/errors.go
// maps to a gRPC code here, and nowhere else in this service. See
// errors_test.go for the sentinel-by-sentinel proof and the count that pins
// this table to the SDK's actual vocabulary.
//
// sdk.ErrNotFound is deliberately handled by the default case rather than
// its own: it is the SDK's REPOSITORY-facing contract sentinel (what a
// SessionRepository/EncounterRepository/CharacterRepository implementation
// returns for a missing id), and the Manager translates it into a
// caller-facing sentinel (ErrNoSession, ErrNoEncounter, ErrNoCharacter, ...)
// before any verb returns (session.ErrNotFound's own doc: "translated from a
// repository's ErrNotFound so hosts match on one vocabulary rather than
// two"). It should never reach a handler; if it somehow does, reporting it
// as Internal is correct -- it signals a storage-layer bug, not a caller
// mistake -- and matches the same-bucket sentinels below rather than
// requiring a distinct case that implies it is expected here.
func statusError(err error) error {
	if err == nil {
		return nil
	}

	switch {
	// NOT_FOUND -- the caller named something that does not exist.
	case errors.Is(err, sdk.ErrNoSession),
		errors.Is(err, sdk.ErrNoEncounter),
		errors.Is(err, sdk.ErrNoCharacter),
		errors.Is(err, sdk.ErrNoMember),
		errors.Is(err, sdk.ErrNoEnding),
		errors.Is(err, sdk.ErrUnknownContent),
		errors.Is(err, sdk.ErrNoLoader),
		errors.Is(err, sdk.ErrNoSheet),
		// ErrNoConnection rejoined the caller-facing set with the door verbs
		// (rpg-project#268): GetDoors/OpenDoor/Unlock name a door, so a door
		// the dungeon does not have is a caller's NotFound again -- see the
		// INTERNAL bucket's note for the years it spent there.
		errors.Is(err, sdk.ErrNoConnection):
		return status.Error(codes.NotFound, err.Error())

	// INVALID_ARGUMENT -- the request itself is malformed: bad shape, bad
	// geometry, an omitted opaque selector, or a required field left empty.
	//
	// ErrNoCrossing was here until session/v0.18.0 DELETED it. On one canvas
	// the composition no longer distinguishes a walled crossing from a missing
	// cell, so nothing could still produce it. A walk stopped by a wall now
	// arrives as ErrBadPosition, which is in this same bucket -- so the code a
	// client sees does not change, only the sentence. The locked-door half of
	// that collapse is repaired: a walk into a locked or shut door arrives on
	// its own sentinel now (rpg-toolkit#1135) and lands in
	// FAILED_PRECONDITION below, so the tomb's most player-visible beat no
	// longer reads as a client bug.
	case errors.Is(err, sdk.ErrNilInput),
		errors.Is(err, sdk.ErrEmptyPath),
		errors.Is(err, sdk.ErrBrokenPath),
		errors.Is(err, sdk.ErrBadPosition),
		errors.Is(err, sdk.ErrNoRef),
		errors.Is(err, sdk.ErrBadRef),
		errors.Is(err, sdk.ErrNoMemberID),
		errors.Is(err, sdk.ErrNoSessionID),
		errors.Is(err, sdk.ErrNoEncounterID),
		errors.Is(err, sdk.ErrInvalidWorld),
		errors.Is(err, sdk.ErrNoCause),
		errors.Is(err, sdk.ErrNoDeclarationID):
		return status.Error(codes.InvalidArgument, err.Error())

	// FAILED_PRECONDITION -- the request is well-formed but the world's
	// current state refuses it (wrong clock, wrong actor, wrong equipment).
	//
	// Three arrive with session v0.15.0-v0.17.0 and all three are this bucket
	// by the SDK's own description of them:
	//
	//   ErrDowned -- "the world is fine, the member is there, and this
	//   particular member cannot do this particular thing." Explicitly NOT
	//   no-such-member: a downed member stays on the map, in the roster, and
	//   readable. NotFound would be a lie about where they are.
	//
	//   ErrLocked -- a locked door refusing. World state, not a malformed
	//   request, and reachable two ways now: OpenDoor's own refusal, and a
	//   walk into one (rpg-toolkit#1135 landed with the door verbs,
	//   rpg-project#268). The message names the DC -- the refusal is the
	//   fiction beat.
	//
	//   ErrDoorShut -- the other half of the #1135 split: a walk into a door
	//   that is merely closed. The remedy is OpenDoor, not new coordinates.
	//
	//   ErrCannotAfford -- a second swing in a turn that bought one. The SDK
	//   is emphatic that this is A FACT ABOUT THE GAME rather than about the
	//   code, split from ErrBadCost precisely so a player who has run out of
	//   actions hears a different sentence from the one a developer needs. Its
	//   message names the currency that ran out; a host may show it or match
	//   the sentinel and say it in its own words.
	//
	//   Two more join with the combat-turn contract (rpg-project#249), and both
	//   are the same shape as the three above -- a well-formed swing the
	//   current world state refuses, never a caller defect:
	//
	//   ErrNotYourTurn -- Attack, Move and EndTurn all refuse it the same way:
	//   the fight is real, the member is real, it simply is not their turn yet.
	//   Afford's per-target declarations announce this before a client ever
	//   sends the swing that would hit it (Declaration.why.reason
	//   NOT_YOUR_TURN).
	//
	//   ErrOutOfReach (rpg-toolkit#1010) -- the final defensive resolution
	//   check found the selected target beyond the compiled attack's reach.
	//   Afford reports each ruled candidate's own availability and why before
	//   execution; the API copies those rows without recomputing reach.
	//
	//   ErrStaleDeclaration -- an echoed opaque selector no longer names the
	//   regenerated available offer. The request is shaped correctly, but the
	//   world changed since Afford, so retry starts from a fresh declaration.
	case errors.Is(err, sdk.ErrInBubble),
		errors.Is(err, sdk.ErrNotInFight),
		errors.Is(err, sdk.ErrClosed),
		errors.Is(err, sdk.ErrNotACharacter),
		errors.Is(err, sdk.ErrBadAttack),
		errors.Is(err, sdk.ErrDowned),
		errors.Is(err, sdk.ErrLocked),
		errors.Is(err, sdk.ErrDoorShut),
		errors.Is(err, sdk.ErrCannotAfford),
		errors.Is(err, sdk.ErrNotYourTurn),
		errors.Is(err, sdk.ErrOutOfReach),
		errors.Is(err, sdk.ErrStaleDeclaration):
		return status.Error(codes.FailedPrecondition, err.Error())

	// ALREADY_EXISTS
	case errors.Is(err, sdk.ErrSessionExists):
		return status.Error(codes.AlreadyExists, err.Error())

	// OUT_OF_RANGE -- Story's resume point aged out of the retention window.
	// The fix is deterministic (resync from zero, design rule 6) rather than
	// a state the caller can wait out, which is what sets this apart from
	// FAILED_PRECONDITION.
	case errors.Is(err, sdk.ErrStoryTrimmed):
		return status.Error(codes.OutOfRange, err.Error())

	// UNAVAILABLE -- a write landed only partially. Retry guidance lives in
	// the SaveReport carried by the SDK's SaveError (design rule 7); errors.As
	// against it is the caller's way to recover which aggregates landed.
	case errors.Is(err, sdk.ErrSaveFailed):
		return status.Error(codes.Unavailable, err.Error())

	// INTERNAL -- storage-side integrity problems, not a caller mistake:
	// stored bytes this module could not have produced, a repository that
	// violated its own contract, or a construction-time failure that should
	// never have reached a running verb in the first place.
	//
	// ErrNoConnection LEFT this bucket with the door verbs (rpg-project#268):
	// it sat here while no verb took a connection ID (rpg-toolkit#1048), and
	// GetDoors/OpenDoor/Unlock name a door again -- a caller naming one the
	// dungeon does not have is a caller mistake, NOT_FOUND above.
	//
	// ErrBadCost joins them at session/v0.17.0: the PROGRAMMER-FACING half of
	// the split ErrCannotAfford describes. It means content or wiring is wrong
	// -- a price keyed to a currency no ledger holds -- and the SDK is explicit
	// that reporting it as "out of actions" would send whoever debugs it to
	// exactly the wrong place. Not reachable from a well-formed call today; it
	// is mapped so that the day something else compiles a price, the failure
	// has a name that is not a lie.
	//
	// ErrBadTurnOutcome is the same shape again: a TurnDriver answered with a
	// TurnOutcome the composition's adapter does not recognize, which is this
	// package's OWN vocabulary going stale against itself, not a caller
	// mistake. Already present in the pinned SDK (v0.21.4) and unmapped here
	// until this audit -- picked up in the same pass as the combat-turn rows
	// above rather than left for the day it actually fires.
	case errors.Is(err, sdk.ErrBadRepository),
		errors.Is(err, sdk.ErrBadCost),
		errors.Is(err, sdk.ErrBadCharacter),
		errors.Is(err, sdk.ErrInvalidSession),
		errors.Is(err, sdk.ErrNilConfig),
		errors.Is(err, sdk.ErrIncompleteConfig),
		errors.Is(err, sdk.ErrBadTurnOutcome):
		return status.Error(codes.Internal, err.Error())

	default:
		return status.Error(codes.Internal, err.Error())
	}
}
