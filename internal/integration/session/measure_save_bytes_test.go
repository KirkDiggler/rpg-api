// measure_save_bytes_test.go is the bytes-per-Move row of rpg-project#879's
// measurement harness (rpg-project#897 leg c): what one Move's save hands
// Redis, and what the store holds after it. A go-redis hook on the harness's
// client sums every argument of each EVALSHA/EVAL whose first key is the
// run's encounter (the fenced save's script), and splits out the rest and
// the appended segment where the script carries them. After each Move the
// row reads the stored bytes: the rest's STRLEN plus every record segment's
// length (and the single blob's STRLEN on a store from before leg c, so the
// same row measures both).

package session_test

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"

	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
)

// The encounter's keys, before and after leg c, as the store spells them.
const (
	measureEncPrefix       = "session-enc:"
	measureLogKeyPrefix    = "session-enc-log:v1alpha2:"
	measureRestKeyPrefix   = "session-enc:v1alpha2:"
	measureLegacyKeyPrefix = "session-enc:v1alpha1:"
)

// saveBytes is a go-redis hook that sums what the encounter's saves hand
// Redis. It counts only scripts whose first key is an encounter key.
type saveBytes struct {
	mu                   sync.Mutex
	total, rest, segment int
	saves                int
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
	total := 0
	for _, a := range args {
		total += argLen(a)
	}
	// The fenced save's ARGV (redis_fenced.go): ARGV[6] is the rest (the
	// whole blob before leg c), ARGV[9] the appended segment when the third
	// key is the record list.
	rest, segment := 0, 0
	if numKeys, ok := args[2].(int); ok {
		argv := 3 + numKeys
		if len(args) > argv+5 {
			rest = argLen(args[argv+5])
		}
		if third, ok := args[5].(string); ok && strings.HasPrefix(third, measureLogKeyPrefix) && len(args) > argv+8 {
			segment = argLen(args[argv+8])
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.total += total
	h.rest += rest
	h.segment += segment
	h.saves++
}

// take returns and resets the sums since the last take.
func (h *saveBytes) take() (total, rest, segment, saves int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	total, rest, segment, saves = h.total, h.rest, h.segment, h.saves
	h.total, h.rest, h.segment, h.saves = 0, 0, 0, 0
	return total, rest, segment, saves
}

func argLen(a interface{}) int {
	switch v := a.(type) {
	case string:
		return len(v)
	case []byte:
		return len(v)
	default:
		return len(fmt.Sprint(v))
	}
}

// storedBytes is what the store holds for an encounter: the rest's STRLEN
// and the record list's total (after leg c), plus the single blob's STRLEN
// (before it). The second return is the record list's length.
func storedBytes(t *testing.T, client *goredis.Client, encounter string) (restLen, logLen, segments int) {
	t.Helper()
	ctx := context.Background()
	rest, err := client.StrLen(ctx, measureRestKeyPrefix+encounter).Result()
	require.NoError(t, err)
	legacy, err := client.StrLen(ctx, measureLegacyKeyPrefix+encounter).Result()
	require.NoError(t, err)
	elems, err := client.LRange(ctx, measureLogKeyPrefix+encounter, 0, -1).Result()
	require.NoError(t, err)
	for _, e := range elems {
		logLen += len(e)
	}
	return int(rest + legacy), logLen, len(elems)
}

// bytesRow is one mode's per-Move save and store sizes.
type bytesRow struct {
	key, mode                       string
	write, rest, segment, saves     []int
	storedRest, storedLog, segments []int
}

// bytesTable renders each mode's per-Move series in order, then p50.
func bytesTable(rows []bytesRow) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\nbytes per Move (rpg-project#897 leg c): write = every arg of the encounter's save scripts; stored = rest STRLEN + record list\n")
	for _, r := range rows {
		series := func(name string, v []int) {
			parts := make([]string, len(v))
			for i, x := range v {
				parts[i] = fmt.Sprint(x)
			}
			fmt.Fprintf(&b, "%s %s %s, in order: %s\n", r.key, r.mode, name, strings.Join(parts, " "))
		}
		series("write bytes", r.write)
		series("rest bytes written", r.rest)
		series("segment bytes written", r.segment)
		series("saves", r.saves)
		series("stored rest bytes", r.storedRest)
		series("stored record bytes", r.storedLog)
		series("stored segments", r.segments)
	}
	fmt.Fprintf(&b, "| keep | mode | n | write p50 | write first | write last | rest p50 | segment p50 | stored rest last | stored record last |\n")
	fmt.Fprintf(&b, "|---|---|---|---|---|---|---|---|---|---|\n")
	for _, r := range rows {
		if len(r.write) == 0 {
			continue
		}
		fmt.Fprintf(&b, "| %s | %s | %d | %d | %d | %d | %d | %d | %d | %d |\n",
			r.key, r.mode, len(r.write), p50int(r.write), r.write[0], r.write[len(r.write)-1],
			p50int(r.rest), p50int(r.segment), r.storedRest[len(r.storedRest)-1], r.storedLog[len(r.storedLog)-1])
	}
	return b.String()
}

func p50int(v []int) int {
	if len(v) == 0 {
		return 0
	}
	s := append([]int(nil), v...)
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
	return s[len(s)/2]
}
