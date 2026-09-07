// Copyright (C) 2024 Kirk Diggler
// SPDX-License-Identifier: GPL-3.0-or-later

// sandboxseed resets the fixed toolkit-contributor sandbox characters through
// the production CharacterService exposed by Envoy. It also supports a
// dev-only repeatable weapon gallery fixture for UI inventory work.
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
	redisclient "github.com/KirkDiggler/rpg-api/internal/redis"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-api/internal/sandboxseed"
)

const (
	defaultAddress      = "localhost:8080"
	defaultRedisAddress = "localhost:6379"

	fixtureDefault       = "default"
	fixtureWeaponGallery = "weapon-gallery"
)

type config struct {
	address      string
	redisAddress string
	fixture      string
	health       bool
}

type galleryStoreHandle interface {
	sandboxseed.CharacterStore
	io.Closer
}

type commandDeps struct {
	connect          func(address string) (*grpc.ClientConn, error)
	characterClient  func(*grpc.ClientConn) sandboxseed.CharacterRPC
	seedDefault      func(context.Context, sandboxseed.CharacterRPC) error
	seedGallery      func(context.Context, *sandboxseed.SeedWeaponGalleryInput) (*sandboxseed.SeedWeaponGalleryOutput, error)
	openGalleryStore func(context.Context, string) (galleryStoreHandle, error)
	checkHealth      func(context.Context, *grpc.ClientConn) error
	stdout           io.Writer
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Fatalf("sandboxseed: %v", err)
	}
}

func run(args []string) error {
	return runWithDeps(args, commandDeps{})
}

func runWithDeps(args []string, deps commandDeps) error {
	config, err := parseConfig(args)
	if err != nil {
		return err
	}
	deps = deps.withDefaults()

	conn, err := deps.connect(config.address)
	if err != nil {
		return fmt.Errorf("connect: %w", err)
	}
	if conn != nil {
		defer func() { _ = conn.Close() }()
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	if config.health {
		return deps.checkHealth(ctx, conn)
	}

	client := deps.characterClient(conn)
	switch config.fixture {
	case fixtureDefault:
		if err := deps.seedDefault(ctx, client); err != nil {
			return fmt.Errorf("seed: %w", err)
		}
		return nil
	case fixtureWeaponGallery:
		store, err := deps.openGalleryStore(ctx, config.redisAddress)
		if err != nil {
			return err
		}
		defer func() { _ = store.Close() }()
		out, err := deps.seedGallery(ctx, &sandboxseed.SeedWeaponGalleryInput{Client: client, Store: store})
		if err != nil {
			return fmt.Errorf("seed %s: %w", fixtureWeaponGallery, err)
		}
		if _, err := fmt.Fprintf(
			deps.stdout,
			"sandboxseed: fixture=%s character_id=%s weapon_count=%d\n",
			fixtureWeaponGallery,
			out.CharacterID,
			out.WeaponCount,
		); err != nil {
			return fmt.Errorf("write seed result: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("unsupported fixture %q", config.fixture)
	}
}

func (d commandDeps) withDefaults() commandDeps {
	if d.connect == nil {
		d.connect = func(address string) (*grpc.ClientConn, error) {
			return grpc.NewClient(address, grpc.WithTransportCredentials(insecure.NewCredentials()))
		}
	}
	if d.characterClient == nil {
		d.characterClient = func(conn *grpc.ClientConn) sandboxseed.CharacterRPC {
			return dnd5ev1alpha1.NewCharacterServiceClient(conn)
		}
	}
	if d.seedDefault == nil {
		d.seedDefault = sandboxseed.Seed
	}
	if d.seedGallery == nil {
		d.seedGallery = sandboxseed.SeedWeaponGallery
	}
	if d.openGalleryStore == nil {
		d.openGalleryStore = openRedisGalleryStore
	}
	if d.checkHealth == nil {
		d.checkHealth = checkHealth
	}
	if d.stdout == nil {
		d.stdout = os.Stdout
	}
	return d
}

func parseConfig(args []string) (*config, error) {
	flags := flag.NewFlagSet("sandboxseed", flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	result := &config{}
	flags.StringVar(&result.address, "address", defaultAddress, "gRPC address for Envoy")
	flags.StringVar(&result.redisAddress, "redis-address", defaultRedisAddress, "Redis address for repository-backed fixtures")
	flags.StringVar(&result.fixture, "fixture", fixtureDefault, "fixture to seed: default or weapon-gallery")
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
	if result.fixture != fixtureDefault && result.fixture != fixtureWeaponGallery {
		return nil, errors.New("fixture must be default or weapon-gallery")
	}
	if result.fixture == fixtureWeaponGallery && !result.health && result.redisAddress == "" {
		return nil, errors.New("redis address is required for weapon-gallery fixture")
	}
	return result, nil
}

func openRedisGalleryStore(ctx context.Context, redisAddress string) (galleryStoreHandle, error) {
	client, err := redisclient.NewClient(redisAddress, nil)
	if err != nil {
		return nil, fmt.Errorf("redis connect: %w", err)
	}
	pingErr := client.Ping(ctx).Err()
	if pingErr != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis ping %s: %w", redisAddress, pingErr)
	}
	repository, err := characterrepo.NewRedis(&characterrepo.RedisConfig{Client: client})
	if err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("character repository: %w", err)
	}
	return &redisGalleryStore{CharacterStore: repository, close: client.Close}, nil
}

type redisGalleryStore struct {
	sandboxseed.CharacterStore
	close func() error
}

func (s *redisGalleryStore) Close() error { return s.close() }

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
