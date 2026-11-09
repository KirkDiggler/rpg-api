// measure_answer_first_beat_test.go is the live half of rpg-project#879's
// measurement harness (S0, R5: the harness is the record). It measures R1 --
// a verb answers within 100 ms at the garrison keep and its first beat lands
// within 250 ms after -- on the server path, for both answer shapes.
//
// What: per world-clock Move, the answer (handler Move returns), the first
// beat, the last beat, the drain, acts per verb, and a Redis GET floor; p50
// and p95. The first beat is the first act the walker receives on the
// ActBroker for this verb -- their own or anyone's they see -- with an id
// above the last act id received before the Move was issued (and above
// Through when the answer continues); the last beat is the verb's last act
// for the walker. A Move with no such act is counted in the "no beat"
// column, never dropped from the percentiles silently, and fails the run in
// AnswerWhole mode, where every verb publishes.
// Keep: content/bench/bench-keep-garrison.yaml (10k hexes, 99 garrison, 20
// patrols, neutral to the party), the party seated on the file's seat, an
// 8-cell walk back and forth along an axial line (a 30 ft walker pays a
// world round every six cells).
// Knobs: RPG_MEASURE=1 (required; the test skips without it, so CI never
// runs it), RPG_MEASURE_N (measured Moves per mode, default 20),
// RPG_MEASURE_MODES ("whole" or "continues"; both by default),
// RPG_MEASURE_KEYS, RPG_MEASURE_REDIS (a loopback Redis it may flush,
// default 127.0.0.1:6491; it refuses port 6379 and a store that already
// holds keys), RPG_MEASURE_ACT_BUFFER (the broker's buffer in commits;
// the production default when unset), RPG_MEASURE_RELOAD=0 (skips the
// cold-reload row: after each mode's walk, N rebuilds of that session from
// the store by stage -- measure_cold_reload_test.go).
// Command:
//
//	redis-server --port 6491 --save '' --daemonize yes
//	RPG_MEASURE=1 RPG_MEASURE_N=30 go test ./internal/integration/session/ \
//	  -run TestMeasure_AnswerAndFirstBeat -count=1 -v -timeout 60m
//
// Caveats: timing ends at the ActBroker channel -- no gRPC transport,
// protobuf encoding or network. Acts are per-recipient segments, and a
// continued verb delivers all its acts (the answered steps' included) after
// its save, so under S1 the first beat lands with the last. The broker runs
// at its production buffer; the table says whether the subscriber lagged.
// Costs grow with every Move (the record grows), so percentiles depend on N.

package session_test

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	sessionhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

