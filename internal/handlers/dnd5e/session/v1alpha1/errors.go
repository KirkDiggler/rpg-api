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
		errors.Is(err, sdk.ErrNoConnection),
		// ErrNoProp (rpg-project#368, Hold) is ErrNoConnection's shape one
		// placement kind over -- a caller naming a thing the dungeon does
		// not have -- and it is ALSO the probe-law collapse sentinel, which
		// is what makes NOT_FOUND the right answer rather than an accident:
		// for a prop standing in space the member cannot see, the
		// composition returns THIS sentinel for every refusal it could have
		// made (no such prop, not holdable, already held, out of range), so
		// all four arrive here as one sentinel and leave as one code
		// carrying one sentence. A guessed id cannot map a room nobody has
		// found, because the answer is byte-identical to the one a
		// nonexistent id gets.
		//
		// The other three sentinels below it in FAILED_PRECONDITION are
		// reachable only for a prop the member CAN see, where refusing by
		// name costs nothing: there is no secret in a pillar.
		errors.Is(err, sdk.ErrNoProp):
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
		errors.Is(err, sdk.ErrNoDeclarationID),
		// ErrBadActivation joins at session/v0.34.0 (rpg-project#300), and it
		// is here rather than in Internal because a CALLER can produce it: an
		// activation naming a target for an ability that takes none. The SDK
		// refuses that rather than ignoring it, precisely so a client that
		// believes it aimed Dodge at somebody is told, and a value quietly
		// dropped never becomes a disagreement nobody finds.
		errors.Is(err, sdk.ErrBadActivation):
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
	//   Afford's one declaration with candidate rows announces this before a
	//   client ever sends the swing that would hit it
	//   (Declaration.why.reason NOT_YOUR_TURN).
	//
	//   ErrOutOfReach (rpg-toolkit#1010) -- the final defensive resolution
	//   check found the selected target beyond the compiled attack's reach.
	//   Afford reports each ruled candidate's own availability and why before
	//   execution; the API copies those rows without recomputing reach.
	//
	//   ErrStaleDeclaration -- an echoed opaque selector no longer names the
	//   regenerated available offer. The request is shaped correctly, but the
	//   world changed since Afford, so retry starts from a fresh declaration.
	//
	//   ErrElsewhere (rpg-project#350, Search) -- the named region is not the
	//   one the searcher currently stands in. The request itself is
	//   well-formed (a real region id, a real member); it is the world's
	//   present state -- where this member happens to be standing -- that
	//   refuses it, the same shape as ErrNotACharacter next to it. Not
	//   NotFound: the SDK deliberately returns this SAME sentinel whether the
	//   named region is real-but-elsewhere or does not exist at all, so a
	//   distinct NotFound here would hand a client a way to probe for regions
	//   it has never seen by the code alone (the probe law, rpg-project#350).
	//
	//   ErrOutOfRange / ErrNotVisible (rpg-toolkit#1404, Interact) -- the
	//   host-seam twins of encounter's ErrOutOfRange/ErrNotVisible: the named
	//   world NPC exists and the actor is real, the reach or the sightline
	//   just doesn't hold right now. Same shape as ErrOutOfReach above -- a
	//   well-formed call the current world state refuses, not a caller
	//   naming something that doesn't exist.
	case errors.Is(err, sdk.ErrInBubble),
		errors.Is(err, sdk.ErrNotInFight),
		errors.Is(err, sdk.ErrClosed),
		errors.Is(err, sdk.ErrNotACharacter),
		errors.Is(err, sdk.ErrElsewhere),
		errors.Is(err, sdk.ErrBadAttack),
		errors.Is(err, sdk.ErrDowned),
		errors.Is(err, sdk.ErrLocked),
		errors.Is(err, sdk.ErrDoorShut),
		errors.Is(err, sdk.ErrCannotAfford),
		errors.Is(err, sdk.ErrNotYourTurn),
		errors.Is(err, sdk.ErrOutOfReach),
		errors.Is(err, sdk.ErrStaleDeclaration),
		errors.Is(err, sdk.ErrOutOfRange),
		errors.Is(err, sdk.ErrNotVisible),
		// Three more arrive with the holdings verbs (rpg-project#368), and
		// all three are this bucket by the same test every row above meets:
		// the request is well-formed and names real things, and it is the
		// world's present state that refuses it.
		//
		//   ErrNotDown (Loot) -- the target is a real member, standing up.
		//   Ordinary rather than probe-scoped on purpose: a body is visible,
		//   and there is no secret in whether somebody is on the floor
		//   (design §4.2).
		//
		//   ErrNotHoldable (Hold) -- a prop the member can see that nobody
		//   declared holdable. A thing nobody declared holdable stays
		//   scenery, so this names the pillar rather than hiding it.
		//
		//   ErrAlreadyHeld (Hold) -- somebody is carrying it. State, not a
		//   malformed request: the remedy is looting the carrier or waiting
		//   for them to drop it, not new arguments.
		//
		// Each is reachable ONLY for a prop or body the member can see; for
		// anything they cannot, the composition collapses the refusal to
		// ErrNoProp in the NOT_FOUND bucket above.
		errors.Is(err, sdk.ErrNotDown),
		errors.Is(err, sdk.ErrNotHoldable),
		errors.Is(err, sdk.ErrAlreadyHeld),
		// Trade (rpg-project#369/#370, wave 1 of rpg-toolkit#1275) adds four,
		// all the same well-formed-call-the-world-refuses shape as the rows
		// above:
		//
		//   ErrGiveNotSupported -- the wire's own TradeRequest.give doc
		//   already states the intended code in writing: a caller that
		//   populates it is refused FAILED_PRECONDITION. Give is a legal
		//   field on a legal message; this wave's own rule refuses it, the
		//   same way ErrCannotAfford refuses a second swing a turn already
		//   bought one of.
		//
		//   ErrInvalidTradeOffer -- Receive does not name exactly one item,
		//   or that item has an empty ID or a nonpositive quantity. Shaped
		//   correctly as a message; this verb's own rule for THIS wave is
		//   what refuses it.
		//
		//   ErrNotAVendor -- the target is a confirmed, visible world NPC
		//   with no npc.CapabilityVendor. Not NotFound: the target exists
		//   and Interact would happily describe it, it just cannot be
		//   traded with.
		//
		//   ErrOutOfStock -- the vendor is real and reachable, and does not
		//   carry the item or not enough of it. World state, not a
		//   malformed request.
		errors.Is(err, sdk.ErrGiveNotSupported),
		errors.Is(err, sdk.ErrInvalidTradeOffer),
		errors.Is(err, sdk.ErrNotAVendor),
		errors.Is(err, sdk.ErrOutOfStock),
		// ErrCannotActivate is ErrCannotAfford's shape one verb further out:
		// an ability that could have run and said no. The SDK documents it as
		// not currently reachable through Activate -- Afford consults the same
		// gates the sheet does, so an unavailable ability is a stale selector
		// first -- and it is mapped anyway, for ErrBadCost's reason: the day
		// the two gates disagree, the failure needs a name that is not a lie.
		errors.Is(err, sdk.ErrCannotActivate):
		return status.Error(codes.FailedPrecondition, err.Error())

	// PERMISSION_DENIED -- the authenticated principal is real but owns no
	// current player seat in this session. NotFound would expose roster state;
	// Unauthenticated would deny that authentication already succeeded.
	case errors.Is(err, sdk.ErrNotSeated):
		return status.Error(codes.PermissionDenied, err.Error())

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
	// ErrBadNPC joins at session/v0.49.0: stored npc.Data that cannot be used
	// as given. It is ErrBadCharacter's shape one content type over -- stored
	// content the host wrote and the SDK cannot build from -- so it belongs
	// beside it rather than in the INVALID_ARGUMENT bucket, where it would
	// blame the caller for a row nobody in this request wrote.
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
		errors.Is(err, sdk.ErrBadNPC),
		errors.Is(err, sdk.ErrInvalidSession),
		errors.Is(err, sdk.ErrNilConfig),
		errors.Is(err, sdk.ErrIncompleteConfig),
		errors.Is(err, sdk.ErrBadTurnOutcome),
		// ErrNoIntel (rpg-project#372) is here for ErrBadCost's reason, and
		// the bucket is the whole argument: NO SessionService RPC NAMES AN
		// INTEL RECORD. A client cannot produce this — it is raised when a
		// spawn carries a record id the dungeon does not declare, and the
		// only thing that spawns is THIS SERVER, forwarding the compiled ids
		// dungeonspec minted (internal/orchestrators/lobby's
		// StartEncounter). So it reports wiring on this side of the wire,
		// not a caller mistake, and NotFound or InvalidArgument would blame
		// a client for a list it never sent.
		//
		// Unreachable through this service today, and mapped anyway: the day
		// a verb does take an intel id, the failure needs a name that is not
		// a lie.
		errors.Is(err, sdk.ErrNoIntel):
		return status.Error(codes.Internal, err.Error())

	default:
		return status.Error(codes.Internal, err.Error())
	}
}
