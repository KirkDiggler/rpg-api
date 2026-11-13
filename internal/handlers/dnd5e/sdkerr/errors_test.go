package sdkerr

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
)

// TestStatusError_CoversEverySDKSentinel is the single test that enforces
// design rule 7: every exported sentinel in rulebooks/dnd5e/session/errors.go
// maps to a gRPC code, in this one table. If the SDK adds a sentinel, this
// test does not fail automatically -- see
// TestStatusError_UnmappedSentinelFallsBackToInternal for the safety net --
// but a new sentinel with no explicit case here should get one deliberately
// rather than silently riding the Internal default.
func TestStatusError_CoversEverySDKSentinel(t *testing.T) {
	tests := sentinelCases()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Wrapped exactly like the SDK wraps its own sentinels
			// ("verb: %w") so the table is proven against errors.Is chains,
			// not bare sentinel identity.
			wrapped := fmt.Errorf("move: step 1: %w", tt.err)
			got := StatusError(wrapped)
			st, ok := status.FromError(got)
			require.True(t, ok, "statusError must always return a gRPC status error")
			require.Equal(t, tt.want, st.Code(), "sentinel %s", tt.name)
		})
	}
}

// sentinelCase is one row of the sentinel table: a mapped SDK sentinel and
// the code it reaches the wire as.
type sentinelCase struct {
	name string
	err  error
	want codes.Code
}

