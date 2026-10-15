package worlddungeons

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"unicode/utf8"

	goredis "github.com/redis/go-redis/v9"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	redisclient "github.com/KirkDiggler/rpg-api/internal/redis"
)

// worldKeyPrefix names one hash per world; each field is a dungeon key.
const worldKeyPrefix = "dungeon:world:"

// maxTxRetries is how many times a write retries after another client
// changed the world's hash between WATCH and EXEC.
const maxTxRetries = 3

type redisRepository struct {
	client redisclient.Client
}

// RedisConfig contains the Redis dependency for the repository.
type RedisConfig struct {
	Client redisclient.Client
}

// Validate verifies that the repository has its required dependency.
func (cfg *RedisConfig) Validate() error {
	if cfg == nil {
		return apierr.InvalidArgument("config cannot be nil")
	}
	if cfg.Client == nil {
		return apierr.InvalidArgument("redis client is required")
	}
	return nil
}

// NewRedis creates a Redis-backed world dungeon repository.
func NewRedis(cfg *RedisConfig) (Repository, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &redisRepository{client: cfg.Client}, nil
}

// storedRecord is the JSON value of one hash field.
type storedRecord struct {
	Owner string `json:"owner"`
	Name  string `json:"name,omitempty"`
	YAML  string `json:"yaml"`
}

func (r *redisRepository) Get(ctx context.Context, in *GetInput) (*GetOutput, error) {
	if in == nil {
		return nil, apierr.InvalidArgument("get input is required")
	}
	if err := validateScope(in.WorldID, in.Key); err != nil {
		return nil, err
	}
	record, found, err := readRecord(ctx, r.client, in.WorldID, in.Key)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("world %q key %q: %w", in.WorldID, in.Key, ErrNotFound)
	}
	return &GetOutput{Record: record}, nil
}

