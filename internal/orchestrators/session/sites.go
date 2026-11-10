package session

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
	"weak"

	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
)

// THE SITE IS CONTENT THE RECORD REFERENCES (rpg-project#879 S5). A compiled
// site (the field, its partitions, door openings, the raw sight answers) is
// built once per content hash and shared by every game on that content. The
// toolkit owns the compile and the bounded cache (encounter.MemorySites);
// this file only constructs the one provider the process uses, at a bound the
// host chose, and says in the log when a compile actually ran.

// DefaultSiteEntries is how many compiled sites the process holds when the
// host names no bound: the shipped shelf plus room for authored dungeons in
// play. At the bound the least recently used site is dropped; a game already
// running on it keeps its pointer, and the next load of that hash compiles it
// again from the stored piece.
const DefaultSiteEntries = 16

// SitesConfig sizes and observes the process's site provider.
type SitesConfig struct {
	// Entries bounds the compiled sites held. Zero is DefaultSiteEntries;
	// negative is refused.
	Entries int

	// Logf, when set, receives one line per site compile: the hash, the
	// milliseconds the resolving call took, and how many hashes have been
	// compiled so far. Nil logs nothing.
	Logf func(format string, args ...any)
}

// Sites is the process's site provider: the toolkit's bounded in-process
// cache (encounter.MemorySites) at the host's bound, plus a log line for every
// compile. One per process, handed to the session orchestrator through
// Config.Sites, so every encounter the server builds or loads (launch, a
// verb's reload, the dungeon map preview through AtlasOf) shares it.
type Sites struct {
	inner *tkencounter.MemorySites
	logf  func(format string, args ...any)

	mu sync.Mutex
	// seen is the last compiled site observed per hash, held weakly so an
	// evicted site is collected. A resolved site that is not the one seen is
	// a fresh compile. Pruned of collected entries as it grows.
	seen      map[tkencounter.SiteHash]weak.Pointer[tkencounter.Site]
	compiles  int
	pruneSize int
}

var _ tkencounter.SiteProvider = (*Sites)(nil)

// ErrBadSitesConfig is a site bound that cannot be honored.
var ErrBadSitesConfig = errors.New("session sites: bad config")

// NewSites is the process's provider under cfg (nil is every default).
func NewSites(cfg *SitesConfig) (*Sites, error) {
	entries := DefaultSiteEntries
	var logf func(string, ...any)
	if cfg != nil {
		if cfg.Entries < 0 {
			return nil, fmt.Errorf("%w: Entries %d is negative", ErrBadSitesConfig, cfg.Entries)
		}
		if cfg.Entries > 0 {
			entries = cfg.Entries
		}
		logf = cfg.Logf
	}
	return &Sites{
		inner:     tkencounter.NewMemorySites(&tkencounter.MemorySitesConfig{Entries: entries}),
		logf:      logf,
		seen:      make(map[tkencounter.SiteHash]weak.Pointer[tkencounter.Site]),
		pruneSize: 4 * entries,
	}, nil
}

// Site implements encounter.SiteProvider: the toolkit's cache answers, and a
// site this provider has not handed out before (a first compile, or a
// recompile after eviction) is logged once.
func (s *Sites) Site(ctx context.Context, in *tkencounter.SiteInput) (*tkencounter.SiteOutput, error) {
	start := time.Now()
	out, err := s.inner.Site(ctx, in)
	if err != nil {
		return nil, err
	}
	s.observe(out.Site, time.Since(start))
	return out, nil
}

// Warm resolves the site of an authored field through the provider, compiling
// it when the provider does not hold it yet, and returns its hash: the boot
// path, so shipped content is compiled before the first launch asks for it.
func (s *Sites) Warm(ctx context.Context, field *tkencounter.FieldInput) (tkencounter.SiteHash, error) {
	if field == nil {
		return "", fmt.Errorf("%w: Warm needs a field", ErrBadSitesConfig)
	}
	out, err := s.Site(ctx, &tkencounter.SiteInput{Field: field})
	if err != nil {
		return "", err
	}
	return out.Site.Hash(), nil
}

// Compiles is how many site compiles this provider has observed: the count
// a test pins "one compile per content" with.
func (s *Sites) Compiles() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.compiles
}

func (s *Sites) observe(site *tkencounter.Site, took time.Duration) {
	if site == nil {
		return
	}
	hash := site.Hash()
	s.mu.Lock()
	if prev, ok := s.seen[hash]; ok && prev.Value() == site {
		s.mu.Unlock()
		return
	}
	s.seen[hash] = weak.Make(site)
	s.compiles++
	n := s.compiles
	if len(s.seen) > s.pruneSize {
		for h, p := range s.seen {
			if p.Value() == nil {
				delete(s.seen, h)
			}
		}
	}
	s.mu.Unlock()
	if s.logf != nil {
		s.logf("site compiled: %s in %.2fms (%d compiled by this process)",
			hash, float64(took.Microseconds())/1000, n)
	}
}