// sentinelCases is every SDK sentinel the table maps, with its code: the one
// list both the code test above and the reason test below iterate, so a
// sentinel added here is held to both.
func sentinelCases() []sentinelCase {
	return []sentinelCase{
		// NOT_FOUND -- the caller named something that does not exist.
		{"ErrNoSession", sdk.ErrNoSession, codes.NotFound},
		{"ErrNoEncounter", sdk.ErrNoEncounter, codes.NotFound},
		// Hold's probe-law collapse sentinel (rpg-project#368): the SAME
		// sentinel answers "this dungeon has no prop by that id" and every
		// refusal about a prop the member cannot see, so this is one row
		// for one sentinel exactly as ErrBadAttack below is.
		{"ErrNoProp", sdk.ErrNoProp, codes.NotFound},
		{"ErrNoCharacter", sdk.ErrNoCharacter, codes.NotFound},
		{"ErrNoMember", sdk.ErrNoMember, codes.NotFound},
		{"ErrNoSeat", sdk.ErrNoSeat, codes.NotFound},
		{"ErrNoEnding", sdk.ErrNoEnding, codes.NotFound},
		{"ErrUnknownContent", sdk.ErrUnknownContent, codes.NotFound},
		{"ErrNoLoader", sdk.ErrNoLoader, codes.NotFound},
		{"ErrNoSheet", sdk.ErrNoSheet, codes.NotFound},

		// PERMISSION_DENIED -- authenticated, but owns no current seat.
		{"ErrNotSeated", sdk.ErrNotSeated, codes.PermissionDenied},

		// INVALID_ARGUMENT -- the request itself is malformed.
		{"ErrNilInput", sdk.ErrNilInput, codes.InvalidArgument},
		{"ErrShadowedRef", sdk.ErrShadowedRef, codes.InvalidArgument},
		// ErrBadActivation is INVALID_ARGUMENT rather than Internal (where its
		// sibling ErrBadCost sits) because a CALLER can produce it: an
		// activation naming a target for an ability that takes none. The SDK
		// refuses that rather than ignoring it, so a client that believed it
		// aimed Dodge at somebody is told.
		{"ErrBadActivation", sdk.ErrBadActivation, codes.InvalidArgument},
		{"ErrEmptyPath", sdk.ErrEmptyPath, codes.InvalidArgument},
		{"ErrBrokenPath", sdk.ErrBrokenPath, codes.InvalidArgument},
		// No ErrNoCrossing row: session/v0.18.0 deleted the sentinel. On one
		// canvas nothing distinguishes a walled crossing from a missing cell,
		// so a walk stopped by a wall is ErrBadPosition now.
		{"ErrBadPosition", sdk.ErrBadPosition, codes.InvalidArgument},
		{"ErrNoRef", sdk.ErrNoRef, codes.InvalidArgument},
		{"ErrBadRef", sdk.ErrBadRef, codes.InvalidArgument},
		{"ErrNoMemberID", sdk.ErrNoMemberID, codes.InvalidArgument},
		{"ErrNoSessionID", sdk.ErrNoSessionID, codes.InvalidArgument},
		{"ErrNoEncounterID", sdk.ErrNoEncounterID, codes.InvalidArgument},
		{"ErrInvalidWorld", sdk.ErrInvalidWorld, codes.InvalidArgument},
		{"ErrNoCause", sdk.ErrNoCause, codes.InvalidArgument},
		{"ErrNoDeclarationID", sdk.ErrNoDeclarationID, codes.InvalidArgument},
		// ErrInvalidUnpackRequest (Unpack, rpg-toolkit#1544) is
		// INVALID_ARGUMENT per the wire's own UnpackRequest doc -- DIFFERENT
		// from Trade's analogous ErrInvalidTradeOffer (FAILED_PRECONDITION
		// below), a deliberate choice this table follows rather than
		// pattern-matches.
		{"ErrInvalidUnpackRequest", sdk.ErrInvalidUnpackRequest, codes.InvalidArgument},
		// The level-up submission's own malformed-request sentinel: a choice
		// id the level never asked for, a count that does not match, an option
		// off the list. Its bucket was pinned by NOTHING until now -- moving
		// it into FAILED_PRECONDITION passed the entire suite, integration
		// included, because the sibling static test only proves a case exists.
		{"ErrBadLevelRequest", sdk.ErrBadLevelRequest, codes.InvalidArgument},
		// A malformed reach (a negative range) is the caller's defect, not a
		// reach that fell short -- ErrOutOfRange keeps FAILED_PRECONDITION.
		{"ErrBadReach", sdk.ErrBadReach, codes.InvalidArgument},
		{"ErrBadRecordID", sdk.ErrBadRecordID, codes.InvalidArgument},
		{"ErrBadEquip", sdk.ErrBadEquip, codes.InvalidArgument},
		{"ErrBadRest", sdk.ErrBadRest, codes.InvalidArgument},
		{"ErrDuplicateMember", sdk.ErrDuplicateMember, codes.InvalidArgument},

		// FAILED_PRECONDITION -- well-formed request, world state refuses it.
		{"ErrInBubble", sdk.ErrInBubble, codes.FailedPrecondition},
		// A sneak refused in plain view (rpg-project#768 R11).
		{"ErrSeen", sdk.ErrSeen, codes.FailedPrecondition},
		{"ErrNotInFight", sdk.ErrNotInFight, codes.FailedPrecondition},
		{"ErrClosed", sdk.ErrClosed, codes.FailedPrecondition},
		{"ErrNotACharacter", sdk.ErrNotACharacter, codes.FailedPrecondition},
		// Two competing attack offers: the player owes a pick (R5).
		{"ErrConflictingOffers", sdk.ErrConflictingOffers, codes.FailedPrecondition},
		{"ErrBadAttack", sdk.ErrBadAttack, codes.FailedPrecondition},
		// Arrived v0.15.0-v0.17.0. ErrDowned is emphatically NOT no-such-member
		// (a downed member stays on the map, in the roster, and readable), and
		// ErrCannotAfford is a fact about the GAME rather than about the code --
		// which is why its programmer-facing twin ErrBadCost is Internal below
		// and not here.
		{"ErrDowned", sdk.ErrDowned, codes.FailedPrecondition},
		{"ErrLocked", sdk.ErrLocked, codes.FailedPrecondition},
		{"ErrCannotAfford", sdk.ErrCannotAfford, codes.FailedPrecondition},
		// Combat-turn contract (rpg-project#249): a well-formed swing the
		// current world state refuses, the same bucket as the three above.
		{"ErrNotYourTurn", sdk.ErrNotYourTurn, codes.FailedPrecondition},
		{"ErrOutOfReach", sdk.ErrOutOfReach, codes.FailedPrecondition},
		{"ErrStaleDeclaration", sdk.ErrStaleDeclaration, codes.Aborted},
		// World NPCs (rpg-toolkit#1404): Interact's own reach/sight refusals,
		// the same shape as ErrOutOfReach above.
		{"ErrOutOfRange", sdk.ErrOutOfRange, codes.FailedPrecondition},
		{"ErrNotVisible", sdk.ErrNotVisible, codes.FailedPrecondition},
		// Speak's sight refusal (rpg-project#454): the threatened member
		// cannot see who is threatening them. FAILED_PRECONDITION like the
		// rows around it, and DELIBERATELY NOT folded into ErrOutOfReach --
		// reach is a distance and this is a sightline, so a client that read
		// the two as one would tell the player to step closer when what they
		// need is to be seen.
		{"ErrUnwitnessed", sdk.ErrUnwitnessed, codes.FailedPrecondition},
		// The offer comes from the NPC (rpg-project#494, rpg-project#569): a
		// real, visible target that offers this speaker no line by that id.
		// FAILED_PRECONDITION with the rows around it, and NOT NotFound --
		// the creature is on the roster and in plain sight, so answering
		// "no such member" would deny something the player can see. Without
		// this row the refusal falls through to Internal, and an ordinary
		// "you cannot talk to that one" reads to a client as a server fault.
		{"ErrNoOption", sdk.ErrNoOption, codes.FailedPrecondition},
		// Holdings (rpg-project#368): Loot's and Hold's own state refusals,
		// each reachable only about a body or prop the member can SEE --
		// for anything they cannot, the composition collapses the refusal
		// to ErrNoProp in the NOT_FOUND rows above (the probe law).
		{"ErrNotDown", sdk.ErrNotDown, codes.FailedPrecondition},
		{"ErrNotHoldable", sdk.ErrNotHoldable, codes.FailedPrecondition},
		{"ErrAlreadyHeld", sdk.ErrAlreadyHeld, codes.FailedPrecondition},
		// Trade (rpg-project#369/#370, buy wave of rpg-toolkit#1275): the
		// same well-formed-call-the-world-refuses family: ErrInvalidTradeOffer
		// (the populated side isn't exactly one valid item, both/neither side
		// carries items), ErrNotAVendor (a real, visible world NPC with no
		// vendor capability), ErrOutOfStock (buying: a real vendor that
		// doesn't carry enough of it). ErrGiveNotSupported (this bucket's
		// original occupant, the buy-only wave's refusal for a populated
		// Give) is RETIRED as of rpg-toolkit#1537 -- giving items now means
		// selling -- and dropped from this table entirely, not just unused.
		{"ErrInvalidTradeOffer", sdk.ErrInvalidTradeOffer, codes.FailedPrecondition},
		{"ErrNotAVendor", sdk.ErrNotAVendor, codes.FailedPrecondition},
		{"ErrOutOfStock", sdk.ErrOutOfStock, codes.FailedPrecondition},
		// Trade learns to charge (rpg-toolkit#1534) and learns to sell
		// (rpg-toolkit#1537): ErrWrongPrice is a caller offering the wrong
		// AMOUNT, symmetrically for a buy's payment or a sell's payout (the
		// server alone decides what's correct, never a trusted client
		// price); ErrInsufficientFunds is the right amount named but the
		// payer (actor buying, or the vendor's own optional Wallet selling)
		// can't cover it; ErrNotInInventory is not owning enough of an item
		// a verb needs to remove -- Trade selling, or Unpack against the
		// pack it names (rpg-toolkit#1544, the sentinel's doc generalized
		// when Unpack reused it). All three the same
		// well-formed-call-the-world-refuses family as the sentinels above.
		{"ErrWrongPrice", sdk.ErrWrongPrice, codes.FailedPrecondition},
		{"ErrInsufficientFunds", sdk.ErrInsufficientFunds, codes.FailedPrecondition},
		{"ErrNotInInventory", sdk.ErrNotInInventory, codes.FailedPrecondition},
		// ErrNotAPack (Unpack, rpg-toolkit#1544): a real, resolvable item
		// that simply isn't a Pack. Same shape as ErrNotAVendor above.
		{"ErrNotAPack", sdk.ErrNotAPack, codes.FailedPrecondition},
		// ErrNotATarget (rpg-project#493 R4): a swing at a placed world NPC.
		// The same wrong-kind-for-this-verb shape as ErrNotAVendor and
		// ErrNotAPack above -- the target exists and is visible, it is just
		// not something you attack -- and DELIBERATELY NOT the
		// ErrStaleDeclaration it used to be: stale sends a host round a
		// re-read loop that answers the same thing every time, which is the
		// whole reason the SDK split this sentinel off.
		{"ErrNotATarget", sdk.ErrNotATarget, codes.FailedPrecondition},
		// ErrCannotActivate is ErrCannotAfford's shape one verb further out:
		// an ability that could have run and said no. The SDK documents it as
		// not currently reachable through Activate — Afford consults the same
		// gates the sheet does — and it is mapped anyway, for ErrBadCost's
		// reason: the day the two gates disagree, the failure needs a name
		// that is not a lie.
		{"ErrCannotActivate", sdk.ErrCannotActivate, codes.FailedPrecondition},

		// A stored world naming a site nobody holds (rpg-project#879 S5).
		// The SDK wraps it beside ErrInvalidWorld; the second row is that
		// exact chain, pinning that it is matched before INVALID_ARGUMENT.
		{"ErrUnknownSite", sdk.ErrUnknownSite, codes.FailedPrecondition},
		{"ErrUnknownSite beside ErrInvalidWorld", fmt.Errorf("site abc: %w: %w", sdk.ErrInvalidWorld, sdk.ErrUnknownSite), codes.FailedPrecondition},

		// A checkpoint saved under an earlier store shape (rpg-project#897
		// legs c and b): the server's own sentinel, refused by the Redis
		// repository and passed up through the verb's wrapping.
		{"ErrCheckpointBeforePieces", sessionorch.ErrCheckpointBeforePieces, codes.FailedPrecondition},
		{"ErrCheckpointBeforePieces through a verb", fmt.Errorf("move: encounter %q: %w", "e1", sessionorch.ErrCheckpointBeforePieces), codes.FailedPrecondition},

		// A stored checkpoint whose pieces disagree (rpg-project#897 leg b),
		// in the exact chain the repository returns: beside
		// sdk.ErrBadRepository, which alone would be INTERNAL.
		{"ErrCheckpointMismatch", fmt.Errorf("encounter %q: journal missing: %w: %w", "e1", sessionorch.ErrCheckpointMismatch, sdk.ErrBadRepository), codes.FailedPrecondition},

		// ALREADY_EXISTS
		{"ErrSessionExists", sdk.ErrSessionExists, codes.AlreadyExists},

		// OUT_OF_RANGE -- deterministic fix (resync from zero), not a state
		// to wait out, which is what sets this apart from FailedPrecondition.
		{"ErrStoryTrimmed", sdk.ErrStoryTrimmed, codes.OutOfRange},

		// ABORTED -- a fenced save refused as stale; nothing landed.
		{"ErrStaleCheckpoint", sdk.ErrStaleCheckpoint, codes.Aborted},

		// UNAVAILABLE -- partial write; retry guidance rides the SaveReport.
		{"ErrSaveFailed", sdk.ErrSaveFailed, codes.Unavailable},

		// INTERNAL -- storage-side integrity problems, not a caller mistake.
		{"ErrBadRepository", sdk.ErrBadRepository, codes.Internal},
		{"ErrBadCost", sdk.ErrBadCost, codes.Internal},
		{"ErrBadCharacter", sdk.ErrBadCharacter, codes.Internal},
		{"ErrBadNPC", sdk.ErrBadNPC, codes.Internal},
		// ErrBadPackContents (Unpack, rpg-toolkit#1544): the named item IS a
		// pack, but one of its own Contents lines doesn't resolve against
		// the catalog -- a content-authoring defect, ErrBadNPC's shape one
		// content type over.
		{"ErrBadPackContents", sdk.ErrBadPackContents, codes.Internal},
		// ErrBadInformation (Afford): a rulebook content defect, never a caller fault.
		{"ErrBadInformation", sdk.ErrBadInformation, codes.Internal},
		{"ErrInvalidSession", sdk.ErrInvalidSession, codes.Internal},
		{"ErrNilConfig", sdk.ErrNilConfig, codes.Internal},
		{"ErrIncompleteConfig", sdk.ErrIncompleteConfig, codes.Internal},
		// Both turn-driver sentinels of rpg-toolkit#1734. ErrAmbiguousConfig
		// is construction-only, like the two above it; ErrNoTurnDriver is
		// raised mid-verb when this server's own per-session driver cache
		// reports success and hands over nothing. Both are server wiring.
		{"ErrAmbiguousConfig", sdk.ErrAmbiguousConfig, codes.Internal},
		{"ErrNoTurnDriver", sdk.ErrNoTurnDriver, codes.Internal},
		// Back to caller-facing with the door verbs (rpg-project#268):
		// GetDoors/OpenDoor/Unlock name a door, so one the dungeon does not
		// have is the caller's NotFound again.
		{"ErrNoConnection", sdk.ErrNoConnection, codes.NotFound},
		// The rpg-toolkit#1135 split, both halves: a walk into a locked door
		// says locked (with the DC), a merely-shut one says shut. World
		// state refusing, never a malformed request.
		{"ErrDoorShut", sdk.ErrDoorShut, codes.FailedPrecondition},
		// The other two advancement sentinels (rpg-project#452). Pinned HERE
		// as well as through the handler and integration tests that exercise
		// them, because those prove a code reaches a client down one PATH,
		// while this proves the sentinel sits in the right BUCKET -- which is
		// exactly what the sibling static test cannot see.
		//
		// The split is the request versus the situation. A malformed
		// submission (ErrBadLevelRequest, above) can succeed if rebuilt. A
		// level not earned or not describable cannot: only waiting, or content
		// landing, changes the answer.
		{"ErrCannotAdvance", sdk.ErrCannotAdvance, codes.FailedPrecondition},
		{"ErrLevelNotOffered", sdk.ErrLevelNotOffered, codes.FailedPrecondition},
		{"ErrArmorInFight", sdk.ErrArmorInFight, codes.FailedPrecondition},
		{"ErrSeatedElsewhere", sdk.ErrSeatedElsewhere, codes.FailedPrecondition},
		{"ErrDoorHeld", sdk.ErrDoorHeld, codes.FailedPrecondition},
		{"ErrLineReserved", sdk.ErrLineReserved, codes.FailedPrecondition},
		{"ErrSharingNotBuilt", sdk.ErrSharingNotBuilt, codes.FailedPrecondition},
		{"ErrInCustody", sdk.ErrInCustody, codes.FailedPrecondition},
		{"ErrJailRefused", sdk.ErrJailRefused, codes.FailedPrecondition},
		{"ErrStaleOption", sdk.ErrStaleOption, codes.Aborted},
		// Already in the pinned SDK before this feature (v0.21.4) and unmapped
		// until this audit: this package's OWN adapter vocabulary going stale
		// against itself, not a caller mistake.
		{"ErrBadTurnOutcome", sdk.ErrBadTurnOutcome, codes.Internal},
		// Intel records (rpg-project#372): raised only when a SPAWN names a
		// record the dungeon does not declare, and the only spawner is this
		// server forwarding compiled ids. Wiring on this side, never a
		// caller's mistake -- see errors.go.
		{"ErrNoIntel", sdk.ErrNoIntel, codes.Internal},
		// Same bucket, same argument (rpg-project#375): only this server's
		// launch names a faction, forwarding the placement's own word.
		{"ErrNoFaction", sdk.ErrNoFaction, codes.Internal},

		// sdk.ErrNotFound is the SDK's repository-facing contract sentinel: the
		// Manager translates it into a caller-facing sentinel (ErrNoSession,
		// ErrNoEncounter, ...) before any verb returns, so it should never
		// reach here. Tested anyway so a regression that DID let it leak is
		// caught as Internal (a storage-layer signal), not silently
		// misread as some caller-facing code.
		{"ErrNotFound (defensive)", sdk.ErrNotFound, codes.Internal},
	}
}

