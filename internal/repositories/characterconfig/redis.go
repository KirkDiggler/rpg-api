package characterconfig

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	goredis "github.com/redis/go-redis/v9"

	redisclient "github.com/KirkDiggler/rpg-api/internal/redis"
)

// keyPrefix namespaces every character configuration record:
// character-config:<character id>, holding JSON {discovery_sharing: bool|null}.
const keyPrefix = "character-config:"

// legacyExplorationPrefix is the session SDK's old per-character exploration
// profile, written by the removed session SetDiscoverySharing verb. It is read
// once, as a character's initial configuration, and never written again.
const legacyExplorationPrefix = "character-exploration:v1:"

// legacyExploration is the old profile's JSON. Only the preference is read;
// the obsolete checks member is ignored as it always was.
type legacyExploration struct {
	// Private is the oldest form, read only when DiscoverySharing is absent:
	// a record written before the explicit choice existed meant shared unless
	// it said private.
	Private bool `json:"private,omitempty"`
	// DiscoverySharing is the explicit choice, when one was made.
	DiscoverySharing *bool `json:"discovery_sharing,omitempty"`
}

// RedisConfig carries what NewRedis needs.
type RedisConfig struct {
	Client redisclient.Client
}

type redisRepository struct {
	client redisclient.Client
}

// NewRedis returns a Redis-backed Repository.
func NewRedis(cfg *RedisConfig) (Repository, error) {
	if cfg == nil || cfg.Client == nil {
		return nil, errors.New("character config: redis client is required")
	}
	return &redisRepository{client: cfg.Client}, nil
}

func key(id string) string { return keyPrefix + id }

// Get reads character-config:<id>. When that record is absent it reads the
// legacy exploration profile once, resolves its preference through the old
// three branches, and seeds character-config:<id> with the result only if
// the record is still absent, so a concurrent Put always wins. The legacy
// record is never written.
func (r *redisRepository) Get(ctx context.Context, input *GetInput) (*GetOutput, error) {
	if input == nil || input.CharacterID == "" {
		return nil, ErrCharacterIDRequired
	}
	data, found, err := r.read(ctx, input.CharacterID)
	if err != nil {
		return nil, err
	}
	if found {
		return &GetOutput{Data: data}, nil
	}
	initial, err := r.readLegacy(ctx, input.CharacterID)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(initial)
	if err != nil {
		return nil, fmt.Errorf("encode character config %q: %w", input.CharacterID, err)
	}
	seeded, err := r.client.SetNX(ctx, key(input.CharacterID), raw, 0).Result()
	if err != nil {
		return nil, fmt.Errorf("seed character config %q: %w", input.CharacterID, err)
	}
	if seeded {
		return &GetOutput{Data: initial}, nil
	}
	// A Put landed between the read and the seed: it is the configuration.
	data, found, err = r.read(ctx, input.CharacterID)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("character config %q vanished after a concurrent write", input.CharacterID)
	}
	return &GetOutput{Data: data}, nil
}

// Put replaces character-config:<id>.
func (r *redisRepository) Put(ctx context.Context, input *PutInput) (*PutOutput, error) {
	if input == nil || input.CharacterID == "" {
		return nil, ErrCharacterIDRequired
	}
	if input.Data == nil {
		return nil, ErrDataRequired
	}
	stored := clone(input.Data)
	raw, err := json.Marshal(stored)
	if err != nil {
		return nil, fmt.Errorf("encode character config %q: %w", input.CharacterID, err)
	}
	if err = r.client.Set(ctx, key(input.CharacterID), raw, 0).Err(); err != nil {
		return nil, fmt.Errorf("put character config %q: %w", input.CharacterID, err)
	}
	return &PutOutput{Data: stored}, nil
}

func (r *redisRepository) read(ctx context.Context, id string) (*Data, bool, error) {
	raw, err := r.client.Get(ctx, key(id)).Bytes()
	if errors.Is(err, goredis.Nil) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("get character config %q: %w", id, err)
	}
	var data Data
	if err = json.Unmarshal(raw, &data); err != nil {
		return nil, false, fmt.Errorf("decode character config %q: %w", id, err)
	}
	return &data, true, nil
}

// readLegacy resolves the old profile into the initial configuration: an
// explicit choice wins; a record without one keeps its old meaning, shared
// unless it said private; no record at all is not chosen.
func (r *redisRepository) readLegacy(ctx context.Context, id string) (*Data, error) {
	raw, err := r.client.Get(ctx, legacyExplorationPrefix+id).Bytes()
	if errors.Is(err, goredis.Nil) {
		return &Data{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get legacy exploration %q: %w", id, err)
	}
	var legacy legacyExploration
	if err = json.Unmarshal(raw, &legacy); err != nil {
		return nil, fmt.Errorf("decode legacy exploration %q: %w", id, err)
	}
	if legacy.DiscoverySharing != nil {
		return &Data{DiscoverySharing: boolPtr(*legacy.DiscoverySharing)}, nil
	}
	return &Data{DiscoverySharing: boolPtr(!legacy.Private)}, nil
}

func clone(in *Data) *Data {
	out := &Data{}
	if in.DiscoverySharing != nil {
		out.DiscoverySharing = boolPtr(*in.DiscoverySharing)
	}
	return out
}

func boolPtr(v bool) *bool { return &v }
