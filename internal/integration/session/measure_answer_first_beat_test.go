package session_test

import (
	"context"
	"fmt"
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
// RPG_MEASURE_N the measured Moves per mode (default 20).
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
	// A garrison verb folds a few hundred acts for one recipient (every
	// patrol step it sees); the default buffer would close the subscriber
	// as lagged.
	t.Setenv(sessionorch.ActBufferEnv, "1000000")
	const warm = 3
	const pathLen = 8 // a 30 ft walker pays a world round every six cells

	client := goredis.NewClient(&goredis.Options{Addr: addr})
	t.Cleanup(func() { _ = client.Close() })
	require.NoError(t, client.Ping(context.Background()).Err(), "redis at %s", addr)

	// The floor: a trivial GET round trip.
	require.NoError(t, client.Set(context.Background(), "measure:floor", "x", 0).Err())
	var floor []time.Duration
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
	}
	var rows []row

	for _, key := range keys {
		raw := readShelf(t, key)
		cs := time.Now()
		compiled, err := sessionworld.Compile(raw)
		require.NoError(t, err, "compiling %s", key)
		compileTook := time.Since(cs)

		for _, answerWhole := range []bool{true, false} {
			mode := "S1 continues"
			if answerWhole {
				mode = "AnswerWhole"
			}
			require.NoError(t, client.FlushDB(context.Background()).Err())

			charRepo, err := characterrepo.NewRedis(&characterrepo.RedisConfig{Client: client})
			require.NoError(t, err)
			orch, err := sessionorch.New(sessionorch.Config{
				Redis: client, Characters: charRepo, TTL: 24 * time.Hour, Dice: testDice{},
				PresentationIDs: idgen.NewSequential("presentation"), AnswerWhole: answerWhole,
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
				for a := range sub.Acts() {
					now := time.Now()
					mu.Lock()
					got = append(got, arrival{id: a.Act.ID, verb: a.Act.Verb, at: now})
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
					p = append(p, &sessionpb.Position{X: float64(x + d*i), Y: float64(y)}) // axial: (q±1, r) is a neighbour
				}
				return p
			}
			r := row{key: key, mode: mode, compile: compileTook, launch: launchTook}
			for i := 0; i < warm+n; i++ {
				if dir == 0 {
					dir = 1
				}
				p := path(dir)
				mu.Lock()
				got = got[:0]
				mu.Unlock()
				t0 := time.Now()
				resp, err := handler.Move(ctx, &sessionpb.MoveRequest{Session: run, Member: "alice", Path: p})
				answered := time.Now()
				require.NoError(t, err, "move %d dir %d from (%d,%d)", i, dir, x, y)
				dctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				require.NoError(t, orch.Drain(dctx))
				drainedAt := time.Since(t0)
				cancel()
				// Let the subscriber goroutine take what the drain published.
				time.Sleep(20 * time.Millisecond)

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
				acts := slices.Clone(got)
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
				firstIdx := 0
				if through := resp.GetThrough(); through != "" {
					firstIdx = len(acts)
					for j, a := range acts {
						if a.id > through {
							firstIdx = j
							break
						}
					}
				}
				if len(acts) > 0 && firstIdx < len(acts) {
					r.first = append(r.first, acts[firstIdx].at.Sub(t0))
					r.last = append(r.last, acts[len(acts)-1].at.Sub(t0))
					after := 0
					if acts[0].at.After(answered) {
						after = 1
					}
					r.firstAfterAnswer = append(r.firstAfterAnswer, after)
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
			require.NoError(t, sub.Close())
			<-done
			require.NoError(t, client.FlushDB(context.Background()).Err())
			rows = append(rows, r)
		}
	}

	var b strings.Builder
	fmt.Fprintf(&b, "\nredis GET floor: p50 %s p95 %s (n=%d)\n", ms(pct(floor, 50)), ms(pct(floor, 95)), len(floor))
	fmt.Fprintf(&b, "| keep | mode | n | continues | answer p50 | answer p95 | first beat p50 | first beat p95 | last beat p50 | last beat p95 | drained p50 | drained p95 | acts/verb | steps answered | acts arrive after answer | launch |\n")
	fmt.Fprintf(&b, "|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|---|\n")
	for _, r := range rows {
		fmt.Fprintf(&b, "| %s | %s | %d | %d/%d | %s | %s | %s | %s | %s | %s | %s | %s | %s | %s | %d/%d | %s |\n",
			r.key, r.mode, len(r.answer), sum(r.continues), len(r.continues),
			ms(pct(r.answer, 50)), ms(pct(r.answer, 95)),
			ms(pct(r.first, 50)), ms(pct(r.first, 95)),
			ms(pct(r.last, 50)), ms(pct(r.last, 95)),
			ms(pct(r.drained, 50)), ms(pct(r.drained, 95)),
			span(r.acts), span(r.steps), sum(r.firstAfterAnswer), len(r.firstAfterAnswer),
			r.launch.Round(time.Millisecond))
	}
	t.Log(b.String())
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
