package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
	"google.golang.org/grpc/status"

	grpc_logging "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	grpc_recovery "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api/gen/go/github.com/KirkDiggler/rpg-api/api/proto/dnd5e/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/services/character"
)

var (
	grpcPort int
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the gRPC server",
	Long:  `Start the RPG API gRPC server with all configured services.`,
	RunE:  runServer,
}

func init() {
	serverCmd.Flags().IntVar(&grpcPort, "port", 50051, "gRPC server port")
}

func runServer(cmd *cobra.Command, args []string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Received shutdown signal, gracefully stopping...")
		cancel()
	}()

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", grpcPort))
	if err != nil {
		return fmt.Errorf("failed to listen: %w", err)
	}

	srv := grpc.NewServer(
		grpc.ChainUnaryInterceptor(
			grpc_logging.UnaryServerInterceptor(grpc_logging.LoggerFunc(logFunc)),
			grpc_recovery.UnaryServerInterceptor(),
		),
		grpc.ChainStreamInterceptor(
			grpc_logging.StreamServerInterceptor(grpc_logging.LoggerFunc(logFunc)),
			grpc_recovery.StreamServerInterceptor(),
		),
	)

	// Initialize services (stub for now)
	// TODO: Replace with real service implementation
	characterService := &stubCharacterService{}

	// Initialize handlers
	characterHandler, err := v1alpha1.NewHandler(&v1alpha1.HandlerConfig{
		CharacterService: characterService,
	})
	if err != nil {
		return fmt.Errorf("failed to create character handler: %w", err)
	}

	// Register services
	dnd5ev1alpha1.RegisterCharacterServiceServer(srv, characterHandler)

	// Register health service
	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(srv, healthServer)

	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	healthServer.SetServingStatus("dnd5e.api.v1alpha1.CharacterService", grpc_health_v1.HealthCheckResponse_SERVING)

	reflection.Register(srv)

	errChan := make(chan error, 1)
	go func() {
		log.Printf("gRPC server starting on port %d...", grpcPort)
		if err := srv.Serve(lis); err != nil {
			errChan <- fmt.Errorf("failed to serve: %w", err)
		}
	}()

	select {
	case <-ctx.Done():
		log.Println("Shutting down gRPC server...")

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()

		stopped := make(chan struct{})
		go func() {
			srv.GracefulStop()
			close(stopped)
		}()

		select {
		case <-shutdownCtx.Done():
			log.Println("Graceful shutdown timeout exceeded, forcing stop")
			srv.Stop()
		case <-stopped:
			log.Println("Server stopped gracefully")
		}

		return nil
	case err := <-errChan:
		return err
	}
}

func logFunc(ctx context.Context, level grpc_logging.Level, msg string, fields ...any) {
	log.Printf("[%v] %s %v", level, msg, fields)
}

// stubCharacterService is a temporary stub implementation
// TODO: Remove when real service is implemented
type stubCharacterService struct{}

func (s *stubCharacterService) CreateDraft(ctx context.Context, input *character.CreateDraftInput) (*character.CreateDraftOutput, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (s *stubCharacterService) GetDraft(ctx context.Context, input *character.GetDraftInput) (*character.GetDraftOutput, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (s *stubCharacterService) ListDrafts(ctx context.Context, input *character.ListDraftsInput) (*character.ListDraftsOutput, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (s *stubCharacterService) DeleteDraft(ctx context.Context, input *character.DeleteDraftInput) (*character.DeleteDraftOutput, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (s *stubCharacterService) UpdateName(ctx context.Context, input *character.UpdateNameInput) (*character.UpdateNameOutput, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (s *stubCharacterService) UpdateRace(ctx context.Context, input *character.UpdateRaceInput) (*character.UpdateRaceOutput, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (s *stubCharacterService) UpdateClass(ctx context.Context, input *character.UpdateClassInput) (*character.UpdateClassOutput, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (s *stubCharacterService) UpdateBackground(ctx context.Context, input *character.UpdateBackgroundInput) (*character.UpdateBackgroundOutput, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (s *stubCharacterService) UpdateAbilityScores(ctx context.Context, input *character.UpdateAbilityScoresInput) (*character.UpdateAbilityScoresOutput, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (s *stubCharacterService) UpdateSkills(ctx context.Context, input *character.UpdateSkillsInput) (*character.UpdateSkillsOutput, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (s *stubCharacterService) ValidateDraft(ctx context.Context, input *character.ValidateDraftInput) (*character.ValidateDraftOutput, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (s *stubCharacterService) FinalizeDraft(ctx context.Context, input *character.FinalizeDraftInput) (*character.FinalizeDraftOutput, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (s *stubCharacterService) GetCharacter(ctx context.Context, input *character.GetCharacterInput) (*character.GetCharacterOutput, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (s *stubCharacterService) ListCharacters(ctx context.Context, input *character.ListCharactersInput) (*character.ListCharactersOutput, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}

func (s *stubCharacterService) DeleteCharacter(ctx context.Context, input *character.DeleteCharacterInput) (*character.DeleteCharacterOutput, error) {
	return nil, status.Error(codes.Unimplemented, "not implemented")
}
