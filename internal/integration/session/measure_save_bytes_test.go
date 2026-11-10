// measure_save_bytes_test.go is the bytes-per-Move row of rpg-project#879's
// measurement harness (rpg-project#897 legs c and b): what one Move's save
// hands Redis, by shape, and what the store holds after it. A go-redis hook
// on the harness's client reads every EVALSHA/EVAL whose first key is the
// run's encounter (the save script, redis_fenced.go) and splits its
// arguments by the script's layout: the head, the roster and knowledge
// fields written (field and value), the journal, and the appended segment.
// The head's world clock tells a Move that raised a round from one that did
// not. After each Move the row reads the stored bytes: every piece key plus
// every record segment.

package session_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"testing"

	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// The encounter's keys as the store spells them (redis_pieces.go,
// redis_record.go).
const (
	measureEncPrefix         = "session-enc:"
	measureLogKeyPrefix      = "session-enc-log:v1alpha2:"
	measureHeadKeyPrefix     = "session-enc:v1alpha3:"
	measureRosterKeyPrefix   = "session-enc-roster:v1alpha3:"
	measureKnowKeyPrefix     = "session-enc-know:v1alpha3:"
	measureJournalKeyPrefix  = "session-enc-journal:v1alpha3:"
	measureScriptHeadArg     = 6
	measureScriptSegmentArg  = 9
	measureScriptHasJournal  = 12
	measureScriptJournalArg  = 13
	measureScriptCountsFirst = 14
)

// saveShapes is one save's bytes by shape and its pieces written.
type saveShapes struct {
	total, head, roster, know, journal, segment, pieces int
	// round is the head's world-clock high water, -1 when unread.
	round int
}

// saveBytes is a go-redis hook that sums what the encounter's saves hand
// Redis, by shape. It counts only scripts whose first key is an encounter
// key.
type saveBytes struct {
	mu    sync.Mutex
	sum   saveShapes
	saves int
}

var _ goredis.Hook = (*saveBytes)(nil)

func (h *saveBytes) DialHook(next goredis.DialHook) goredis.DialHook {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		return next(ctx, network, addr)
	}
}

func (h *saveBytes) ProcessPipelineHook(next goredis.ProcessPipelineHook) goredis.ProcessPipelineHook {
	return next
}

func (h *saveBytes) ProcessHook(next goredis.ProcessHook) goredis.ProcessHook {
	return func(ctx context.Context, cmd goredis.Cmder) error {
		h.observe(cmd)
		return next(ctx, cmd)
	}
}

// observe adds one script call's bytes when its first key is an encounter.
func (h *saveBytes) observe(cmd goredis.Cmder) {
	name := cmd.Name()
	if name != "evalsha" && name != "eval" {
		return
	}
	args := cmd.Args()
	if len(args) < 4 {
		return
	}
	first, ok := args[3].(string)
	if !ok || !strings.HasPrefix(first, measureEncPrefix) {
		return
	}
	numKeys, ok := args[2].(int)
	if !ok {
		return
	}
	// ARGV[n] is args[2+numKeys+n].
	argv := func(n int) interface{} {
		if i := 2 + numKeys + n; i < len(args) {
			return args[i]
		}
		return nil
	}
	count := func(n int) int {
		v, _ := argv(n).(int)
		return v
	}
	one := saveShapes{round: -1, pieces: 1}
	for _, a := range args {
		one.total += argLen(a)
	}
	head := argv(measureScriptHeadArg)
	one.head = argLen(head)
	one.round = headRound(head)
	if argv(8) == "1" {
		one.segment = argLen(argv(measureScriptSegmentArg))
	}
	if argv(measureScriptHasJournal) == "1" {
		one.journal = argLen(argv(measureScriptJournalArg))
		one.pieces++
	}
	nRosterSet, nRosterDel := count(measureScriptCountsFirst), count(measureScriptCountsFirst+1)
	nKnowSet := count(measureScriptCountsFirst + 2)
	at := measureScriptCountsFirst + 4
	for i := 0; i < 2*nRosterSet; i++ {
		one.roster += argLen(argv(at + i))
	}
	at += 2*nRosterSet + nRosterDel
	for i := 0; i < 2*nKnowSet; i++ {
		one.know += argLen(argv(at + i))
	}
	one.pieces += nRosterSet + nKnowSet
	h.mu.Lock()
	defer h.mu.Unlock()
	h.sum.total += one.total
	h.sum.head += one.head
	h.sum.roster += one.roster
	h.sum.know += one.know
	h.sum.journal += one.journal
	h.sum.segment += one.segment
	h.sum.pieces += one.pieces
	if one.round >= 0 {
		h.sum.round = one.round
	}
	h.saves++
}

// headRound reads the world-clock high water from a head piece's value
// ("<mark>\n<JSON>"), -1 when it does not read.
func headRound(v interface{}) int {
	var raw string
	switch b := v.(type) {
	case string:
		raw = b
	case []byte:
		raw = string(b)
	default:
		return -1
	}
	_, body, ok := strings.Cut(raw, "\n")
	if !ok {
		return -1
	}
	var head struct {
		Clock struct {
			HighWater int `json:"high_water"`
		} `json:"clock"`
	}
	if json.Unmarshal([]byte(body), &head) != nil {
		return -1
	}
	return head.Clock.HighWater
}

