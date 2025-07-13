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
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	grpc_logging "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	grpc_recovery "github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/recovery"
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

	healthServer := health.NewServer()
	grpc_health_v1.RegisterHealthServer(srv, healthServer)

	healthServer.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)

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
