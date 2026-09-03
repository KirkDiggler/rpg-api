package sessionv1alpha1

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
)

// TestStatusError_CoversEverySDKSentinel is the single test that enforces
// design rule 7: every exported sentinel in rulebooks/dnd5e/session/errors.go
// maps to a gRPC code, in this one table. If the SDK adds a sentinel, this
// test does not fail automatically -- see
// TestStatusError_UnmappedSentinelFallsBackToInternal for the safety net --
// but a new sentinel with no explicit case here should get one deliberately
// rather than silently riding the Internal default.
func TestStatusError_CoversEverySDKSentinel(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want codes.Code
	}{
		// NOT_FOUND -- the caller named something that does not exist.
		{"ErrNoSession", sdk.ErrNoSession, codes.NotFound},
		{"ErrNoEncounter", sdk.ErrNoEncounter, codes.NotFound},
		{"ErrNoCharacter", sdk.ErrNoCharacter, codes.NotFound},
		{"ErrNoMember", sdk.ErrNoMember, codes.NotFound},
		{"ErrNoEnding", sdk.ErrNoEnding, codes.NotFound},
		{"ErrUnknownContent", sdk.ErrUnknownContent, codes.NotFound},
		{"ErrNoLoader", sdk.ErrNoLoader, codes.NotFound},
		{"ErrNoSheet", sdk.ErrNoSheet, codes.NotFound},

		// INVALID_ARGUMENT -- the request itself is malformed.
		{"ErrNilInput", sdk.ErrNilInput, codes.InvalidArgument},
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

		// FAILED_PRECONDITION -- well-formed request, world state refuses it.
		{"ErrInBubble", sdk.ErrInBubble, codes.FailedPrecondition},
		{"ErrNotInFight", sdk.ErrNotInFight, codes.FailedPrecondition},
		{"ErrClosed", sdk.ErrClosed, codes.FailedPrecondition},
		{"ErrNotACharacter", sdk.ErrNotACharacter, codes.FailedPrecondition},
		// Search's own refusal (rpg-project#350): the named region is not
		// the one the searcher stands in, or does not exist -- the SDK
		// returns the SAME sentinel for both on purpose (the probe law),
		// so this is one row for one sentinel, not two.
		{"ErrElsewhere", sdk.ErrElsewhere, codes.FailedPrecondition},
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
		{"ErrStaleDeclaration", sdk.ErrStaleDeclaration, codes.FailedPrecondition},
		// ErrCannotActivate is ErrCannotAfford's shape one verb further out:
		// an ability that could have run and said no. The SDK documents it as
		// not currently reachable through Activate — Afford consults the same
		// gates the sheet does — and it is mapped anyway, for ErrBadCost's
		// reason: the day the two gates disagree, the failure needs a name
		// that is not a lie.
		{"ErrCannotActivate", sdk.ErrCannotActivate, codes.FailedPrecondition},

		// ALREADY_EXISTS
		{"ErrSessionExists", sdk.ErrSessionExists, codes.AlreadyExists},

		// OUT_OF_RANGE -- deterministic fix (resync from zero), not a state
		// to wait out, which is what sets this apart from FailedPrecondition.
		{"ErrStoryTrimmed", sdk.ErrStoryTrimmed, codes.OutOfRange},

		// UNAVAILABLE -- partial write; retry guidance rides the SaveReport.
		{"ErrSaveFailed", sdk.ErrSaveFailed, codes.Unavailable},

		// INTERNAL -- storage-side integrity problems, not a caller mistake.
		{"ErrBadRepository", sdk.ErrBadRepository, codes.Internal},
		{"ErrBadCost", sdk.ErrBadCost, codes.Internal},
		{"ErrBadCharacter", sdk.ErrBadCharacter, codes.Internal},
		{"ErrInvalidSession", sdk.ErrInvalidSession, codes.Internal},
		{"ErrNilConfig", sdk.ErrNilConfig, codes.Internal},
		{"ErrIncompleteConfig", sdk.ErrIncompleteConfig, codes.Internal},
		// Back to caller-facing with the door verbs (rpg-project#268):
		// GetDoors/OpenDoor/Unlock name a door, so one the dungeon does not
		// have is the caller's NotFound again.
		{"ErrNoConnection", sdk.ErrNoConnection, codes.NotFound},
		// The rpg-toolkit#1135 split, both halves: a walk into a locked door
		// says locked (with the DC), a merely-shut one says shut. World
		// state refusing, never a malformed request.
		{"ErrDoorShut", sdk.ErrDoorShut, codes.FailedPrecondition},
		// Already in the pinned SDK before this feature (v0.21.4) and unmapped
		// until this audit: this package's OWN adapter vocabulary going stale
		// against itself, not a caller mistake.
		{"ErrBadTurnOutcome", sdk.ErrBadTurnOutcome, codes.Internal},

		// sdk.ErrNotFound is the SDK's repository-facing contract sentinel: the
		// Manager translates it into a caller-facing sentinel (ErrNoSession,
		// ErrNoEncounter, ...) before any verb returns, so it should never
		// reach here. Tested anyway so a regression that DID let it leak is
		// caught as Internal (a storage-layer signal), not silently
		// misread as some caller-facing code.
		{"ErrNotFound (defensive)", sdk.ErrNotFound, codes.Internal},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Wrapped exactly like the SDK wraps its own sentinels
			// ("verb: %w") so the table is proven against errors.Is chains,
			// not bare sentinel identity.
			wrapped := fmt.Errorf("move: step 1: %w", tt.err)
			got := statusError(wrapped)
			st, ok := status.FromError(got)
			require.True(t, ok, "statusError must always return a gRPC status error")
			require.Equal(t, tt.want, st.Code(), "sentinel %s", tt.name)
		})
	}
}

// errorsGoDefaultCaseSentinels names every SDK sentinel statusError leaves to
// the default case ON PURPOSE, so TestStatusError_MapsEverySDKSentinel below
// does not fail on them -- each entry's reason is carried in errors.go's own
// doc, not here; this map is only the allowlist that check reads.
var errorsGoDefaultCaseSentinels = map[string]bool{
	// The SDK's repository-facing contract sentinel: the Manager translates
	// it into a caller-facing one (ErrNoSession, ErrNoEncounter, ...) before
	// any verb returns, so it should never reach a handler. See errors.go's
	// own doc on statusError for the full reasoning.
	"ErrNotFound": true,
	// Session v0.48.0 adds these for its Interact verb, which this proto
	// service does not expose. Keeping the defensive Internal fallback avoids
	// widening this activation-event change into a new transport contract;
	// the API leg that adds Interact must choose and test its public codes.
	"ErrOutOfRange": true,
	"ErrNotVisible": true,
}

// sdkSentinelNames reads every exported Err* sentinel declared in the PINNED
// rulebooks/dnd5e/session module's errors.go, straight from the module
// cache -- not a local checkout (which routinely sits on unmerged branches)
// and not a hand-maintained count (which can go stale silently: a table and
// a constant that both stay put pass a length check even when the SDK
// gained a sentinel neither of them learned about -- exactly how ErrElsewhere
// slipped past this file once). go list resolves the pinned module's
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
	got := statusError(unrecognized)
	st, ok := status.FromError(got)
	require.True(t, ok)
	require.Equal(t, codes.Internal, st.Code())
}

func TestStatusError_Nil_ReturnsNil(t *testing.T) {
	require.NoError(t, statusError(nil))
}