// TestStatusError_OnlyAMovedOnOfferIsAborted pins rpg-project#925: ABORTED
// is the code that tells a client "what you were offered has moved on --
// re-read and pick again", so exactly the stale refusals reach it -- a stale
// declaration, a stale Speak option, and a save that lost its checkpoint
// race. Every other sentinel in the shared table maps elsewhere, so a future
// sentinel cannot silently join ABORTED, nor these leave it.
func TestStatusError_OnlyAMovedOnOfferIsAborted(t *testing.T) {
	want := map[string]bool{"ErrStaleDeclaration": true, "ErrStaleOption": true, "ErrStaleCheckpoint": true}
	got := map[string]bool{}
	for _, tt := range sentinelCases() {
		if tt.want == codes.Aborted {
			got[tt.name] = true
		}
	}
	require.Equal(t, want, got)
}

// errorsGoDefaultCaseSentinels names every SDK sentinel statusError leaves to
// the default case ON PURPOSE, so TestStatusError_MapsEverySDKSentinel below
// does not fail on them -- each entry's reason is documented either in
// errors.go or beside the relevant allowlist entry here.
var errorsGoDefaultCaseSentinels = map[string]bool{
	// The SDK's repository-facing contract sentinel: the Manager translates
	// it into a caller-facing one (ErrNoSession, ErrNoEncounter, ...) before
	// any verb returns, so it should never reach a handler. See errors.go's
	// own doc on statusError for the full reasoning.
	"ErrNotFound": true,

	// A PRODUCER DEFECT, AND INTERNAL IS THE HONEST CODE (rpg-project#462).
	// ErrNoCalculation fires when a verb in THIS build rolled a d20 and then
	// wrote the beat with no arithmetic behind the total — a seam that has
	// quietly gone back to publishing one number, which is the exact failure
	// that slice exists to end. The SDK refuses it loudly "where it is still a
	// bug rather than a story" (its own doc).
	//
	// SO IT IS NOT FAILED_PRECONDITION, and the distinction is the whole
	// point of the bucket above it. Every row there is a well-formed call that
	// the world's present state refuses, and the player can do something about
	// it: step into the light, wait for your turn, get closer. There is
	// nothing a player or a client can do about this one. Telling them "that
	// is not allowed right now" would send them looking for a game reason for
	// a server bug, and a dock that blames the player for a producer defect is
	// how somebody stops trusting the panel.
	//
	// It is NOT returned for a verb that rolled nothing — opening an unlocked
	// door faces no DC and nil is the truth there — so reaching a handler at
	// all means this build is broken, which is what Internal says.
	"ErrNoCalculation": true,
}

