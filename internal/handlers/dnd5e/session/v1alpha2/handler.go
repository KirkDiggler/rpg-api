// Package sessionv1alpha2 serves session v1alpha2: the record stream
// (rpg-project#628) and the act stream (rpg-project#630), ideas/event-stream
// R5, R12, R13, R15, R22, R28. v1alpha2 holds only the streams; every verb
// stays on v1alpha1 (R2).
//
// THE ACT STREAM IS PER RECIPIENT. StoryHandler serves one member's acts,
// folded by the SDK from that member's own story; the request's member is the
// recipient and the seat gate.
//
// THE RECORD IS NOT PER RECIPIENT. It is one log of the whole run, every
// creature's beats and every per-observer beat included, and a reader gets
// all of it (R13, R26); rpg-api neither filters nor regroups it.
//
// IN DEV MODE ANY RUN'S RECORD CAN BE READ (rpg-project#1001, ruled: "in dev
// mode we can play any record"). The record service answers only in
// development mode (HandlerConfig.Enabled), and there it asks for a
// signed-in caller and the run's session id, no seat: a reader who was never
// seated, a member who left, and a reader of a run that has ended all read
// the same whole record. Outside development mode it refuses every call, as
// it always has. Exact JSON bytes are carried in fields_json;
// the deprecated Struct is only an approximate projection. A payload that cannot
// become a Struct still retains its identity and raw bytes so the reader reports
// unavailable/unsupported evidence without a hole or an approximate fallback.
package sessionv1alpha2

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
)

//go:generate mockgen -destination=mock/mock_manager.go -package=sessionv1alpha2mock github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha2 Manager,Gate

// Manager is the slice of the toolkit session.Manager these services call.
type Manager interface {
	Record(ctx context.Context, in *sdk.RecordReadInput) (*sdk.RecordReadOutput, error)
	Acts(ctx context.Context, in *sdk.ActsInput) (*sdk.ActsOutput, error)
}

// Gate is the story's seat check, shared with v1alpha1's StreamEvents: the
// caller must be seated as the member it names (sessionaccess.Access). The
// record does not use it (see the package comment).
type Gate interface {
	CallerMemberSeated(ctx context.Context, session, member string) error
}

// Handler implements sessionpb.RecordServiceServer.
type Handler struct {
	sessionpb.UnimplementedRecordServiceServer

	manager Manager
	broker  *sessionorch.RecordBroker
	enabled bool
}

var errRecordDisabled = status.Error(codes.PermissionDenied, "diagnostic record access is disabled")

// HandlerConfig supplies required dependencies and explicit diagnostic access.
type HandlerConfig struct {
	Manager Manager
	Broker  *sessionorch.RecordBroker
	// Enabled is development/playtest access, never inferred from client UI.
	// False refuses both record RPCs; the ordinary story service is separate.
	Enabled bool
}

// New builds a Handler, refusing a config missing any dependency.
func New(cfg *HandlerConfig) (*Handler, error) {
	if cfg == nil {
		return nil, errors.New("session v1alpha2 handler: HandlerConfig is required")
	}
	if cfg.Manager == nil {
		return nil, errors.New("session v1alpha2 handler: HandlerConfig.Manager is required")
	}
	if cfg.Broker == nil {
		return nil, errors.New("session v1alpha2 handler: HandlerConfig.Broker is required")
	}
	return &Handler{manager: cfg.Manager, broker: cfg.Broker, enabled: cfg.Enabled}, nil
}

// StoryHandler implements sessionpb.StoryServiceServer: the act stream.
type StoryHandler struct {
	sessionpb.UnimplementedStoryServiceServer

	manager Manager
	broker  *sessionorch.ActBroker
	gate    Gate
}

// StoryHandlerConfig is everything the story handler needs; every field is
// required.
type StoryHandlerConfig struct {
	Manager Manager
	Broker  *sessionorch.ActBroker
	Gate    Gate
}

// NewStory builds a StoryHandler, refusing a config missing any dependency.
func NewStory(cfg *StoryHandlerConfig) (*StoryHandler, error) {
	if cfg == nil {
		return nil, errors.New("session v1alpha2 story handler: StoryHandlerConfig is required")
	}
	if cfg.Manager == nil {
		return nil, errors.New("session v1alpha2 story handler: StoryHandlerConfig.Manager is required")
	}
	if cfg.Broker == nil {
		return nil, errors.New("session v1alpha2 story handler: StoryHandlerConfig.Broker is required")
	}
	if cfg.Gate == nil {
		return nil, errors.New("session v1alpha2 story handler: StoryHandlerConfig.Gate is required")
	}
	return &StoryHandler{manager: cfg.Manager, broker: cfg.Broker, gate: cfg.Gate}, nil
}

const (
	errReaderSessionRequired = "session is required"
	errReaderNotSignedIn     = "a signed-in caller is required"
)

// readerOf is who may read a run's record in development mode: any signed-in
// caller who names the run. No seat is asked for; the member a request
// names, if any, is not consulted.
func readerOf(ctx context.Context, session string) error {
	if auth.GetPlayerID(ctx) == "" {
		return status.Error(codes.Unauthenticated, errReaderNotSignedIn)
	}
	if session == "" {
		return status.Error(codes.InvalidArgument, errReaderSessionRequired)
	}
	return nil
}
