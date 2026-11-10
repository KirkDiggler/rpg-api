// measure_cold_reload_test.go is the cold-reload row of rpg-project#879's
// measurement harness (ruling R6: the resident encounter is a cache behind a
// lease, and a miss rebuilds from the store). For the session the measure
// test just walked, it times a rebuild from Redis by stage, outside any verb,
// with the same functions the session's load path calls:
//
//   - fetch: the GET of the session-enc blob (the encounter repository's
//     read, without its decode), and the session record's GET beside it;
//   - decode: the blob's JSON into encounter.EncounterData (the repository's
//     own json.Unmarshal);
//   - site build: encounter.LoadEncounter over the decoded data with
//     encounter.RefusingCapabilities -- the composition's own load (field
//     compile, partitions, doors, members) with no session seam attached.
//     Since rpg-project#879 S5 the checkpoint names its site by hash, so this
//     load resolves it through the orchestrator's provider (warm: the site
//     is shared, nothing compiles);
//   - site piece and cold site: the stored site piece's GET + decode
//     (GetSite), and the same load on a FRESH provider handed that piece --
//     what the first load on a cold process pays;
//   - read load: Manager.Where, the session's whole read-path reload (lock,
//     session record, driver, fetch, decode, the standing seam and read
//     capabilities, the encounter's load); its residual after the stages
//     above is the attach stage;
//   - write: ToData on the loaded encounter, the JSON encode, and the SET of
//     those bytes under a scratch key (the session's own key is never
//     written).
//
// The attach stage (the session's standingFor, read/write capabilities,
// prepareExploration's sheet staging, OpenAct) is unexported in
// rulebooks/dnd5e/session, so it is not timed alone: the row reports the
// read load whole and the residual. The write-path opener (openForChange)
// adds ledger load, write capabilities, sheet staging and the act open on
// top of the read load, and is only reachable through a verb.

package session_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	sessionhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha1"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	"github.com/KirkDiggler/rpg-api/internal/pkg/idgen"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

// The store keys the orchestrator's repositories use
// (internal/orchestrators/session/redis_repos.go); the harness reads the
// same bytes they read.
const (
	measureSessionKeyPrefix = "session:v1alpha1:"
	measureEncKeyPrefix     = "session-enc:v1alpha1:"
	measureSiteKeyPrefix    = "session-site:v1alpha1:"
	measureScratchKey       = "measure:reload:scratch"
	// measureScratchEncounter is the encounter id the fenced-save row writes
	// (rpg-project#879 S12a): its blob and checkpoint version, never the
	// run's own.
	measureScratchEncounter = "measure-reload-scratch"
)

// reloadRow is one cold-reload measurement over n rebuilds of one session.
type reloadRow struct {
	key, mode                         string
	n                                 int
	sessionBytes, encBytes, outBytes  int
	sessionFetch, sessionDecode       []time.Duration
	fetch, decode, repoGet, siteBuild []time.Duration
	readLoad, attach                  []time.Duration
	toData, encode, persist           []time.Duration
	fencedGet, fencedSave             []time.Duration
	pieceBytes                        int
	pieceGet, coldBuild               []time.Duration
	coldMove, warmMove                []time.Duration
}

