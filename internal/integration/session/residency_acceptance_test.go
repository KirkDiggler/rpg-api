package session_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

	"github.com/KirkDiggler/rpg-api/internal/entities"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

// THE RESIDENT COPY AGAINST REAL REDIS (rpg-project#879 S6, R6,
// rpg-project#927). A write verb keeps its encounter in memory, and the next
// verb on the same session checks it with one GET of the checkpoint version
// before running on it. Each scene counts what the verb read from the store:
// a warm verb reads no piece of the encounter but the version; a verb after
// another writer's save, after a stale save, or on a process that keeps none
// reads the encounter back.

const residencyRun = "resident-run"

// residentProcess is one api process over a shared Redis: its own client
// (so its command log is its own), its own orchestrator and residency.
type residentProcess struct {
	orch *sessionorch.Orchestrator
	seen *commandLog
}

func newResidentProcess(t *testing.T, mr *miniredis.Miniredis, residency *sessionorch.Residency) *residentProcess {
	t.Helper()
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	chars, err := characterrepo.NewRedis(&characterrepo.RedisConfig{Client: client})
	require.NoError(t, err)
	orch, err := sessionorch.New(sessionorch.Config{
		Redis: client, Characters: chars, TTL: 24 * time.Hour, Dice: testDice{},
		PresentationIDs: idgen.NewSequential("presentation"), AnswerWhole: true,
		Residency: residency,
	})
	require.NoError(t, err)
	seen := &commandLog{}
	client.AddHook(seen)
	return &residentProcess{orch: orch, seen: seen}
}

// launchResidentRun seeds alice and bob and launches the tomb with both
// seated, through p.
func launchResidentRun(t *testing.T, mr *miniredis.Miniredis, p *residentProcess) {
	t.Helper()
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	chars, err := characterrepo.NewRedis(&characterrepo.RedisConfig{Client: client})
	require.NoError(t, err)
	for _, who := range []string{"alice", "bob"} {
		_, createErr := chars.Create(context.Background(), characterrepo.CreateInput{
			Character: &entities.Character{Data: armedFighter(who, "player-"+who)},
		})
		require.NoError(t, createErr)
	}
	_, err = p.orch.Manager.Launch(context.Background(),
		launchInput(residencyRun, buildThreeRoomTomb(t), []partySeat{seatAt("alice", 1, 1), seatAt("bob", 1, 2)}))
	require.NoError(t, err)
}

// move walks member one cell through p and returns what the verb read of
// the encounter.
func (p *residentProcess) move(t *testing.T, member string, to spatial.Position) (encounterReads, error) {
	t.Helper()
	p.seen.reset()
	_, err := p.orch.Manager.Move(context.Background(), &sdk.MoveInput{
		Session: residencyRun, Member: member, Path: []spatial.Position{to},
	})
	return p.seen.encounterReads(), err
}

func TestResidency_AWarmVerbReadsOnlyTheVersion(t *testing.T) {
	mr := miniredis.RunT(t)
	p := newResidentProcess(t, mr, nil)
	launchResidentRun(t, mr, p)

	// Launch keeps the board it saved: the first verb is already warm.
	warm, err := p.move(t, "alice", at(2, 1))
	require.NoError(t, err)
	require.Equal(t, encounterReads{"get session-enc-ver:v1alpha1:" + encounterOf(t, mr)}, warm,
		"the first verb after Launch reads the version and no piece of the encounter")

	// The same verb on a process holding no copy reads the encounter back:
	// the count above can tell a load from none.
	cold, err := newResidentProcess(t, mr, nil).move(t, "alice", at(3, 1))
	require.NoError(t, err)
	require.True(t, cold.loaded(), "a process with no copy loads: %v", cold)

	reads, err := p.move(t, "alice", at(4, 1))
	require.NoError(t, err)
	require.True(t, reads.loaded(), "the other process saved: this one reloads (%v)", reads)

	warm, err = p.move(t, "alice", at(5, 1))
	require.NoError(t, err)
	require.Equal(t, encounterReads{"get session-enc-ver:v1alpha1:" + encounterOf(t, mr)}, warm,
		"the next verb on the session reads the version and no piece of the encounter")
	require.Equal(t, 1, p.orch.Residency.Len(), "the copy is kept again after the warm verb")

	again, err := p.move(t, "bob", at(2, 2))
	require.NoError(t, err)
	require.False(t, again.loaded(), "a verb by another member of the same session is warm too: %v", again)
	require.Equal(t, sessionorch.ResidencyStats{
		Hits:   3,
		Misses: map[sdk.MissReason]int64{sdk.MissVersionMoved: 1},
	}, p.orch.Residency.Stats(), "three warm verbs, one reload after the other process saved")
}

