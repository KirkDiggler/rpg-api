package sessionpresentation

import "context"

//go:generate mockgen -destination=mock/mock_service.go -package=sessionpresentationmock github.com/KirkDiggler/rpg-api/internal/orchestrators/sessionpresentation Service

type PublishInput struct {
	Session string
	Member  string
	Draft   Draft
}

type PublishOutput struct {
	Plan Plan
}

type SubscribeInput struct {
	Session string
	Member  string
}

type Subscription interface {
	Plans() <-chan Plan
	Close() error
}

type Service interface {
	Publish(context.Context, *PublishInput) (*PublishOutput, error)
	Subscribe(context.Context, *SubscribeInput) (Subscription, error)
}
