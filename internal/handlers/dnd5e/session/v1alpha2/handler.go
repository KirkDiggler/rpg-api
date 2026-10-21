// Package sessionv1alpha2 serves session v1alpha2: the record stream
// (rpg-project#628, ideas/event-stream R5, R12, R13, R15, R28). v1alpha2
// holds only the streams; every verb stays on v1alpha1 (R2).
//
// THE RECORD IS NOT PER RECIPIENT. Every seated caller receives the whole
// record, per-observer beats included, before release (R13, R26); rpg-api
// neither filters nor regroups it. A beat whose fields cannot be spelled as a
// Struct still goes out — id, beat and act intact, fields empty — so one bad
// payload never leaves a hole in the stream.
package sessionv1alpha2

import (
	"context"
	"errors"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
)

//go:generate mockgen -destination=mock/mock_manager.go -package=sessionv1alpha2mock github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha2 Manager,Gate

// Manager is the slice of the toolkit session.Manager this service calls.
type Manager interface {
	Record(ctx context.Context, in *sdk.RecordReadInput) (*sdk.RecordReadOutput, error)
}

// Gate is the seat check both RPCs share with v1alpha1's StreamEvents:
// the caller must be seated as the member it names (sessionaccess.Access).
type Gate interface {
	CallerMemberSeated(ctx context.Context, session, member string) error
}

// Handler implements sessionpb.RecordServiceServer.
type Handler struct {
	sessionpb.UnimplementedRecordServiceServer

	manager Manager
	broker  *sessionorch.RecordBroker
	gate    Gate
}

// HandlerConfig is everything the handler needs; every field is required.
type HandlerConfig struct {
	Manager Manager
	Broker  *sessionorch.RecordBroker
	Gate    Gate
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
	if cfg.Gate == nil {
		return nil, errors.New("session v1alpha2 handler: HandlerConfig.Gate is required")
	}
	return &Handler{manager: cfg.Manager, broker: cfg.Broker, gate: cfg.Gate}, nil
}