// measureColdReload rebuilds run's encounter from the store n times, by
// stage. It runs after the walk, outside any verb, and writes only the
// scratch key.
func measureColdReload(
	t *testing.T, client *goredis.Client, orch *sessionorch.Orchestrator, run, member, key, mode string, n int,
) reloadRow {
	t.Helper()
	ctx := context.Background()
	encRepo := sessionorch.NewEncounterRepository(client, 0)
	fenced, ok := encRepo.(sdk.FencedEncounterRepository)
	require.True(t, ok, "the Redis encounter repository carries the fenced capability")
	var scratchVersion uint64
	r := reloadRow{key: key, mode: mode, n: n}

	// One untimed pass so the first measured rebuild is not the process's
	// first touch of these code paths.
	_, err := orch.Manager.Where(ctx, &sdk.WhereInput{Session: run, Member: member})
	require.NoError(t, err)

	for range n {
		// fetch + decode: the session record.
		s := time.Now()
		sessRaw, err := client.Get(ctx, measureSessionKeyPrefix+run).Bytes()
		r.sessionFetch = append(r.sessionFetch, time.Since(s))
		require.NoError(t, err)
		var sess sdk.SessionData
		s = time.Now()
		require.NoError(t, json.Unmarshal(sessRaw, &sess))
		r.sessionDecode = append(r.sessionDecode, time.Since(s))
		r.sessionBytes = len(sessRaw)

		// fetch: the encounter blob.
		s = time.Now()
		raw, err := client.Get(ctx, measureEncKeyPrefix+sess.Encounter).Bytes()
		fetch := time.Since(s)
		require.NoError(t, err)
		r.fetch = append(r.fetch, fetch)
		r.encBytes = len(raw)

		// decode: bytes into the composition's data type.
		var data tkencounter.EncounterData
		s = time.Now()
		require.NoError(t, json.Unmarshal(raw, &data))
		decode := time.Since(s)
		r.decode = append(r.decode, decode)

		// The repository's own read (GET + decode in one), as a cross-check.
		s = time.Now()
		_, err = encRepo.GetEncounter(ctx, sess.Encounter)
		r.repoGet = append(r.repoGet, time.Since(s))
		require.NoError(t, err)

		// The fenced read the session takes: blob and version in one MGET,
		// plus decode.
		s = time.Now()
		_, err = fenced.GetFencedEncounter(ctx, &sdk.GetFencedEncounterInput{Encounter: sess.Encounter})
		r.fencedGet = append(r.fencedGet, time.Since(s))
		require.NoError(t, err)

		// site build: the encounter's load with no session seam, on the
		// orchestrator's (warm) site provider.
		s = time.Now()
		enc, err := tkencounter.LoadEncounter(&tkencounter.LoadEncounterInput{
			Data: data, Capabilities: tkencounter.RefusingCapabilities(), Sites: orch.Sites,
		})
		build := time.Since(s)
		require.NoError(t, err)
		r.siteBuild = append(r.siteBuild, build)

		// The stored site piece, and the load a cold process pays: a fresh
		// provider, handed the piece, compiles the site once.
		siteStore, ok := encRepo.(sdk.SiteRepository)
		require.True(t, ok, "the Redis encounter repository carries the site capability")
		s = time.Now()
		piece, err := siteStore.GetSite(ctx, &sdk.GetSiteInput{Hash: data.SiteHash})
		r.pieceGet = append(r.pieceGet, time.Since(s))
		require.NoError(t, err)
		pieceLen, err := client.StrLen(ctx, measureSiteKeyPrefix+string(data.SiteHash)).Result()
		require.NoError(t, err)
		r.pieceBytes = int(pieceLen)
		coldSites, err := sessionorch.NewSites(nil)
		require.NoError(t, err)
		s = time.Now()
		_, err = tkencounter.LoadEncounter(&tkencounter.LoadEncounterInput{
			Data: data, Capabilities: tkencounter.RefusingCapabilities(), Sites: coldSites, Site: piece.Data,
		})
		r.coldBuild = append(r.coldBuild, time.Since(s))
		require.NoError(t, err)

		// write: ToData, encode, persist (scratch key).
		s = time.Now()
		out := enc.ToData()
		r.toData = append(r.toData, time.Since(s))
		s = time.Now()
		b, err := json.Marshal(&out)
		r.encode = append(r.encode, time.Since(s))
		require.NoError(t, err)
		r.outBytes = len(b)
		s = time.Now()
		require.NoError(t, client.Set(ctx, measureScratchKey, b, 0).Err())
		r.persist = append(r.persist, time.Since(s))

		// The fenced save the session takes, on a scratch encounter id:
		// encode plus the one script (compare with 5b + 5c).
		s = time.Now()
		saved, err := fenced.SaveFenced(ctx, &sdk.SaveFencedInput{
			Encounter: measureScratchEncounter, Data: &out, Expected: scratchVersion,
		})
		r.fencedSave = append(r.fencedSave, time.Since(s))
		require.NoError(t, err)
		scratchVersion = saved.Version

		// read load: the session's whole read-path reload, and the attach
		// residual (what is left after both fetches, both decodes and the
		// bare build).
		s = time.Now()
		_, err = orch.Manager.Where(ctx, &sdk.WhereInput{Session: run, Member: member})
		whole := time.Since(s)
		require.NoError(t, err)
		r.readLoad = append(r.readLoad, whole)
		sessCost := r.sessionFetch[len(r.sessionFetch)-1] + r.sessionDecode[len(r.sessionDecode)-1]
		r.attach = append(r.attach, whole-sessCost-fetch-decode-build)
	}
	require.NoError(t, client.Del(ctx, measureScratchKey,
		measureEncKeyPrefix+measureScratchEncounter, "session-enc-ver:v1alpha1:"+measureScratchEncounter).Err())
	return r
}