// take returns and resets the sums since the last take.
func (h *saveBytes) take() (saveShapes, int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	out, saves := h.sum, h.saves
	h.sum, h.saves = saveShapes{round: -1}, 0
	return out, saves
}

func argLen(a interface{}) int {
	switch v := a.(type) {
	case nil:
		return 0
	case string:
		return len(v)
	case []byte:
		return len(v)
	default:
		return len(fmt.Sprint(v))
	}
}

// storedBytes is what the store holds for an encounter: every piece key's
// bytes (head, both hashes' fields and values, journal) and the record
// list's total. The third return is the record list's length.
func storedBytes(t *testing.T, client *goredis.Client, encounter string) (restLen, logLen, segments int) {
	t.Helper()
	ctx := context.Background()
	for _, key := range []string{measureHeadKeyPrefix, measureJournalKeyPrefix} {
		n, err := client.StrLen(ctx, key+encounter).Result()
		require.NoError(t, err)
		restLen += int(n)
	}
	for _, key := range []string{measureRosterKeyPrefix, measureKnowKeyPrefix} {
		fields, err := client.HGetAll(ctx, key+encounter).Result()
		require.NoError(t, err)
		for f, v := range fields {
			restLen += len(f) + len(v)
		}
	}
	elems, err := client.LRange(ctx, measureLogKeyPrefix+encounter, 0, -1).Result()
	require.NoError(t, err)
	for _, e := range elems {
		logLen += len(e)
	}
	return restLen, logLen, len(elems)
}

// bytesRow is one mode's per-Move save and store sizes.
type bytesRow struct {
	key, mode string
	moves     []saveShapes
	saves     []int
	// rounds marks a Move whose save raised the world round.
	rounds                          []bool
	storedRest, storedLog, segments []int
}

// add records one Move: what its saves handed Redis and what the store holds.
func (r *bytesRow) add(shapes saveShapes, saves int, prevRound int, storedRest, storedLog, segments int) {
	r.moves = append(r.moves, shapes)
	r.saves = append(r.saves, saves)
	r.rounds = append(r.rounds, prevRound >= 0 && shapes.round > prevRound)
	r.storedRest = append(r.storedRest, storedRest)
	r.storedLog = append(r.storedLog, storedLog)
	r.segments = append(r.segments, segments)
}

// bytesTable renders each mode's per-Move series in order, then p50 by
// shape for every Move, the Moves without a round and the Moves with one.
func bytesTable(rows []bytesRow) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\nbytes per Move by shape (rpg-project#897 leg b): write = every arg of the encounter's save scripts; "+
		"stored = every piece key + record list\n")
	shapes := []struct {
		name string
		f    func(saveShapes) int
	}{
		{"write", func(s saveShapes) int { return s.total }},
		{"head", func(s saveShapes) int { return s.head }},
		{"roster", func(s saveShapes) int { return s.roster }},
		{"knowledge", func(s saveShapes) int { return s.know }},
		{"journal", func(s saveShapes) int { return s.journal }},
		{"segment", func(s saveShapes) int { return s.segment }},
		{"pieces", func(s saveShapes) int { return s.pieces }},
	}
	for _, r := range rows {
		for _, s := range shapes {
			parts := make([]string, len(r.moves))
			for i, m := range r.moves {
				parts[i] = fmt.Sprint(s.f(m))
			}
			fmt.Fprintf(&b, "%s %s %s, in order: %s\n", r.key, r.mode, s.name, strings.Join(parts, " "))
		}
		rounds := make([]string, len(r.rounds))
		for i, round := range r.rounds {
			rounds[i] = map[bool]string{true: "R", false: "."}[round]
		}
		fmt.Fprintf(&b, "%s %s round Moves (R), in order: %s\n", r.key, r.mode, strings.Join(rounds, ""))
		fmt.Fprintf(&b, "%s %s stored rest bytes, last: %d; stored record bytes, last: %d; segments, last: %d\n",
			r.key, r.mode, r.storedRest[len(r.storedRest)-1], r.storedLog[len(r.storedLog)-1], r.segments[len(r.segments)-1])
	}
	fmt.Fprintf(&b, "| keep | mode | Moves | n |")
	for _, s := range shapes {
		fmt.Fprintf(&b, " %s p50 |", s.name)
	}
	fmt.Fprintf(&b, "\n|---|---|---|---|%s\n", strings.Repeat("---|", len(shapes)))
	for _, r := range rows {
		for _, kind := range []struct {
			name string
			keep func(bool) bool
		}{
			{"all", func(bool) bool { return true }},
			{"no round", func(round bool) bool { return !round }},
			{"round", func(round bool) bool { return round }},
		} {
			var picked []saveShapes
			for i, m := range r.moves {
				if kind.keep(r.rounds[i]) {
					picked = append(picked, m)
				}
			}
			if len(picked) == 0 {
				continue
			}
			fmt.Fprintf(&b, "| %s | %s | %s | %d |", r.key, r.mode, kind.name, len(picked))
			for _, s := range shapes {
				vals := make([]int, len(picked))
				for i, m := range picked {
					vals[i] = s.f(m)
				}
				fmt.Fprintf(&b, " %d |", p50int(vals))
			}
			fmt.Fprintf(&b, "\n")
		}
	}
	return b.String()
}

func p50int(v []int) int {
	if len(v) == 0 {
		return 0
	}
	s := append([]int(nil), v...)
	sort.Ints(s)
	return s[len(s)/2]
}
