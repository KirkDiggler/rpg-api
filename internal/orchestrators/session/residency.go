package session

import (
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"sync"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

// THE RESIDENT COPY IS A CACHE OF THE STORE (rpg-project#879 S6, R6,
// rpg-project#927). The session SDK keeps each session's write-verb
// encounter in memory between verbs and checks it against the checkpoint
// version the Redis repository reads in one GET (GetCheckpointVersion,
// redis_fenced.go); on a match the next verb skips the fenced read and the
// load, on a miss it loads as before. The SDK owns the copy, the check and
// every drop rule; this file only constructs the one residency the process
// uses, at the bounds the host chose. Nothing stored changes, so a restart
// simply starts cold.

// DefaultResidentEntries is how many sessions' copies the process keeps when
// the host names no bound: the SDK's default.
const DefaultResidentEntries = sdk.DefaultResidentEntries

// DefaultResidentMaxMB is how many estimated MiB the process's copies may
// hold together when the host names no bound: the SDK's default.
const DefaultResidentMaxMB = sdk.DefaultResidentMaxBytes >> 20

// ResidencyConfig sizes the process's residency.
type ResidencyConfig struct {
	// Entries bounds how many sessions' copies are kept, the least recently
	// kept evicted first. Zero is DefaultResidentEntries; negative keeps
	// none, so every verb loads, as before S6.
	Entries int

	// MaxMB bounds the copies' estimated memory together, in MiB (the SDK's
	// estimate grows with each run's record). Zero is DefaultResidentMaxMB;
	// negative is refused.
	MaxMB int
}

// ErrBadResidencyConfig is a residency bound that cannot be honored.
var ErrBadResidencyConfig = errors.New("session residency: bad config")

// Residency is the process's residency: the SDK's bounded in-process one
// (sdk.MemoryResidency) at the host's bounds, and the observer of every
// verb's hit or miss. One per process, handed to the session orchestrator
// through Config.Residency, so every Manager the process builds keeps its
// copies in one bound and counts them in one place.
//
// IT COUNTS, IT DOES NOT EXPORT. The counts are read by Stats (the
// measurement harness, a test); a miss is one debug log line with its
// reason. No metrics system is introduced here.
type Residency struct {
	*sdk.MemoryResidency

	entries int
	maxMB   int

	mu     sync.Mutex
	hits   int64
	misses map[sdk.MissReason]int64
}

var (
	_ sdk.Residency         = (*Residency)(nil)
	_ sdk.ResidencyObserver = (*Residency)(nil)
)

// ResidencyStats is what a residency has observed since it was built.
type ResidencyStats struct {
	// Hits is how many write verbs ran on a kept copy.
	Hits int64
	// Misses is how many loaded, by the gate that sent them to the store.
	Misses map[sdk.MissReason]int64
}

// MissCount is every miss, whatever the reason.
func (s ResidencyStats) MissCount() int64 {
	var n int64
	for _, c := range s.Misses {
		n += c
	}
	return n
}

// NewResidency is the process's residency under cfg (nil is every default).
func NewResidency(cfg *ResidencyConfig) (*Residency, error) {
	entries, maxMB := DefaultResidentEntries, DefaultResidentMaxMB
	if cfg != nil {
		if cfg.MaxMB < 0 {
			return nil, fmt.Errorf("%w: MaxMB %d is negative", ErrBadResidencyConfig, cfg.MaxMB)
		}
		if cfg.Entries != 0 {
			entries = cfg.Entries
		}
		if cfg.MaxMB > 0 {
			maxMB = cfg.MaxMB
		}
	}
	if entries < 0 {
		entries = -1
	}
	return &Residency{
		MemoryResidency: sdk.NewResidency(&sdk.ResidencyConfig{
			MaxEntries: entries,
			MaxBytes:   int64(maxMB) << 20,
		}),
		entries: entries,
		maxMB:   maxMB,
		misses:  map[sdk.MissReason]int64{},
	}, nil
}

// Hit implements sdk.ResidencyObserver.
func (r *Residency) Hit(string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.hits++
}

// Miss implements sdk.ResidencyObserver: counted, and one debug line.
func (r *Residency) Miss(session string, reason sdk.MissReason) {
	r.mu.Lock()
	r.misses[reason]++
	r.mu.Unlock()
	slog.Debug("session residency miss", "session", session, "reason", string(reason))
}

// Stats is a copy of the counts observed so far.
func (r *Residency) Stats() ResidencyStats {
	r.mu.Lock()
	defer r.mu.Unlock()
	return ResidencyStats{Hits: r.hits, Misses: maps.Clone(r.misses)}
}

// KeepsNone reports whether the residency was bounded to keep no copy.
func (r *Residency) KeepsNone() bool { return r.entries < 0 }

// Bounds says what the residency keeps, for the boot log.
func (r *Residency) Bounds() string {
	if r.KeepsNone() {
		return "keeps none (every verb loads)"
	}
	return fmt.Sprintf("keeps up to %d sessions, %d MiB estimated", r.entries, r.maxMB)
}
