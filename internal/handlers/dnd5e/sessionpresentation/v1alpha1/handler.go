package sessionpresentationv1alpha1

import (
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	presentationpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/presentation/v1alpha1"
	sessionaccess "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/sessionaccess"
	orchsessionpresentation "github.com/KirkDiggler/rpg-api/internal/orchestrators/sessionpresentation"
)

const (
	errHandlerConfigRequired          = "session presentation handler: HandlerConfig is required"
	errHandlerServiceRequired         = "session presentation handler: HandlerConfig.Service is required"
	errHandlerAccessRequired          = "session presentation handler: HandlerConfig.Access is required"
	errPublishOutputRequired          = "session presentation: publish returned no output"
	errSubscriptionRequired           = "session presentation: subscribe returned no subscription"
	errInvalidDiceThrowPlan           = "invalid dice throw plan"
	errDiceThrowAttemptAlreadyExists  = "dice throw attempt already exists"
	errSessionPresentationUnavailable = "session presentation unavailable"
)

type HandlerConfig struct {
	Service orchsessionpresentation.Service
	Access  *sessionaccess.Access
}

type Handler struct {
	presentationpb.UnimplementedSessionPresentationServiceServer

	service orchsessionpresentation.Service
	access  *sessionaccess.Access
}

func New(cfg *HandlerConfig) (*Handler, error) {
	if cfg == nil {
		return nil, errors.New(errHandlerConfigRequired)
	}
	if cfg.Service == nil {
		return nil, errors.New(errHandlerServiceRequired)
	}
	if cfg.Access == nil {
		return nil, errors.New(errHandlerAccessRequired)
	}
	return &Handler{service: cfg.Service, access: cfg.Access}, nil
}

func sessionPresentationRPCError(err error) error {
	switch {
	case errors.Is(err, orchsessionpresentation.ErrInvalidPlan):
		return status.Error(codes.InvalidArgument, errInvalidDiceThrowPlan)
	case errors.Is(err, orchsessionpresentation.ErrConflict):
		return status.Error(codes.AlreadyExists, errDiceThrowAttemptAlreadyExists)
	case errors.Is(err, orchsessionpresentation.ErrClosed):
		return status.Error(codes.Internal, errSessionPresentationUnavailable)
	default:
		return status.Error(codes.Internal, errSessionPresentationUnavailable)
	}
}
