package session

import (
	"context"
	"errors"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

// recordPublisher hands the same committed evidence to independent consumers.
// A capture failure neither suppresses browser delivery nor changes the record.
// Both consumers own their asynchronous buffers; this adds no worker or rules.
type recordPublisher struct {
	broker  *RecordBroker
	capture sdk.RecordStream
}

func (p *recordPublisher) PublishRecord(ctx context.Context, events []sdk.RecordEvent) error {
	browserErr := p.broker.PublishRecord(ctx, events)
	if p.capture == nil {
		return browserErr
	}
	return errors.Join(browserErr, p.capture.PublishRecord(ctx, events))
}
