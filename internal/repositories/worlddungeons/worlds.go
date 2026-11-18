package worlddungeons

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/KirkDiggler/rpg-api/internal/apierr"
	redisclient "github.com/KirkDiggler/rpg-api/internal/redis"
)

// worldScanBatch is the SCAN count hint when walking every world's hash.
const worldScanBatch = 100

// ListWorldIDsInput names the Redis the world store lives in.
type ListWorldIDsInput struct {
	Client redisclient.Client
}

// ListWorldIDsOutput is every world that holds at least one dungeon, sorted.
type ListWorldIDsOutput struct {
	WorldIDs []string
}

// ListWorldIDs walks the world store's hashes and names their worlds. It is
// for operator tooling that must reach every author's saved dungeons (the
// noted-words converter, rpg-project#1005), not for a request path: the
// Repository is scoped to one world by design, and stays so.
func ListWorldIDs(ctx context.Context, in *ListWorldIDsInput) (*ListWorldIDsOutput, error) {
	if in == nil || in.Client == nil {
		return nil, errors.New("worlddungeons: a redis client is required")
	}
	out := &ListWorldIDsOutput{WorldIDs: []string{}}
	var cursor uint64
	for {
		keys, next, err := in.Client.ScanType(ctx, cursor, worldKeyPrefix+"*", worldScanBatch, "hash").Result()
		if err != nil {
			return nil, apierr.Wrapf(err, "scan world dungeon hashes")
		}
		for _, k := range keys {
			out.WorldIDs = append(out.WorldIDs, strings.TrimPrefix(k, worldKeyPrefix))
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	sort.Strings(out.WorldIDs)
	return out, nil
}
