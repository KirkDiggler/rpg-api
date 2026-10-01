package world

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	goredis "github.com/redis/go-redis/v9"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	redisclient "github.com/KirkDiggler/rpg-api/internal/redis"
)

const worldKeyPrefix = "world:v1:"

type RedisConfig struct{ Client redisclient.Client }
type redisRepository struct{ client redisclient.Client }

func NewRedis(cfg *RedisConfig) (Repository, error) {
	if cfg == nil || cfg.Client == nil {
		return nil, apierr.InvalidArgument("world repository requires a redis client")
	}
	return &redisRepository{client: cfg.Client}, nil
}

func (r *redisRepository) Get(ctx context.Context, input *GetInput) (*GetOutput, error) {
	if input == nil || !canonicalID(input.WorldID) {
		return nil, apierr.InvalidArgument("canonical world ID is required")
	}
	data, err := r.client.Get(ctx, worldKeyPrefix+input.WorldID).Bytes()
	if errors.Is(err, goredis.Nil) {
		return nil, apierr.NotFound("world is not configured")
	}
	if err != nil {
		return nil, apierr.Wrap(err, "read world configuration")
	}
	world, err := decode(data, input.WorldID)
	if err != nil {
		return nil, err
	}
	return &GetOutput{World: world}, nil
}

func (r *redisRepository) SetRoles(ctx context.Context, input *SetRolesInput) (*SetRolesOutput, error) {
	if input == nil || !validWorld(input.World) {
		return nil, apierr.InvalidArgument("world and all three canonical role IDs are required")
	}
	world := *input.World
	data, err := json.Marshal(world)
	if err != nil {
		return nil, apierr.Wrap(err, "encode world configuration")
	}
	// Authorization policy is durable: no gameplay/session TTL applies.
	if err := r.client.Set(ctx, worldKeyPrefix+world.WorldID, data, 0).Err(); err != nil {
		return nil, apierr.Wrap(err, "save world configuration")
	}
	return &SetRolesOutput{World: &world}, nil
}

func (r *redisRepository) SetMemberRoles(ctx context.Context, input *SetMemberRolesInput) (*SetMemberRolesOutput, error) {
	if input == nil || !canonicalID(input.WorldID) || !canonicalID(input.ExpectedAdminRoleID) ||
		!canonicalID(input.BuilderRoleID) || !canonicalID(input.PlayerRoleID) {
		return nil, apierr.InvalidArgument("world, expected admin and member role IDs are required")
	}
	key := worldKeyPrefix + input.WorldID
	var saved *entities.World
	err := r.client.Watch(ctx, func(tx *goredis.Tx) error {
		data, err := tx.Get(ctx, key).Bytes()
		if errors.Is(err, goredis.Nil) {
			return apierr.NotFound("world is not configured")
		}
		if err != nil {
			return err
		}
		world, err := decode(data, input.WorldID)
		if err != nil {
			return err
		}
		if world.AdminRoleID != input.ExpectedAdminRoleID {
			return apierr.Aborted("world admin policy changed; retry authorization")
		}
		world.BuilderRoleID = input.BuilderRoleID
		world.PlayerRoleID = input.PlayerRoleID
		stored, err := json.Marshal(world)
		if err != nil {
			return err
		}
		_, err = tx.TxPipelined(ctx, func(pipe goredis.Pipeliner) error {
			pipe.Set(ctx, key, stored, 0)
			return nil
		})
		if err != nil {
			return err
		}
		saved = world
		return nil
	}, key)
	if errors.Is(err, goredis.TxFailedErr) {
		return nil, apierr.Aborted("world policy changed; retry authorization")
	}
	if err != nil {
		return nil, apierr.Wrap(err, "update world member roles")
	}
	return &SetMemberRolesOutput{World: saved}, nil
}

func decode(data []byte, worldID string) (*entities.World, error) {
	var world entities.World
	if err := json.Unmarshal(data, &world); err != nil {
		return nil, apierr.Wrap(err, "decode world configuration")
	}
	if world.WorldID != worldID || !validWorld(&world) {
		return nil, apierr.Internal("stored world configuration is invalid or belongs to another world")
	}
	return &world, nil
}

func validWorld(world *entities.World) bool {
	return world != nil && canonicalID(world.WorldID) && canonicalID(world.AdminRoleID) &&
		canonicalID(world.BuilderRoleID) && canonicalID(world.PlayerRoleID)
}

func canonicalID(value string) bool {
	n, err := strconv.ParseUint(value, 10, 64)
	return err == nil && n != 0 && strconv.FormatUint(n, 10) == value
}