func (r *redisRepository) List(ctx context.Context, in *ListInput) (*ListOutput, error) {
	if in == nil {
		return nil, apierr.InvalidArgument("list input is required")
	}
	if in.WorldID == "" {
		return nil, apierr.InvalidArgument("world ID is required")
	}
	stored, err := r.client.HGetAll(ctx, worldKey(in.WorldID)).Result()
	if err != nil {
		return nil, apierr.Wrapf(err, "list dungeons for world %q", in.WorldID)
	}
	keys := make([]string, 0, len(stored))
	for key := range stored {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := &ListOutput{Records: make([]Record, 0, len(keys))}
	for _, key := range keys {
		record, decodeErr := decodeRecord([]byte(stored[key]), in.WorldID, key)
		if decodeErr != nil {
			return nil, decodeErr
		}
		out.Records = append(out.Records, record)
	}
	return out, nil
}

func (r *redisRepository) Put(ctx context.Context, in *PutInput) (*PutOutput, error) {
	if in == nil {
		return nil, apierr.InvalidArgument("put input is required")
	}
	if err := validateWrite(in.WorldID, in.Owner, in.Key); err != nil {
		return nil, err
	}
	if !utf8.Valid(in.YAML) {
		return nil, apierr.InvalidArgumentf("dungeon %q is not UTF-8 text", in.Key)
	}
	value, err := encodeRecord(in.Owner, in.Name, in.YAML)
	if err != nil {
		return nil, apierr.Wrapf(err, "encode dungeon %q in world %q", in.Key, in.WorldID)
	}
	hash := worldKey(in.WorldID)
	err = r.transact(ctx, in.WorldID, in.Key, func(tx *goredis.Tx) error {
		existing, found, readErr := readRecord(ctx, tx, in.WorldID, in.Key)
		if readErr != nil {
			return readErr
		}
		if found && existing.Owner != in.Owner {
			return fmt.Errorf("world %q key %q: %w", in.WorldID, in.Key, ErrOwnedByOther)
		}
		_, pipeErr := tx.TxPipelined(ctx, func(pipe goredis.Pipeliner) error {
			pipe.HSet(ctx, hash, in.Key, value)
			return nil
		})
		return pipeErr
	})
	if err != nil {
		return nil, err
	}
	return &PutOutput{}, nil
}

func (r *redisRepository) Delete(ctx context.Context, in *DeleteInput) (*DeleteOutput, error) {
	if in == nil {
		return nil, apierr.InvalidArgument("delete input is required")
	}
	if err := validateWrite(in.WorldID, in.Owner, in.Key); err != nil {
		return nil, err
	}
	hash := worldKey(in.WorldID)
	err := r.transact(ctx, in.WorldID, in.Key, func(tx *goredis.Tx) error {
		existing, found, readErr := readRecord(ctx, tx, in.WorldID, in.Key)
		if readErr != nil {
			return readErr
		}
		if !found {
			return fmt.Errorf("world %q key %q: %w", in.WorldID, in.Key, ErrNotFound)
		}
		if existing.Owner != in.Owner {
			return fmt.Errorf("world %q key %q: %w", in.WorldID, in.Key, ErrOwnedByOther)
		}
		_, pipeErr := tx.TxPipelined(ctx, func(pipe goredis.Pipeliner) error {
			pipe.HDel(ctx, hash, in.Key)
			return nil
		})
		return pipeErr
	})
	if err != nil {
		return nil, err
	}
	return &DeleteOutput{}, nil
}

func (r *redisRepository) Rename(ctx context.Context, in *RenameInput) (*RenameOutput, error) {
	if in == nil {
		return nil, apierr.InvalidArgument("rename input is required")
	}
	if err := validateWrite(in.WorldID, in.Owner, in.Key); err != nil {
		return nil, err
	}
	if in.NewKey == "" {
		return nil, apierr.InvalidArgument("new key is required")
	}
	if in.Rewrite == nil {
		return nil, apierr.InvalidArgument("rewrite is required")
	}
	hash := worldKey(in.WorldID)
	err := r.transact(ctx, in.WorldID, in.Key, func(tx *goredis.Tx) error {
		source, found, readErr := readRecord(ctx, tx, in.WorldID, in.Key)
		if readErr != nil {
			return readErr
		}
		if !found {
			return fmt.Errorf("world %q key %q: %w", in.WorldID, in.Key, ErrNotFound)
		}
		if source.Owner != in.Owner {
			return fmt.Errorf("world %q key %q: %w", in.WorldID, in.Key, ErrOwnedByOther)
		}
		if in.NewKey == in.Key {
			return fmt.Errorf("world %q key %q: %w", in.WorldID, in.NewKey, ErrKeyExists)
		}
		target, taken, readErr := readRecord(ctx, tx, in.WorldID, in.NewKey)
		if readErr != nil {
			return readErr
		}
		if taken {
			if target.Owner == in.Owner {
				return fmt.Errorf("world %q key %q: %w", in.WorldID, in.NewKey, ErrKeyExists)
			}
			return fmt.Errorf("world %q key %q: %w", in.WorldID, in.NewKey, ErrOwnedByOther)
		}
		next, rewriteErr := in.Rewrite(append([]byte(nil), source.YAML...))
		if rewriteErr != nil {
			return rewriteErr
		}
		if !utf8.Valid(next) {
			return apierr.Internalf("rewritten dungeon %q is not UTF-8 text", in.NewKey)
		}
		value, encodeErr := encodeRecord(source.Owner, source.Name, next)
		if encodeErr != nil {
			return apierr.Wrapf(encodeErr, "encode dungeon %q in world %q", in.NewKey, in.WorldID)
		}
		_, pipeErr := tx.TxPipelined(ctx, func(pipe goredis.Pipeliner) error {
			pipe.HSet(ctx, hash, in.NewKey, value)
			pipe.HDel(ctx, hash, in.Key)
			return nil
		})
		return pipeErr
	})
	if err != nil {
		return nil, err
	}
	return &RenameOutput{}, nil
}

// transact runs fn under WATCH on the world's hash, retrying when another
// client changed the hash before EXEC.
func (r *redisRepository) transact(ctx context.Context, worldID, key string, fn func(*goredis.Tx) error) error {
	var err error
	for attempt := 0; attempt <= maxTxRetries; attempt++ {
		err = r.client.Watch(ctx, fn, worldKey(worldID))
		if !errors.Is(err, goredis.TxFailedErr) {
			return err
		}
	}
	return apierr.WrapWithCodef(err, apierr.CodeAborted,
		"world %q key %q changed concurrently %d times", worldID, key, maxTxRetries+1)
}

// hashReader is the HGET both the client and a WATCH transaction answer.
type hashReader interface {
	HGet(ctx context.Context, key, field string) *goredis.StringCmd
}

func readRecord(ctx context.Context, reader hashReader, worldID, key string) (Record, bool, error) {
	stored, err := reader.HGet(ctx, worldKey(worldID), key).Bytes()
	if errors.Is(err, goredis.Nil) {
		return Record{}, false, nil
	}
	if err != nil {
		return Record{}, false, apierr.Wrapf(err, "get dungeon %q in world %q", key, worldID)
	}
	record, err := decodeRecord(stored, worldID, key)
	if err != nil {
		return Record{}, false, err
	}
	return record, true, nil
}

func encodeRecord(owner, name string, yaml []byte) ([]byte, error) {
	return json.Marshal(storedRecord{Owner: owner, Name: name, YAML: string(yaml)})
}

func decodeRecord(stored []byte, worldID, key string) (Record, error) {
	var value storedRecord
	if err := json.Unmarshal(stored, &value); err != nil {
		return Record{}, apierr.WrapWithCodef(err, apierr.CodeInternal,
			"decode dungeon %q in world %q", key, worldID)
	}
	if value.Owner == "" {
		return Record{}, apierr.Internalf("dungeon %q in world %q has no owner", key, worldID)
	}
	return Record{Key: key, Owner: value.Owner, Name: value.Name, YAML: []byte(value.YAML)}, nil
}

func validateScope(worldID, key string) error {
	if worldID == "" {
		return apierr.InvalidArgument("world ID is required")
	}
	if key == "" {
		return apierr.InvalidArgument("dungeon key is required")
	}
	return nil
}

func validateWrite(worldID, owner, key string) error {
	if err := validateScope(worldID, key); err != nil {
		return err
	}
	if owner == "" {
		return apierr.InvalidArgument("owner is required")
	}
	return nil
}

func worldKey(worldID string) string {
	return worldKeyPrefix + worldID
}
