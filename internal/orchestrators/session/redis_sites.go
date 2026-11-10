package session

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	goredis "github.com/redis/go-redis/v9"

	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

// sessionSiteKeyPrefix keys a compiled site's stored piece by its content
// hash (rpg-project#879 S5): session-site:v1alpha1:<hash>. A checkpoint names
// its site by that hash and never carries the field, so a process whose
// provider does not hold the hash (a restart, an eviction) reads the piece
// here and compiles it once.
//
// NO TTL. A piece is content, not session state: it must outlive every
// checkpoint that names it, and a checkpoint's own TTL is refreshed by every
// save while a piece is written once at launch, so any fixed TTL on the
// piece could expire under a game still being played. Pieces are
// content-addressed, so their number is the number of distinct site versions
// ever launched; collecting the ones no checkpoint names is a follow-up, not
// this slice.
const sessionSiteKeyPrefix = "session-site:v1alpha1:"

// The Redis encounter repository carries the session's optional site
// capability; the SDK finds it by type assertion on Config.Encounters.
var _ sdk.SiteRepository = (*redisEncounterRepository)(nil)

// GetSite implements sdk.SiteRepository: the stored piece for in.Hash, or an
// sdk.ErrNotFound error when none is stored.
func (r *redisEncounterRepository) GetSite(ctx context.Context, in *sdk.GetSiteInput) (*sdk.GetSiteOutput, error) {
	if in == nil || in.Hash == "" {
		return nil, errors.New("session: GetSite hash is required")
	}
	b, err := r.client.Get(ctx, sessionSiteKeyPrefix+string(in.Hash)).Bytes()
	if err != nil {
		if errors.Is(err, goredis.Nil) {
			return nil, fmt.Errorf("site %s: %w", in.Hash, sdk.ErrNotFound)
		}
		return nil, fmt.Errorf("get site %s: %w", in.Hash, err)
	}
	var data tkencounter.SiteData
	if err := json.Unmarshal(b, &data); err != nil {
		return nil, fmt.Errorf("unmarshal stored site %s: %w", in.Hash, err)
	}
	return &sdk.GetSiteOutput{Data: &data}, nil
}

// PutSite implements sdk.SiteRepository. Content-addressed and immutable, so
// a hash already stored is a no-op success: EXISTS first, so a launch on
// content already stored (every launch after the first) neither encodes nor
// ships the piece, then SETNX, so two launches racing on new content write it
// once and neither overwrites the other's identical bytes.
func (r *redisEncounterRepository) PutSite(ctx context.Context, in *sdk.PutSiteInput) (*sdk.PutSiteOutput, error) {
	if in == nil || in.Hash == "" {
		return nil, errors.New("session: PutSite hash is required")
	}
	if in.Data == nil {
		return nil, errors.New("session: PutSite data is required")
	}
	key := sessionSiteKeyPrefix + string(in.Hash)
	n, err := r.client.Exists(ctx, key).Result()
	if err != nil {
		return nil, fmt.Errorf("exists site %s: %w", in.Hash, err)
	}
	if n > 0 {
		return &sdk.PutSiteOutput{}, nil
	}
	b, err := json.Marshal(in.Data)
	if err != nil {
		return nil, fmt.Errorf("marshal site %s: %w", in.Hash, err)
	}
	if err := r.client.SetNX(ctx, key, b, 0).Err(); err != nil {
		return nil, fmt.Errorf("set site %s: %w", in.Hash, err)
	}
	return &sdk.PutSiteOutput{}, nil
}