// TestMeasure_AnswerAndFirstBeat measures rpg-project#879 R1 on the live
// server path (the v1alpha1 handler's Move over the orchestrator, a real
// Redis, the ActBroker the stream reads): how long a world-clock Move takes
// to answer, and how long until the walker's first and last act of that verb
// reach a subscriber, with the continuation runner (S1) and without it
// (AnswerWhole, today's whole-walk answer).
//
// MEASUREMENT ONLY: it never runs in CI. It needs RPG_MEASURE=1 and a Redis
// it may flush (RPG_MEASURE_REDIS, default 127.0.0.1:6491):
//
//	redis-server --port 6491 --save '' --daemonize yes
//	RPG_MEASURE=1 go test ./internal/integration/session/ -run TestMeasure_AnswerAndFirstBeat -count=1 -v -timeout 30m
//
// RPG_MEASURE_KEYS picks the shelf files (content/bench/<key>.yaml or
// content/<key>.yaml), default bench-keep-garrison (the plain bench-keep
// forms a fight at launch, so its Move is not a world-clock walk);
// RPG_MEASURE_N the measured Moves per mode (default 20); RPG_MEASURE_MODES
// "whole" or "continues" runs one mode only, for a profile:
//
//	RPG_MEASURE=1 RPG_MEASURE_MODES=whole RPG_MEASURE_N=10 go test ./internal/integration/session/ \
//	  -run TestMeasure_AnswerAndFirstBeat -count=1 -v -cpuprofile cpu.out
func TestMeasure_AnswerAndFirstBeat(t *testing.T) {
	if os.Getenv("RPG_MEASURE") != "1" {
		t.Skip("measurement: set RPG_MEASURE=1 (and a flushable Redis at RPG_MEASURE_REDIS)")
	}
	addr := envOr("RPG_MEASURE_REDIS", "127.0.0.1:6491")
	keys := strings.Split(envOr("RPG_MEASURE_KEYS", "bench-keep-garrison"), ",")
	n := 20
	if v := os.Getenv("RPG_MEASURE_N"); v != "" {
		_, err := fmt.Sscanf(v, "%d", &n)
		require.NoError(t, err)
	}
	// RPG_MEASURE_MODES narrows the run to one mode ("whole" or
	// "continues"), so a -cpuprofile covers only that mode's Moves.
	modes := []bool{true, false}
	switch os.Getenv("RPG_MEASURE_MODES") {
	case "whole":
		modes = []bool{true}
	case "continues":
		modes = []bool{false}
	}
	// The broker runs at its production buffer unless RPG_MEASURE_ACT_BUFFER
	// says otherwise; a subscriber that lags is reported in the table.
	if v := os.Getenv("RPG_MEASURE_ACT_BUFFER"); v != "" {
		t.Setenv(sessionorch.ActBufferEnv, v)
	}
	const warm = 3
	// RPG_MEASURE_PATH is the walk's length in cells (default 8; a 30 ft
	// walker pays a world round every six cells), so a run at 1, 4 and 8
	// separates what a verb pays once from what each step pays.
	pathLen := 8
	if v := os.Getenv("RPG_MEASURE_PATH"); v != "" {
		_, err := fmt.Sscanf(v, "%d", &pathLen)
		require.NoError(t, err)
		require.Positive(t, pathLen, "RPG_MEASURE_PATH")
	}

	client := goredis.NewClient(&goredis.Options{Addr: addr})
	t.Cleanup(func() { _ = client.Close() })
	require.NoError(t, client.Ping(context.Background()).Err(), "redis at %s", addr)
	guardFlushable(t, client, addr)

	// The floor: a trivial GET round trip.
	require.NoError(t, client.Set(context.Background(), "measure:floor", "x", 0).Err())
	floor := make([]time.Duration, 0, 200)
	for range 200 {
		s := time.Now()
		require.NoError(t, client.Get(context.Background(), "measure:floor").Err())
		floor = append(floor, time.Since(s))
	}

	type row struct {
		key, mode                                string
		answer, first, last, drained             []time.Duration
		continues, acts, firstAfterAnswer, steps []int
		compile, launch                          time.Duration
		lagged                                   bool
		noBeat                                   int
	}
	rows := make([]row, 0, len(modes)*len(keys))
	// RPG_MEASURE_RELOAD=0 skips the cold-reload row (measure_cold_reload_test.go).
	reloadOn := os.Getenv("RPG_MEASURE_RELOAD") != "0"
	var reloads []reloadRow
	moveTotals := map[string][2][]time.Duration{}

	for _, key := range keys {
		raw := readShelf(t, key)
		cs := time.Now()
		compiled, err := sessionworld.Compile(raw)
		require.NoError(t, err, "compiling %s", key)
		compileTook := time.Since(cs)

		for _, answerWhole := range modes {
			mode := "S1 continues"
			if answerWhole {
				mode = "AnswerWhole"
			}
			require.NoError(t, client.FlushDB(context.Background()).Err())

			charRepo, err := characterrepo.NewRedis(&characterrepo.RedisConfig{Client: client})
			require.NoError(t, err)
			// The production runner, counted: Orchestrator.Drain is the
			// shutdown drain (it refuses every later continuation), so the
			// run waits on its own count between Moves instead.
			runner := &countedRunner{inner: sessionorch.NewInProcessContinuationRunner()}
			var continuations sdk.ContinuationRunner = runner
			if answerWhole {
				continuations = nil
			}
			orch, err := sessionorch.New(sessionorch.Config{
				Redis: client, Characters: charRepo, TTL: 24 * time.Hour, Dice: testDice{},
				PresentationIDs: idgen.NewSequential("presentation"),
				AnswerWhole:     answerWhole, Continuations: continuations,
			})
			require.NoError(t, err)
			handler, err := sessionhandler.New(&sessionhandler.HandlerConfig{
				Manager: orch.Manager, Characters: charRepo, Preferences: orch.Preferences,
			})
			require.NoError(t, err)

			ctx := auth.WithPlayerID(context.Background(), "player-alice")
			_, err = charRepo.Create(context.Background(), characterrepo.CreateInput{
				Character: &entities.Character{Data: armedFighter("alice", "player-alice")},
			})
			require.NoError(t, err)

			run := "measure-" + key
			ls := time.Now()
			out := (&acceptanceHarness{manager: orch}).launchAtStart(t, run, compiled.Spec, "alice")
			launchTook := time.Since(ls)
			start := launched(t, out, "alice").Position
			t.Logf("%s %s: compile %v, launch %v, %d members, alice at (%v,%v)",
				key, mode, compileTook, launchTook, len(out.Members), start.X, start.Y)

			sub, err := orch.ActBroker.Subscribe(run, "alice")
			require.NoError(t, err)
			var mu sync.Mutex
			type arrival struct {
				id, verb string
				at       time.Time
			}
			var got []arrival
			done := make(chan struct{})
			go func() {
				defer close(done)
				for unit := range sub.Units() {
					now := time.Now()
					mu.Lock()
					for _, a := range unit {
						got = append(got, arrival{id: a.Act.ID, verb: a.Act.Verb, at: now})
					}
					mu.Unlock()
				}
			}()

			// Back and forth along the seat's axial q line: find a direction where a
			// whole pathLen walk lands, then alternate.
			x, y := int(start.X), int(start.Y)
			dir := 0
			path := func(d int) []*sessionpb.Position {
				p := make([]*sessionpb.Position, 0, pathLen)
				for i := 1; i <= pathLen; i++ {
					p = append(p, &sessionpb.Position{X: float64(x + d*i), Y: float64(y)}) // axial: (q±1, r) is a neighbor
				}
				return p
			}
			r := row{key: key, mode: mode, compile: compileTook, launch: launchTook}
			for i := 0; i < warm+n; i++ {
				if dir == 0 {
					dir = 1
				}
				p := path(dir)
				// The high-water id: the last act the walker received before
				// this Move. Only acts above it are this verb's.
				mu.Lock()
				highWater := ""
				if len(got) > 0 {
					highWater = got[len(got)-1].id
				}
				mu.Unlock()
				t0 := time.Now()
				resp, err := handler.Move(ctx, &sessionpb.MoveRequest{Session: run, Member: "alice", Path: p})
				answered := time.Now()
				require.NoError(t, err, "move %d dir %d from (%d,%d)", i, dir, x, y)
				dctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				require.NoError(t, runner.wait(dctx))
				drainedAt := time.Since(t0)
				cancel()
				// Wait until the subscriber holds the verb's last act for the
				// walker, as the catch-up read names it (no fixed sleep).
				expected, err := orch.Manager.Acts(context.Background(), &sdk.ActsInput{
					Session: run, Member: "alice", AfterID: highWater,
				})
				require.NoError(t, err)
				if len(expected.Acts) > 0 {
					want := expected.Acts[len(expected.Acts)-1].ID
					require.Eventually(t, func() bool {
						mu.Lock()
						defer mu.Unlock()
						return len(got) > 0 && got[len(got)-1].id >= want
					}, 30*time.Second, time.Millisecond, "the subscriber never received act %s", want)
				}

				where, err := orch.Manager.Where(context.Background(), &sdk.WhereInput{Session: run, Member: "alice"})
				require.NoError(t, err)
				nx, ny := int(where.Position.X), int(where.Position.Y)
				end := p[len(p)-1]
				if nx != int(end.GetX()) || ny != int(end.GetY()) {
					t.Logf("move %d dir %d from (%d,%d) stopped at (%d,%d) (%d steps answered)",
						i, dir, x, y, nx, ny, len(resp.GetSteps()))
					require.Less(t, i, warm, "a measured move must land whole")
					dir = -dir
					x, y = nx, ny
					continue
				}
				x, y = nx, ny
				dir = -dir

				mu.Lock()
				var acts []arrival
				for _, a := range got {
					if a.id > highWater {
						acts = append(acts, a)
					}
				}
				mu.Unlock()
				if i < warm {
					if i == 0 {
						ids := make([]string, 0, len(acts))
						for _, a := range acts {
							ids = append(ids, a.verb+"@"+a.id)
						}
						t.Logf("sample acts (continues=%v through=%q): %v", resp.GetContinues(), resp.GetThrough(), ids)
					}
					continue
				}
				r.answer = append(r.answer, answered.Sub(t0))
				r.drained = append(r.drained, drainedAt)
				r.steps = append(r.steps, len(resp.GetSteps()))
				c := 0
				if resp.GetContinues() {
					c = 1
				}
				r.continues = append(r.continues, c)
				r.acts = append(r.acts, len(acts))
				// First beat: the first act whose id is after Through when the
				// answer continues (the acts at or before it are the answered
				// steps, delivered with the rest; record ids are ULIDs, so
				// "after" is a string compare); the verb's first act otherwise.
				after := highWater
				if through := resp.GetThrough(); through > after {
					after = through
				}
				firstIdx := len(acts)
				for j, a := range acts {
					if a.id > after {
						firstIdx = j
						break
					}
				}
				if firstIdx == len(acts) {
					r.noBeat++
					require.False(t, answerWhole, "move %d: no act for the walker after %q; a whole verb always publishes", i, after)
				} else {
					r.first = append(r.first, acts[firstIdx].at.Sub(t0))
					r.last = append(r.last, acts[len(acts)-1].at.Sub(t0))
					arrivedAfter := 0
					if acts[0].at.After(answered) {
						arrivedAfter = 1
					}
					r.firstAfterAnswer = append(r.firstAfterAnswer, arrivedAfter)
				}
			}
			series := make([]string, 0, len(r.answer))
			for j := range r.answer {
				series = append(series, fmt.Sprintf("%d", r.answer[j].Milliseconds()))
			}
			t.Logf("%s %s answer ms per move, in order: %s", key, mode, strings.Join(series, " "))
			series = series[:0]
			for j := range r.first {
				series = append(series, fmt.Sprintf("%d", r.first[j].Milliseconds()))
			}
			t.Logf("%s %s first-beat ms per move, in order: %s", key, mode, strings.Join(series, " "))
			lagged, lastID := sub.Lagged()
			r.lagged = lagged
			if lagged {
				t.Logf("%s %s: the subscriber lagged at the broker's buffer (last id %s)", key, mode, lastID)
			}
			require.NoError(t, sub.Close())
			<-done
			if reloadOn {
				reloads = append(reloads, measureColdReload(t, client, orch, run, "alice", key, mode, n))
				moveTotals[key+"|"+mode] = [2][]time.Duration{r.answer, r.drained}
			}
			require.NoError(t, client.FlushDB(context.Background()).Err())
			rows = append(rows, r)
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "\nredis GET floor: p50 %s p95 %s (n=%d)\n", ms(pct(floor, 50)), ms(pct(floor, 95)), len(floor))
	fmt.Fprintf(&b, "| keep | mode | n | continues | answer p50 | answer p95 | first beat p50 | first beat p95 | last beat p50 | last beat p95 | drained p50 | drained p95 | acts/verb | steps answered | no beat | acts arrive after answer | lagged | launch |\n")
	fmt.Fprintf(&b, "|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "| %s | %s | %d | %d/%d | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %d | %d/%d | %v | %s |\n",
			r.key, r.mode, len(r.answer), sum(r.continues), len(r.continues),
			ms(pct(r.answer, 50)), ms(pct(r.answer, 95)),
			ms(pct(r.first, 50)), ms(pct(r.first, 95)),
			ms(pct(r.last, 50)), ms(pct(r.last, 95)),
			ms(pct(r.drained, 50)), ms(pct(r.drained, 95)),
			span(r.acts), span(r.steps), r.noBeat, sum(r.firstAfterAnswer), len(r.firstAfterAnswer), r.lagged,
			r.launch.Round(time.Millisecond))
	}
	if reloadOn {
		b.WriteString(reloadTable(reloads, moveTotals))
	}
	stepRows := make([]stepRow, 0, len(rows))
	for _, r := range rows {
		stepRows = append(stepRows, stepRow{key: r.key, mode: r.mode, answer: r.answer, steps: r.steps, continues: r.continues})
	}
	b.WriteString(perStepTable(stepRows, pathLen))
	t.Log(b.String())
}

