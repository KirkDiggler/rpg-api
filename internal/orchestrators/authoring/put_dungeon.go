package authoring

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"gopkg.in/yaml.v3"

	"github.com/KirkDiggler/rpg-toolkit/encounter/dungeonspec"

	"github.com/KirkDiggler/rpg-api/internal/content"
	"github.com/KirkDiggler/rpg-api/internal/dungeonregistry"
)

// keyCharsetRe is the key-naming safety constraint (design.md's Key
// rules): key names a server-side file write, so this isn't just tidiness
// — it's the same posture a path-safety allowlist would need for any
// caller-supplied filename component.
var keyCharsetRe = regexp.MustCompile(`^[a-z0-9-]+$`)

// defaultPreviewSeed is the fixed seed PutDungeon compiles at for both the
// validate_only preview and the real write's FloorPlan response. Not
// caller-supplied: seed only affects rolled content, which the board
// keeps off the compiled-layout grid entirely (design.md's "rolled
// content" panel) — FloorPlan carries none of that, so there is nothing
// for a caller-supplied seed to usefully control here.
const defaultPreviewSeed = 1

// MalformedRequestError means the REQUEST itself is invalid — key fails
// the [a-z0-9-]+ charset, or key doesn't match the YAML's own declared
// key: field — detected BEFORE any decode/compile ever runs. The handler
// must translate this into a transport InvalidArgument status carrying NO
// response body (plan.md S1's Error transport decision): a non-OK gRPC
// status drops PutDungeonResponse entirely, so there is nothing
// meaningful to populate for this case — unlike a well-formed request
// whose YAML CONTENT fails validate/compile, which is PutDungeonOutput's
// Success=false path instead, a distinct case this type must never be
// confused with.
type MalformedRequestError struct {
	Message string
}

func (e *MalformedRequestError) Error() string { return e.Message }

// PutDungeonInput carries the entity-typed PutDungeon request. The
// handler builds it after no envelope validation beyond auth — every
// other check (key charset, key/YAML match, YAML content) is this
// orchestrator's job, per this repo's outside-in layering.
type PutDungeonInput struct {
	Key          string
	YAML         string
	ValidateOnly bool
}

// PutDungeonOutput is the well-formed-request result: EITHER the YAML's
// CONTENT failed provider validation (Success=false, FieldErrors carries
// exact source paths/messages/codes, FloorPlan nil)
// OR it succeeded (Success=true, FloorPlan set). A malformed REQUEST
// never produces one of these — PutDungeon returns a
// *MalformedRequestError instead, never a *PutDungeonOutput, for that
// case (see MalformedRequestError's doc for why the two must stay
// distinguishable in type/shape, not collapsed into one "return an
// error" path).
type PutDungeonOutput struct {
	Success     bool
	FieldErrors []FieldError
	FloorPlan   *FloorPlan
}

// yamlKeyHeader is the minimal shape needed to peek a YAML document's
// declared key: field without running dungeonspec's full strict Decode —
// mirrors internal/content's own specHeader/headerKey for the identical
// reason: index/compare by key without paying for (or being sensitive to)
// full schema validation yet.
type yamlKeyHeader struct {
	Key string `yaml:"key"`
}

// peekYAMLKey extracts yamlText's declared key: field for the
// key/YAML-key mismatch check, which must run BEFORE any real
// decode/compile (plan.md S1's ordering). ok=false means the header
// couldn't be read at all (badly malformed YAML, or no key: field) — this
// is deliberately NOT treated as a mismatch by the caller: it falls
// through to dungeonspec.Load instead, whose real decode error becomes
// the well-formed-request, content-failed case with a far more useful
// message than a generic "key mismatch" would be.
func peekYAMLKey(yamlText string) (key string, ok bool) {
	var header yamlKeyHeader
	if err := yaml.Unmarshal([]byte(yamlText), &header); err != nil {
		return "", false
	}
	return header.Key, header.Key != ""
}