func TestResidency_AnotherProcessSaveForcesAReload(t *testing.T) {
	mr := miniredis.RunT(t)
	a := newResidentProcess(t, mr, nil)
	b := newResidentProcess(t, mr, nil)
	launchResidentRun(t, mr, a)

	_, err := a.move(t, "alice", at(2, 1))
	require.NoError(t, err)
	reads, err := b.move(t, "bob", at(2, 2))
	require.NoError(t, err)
	require.True(t, reads.loaded(), "b holds no copy: it loads")

	reads, err = a.move(t, "alice", at(3, 1))
	require.NoError(t, err, "a's copy is never used past b's save, so the fence never refuses a")
	require.True(t, reads.loaded(), "b's save moved the version: a reloads (%v)", reads)

	// b's copy is now behind a's save in turn.
	reads, err = b.move(t, "bob", at(3, 2))
	require.NoError(t, err)
	require.True(t, reads.loaded(), "a's save moved the version: b reloads (%v)", reads)

	reads, err = b.move(t, "bob", at(4, 2))
	require.NoError(t, err)
	require.False(t, reads.loaded(), "nothing else saved: b's copy is warm (%v)", reads)
	require.Equal(t, sessionorch.ResidencyStats{
		Hits:   1,
		Misses: map[sdk.MissReason]int64{sdk.MissVersionMoved: 1},
	}, a.orch.Residency.Stats())
	require.Equal(t, sessionorch.ResidencyStats{
		Hits:   1,
		Misses: map[sdk.MissReason]int64{sdk.MissNoCopy: 1, sdk.MissVersionMoved: 1},
	}, b.orch.Residency.Stats())
}

func TestResidency_AStaleSaveDropsTheCopy(t *testing.T) {
	mr := miniredis.RunT(t)
	p := newResidentProcess(t, mr, nil)
	launchResidentRun(t, mr, p)
	_, err := p.move(t, "alice", at(2, 1))
	require.NoError(t, err)

	// Another writer lands a save between this verb's version check and its
	// save: the copy was current when taken, the fence refuses its save.
	late := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = late.Close() })
	p.seen.beforeScript = func(ctx context.Context) error {
		return late.Incr(ctx, "session-enc-ver:v1alpha1:"+encounterOf(t, mr)).Err()
	}
	reads, err := p.move(t, "alice", at(3, 1))
	require.ErrorIs(t, err, sdk.ErrStaleCheckpoint)
	require.False(t, reads.loaded(), "the refused verb ran on the copy (%v)", reads)
	require.Zero(t, p.orch.Residency.Len(), "a stale save keeps nothing")

	p.seen.beforeScript = nil
	reads, err = p.move(t, "alice", at(3, 1))
	require.NoError(t, err)
	require.True(t, reads.loaded(), "the next verb reloads from the store (%v)", reads)
	require.Equal(t, sessionorch.ResidencyStats{
		Hits:   2,
		Misses: map[sdk.MissReason]int64{sdk.MissNoCopy: 1},
	}, p.orch.Residency.Stats(), "the refused verb was a hit; the copy it held was dropped, so the next has none")
}