// stepRow is one mode's answers beside the steps each answer covered.
type stepRow struct {
	key, mode string
	answer    []time.Duration
	steps     []int
	continues []int
}

// perStepTable splits each mode's answer by the steps it covered and by
// whether it continued: p50 per group, and a least-squares line answer =
// fixed + steps x per-step over the Moves that continued. A continued S1
// answer is the load plus the walker's own steps up to the step that pays a
// round, nothing saved, so the slope is the live cost of one non-raising
// step and the intercept what the verb pays once before its first step. An
// answer that did not continue (the walk ended before a pace came due, or
// AnswerWhole) also carries the save and the commit, so it is tabled but
// left out of the line.
func perStepTable(rows []stepRow, pathLen int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\nper step (path %d cells): answer by steps answered\n", pathLen)
	fmt.Fprintf(&b, "| keep | mode | steps answered | continued | n | answer p50 | per step (p50 / steps) |\n|---|---|---|---|---|---|---|\n")
	for _, r := range rows {
		by := map[[2]int][]time.Duration{}
		for i := range r.answer {
			k := [2]int{r.steps[i], r.continues[i]}
			by[k] = append(by[k], r.answer[i])
		}
		groups := make([][2]int, 0, len(by))
		for k := range by {
			groups = append(groups, k)
		}
		slices.SortFunc(groups, func(a, c [2]int) int {
			if a[1] != c[1] {
				return c[1] - a[1]
			}
			return a[0] - c[0]
		})
		for _, k := range groups {
			p := pct(by[k], 50)
			per := "-"
			if k[0] > 0 {
				per = ms(p / time.Duration(k[0]))
			}
			fmt.Fprintf(&b, "| %s | %s | %d | %v | %d | %s | %s |\n", r.key, r.mode, k[0], k[1] == 1, len(by[k]), ms(p), per)
		}
	}
	fmt.Fprintf(&b, "\n| keep | mode | fit over continued moves: fixed per verb | fit: per step | moves fitted |\n|---|---|---|---|---|\n")
	for _, r := range rows {
		var xs []int
		var ys []time.Duration
		for i := range r.answer {
			if r.continues[i] == 1 {
				xs, ys = append(xs, r.steps[i]), append(ys, r.answer[i])
			}
		}
		fixed, slope, ok := fitLine(xs, ys)
		if !ok {
			fmt.Fprintf(&b, "| %s | %s | - | - | %d |\n", r.key, r.mode, len(xs))
			continue
		}
		fmt.Fprintf(&b, "| %s | %s | %.2f ms | %.2f ms | %d |\n", r.key, r.mode, fixed, slope, len(xs))
	}
	return b.String()
}