// sdkSentinelNames reads every exported Err* sentinel declared in the PINNED
// rulebooks/dnd5e/session module's errors.go, straight from the module
// cache -- not a local checkout (which routinely sits on unmerged branches)
// and not a hand-maintained count (which can go stale silently: a table and
// a constant that both stay put pass a length check even when the SDK
// gained a sentinel neither of them learned about -- exactly how a sentinel like
// ErrConflictingOffers could slip past this file). go list resolves the pinned module's
// on-disk directory directly from go.mod, so this always reads the version
// actually built.
func sdkSentinelNames(t *testing.T) []string {
	t.Helper()

	out, err := exec.CommandContext(t.Context(), "go", "list", "-m", "-f", "{{.Dir}}",
		"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session").Output()
	require.NoError(t, err, "resolve the pinned session module's directory via go list")
	dir := strings.TrimSpace(string(out))

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filepath.Join(dir, "errors.go"), nil, 0)
	require.NoError(t, err, "parse the pinned session module's errors.go")

	var names []string
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, name := range vs.Names {
				if name.IsExported() && strings.HasPrefix(name.Name, "Err") {
					names = append(names, name.Name)
				}
			}
		}
	}
	require.NotEmpty(t, names, "parsed zero sentinels out of the pinned session module's errors.go -- the parse itself is broken, not the SDK")
	return names
}

