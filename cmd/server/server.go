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

	grpc_logging "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	grpc_recovery "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/clients/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/clients/external"
	"github.com/KirkDiggler/rpg-api/internal/engine"
	"github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/orchestrators/character"
	"github.com/KirkDiggler/rpg-api/internal/pkg/clock"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	"github.com/KirkDiggler/rpg-api/internal/redis"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	characterdraftrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character_draft"
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

func runServer(_ *cobra.Command, _ []string) error {
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

	charRepo, err := characterrepo.NewRedis(&characterrepo.RedisConfig{
		Client: mustRedisClient(),
	})
	if err != nil {
		return fmt.Errorf("failed to create character repository: %w", err)
	}

	draftRepo, err := characterdraftrepo.NewRedis(&characterdraftrepo.Config{
		Clock:       clock.New(),
		IDGenerator: idgen.NewPrefixed("draft-"),
		Client:      mustRedisClient(),
	})
	if err != nil {
		return fmt.Errorf("failed to create character draft repository: %w", err)
	}

	e, err := engine.New(&engine.Config{})
	if err != nil {
		return fmt.Errorf("failed to create engine: %w", err)
	}

	client, err := external.New(&external.Config{
		BaseURL:     "https://www.dnd5eapi.co/api/2014/",
		CacheTTL:    24 * time.Hour,
		HTTPTimeout: 30 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("failed to create external client: %w", err)
	}

	// Initialize services
	characterService, err := character.New(&character.Config{
		CharacterRepo:      charRepo,
		CharacterDraftRepo: draftRepo,
		Engine:             e,
		ExternalClient:     client,
	})
	if err != nil {
		return fmt.Errorf("failed to create character service: %w", err)
	}

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
	// Extract useful information from fields
	var method, code, errorMsg string
	var timeMs float64

	// Parse fields (they come in pairs: key, value)
	for i := 0; i < len(fields)-1; i += 2 {
		key, ok := fields[i].(string)
		if !ok {
			continue
		}

		switch key {
		case "grpc.method":
			method, _ = fields[i+1].(string)
		case "grpc.code":
			if codeVal, ok := fields[i+1].(codes.Code); ok {
				code = codeVal.String()
			} else if codeStr, ok := fields[i+1].(string); ok {
				code = codeStr
			}
		case "grpc.error":
			if err, ok := fields[i+1].(error); ok {
				errorMsg = err.Error()
			} else if errStr, ok := fields[i+1].(string); ok {
				errorMsg = errStr
			}
		case "grpc.time_ms":
			timeMs, _ = fields[i+1].(float64)
		}
	}

	// Format based on message type
	switch msg {
	case "started call":
		log.Printf("→ %s started", method)
	case "finished call":
		if code == "OK" || code == "0" {
			log.Printf("✓ %s completed in %.2fms", method, timeMs)
		} else {
			log.Printf("✗ %s failed (%s) in %.2fms: %s", method, code, timeMs, errorMsg)
		}
	default:
		// Fallback to original format for other messages
		log.Printf("[%v] %s %v", level, msg, fields)
	}
}

func mustRedisClient() redis.Client {
	client, err := redis.NewClient("localhost:6379", nil)
	if err != nil {
		panic(err)
	}
	return client
}
