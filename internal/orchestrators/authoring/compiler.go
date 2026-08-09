package authoring

import (
	"context"

	"github.com/KirkDiggler/rpg-toolkit/encounter/dungeonspec"
)

// CompileMode makes the lifecycle requested by the API explicit at the
// toolkit boundary. Draft compilation is structural and may return an empty,
// tiny, or disconnected floor. Strict compilation is the only mode accepted
// for an authored registry replacement or encounter start.
type CompileMode string

const (
	CompileModeDraft  CompileMode = "draft"
	CompileModeStrict CompileMode = "strict"
)

// FieldError is provider validation feedback. Field is the exact authored
// source path (for example "regions[1].cells" or "start"); the API must not
// parse or reconstruct it from Message.
type FieldError struct {
	Field   string
	Message string
	Code    string
}

// CompileDungeonInput is the complete candidate passed to the provider.
// Previous is opaque compiler state used only for candidate-update checks;
// API orchestration never reads or modifies it.
type CompileDungeonInput struct {
	Source              []byte
	Mode                CompileMode
	PartyStartSeatCount int
	PreviewSeed         int64
	Previous            *dungeonspec.CompiledDungeon
}

// CompileDungeonOutput is either a complete provider result (Compiled and
// FloorPlan) or exact validation feedback. Provider/transport failures are the
// Go error return; authored validation is FieldErrors so the handler can keep
// the existing gRPC-OK authoring response.
type CompileDungeonOutput struct {
	Compiled    dungeonspec.CompiledDungeon
	FloorPlan   *FloorPlan
	Name        string
	FieldErrors []FieldError
}

//go:generate mockgen -destination=mock/mock_compiler.go -package=authoringmock github.com/KirkDiggler/rpg-api/internal/orchestrators/authoring Compiler

// Compiler is the narrow, protobuf-free toolkit seam demanded by Wave A. It
// owns decode, union/envelope construction, PartyCap/entrance/connectivity and
// every dependent-content decision. API supplies lifecycle intent and stores
// the returned authored compiler state; it never computes topology.
type Compiler interface {
	CompileDungeon(ctx context.Context, in *CompileDungeonInput) (*CompileDungeonOutput, error)
}
