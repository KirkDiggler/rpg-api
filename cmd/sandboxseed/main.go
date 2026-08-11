// Copyright (C) 2024 Kirk Diggler
// SPDX-License-Identifier: GPL-3.0-or-later

// sandboxseed resets the fixed toolkit-contributor sandbox characters through
// the production CharacterService exposed by Envoy.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	grpc_health_v1 "google.golang.org/grpc/health/grpc_health_v1"

	dnd5ev1alpha1 "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/sandboxseed"
)

const defaultAddress = "localhost:8080"

type config struct {
	address string
	health  bool
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatalf("sandboxseed: %v", err)
	}
}

func run(args []string) error {
	config, err := parseConfig(args)
	if err != nil {
		return err
	}

	conn, err := grpc.NewClient(config.address, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	if config.health {
		return checkHealth(ctx, conn)
	}
	if err := sandboxseed.Seed(ctx, dnd5ev1alpha1.NewCharacterServiceClient(conn)); err != nil {
		return fmt.Errorf("seed: %w", err)
	}
	return nil
}

func parseConfig(args []string) (*config, error) {
	flags := flag.NewFlagSet("sandboxseed", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	result := &config{}
	flags.StringVar(&result.address, "address", defaultAddress, "gRPC address for Envoy")
	flags.BoolVar(&result.health, "health", false, "check Envoy gRPC health only")
	if err := flags.Parse(args); err != nil {
		return nil, err
	}
	if flags.NArg() != 0 {
		return nil, fmt.Errorf("unexpected positional arguments: %v", flags.Args())
	}
	if result.address == "" {
		return nil, errors.New("address is required")
	}
	return result, nil
}

func checkHealth(ctx context.Context, conn *grpc.ClientConn) error {
	response, err := grpc_health_v1.NewHealthClient(conn).Check(ctx, &grpc_health_v1.HealthCheckRequest{})
	if err != nil {
		return fmt.Errorf("health check: %w", err)
	}
	if response.GetStatus() != grpc_health_v1.HealthCheckResponse_SERVING {
		return fmt.Errorf("health check is not serving: %s", response.GetStatus())
	}
	fmt.Println("sandboxseed: health=SERVING")
	return nil
}