// PutDungeon validates, compiles, and (unless ValidateOnly) persists +
// registers a dungeonspec YAML. Order matters throughout:
//
//  1. Key charset, then key/YAML key: match — cheap, no compile needed.
//     Either failure is a malformed REQUEST (*MalformedRequestError).
//  2. Compile the complete candidate standalone in Draft or Strict mode.
//     Prior compiled state is deliberately not an input: explicit deletion is
//     legal when the replacement candidate itself validates. Provider field
//     failures return PutDungeonOutput with Success=false.
//  3. ValidateOnly stops here: FloorPlan returned, nothing persisted,
//     nothing registered.
//  4. Write-through to ContentDir FIRST. A write failure fails the RPC
//     (a real Go error, not a PutDungeonOutput) and leaves the registry
//     untouched — this ordering is what prevents a spec from playing now
//     and vanishing after the next restart because it was never actually
//     written to disk (design.md's "write-then-swap" decision).
//  5. Only on a successful write does the registry swap happen.
func (o *Orchestrator) PutDungeon(ctx context.Context, in *PutDungeonInput) (*PutDungeonOutput, error) {
	if in == nil {
		return nil, errors.New("authoring orchestrator: PutDungeonInput is required")
	}

	if !keyCharsetRe.MatchString(in.Key) {
		return nil, &MalformedRequestError{
			Message: fmt.Sprintf("key %q must match ^[a-z0-9-]+$", in.Key),
		}
	}
	if declaredKey, ok := peekYAMLKey(in.YAML); ok && declaredKey != in.Key {
		return nil, &MalformedRequestError{
			Message: fmt.Sprintf("key %q does not match the YAML's declared key %q", in.Key, declaredKey),
		}
	}

	raw := []byte(in.YAML)
	mode := CompileModeStrict
	if in.ValidateOnly {
		mode = CompileModeDraft
	}
	compileInput := &CompileDungeonInput{
		Source:              raw,
		Mode:                mode,
		PartyStartSeatCount: o.partyStartSeatCount,
		PreviewSeed:         defaultPreviewSeed,
	}
	compiled, compileErr := o.compiler.CompileDungeon(ctx, compileInput)
	if compileErr != nil {
		return nil, fmt.Errorf("authoring orchestrator: compile dungeon %q: %w", in.Key, compileErr)
	}
	if compiled == nil {
		return nil, fmt.Errorf("authoring orchestrator: compile dungeon %q: provider returned no output", in.Key)
	}
	if len(compiled.FieldErrors) > 0 {
		fieldErrors := cloneFieldErrors(compiled.FieldErrors)
		return &PutDungeonOutput{Success: false, FieldErrors: fieldErrors}, nil
	}
	if compiled.FloorPlan == nil {
		return nil, fmt.Errorf("authoring orchestrator: compile dungeon %q: provider returned no floor plan", in.Key)
	}
	if compiled.FloorPlan.FloorSource != FloorSourceBounds && compiled.FloorPlan.FloorSource != FloorSourceRegions {
		return nil, fmt.Errorf("authoring orchestrator: compile dungeon %q: provider returned unresolved floor source", in.Key)
	}

	if in.ValidateOnly {
		return &PutDungeonOutput{Success: true, FloorPlan: compiled.FloorPlan}, nil
	}

	if err := o.writeThrough(in.Key, in.YAML); err != nil {
		return nil, fmt.Errorf("authoring orchestrator: write dungeon %q: %w", in.Key, err)
	}

	o.registry.Put(in.Key, dungeonregistry.Entry{
		Compiled: compiled.Compiled,
		Name:     captureName(raw, in.Key),
	})

	return &PutDungeonOutput{Success: true, FloorPlan: compiled.FloorPlan}, nil
}

func cloneFieldErrors(errors []FieldError) []FieldError {
	return append([]FieldError(nil), errors...)
}

// captureName reads raw's declared name: field the same way
// lobbyorch.LoadContentRegistry does (dungeonspec.Decode, separately from
// Load — CompiledDungeon carries no Name field). raw already passed
// dungeonspec.Load by the time this is called, so Decode succeeding is
// not in question — the fallback exists purely as defensive belt-and-
// suspenders, matching this repo's style elsewhere, not a reachable
// production path.
func captureName(raw []byte, fallback string) string {
	spec, err := dungeonspec.Decode(raw)
	if err != nil || spec.Name == "" {
		return fallback
	}
	return spec.Name
}

// writeThrough persists yamlText to the originating file for key (found
// via content.FilenameForKey) or, for a brand new key, ContentDir/<key>.yaml
// — design.md's Key rules: writing through to the originating file is
// what prevents a second file from ever declaring an already-used key,
// which content's registry (indexed by declared key, not filename) would
// otherwise resolve unpredictably.
func (o *Orchestrator) writeThrough(key, yamlText string) error {
	filename, ok, err := content.FilenameForKey(o.contentDir, key)
	if err != nil {
		return fmt.Errorf("resolve originating file for key %q: %w", key, err)
	}
	if !ok {
		filename = key + ".yaml"
	}
	path := filepath.Join(o.contentDir, filename)
	// ContentDir is operator-controlled, same posture as content.readOverrideDir.
	if err := os.WriteFile(path, []byte(yamlText), 0o600); err != nil { //nolint:gosec
		return fmt.Errorf("write %q: %w", path, err)
	}
	return nil
}
