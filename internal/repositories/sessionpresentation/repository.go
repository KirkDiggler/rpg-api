// Package sessionpresentation persists shared dice presentation plans and subscriptions.
package sessionpresentation

import (
	"context"
	"errors"
)

//go:generate mockgen -destination=mock/mock_repository.go -package=sessionpresentationmock github.com/KirkDiggler/rpg-api/internal/repositories/sessionpresentation Repository

var (
	ErrConflict = errors.New("session presentation repository: conflict")
	ErrClosed   = errors.New("session presentation repository: closed")
)

type PublishInput struct {
	Session        string
	PresentationID string
	Attempt        uint32
	Payload        []byte
}

type PublishOutput struct {
	Payload []byte
}

type SubscribeInput struct {
	Session string
}

type Subscription interface {
	Payloads() <-chan []byte
	Close() error
}

type Repository interface {
	Publish(context.Context, *PublishInput) (*PublishOutput, error)
	Subscribe(context.Context, *SubscribeInput) (Subscription, error)
}

func validatePublishInput(input *PublishInput) error {
	if input == nil {
		return errors.New("publish input is required")
	}
	if input.Session == "" {
		return errors.New("publish input session is required")
	}
	if input.PresentationID == "" {
		return errors.New("publish input presentation id is required")
	}
	if input.Attempt == 0 {
		return errors.New("publish input attempt is required")
	}
	if len(input.Payload) == 0 {
		return errors.New("publish input payload is required")
	}
	return nil
}

func validateSubscribeInput(input *SubscribeInput) error {
	if input == nil {
		return errors.New("subscribe input is required")
	}
	if input.Session == "" {
		return errors.New("subscribe input session is required")
	}
	return nil
}