// fitLine is ordinary least squares of y (ms) on x; ok is false when x does
// not vary.
func fitLine(x []int, y []time.Duration) (intercept, slope float64, ok bool) {
	n := float64(len(x))
	if n < 2 {
		return 0, 0, false
	}
	var sx, sy, sxx, sxy float64
	for i := range x {
		xi, yi := float64(x[i]), float64(y[i])/float64(time.Millisecond)
		sx += xi
		sy += yi
		sxx += xi * xi
		sxy += xi * yi
	}
	d := n*sxx - sx*sx
	if d == 0 {
		return 0, 0, false
	}
	slope = (n*sxy - sx*sy) / d
	return (sy - slope*sx) / n, slope, true
}

// countedRunner is the in-process continuation runner with a count of the
// continuations in flight, so a measurement can wait for one verb's rest
// without the shutdown drain.
type countedRunner struct {
	inner *sessionorch.InProcessContinuationRunner
	wg    sync.WaitGroup
}

func (r *countedRunner) Run(ctx context.Context, in *sdk.RunContinuationInput) error {
	if in == nil {
		return r.inner.Run(ctx, in)
	}
	counted := *in
	r.wg.Add(1)
	cont := in.Continue
	counted.Continue = func(ctx context.Context) (*sdk.ContinuationReport, error) {
		defer r.wg.Done()
		return cont(ctx)
	}
	if err := r.inner.Run(ctx, &counted); err != nil {
		r.wg.Done()
		return err
	}
	return nil
}

