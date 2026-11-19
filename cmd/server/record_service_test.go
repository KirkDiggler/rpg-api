package main

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	sessionmock "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha2/mock"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
)

// Holds the actual handler produced by server wiring, not a mocked RPC.
type recordRegistrar struct{ handler sessionpb.RecordServiceServer }

func (r *recordRegistrar) RegisterService(_ *grpc.ServiceDesc, impl any) {
	r.handler = impl.(sessionpb.RecordServiceServer)
}

// devModeValues is every AUTH_DEV_MODE value the parse must read as off,
// and the one it reads as on. Unset is checked apart.
var devModeValues = map[string]bool{
	"": false, "false": false, "0": false, "TRUE": false, "True": false,
	" true": false, "true ": false, "1": false, "yes": false, "on": false,
	"true": true,
}

// TestDevModeIsExactlyTrue pins the one read of AUTH_DEV_MODE: exactly
// "true" is development mode; anything else, unset included, is not.
func TestDevModeIsExactlyTrue(t *testing.T) {
	for value, want := range devModeValues {
		t.Run(fmt.Sprintf("%q", value), func(t *testing.T) {
			t.Setenv(envAuthDevMode, value)
			require.Equal(t, want, configuredDevMode())
			require.Equal(t, want, newAuthConfig(false).DevMode, "the auth configuration carries the same decision")
		})
	}
	t.Run("unset", func(t *testing.T) {
		t.Setenv(envAuthDevMode, "x")
		require.NoError(t, os.Unsetenv(envAuthDevMode))
		require.False(t, configuredDevMode())
	})
}

// TestRecordServiceServesExactlyInDevMode is the composition, without a
// server: AUTH_DEV_MODE is read once (newAuthConfig) and the record service
// is built from that configuration. Off, every read is refused before any
// data is touched; on, a signed-in caller reads a run with no seat.
func TestRecordServiceServesExactlyInDevMode(t *testing.T) {
	for value, development := range devModeValues {
		t.Run(fmt.Sprintf("%q", value), func(t *testing.T) {
			t.Setenv(envAuthDevMode, value)
			ctrl := gomock.NewController(t)
			manager := sessionmock.NewMockManager(ctrl)
			var registrar recordRegistrar
			require.NoError(t, registerRecordService(&recordServiceInput{Registrar: &registrar,
				Auth: newAuthConfig(false), Manager: manager, Broker: sessionorch.NewRecordBroker(1)}))
			if development {
				manager.EXPECT().Record(gomock.Any(), gomock.Any()).Return(&sdk.RecordReadOutput{}, nil)
			}
			out, err := registrar.handler.GetRecord(auth.WithPlayerID(context.Background(), "alice"),
				&sessionpb.GetRecordRequest{Session: "run"})
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

// TestRecordServiceNeedsTheAuthConfiguration: no configuration, no service.
func TestRecordServiceNeedsTheAuthConfiguration(t *testing.T) {
	var registrar recordRegistrar
	err := registerRecordService(&recordServiceInput{Registrar: &registrar,
		Manager: sessionmock.NewMockManager(gomock.NewController(t)), Broker: sessionorch.NewRecordBroker(1)})
	require.Error(t, err)
}
