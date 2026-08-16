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
		errors.Is(err, sdk.ErrNoSheet):
		return status.Error(codes.NotFound, err.Error())

	// INVALID_ARGUMENT -- the request itself is malformed: bad shape, bad
	// geometry, or a required field left empty. ErrNoCrossing (added at
	// session v0.12.0, rpg-toolkit#1048): a Move path steps into another
	// room with no doorway joining the two cells -- the caller's own path
	// computation was wrong, same bucket as ErrBrokenPath/ErrBadPosition.
	case errors.Is(err, sdk.ErrNilInput),
		errors.Is(err, sdk.ErrEmptyPath),
		errors.Is(err, sdk.ErrBrokenPath),
		errors.Is(err, sdk.ErrNoCrossing),
		errors.Is(err, sdk.ErrBadPosition),
		errors.Is(err, sdk.ErrNoRef),
		errors.Is(err, sdk.ErrBadRef),
		errors.Is(err, sdk.ErrNoMemberID),
		errors.Is(err, sdk.ErrNoSessionID),
		errors.Is(err, sdk.ErrNoEncounterID),
		errors.Is(err, sdk.ErrInvalidWorld),
		errors.Is(err, sdk.ErrNoCause):
		return status.Error(codes.InvalidArgument, err.Error())

	// FAILED_PRECONDITION -- the request is well-formed but the world's
	// current state refuses it (wrong clock, wrong actor, wrong equipment).
	case errors.Is(err, sdk.ErrInBubble),
		errors.Is(err, sdk.ErrNotInFight),
		errors.Is(err, sdk.ErrClosed),
		errors.Is(err, sdk.ErrNotACharacter),
		errors.Is(err, sdk.ErrBadAttack):
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
	// never have reached a running verb in the first place. ErrNoConnection
	// belongs here as of session v0.12.0, not with the caller-facing
	// sentinels above: no verb takes a connection ID any more
	// (rpg-toolkit#1048 retired Traverse) -- a caller names cells, and the
	// package finds the doorway joining them itself. Its own doc now says
	// so plainly: if this appears, the package derived a crossing the
	// composition then rejected, which is a defect HERE, not in the call.
	case errors.Is(err, sdk.ErrBadRepository),
		errors.Is(err, sdk.ErrBadCharacter),
		errors.Is(err, sdk.ErrInvalidSession),
		errors.Is(err, sdk.ErrNilConfig),
		errors.Is(err, sdk.ErrIncompleteConfig),
		errors.Is(err, sdk.ErrNoConnection):
		return status.Error(codes.Internal, err.Error())

	default:
		return status.Error(codes.Internal, err.Error())
	}
}
