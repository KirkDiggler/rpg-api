package main

import (
	"errors"

	"google.golang.org/grpc"

	"github.com/KirkDiggler/rpg-api/internal/auth"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	sessionhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha2"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
)

type recordServiceInput struct {
	Registrar grpc.ServiceRegistrar
	// Auth is the server's auth configuration (newAuthConfig): the record
	// service serves exactly when it accepts development authentication.
	// Taken whole rather than as a bool, so no call site can hand the
	// service a development switch of its own.
	Auth    *auth.InterceptorConfig
	Manager sessionhandler.Manager
	Broker  *sessionorch.RecordBroker
}

// registerRecordService binds server-owned development mode to diagnostic
// access. It does not govern the ordinary StoryService or server-side capture.
func registerRecordService(in *recordServiceInput) error {
	if in == nil || in.Registrar == nil {
		return errors.New("record service: registrar is required")
	}
	if in.Auth == nil {
		return errors.New("record service: auth configuration is required")
	}
	handler, err := sessionhandler.New(&sessionhandler.HandlerConfig{
		Manager: in.Manager, Broker: in.Broker, Enabled: in.Auth.DevMode,
	})
	if err != nil {
		return err
	}
	sessionpb.RegisterRecordServiceServer(in.Registrar, handler)
	return nil
}
