package authoring

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"gopkg.in/yaml.v3"

	tkenc "github.com/KirkDiggler/rpg-toolkit/encounter"
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
// CONTENT failed dungeonspec's validate/compile (Success=false,
// FieldError carries the one flat v1-limitation message, FloorPlan nil)
// OR it succeeded (Success=true, FloorPlan set). A malformed REQUEST
// never produces one of these — PutDungeon returns a
// *MalformedRequestError instead, never a *PutDungeonOutput, for that
// case (see MalformedRequestError's doc for why the two must stay
// distinguishable in type/shape, not collapsed into one "return an
// error" path).
type PutDungeonOutput struct {
	Success    bool
	FieldError string
	FloorPlan  *FloorPlan
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
//  2. dungeonspec.Load (decode + validate + compile). A failure here is a
//     well-formed request whose CONTENT is the problem — PutDungeonOutput
//     with Success=false, distinct in TYPE from step 1's failure, never
//     collapsed into the same path.
//  3. Build the FloorPlan (a throwaway InitDungeon call, mirroring
//     dungeonspec.WorkbenchReport's own pattern) — a failure here is
//     ALSO a content failure (rpg-toolkit#842's own gate finding: a spec
//     can Load cleanly and still fail InitDungeon), same Success=false
//     treatment as step 2.
//  4. ValidateOnly stops here: FloorPlan returned, nothing persisted,
//     nothing registered.
//  5. Write-through to ContentDir FIRST. A write failure fails the RPC
//     (a real Go error, not a PutDungeonOutput) and leaves the registry
//     untouched — this ordering is what prevents a spec from playing now
//     and vanishing after the next restart because it was never actually
//     written to disk (design.md's "write-then-swap" decision).
//  6. Only on a successful write does the registry swap happen.
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
	compiled, loadErr := dungeonspec.Load(raw)
	if loadErr != nil {
		return &PutDungeonOutput{Success: false, FieldError: loadErr.Error()}, nil
	}

	floorPlan, buildErr := buildFloorPlan(ctx, compiled, defaultPreviewSeed)
	if buildErr != nil {
		return &PutDungeonOutput{Success: false, FieldError: buildErr.Error()}, nil
	}

	if in.ValidateOnly {
		return &PutDungeonOutput{Success: true, FloorPlan: floorPlan}, nil
	}

	if err := o.writeThrough(in.Key, in.YAML); err != nil {
		return nil, fmt.Errorf("authoring orchestrator: write dungeon %q: %w", in.Key, err)
	}

	o.registry.Put(in.Key, dungeonregistry.Entry{Compiled: compiled, Name: captureName(raw, in.Key)})

	return &PutDungeonOutput{Success: true, FloorPlan: floorPlan}, nil
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

// buildFloorPlan runs a throwaway Encounter.InitDungeon(compiled.Params)
// at seed — the same compile-and-describe pattern
// dungeonspec.WorkbenchReport already uses outside a server (reused here,
// not reinvented) — then projects the result into FloorPlan. Rooms'
// StartColumn and connectors' Column replicate InitDungeon's own chain-
// layout arithmetic exactly (rpg-toolkit's dungeon.go: starts[i] = x;
// x += region.Width + 1 per region; a connector's column is its LEFT
// region's start + that region's width) — this is the one place in the
// whole arc allowed to do that arithmetic, because it's the producer of
// the compiled layout, not a consumer re-deriving it. Entrance comes
// directly off the throwaway encounter's committed Data.Space.Entrance
// (a core.Hex cube coordinate) via Hex.ToPosition(), which itself calls
// spatial.CubeCoordinate.ToOffsetCoordinate() under pointy-top orientation
// — reused via the toolkit's own existing conversion helper (the same one
// dungeonspec.WorkbenchReport's writeFloorPlan already calls for this
// exact purpose) rather than hand-rolled here.
func buildFloorPlan(ctx context.Context, compiled dungeonspec.CompiledDungeon, seed int64) (*FloorPlan, error) {
	params := compiled.Params
	params.RandomSeed = seed

	transport := tkenc.NewInMemoryTransport()
	broker := tkenc.NewBroker(transport)
	enc := tkenc.New(ctx, "authoring-preview", broker)
	if err := enc.InitDungeon(params); err != nil {
		return nil, fmt.Errorf("init dungeon: %w", err)
	}

	regions := params.Regions
	starts := make([]int, len(regions))
	x := 0
	for i, r := range regions {
		starts[i] = x
		x += r.Width + 1
	}

	rooms := make([]FloorPlanRoom, len(regions))
	for i, r := range regions {
		rooms[i] = FloorPlanRoom{
			ID:          r.ID,
			Archetype:   string(r.Archetype),
			Width:       r.Width,
			StartColumn: starts[i],
		}
	}

	connectorParams := params.Connectors
	connectors := make([]FloorPlanConnector, len(connectorParams))
	for i, c := range connectorParams {
		connectors[i] = FloorPlanConnector{
			DoorID:     string(c.DoorID),
			Locked:     c.Locked,
			FromRoomID: regions[i].ID,
			ToRoomID:   regions[i+1].ID,
			Column:     starts[i] + regions[i].Width,
		}
	}

	generatedEdges, err := enc.DescribeGeneratedEdges(tkenc.DescribeGeneratedEdgesInput{})
	if err != nil {
		return nil, fmt.Errorf("describe generated edges: %w", err)
	}
	edges := make([]FloorPlanEdge, len(generatedEdges.Edges))
	for i, edge := range generatedEdges.Edges {
		from := edge.From.ToPosition()
		to := edge.To.ToPosition()
		edges[i] = FloorPlanEdge{
			From:   FloorPlanCell{Column: int(from.X), Row: int(from.Y)},
			To:     FloorPlanCell{Column: int(to.X), Row: int(to.Y)},
			Kind:   FloorPlanEdgeKind(edge.Kind),
			DoorID: string(edge.DoorID),
		}
	}

	entrancePos := enc.ToData().Space.Entrance.ToPosition()

	return &FloorPlan{
		Rooms:      rooms,
		Connectors: connectors,
		Height:     params.Height,
		DoorRow:    params.Height / 2,
		Entrance:   FloorPlanCell{Column: int(entrancePos.X), Row: int(entrancePos.Y)},
		Edges:      edges,
	}, nil
}