// reloadTable renders the reload rows beside the walk's Move totals, p50 and
// p90.
func reloadTable(rows []reloadRow, moves map[string][2][]time.Duration) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\ncold reload by stage (p50 / p90 ms)\n")
	fmt.Fprintf(&b, "| keep | mode | n | stage | bytes | p50 | p90 |\n|---|---|---|---|---|---|---|\n")
	for _, r := range rows {
		line := func(stage string, bytes int, d []time.Duration) {
			bs := "-"
			if bytes > 0 {
				bs = fmt.Sprint(bytes)
			}
			fmt.Fprintf(&b, "| %s | %s | %d | %s | %s | %s | %s |\n",
				r.key, r.mode, r.n, stage, bs, ms(pct(d, 50)), ms(pct(d, 90)))
		}
		line("session record fetch", r.sessionBytes, r.sessionFetch)
		line("session record decode", 0, r.sessionDecode)
		line("1 fetch (enc blob GET)", r.encBytes, r.fetch)
		line("2 decode (JSON → EncounterData)", 0, r.decode)
		line("1+2 repository GetEncounter", 0, r.repoGet)
		line("3 site build (LoadEncounter, refusing caps, warm provider)", 0, r.siteBuild)
		line("3p site piece GetSite (GET + decode)", r.pieceBytes, r.pieceGet)
		line("3c cold site build (LoadEncounter, fresh provider + piece)", 0, r.coldBuild)
		line("4 attach (read load − record − 1 − 2 − 3)", 0, r.attach)
		line("read load whole (Manager.Where)", 0, r.readLoad)
		line("5a ToData", 0, r.toData)
		line("5b encode (JSON)", r.outBytes, r.encode)
		line("5c persist (SET)", r.outBytes, r.persist)
		line("1+2 fenced GetFencedEncounter (MGET + decode)", 0, r.fencedGet)
		line("5b+5d fenced SaveFenced (encode + script)", r.outBytes, r.fencedSave)
		if m, ok := moves[r.key+"|"+r.mode]; ok {
			line("Move answer (walk above)", 0, m[0])
			line("Move drained (walk above)", 0, m[1])
		}
		if len(r.coldMove) > 0 {
			line("Move whole, cold provider (fresh process, first verb)", 0, r.coldMove)
			line("Move whole, warm provider (same process, next verb)", 0, r.warmMove)
		}
	}
	return b.String()
}

// measureColdMove times n pairs of whole Moves on run: the first on a fresh
// orchestrator with a fresh site provider (a cold process: the site comes
// from the stored piece and compiles inside the verb), the second on the same
// orchestrator right after (the provider warm). Both answer whole, so each
// covers the load, the walk and the save. The walk goes back and forth along
// the axial line from where the member stands; position is read after each
// timed Move, never before, so nothing warms the provider ahead of the verb.
func measureColdMove(
	t *testing.T, client *goredis.Client, orch *sessionorch.Orchestrator, chars characterrepo.Repository,
	run, member string, n, pathLen int, r *reloadRow,
) {
	t.Helper()
	ctx := auth.WithPlayerID(context.Background(), "player-"+member)
	where, err := orch.Manager.Where(context.Background(), &sdk.WhereInput{Session: run, Member: member})
	require.NoError(t, err)
	x, y := int(where.Position.X), int(where.Position.Y)
	dir := 1
	move := func(h *sessionhandler.Handler, m *sdk.Manager) time.Duration {
		var took time.Duration
		for attempt := 0; ; attempt++ {
			require.Less(t, attempt, 2, "no direction walks whole from (%d,%d)", x, y)
			p := make([]*sessionpb.Position, 0, pathLen)
			for i := 1; i <= pathLen; i++ {
				p = append(p, &sessionpb.Position{X: float64(x + dir*i), Y: float64(y)})
			}
			s := time.Now()
			_, err := h.Move(ctx, &sessionpb.MoveRequest{Session: run, Member: member, Path: p})
			took = time.Since(s)
			dir = -dir
			if err != nil {
				continue
			}
			got, err := m.Where(context.Background(), &sdk.WhereInput{Session: run, Member: member})
			require.NoError(t, err)
			nx, ny := int(got.Position.X), int(got.Position.Y)
			end := p[len(p)-1]
			landed := nx == int(end.GetX()) && ny == int(end.GetY())
			x, y = nx, ny
			if landed {
				return took
			}
		}
	}
	for range n {
		sites, err := sessionorch.NewSites(nil)
		require.NoError(t, err)
		fresh, err := sessionorch.New(sessionorch.Config{
			Redis: client, Characters: chars, TTL: 24 * time.Hour, Dice: testDice{},
			PresentationIDs: idgen.NewSequential("presentation"), AnswerWhole: true, Sites: sites,
		})
		require.NoError(t, err)
		h, err := sessionhandler.New(&sessionhandler.HandlerConfig{
			Manager: fresh.Manager, Characters: chars, Preferences: fresh.Preferences,
		})
		require.NoError(t, err)
		before := sites.Compiles()
		r.coldMove = append(r.coldMove, move(h, fresh.Manager))
		require.Equal(t, before+1, sites.Compiles(), "the cold Move compiled the site from the piece")
		r.warmMove = append(r.warmMove, move(h, fresh.Manager))
	}
}