// statusErrorMappedSentinels parses THIS package's own errors.go -- the
// production file, not this test -- and returns every sdk.Err* name
// referenced anywhere in it. That is a direct, static answer to "does
// statusError have a case for this sentinel", independent of whatever rows
// TestStatusError_CoversEverySDKSentinel's table happens to carry.
func statusErrorMappedSentinels(t *testing.T) map[string]bool {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "errors.go", nil, 0)
	require.NoError(t, err, "parse this package's own errors.go")

	mapped := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		ident, ok := sel.X.(*ast.Ident)
		if !ok || ident.Name != "sdk" {
			return true
		}
		if strings.HasPrefix(sel.Sel.Name, "Err") {
			mapped[sel.Sel.Name] = true
		}
		return true
	})
	return mapped
}

// TestStatusError_MapsEverySDKSentinel is design rule 7's actual enforcement:
// every exported sentinel the PINNED session module carries today has a case
// in statusError's switch, checked by reading both sides from source rather
// than trusting a count a human was supposed to keep in sync. This replaces
// the old sentinelCount constant, which could not catch its own staleness --
// a table and a count that both stay put both look correct. A sentinel this
// test flags should get a deliberate case in errors.go (with reasoning,
// matching every other bucket's doc), not just a row added here to satisfy
// the check.
func TestStatusError_MapsEverySDKSentinel(t *testing.T) {
	mapped := statusErrorMappedSentinels(t)

	for _, name := range sdkSentinelNames(t) {
		if errorsGoDefaultCaseSentinels[name] {
			continue
		}
		t.Run(name, func(t *testing.T) {
			require.True(t, mapped[name],
				"sdk.%s has no case in statusError's switch (errors.go) -- add one deliberately, "+
					"or add it to errorsGoDefaultCaseSentinels here with the same reasoning errors.go's "+
					"ErrNotFound note carries if it is meant to fall through to Internal", name)
		})
	}
}

