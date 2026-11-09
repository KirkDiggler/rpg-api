package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	sessionmock "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha2/mock"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
)

// Holds the actual handler produced by server wiring, not a mocked RPC.
type recordRegistrar struct{ handler sessionpb.RecordServiceServer }

func (r *recordRegistrar) RegisterService(_ *grpc.ServiceDesc, impl any) {
	r.handler = impl.(sessionpb.RecordServiceServer)
}

func TestRecordServiceWiringIsDevelopmentOnly(t *testing.T) {
	for _, development := range []bool{false, true} {
		t.Run(map[bool]string{false: "release", true: "development"}[development], func(t *testing.T) {
			ctrl := gomock.NewController(t)
			manager, gate := sessionmock.NewMockManager(ctrl), sessionmock.NewMockGate(ctrl)
			var registrar recordRegistrar
			err := registerRecordService(&recordServiceInput{Registrar: &registrar, Development: development,
				Manager: manager, Broker: sessionorch.NewRecordBroker(1), Gate: gate})
			require.NoError(t, err)
			if development {
				gate.EXPECT().CallerMemberSeated(gomock.Any(), "run", "alice").Return(nil)
				manager.EXPECT().Record(gomock.Any(), gomock.Any()).Return(&sdk.RecordReadOutput{}, nil)
			}
			out, err := registrar.handler.GetRecord(context.Background(), &sessionpb.GetRecordRequest{Session: "run", Member: "alice"})
			if development {
				require.NoError(t, err)
				require.NotNil(t, out)
			} else {
				require.Equal(t, codes.PermissionDenied, status.Code(err))
				require.Nil(t, out)
			}
		})
	}
}
