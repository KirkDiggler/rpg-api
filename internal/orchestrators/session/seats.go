package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	goredis "github.com/redis/go-redis/v9"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	redisclient "github.com/KirkDiggler/rpg-api/internal/redis"
)

// seatKeyPrefix is distinct from every other session-store prefix (S13: one
// repository per data type). A seat is keyed by character, never by session.
const seatKeyPrefix = "session-seat:v1alpha1:"

// redisSeatRepository is the Redis-backed sdk.SeatRepository. It lives in the
// same Redis the session, encounter and character records use, and round-trips
// the SDK's SeatData without deciding anything about it.
//
// # A seat lapses with the run it names
//
// Session records carry the host's TTL and a seat does not: a seat is written
// once, at Launch or Join, while its run is re-saved on every verb, so a TTL
// on the seat would expire it under a live run and let an unseated verb write
// a seated sheet. Instead GetSeat answers a seat whose session record no
// longer exists as never seated. That is the storage fact — the run expired,
// so nothing holds the character — and without it a character whose run
// timed out without Exit or End would be refused every launch forever
// (ErrSeatedElsewhere naming a run nobody can open).
type redisSeatRepository struct {
	client redisclient.Client
}

// NewSeatRepository returns a Redis-backed sdk.SeatRepository.
func NewSeatRepository(client redisclient.Client) sdk.SeatRepository {
	return &redisSeatRepository{client: client}
}

// GetSeat implements sdk.SeatRepository.
func (r *redisSeatRepository) GetSeat(ctx context.Context, character string) (*sdk.SeatData, error) {
	if character == "" {
		return nil, errors.New("session: GetSeat character is required")
	}
	b, err := r.client.Get(ctx, seatKeyPrefix+character).Bytes()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return nil, fmt.Errorf("seat %q: %w", character, sdk.ErrNotFound)
		}
		return nil, fmt.Errorf("get seat %q: %w", character, err)
	}
	var out sdk.SeatData
	if decodeErr := json.Unmarshal(b, &out); decodeErr != nil {
		return nil, fmt.Errorf("unmarshal stored seat %q: %w", character, decodeErr)
	}
	if out.Session == "" {
		return &out, nil
	}
	live, err := r.client.Exists(ctx, sessionKeyPrefix+out.Session).Result()
	if err != nil {
		return nil, fmt.Errorf("check run of seat %q: %w", character, err)
	}
	if live == 0 {
		return nil, fmt.Errorf("seat %q names lapsed session %q: %w", character, out.Session, sdk.ErrNotFound)
	}
	return &out, nil
}

// SaveSeat implements sdk.SeatRepository.
func (r *redisSeatRepository) SaveSeat(ctx context.Context, data *sdk.SeatData) error {
	if data == nil {
		return errors.New("session: SaveSeat data is required")
	}
	if data.Character == "" {
		return errors.New("session: SaveSeat data.Character is required")
	}
	b, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal seat %q: %w", data.Character, err)
	}
	if err := r.client.Set(ctx, seatKeyPrefix+data.Character, b, 0).Err(); err != nil {
		return fmt.Errorf("set seat %q: %w", data.Character, err)
	}
	return nil
}