func TestStatusError_UnmappedSentinelFallsBackToInternal(t *testing.T) {
	unrecognized := fmt.Errorf("some future sentinel the table has not been updated for")
	got := StatusError(unrecognized)
	st, ok := status.FromError(got)
	require.True(t, ok)
	require.Equal(t, codes.Internal, st.Code())
}

func TestStatusError_Nil_ReturnsNil(t *testing.T) {
	require.NoError(t, StatusError(nil))
}

// TestStatusError_DoorHeldCarriesItsReason pins rpg-project#852 R7: a held
// door's refusal reaches the client as FAILED_PRECONDITION with the session's
// reason verbatim, naming the holder, even under the session's verb wrap.
func TestStatusError_DoorHeldCarriesItsReason(t *testing.T) {
	got := StatusError(fmt.Errorf("open door: %w", &sdk.DoorHeldError{Reason: "Guard bars the door"}))
	st, ok := status.FromError(got)
	require.True(t, ok)
	require.Equal(t, codes.FailedPrecondition, st.Code())
	require.Equal(t, "Guard bars the door", st.Message(), "the reason alone, without the verb's wrap")
}

// TestStatusError_StaleSaveIsAbortedNotUnavailable pins rpg-project#879 S12a:
// the SDK's SaveError for a stale fenced save unwraps to BOTH ErrSaveFailed
// and ErrStaleCheckpoint, and the client must hear ABORTED (nothing landed,
// retry the verb), never UNAVAILABLE (landed partially).
func TestStatusError_StaleSaveIsAbortedNotUnavailable(t *testing.T) {
	stale := fmt.Errorf("move: %w", &sdk.SaveError{
		Report: sdk.SaveReport{Failed: []string{"encounter:enc-1"}},
		Err:    fmt.Errorf("saving world: %w", sdk.ErrStaleCheckpoint),
	})
	require.ErrorIs(t, stale, sdk.ErrSaveFailed, "the premise: a stale save is also a failed save")
	st, ok := status.FromError(StatusError(stale))
	require.True(t, ok)
	require.Equal(t, codes.Aborted, st.Code())
	require.Contains(t, st.Message(), "stale checkpoint")
}