func TestResidency_KeepingNoneLoadsEveryVerb(t *testing.T) {
	mr := miniredis.RunT(t)
	none, err := sessionorch.NewResidency(&sessionorch.ResidencyConfig{Entries: -1})
	require.NoError(t, err)
	p := newResidentProcess(t, mr, none)
	launchResidentRun(t, mr, p)

	for i, to := range []spatial.Position{at(2, 1), at(3, 1), at(4, 1)} {
		reads, err := p.move(t, "alice", to)
		require.NoError(t, err)
		require.True(t, reads.loaded(), "verb %d loads (%v)", i+1, reads)
		require.Zero(t, none.Len())
	}
	require.Equal(t, sessionorch.ResidencyStats{
		Misses: map[sdk.MissReason]int64{sdk.MissNoCopy: 3},
	}, none.Stats())
}

// encounterOf is the storage id of the one encounter in the store.
func encounterOf(t *testing.T, mr *miniredis.Miniredis) string {
	t.Helper()
	keys := mr.Keys()
	var ids []string
	for _, k := range keys {
		if id, ok := strings.CutPrefix(k, "session-enc-ver:v1alpha1:"); ok {
			ids = append(ids, id)
		}
	}
	require.Len(t, ids, 1, "one run, one encounter")
	return ids[0]
}

// encounterReads is every command a verb sent that read an encounter key
// (any session-enc* key), as "name key", in order. The save's script is a
// write and is not listed.
type encounterReads []string

// loaded reports whether the verb read the encounter's head: the fenced read
// a resident copy skips.
func (r encounterReads) loaded() bool {
	for _, line := range r {
		if strings.HasPrefix(line, "get session-enc:v1alpha3:") {
			return true
		}
	}
	return false
}

// commandLog records every command a client sends, as "name key", and can
// run a step right before the first script (the save) of a verb.
type commandLog struct {
	mu           sync.Mutex
	commands     []string
	beforeScript func(ctx context.Context) error
	fired        bool
}

func (l *commandLog) reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.commands, l.fired = nil, false
}

func (l *commandLog) encounterReads() encounterReads {
	l.mu.Lock()
	defer l.mu.Unlock()
	reads := encounterReads{}
	for _, line := range l.commands {
		name, key, _ := strings.Cut(line, " ")
		if name == "evalsha" || name == "eval" || name == "script" {
			continue
		}
		if strings.HasPrefix(key, "session-enc") {
			reads = append(reads, line)
		}
	}
	return reads
}

func (l *commandLog) record(cmd goredis.Cmder) {
	line := cmd.Name()
	if args := cmd.Args(); len(args) > 1 {
		line += " " + fmt.Sprint(args[1])
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.commands = append(l.commands, line)
}

func (l *commandLog) DialHook(next goredis.DialHook) goredis.DialHook { return next }
func (l *commandLog) ProcessPipelineHook(next goredis.ProcessPipelineHook) goredis.ProcessPipelineHook {
	return func(ctx context.Context, cmds []goredis.Cmder) error {
		for _, cmd := range cmds {
			l.record(cmd)
		}
		return next(ctx, cmds)
	}
}
func (l *commandLog) ProcessHook(next goredis.ProcessHook) goredis.ProcessHook {
	return func(ctx context.Context, cmd goredis.Cmder) error {
		l.record(cmd)
		if name := cmd.Name(); name == "evalsha" || name == "eval" {
			if err := l.scriptStep(ctx); err != nil {
				return err
			}
		}
		return next(ctx, cmd)
	}
}

func (l *commandLog) scriptStep(ctx context.Context) error {
	l.mu.Lock()
	step := l.beforeScript
	if step == nil || l.fired {
		l.mu.Unlock()
		return nil
	}
	l.fired = true
	l.mu.Unlock()
	if err := step(ctx); err != nil {
		return errors.Join(errors.New("before-script step"), err)
	}
	return nil
}