func (r *countedRunner) wait(ctx context.Context) error {
	done := make(chan struct{})
	go func() { r.wg.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// guardFlushable refuses a Redis the run was not pointed at: it must be on
// loopback, not on 6379 (the default every local store uses), answer on the
// port it was dialed on, and hold no keys before the run.
func guardFlushable(t *testing.T, client *goredis.Client, addr string) {
	t.Helper()
	host, port, err := net.SplitHostPort(addr)
	require.NoError(t, err, "RPG_MEASURE_REDIS %q", addr)
	ip := net.ParseIP(host)
	require.True(t, host == "localhost" || (ip != nil && ip.IsLoopback()),
		"RPG_MEASURE_REDIS %q: the run flushes its store, so it must be a loopback Redis", addr)
	require.NotEqual(t, "6379", port, "RPG_MEASURE_REDIS %q: refusing to flush the default Redis port", addr)
	info, err := client.Info(context.Background(), "server").Result()
	require.NoError(t, err)
	require.Contains(t, info, "tcp_port:"+port, "RPG_MEASURE_REDIS %q: the server does not report that port", addr)
	size, err := client.DBSize(context.Background()).Result()
	require.NoError(t, err)
	require.Zero(t, size, "RPG_MEASURE_REDIS %q holds %d keys; the run flushes its store, so point it at an empty one", addr, size)
}

func envOr(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}

func readShelf(t *testing.T, key string) []byte {
	t.Helper()
	dir := dungeonstest.ContentDir(t)
	for _, p := range []string{filepath.Join(dir, "bench", key+".yaml"), filepath.Join(dir, key+".yaml")} {
		raw, err := os.ReadFile(p) //nolint:gosec // the shipped content directory
		if err == nil {
			return raw
		}
	}
	require.FailNowf(t, "no shelf file", "%s", key)
	return nil
}

func pct(d []time.Duration, p int) time.Duration {
	if len(d) == 0 {
		return 0
	}
	s := slices.Clone(d)
	slices.Sort(s)
	i := (len(s)*p + 99) / 100
	if i < 1 {
		i = 1
	}
	return s[i-1]
}

func ms(d time.Duration) string { return fmt.Sprintf("%.2f ms", float64(d)/float64(time.Millisecond)) }

func sum(v []int) int {
	s := 0
	for _, x := range v {
		s += x
	}
	return s
}

func span(v []int) string {
	if len(v) == 0 {
		return "-"
	}
	lo, hi := slices.Min(v), slices.Max(v)
	if lo == hi {
		return fmt.Sprint(lo)
	}
	return fmt.Sprintf("%d–%d", lo, hi)
}