// TestStatusError_CustodyRefusalsAreTheSentenceAlone pins rpg-project#925:
// each custody refusal reaches the player as FAILED_PRECONDITION whose whole
// message is the session's sentence -- no "move: " or "speak: " -- and a
// move imposed on a held member is worded for the member acting: in the
// second person when it is their own character it names.
func TestStatusError_CustodyRefusalsAreTheSentenceAlone(t *testing.T) {
	for _, tc := range []struct {
		name, viewer, want string
		err                error
	}{
		{"walking away", "alice", "The guards hold you; you cannot walk away.", fmt.Errorf("move: %w", sdk.ErrInCustody)},
		{"the cells refuse", "alice", "The guards cannot take you to the cells now.", fmt.Errorf("speak: %w", sdk.ErrJailRefused)},
		{"a move imposed on yourself", "alice", "The guards hold you; you cannot be moved.", fmt.Errorf("cast: %w", &sdk.CustodyMoveError{Member: "alice"})},
		{"a move imposed on another", "bob", "The guards hold alice; they cannot be moved.", fmt.Errorf("attack: %w", &sdk.CustodyMoveError{Member: "alice"})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			st, ok := status.FromError(StatusErrorFor(tc.err, tc.viewer))
			require.True(t, ok)
			require.Equal(t, codes.FailedPrecondition, st.Code())
			require.Equal(t, tc.want, st.Message())
		})
	}
}
