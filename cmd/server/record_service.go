package main

import (
	"errors"

	"google.golang.org/grpc"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	sessionhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha2"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
)

type recordServiceInput struct {
	Registrar   grpc.ServiceRegistrar
	Development bool
	Manager     sessionhandler.Manager
	Broker      *sessionorch.RecordBroker
	Gate        sessionhandler.Gate
}

// registerRecordService binds server-owned development mode to diagnostic
// access. It does not govern the ordinary StoryService or server-side capture.
func registerRecordService(in *recordServiceInput) error {
	if in == nil || in.Registrar == nil {
		return errors.New("record service: registrar is required")
	}
	handler, err := sessionhandler.New(&sessionhandler.HandlerConfig{
		Manager: in.Manager, Broker: in.Broker, Gate: in.Gate, Enabled: in.Development,
	})
	if err != nil {
		return err
	}
	sessionpb.RegisterRecordServiceServer(in.Registrar, handler)
	return nil
}
