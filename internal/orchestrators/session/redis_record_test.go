package session_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-toolkit/core"
	"github.com/KirkDiggler/rpg-toolkit/play/record"
	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	redisclient "github.com/KirkDiggler/rpg-api/internal/redis"
)

// The keys the appended record lives under (redis_record.go), spelled out so
// a renamed constant is a failing test rather than a silent move.
const (
	restKey   = "session-enc:v1alpha2:"
	logKey    = "session-enc-log:v1alpha2:"
	verKey    = "session-enc-ver:v1alpha1:"
	legacyKey = "session-enc:v1alpha1:"
)

// AppendedRecordRepoSuite pins the Redis encounter repository's appending
// capability (rpg-project#897 leg c): the rest replaced, one segment appended
// per save that wrote beats, segments wholly below the floor dropped, the
// read joined whole, and a checkpoint under the old single-blob key refused
// by name.
type AppendedRecordRepoSuite struct {
	suite.Suite
	ctx       context.Context
	mr        *miniredis.Miniredis
	client    redisclient.Client
	plain     sdk.EncounterRepository
	appending sdk.AppendingEncounterRepository
	log       *record.Log
	ticks     uint64
}

func TestAppendedRecordRepoSuite(t *testing.T) { suite.Run(t, new(AppendedRecordRepoSuite)) }

func (s *AppendedRecordRepoSuite) SetupTest() {
	s.ctx = context.Background()
	s.mr = miniredis.RunT(s.T())
	s.client = goredis.NewClient(&goredis.Options{Addr: s.mr.Addr()})
	s.plain = sessionorch.NewEncounterRepository(s.client, fencedTTL)
	appending, ok := s.plain.(sdk.AppendingEncounterRepository)
	s.Require().True(ok, "the Redis encounter repository must carry the appending capability")
	s.appending = appending
	log, err := record.NewLog()
	s.Require().NoError(err)
	s.log = log
	s.ticks = 0
}

func (s *AppendedRecordRepoSuite) TearDownTest() { _ = s.client.Close() }

// beats appends n entries to the suite's record, alternating two audiences
// and the empty one, so every segment carries a table of its own.
func (s *AppendedRecordRepoSuite) beats(n int) {
	audiences := [][]core.EntityID{{"alice"}, {"alice", "bob"}, nil}
	for i := 0; i < n; i++ {
		s.ticks++
		_, err := s.log.Append(&record.AppendInput{
			At:       s.ticks,
			Audience: audiences[int(s.ticks)%len(audiences)],
			Tags:     map[string]string{"kind": "beat"},
			Payload:  []byte(fmt.Sprintf(`{"n":%d}`, s.ticks)),
		})
		s.Require().NoError(err)
	}
}

func (s *AppendedRecordRepoSuite) next() uint64 {
	next, err := s.log.NextSeq()
	s.Require().NoError(err)
	return next
}

// rest is the world with the record reduced to its head, as the encounter's
// ToCheckpoint writes it. Retention unbounded, as every launched run is.
func (s *AppendedRecordRepoSuite) rest() *tkencounter.EncounterData {
	w := populatedStoredEncounter()
	w.Retention = tkencounter.RetentionUnbounded
	w.Log = s.log.DataFrom(s.next())
	return w
}

// whole is the same world with the whole record, as ToData writes it.
func (s *AppendedRecordRepoSuite) whole() *tkencounter.EncounterData {
	w := populatedStoredEncounter()
	w.Retention = tkencounter.RetentionUnbounded
	w.Log = s.log.ToData()
	return w
}

// save appends the record from since on the version expected.
func (s *AppendedRecordRepoSuite) save(id string, since, floor, expected uint64) (*sdk.SaveFencedOutput, error) {
	return s.appending.SaveAppended(s.ctx, &sdk.SaveAppendedInput{
		SaveFencedInput: sdk.SaveFencedInput{Encounter: id, Data: s.rest(), Expected: expected},
		Appended:        s.log.DataFrom(since),
		Since:           since,
		Floor:           floor,
	})
}

func (s *AppendedRecordRepoSuite) loaded(id string) *sdk.GetFencedEncounterOutput {
	got, err := s.appending.GetFencedEncounter(s.ctx, &sdk.GetFencedEncounterInput{Encounter: id})
	s.Require().NoError(err)
	return got
}

func (s *AppendedRecordRepoSuite) llen(id string) int64 {
	n, err := s.client.LLen(s.ctx, logKey+id).Result()
	s.Require().NoError(err)
	return n
}

func (s *AppendedRecordRepoSuite) TestLaunchThenAppend_GrowsTheListByOne_AndLoadsWhole() {
	s.beats(5)
	out, err := s.save("enc-1", 1, 0, 0)
	s.Require().NoError(err)
	s.Equal(uint64(1), out.Version)
	s.Equal(int64(1), s.llen("enc-1"))

	since := s.next()
	s.beats(3)
	out, err = s.save("enc-1", since, 0, 1)
	s.Require().NoError(err)
	s.Equal(uint64(2), out.Version)
	s.Equal(int64(2), s.llen("enc-1"), "the second save appended one segment")

	got := s.loaded("enc-1")
	s.Equal(uint64(2), got.Version)
	s.Equal(s.log.ToData(), got.Data.Log, "the read joins the rest and segments into the whole record")

	plain, err := s.plain.GetEncounter(s.ctx, "enc-1")
	s.Require().NoError(err)
	s.Equal(s.log.ToData(), plain.Log, "the plain read is whole too")

	var stored tkencounter.EncounterData
	raw, err := s.client.Get(s.ctx, restKey+"enc-1").Bytes()
	s.Require().NoError(err)
	s.Require().NoError(json.Unmarshal(raw, &stored))
	s.Empty(stored.Log.Entries, "the rest key never carries the record")
}

func (s *AppendedRecordRepoSuite) TestBeatlessSave_AppendsNothing_AndStillMovesTheVersion() {
	s.beats(4)
	_, err := s.save("enc-1", 1, 0, 0)
	s.Require().NoError(err)

	out, err := s.save("enc-1", s.next(), 0, 1)
	s.Require().NoError(err)
	s.Equal(uint64(2), out.Version)
	s.Equal(int64(1), s.llen("enc-1"), "an empty segment is not stored")
	s.Equal(s.log.ToData(), s.loaded("enc-1").Data.Log)
}

func (s *AppendedRecordRepoSuite) TestStaleAppend_WritesNothing() {
	s.beats(4)
	_, err := s.save("enc-1", 1, 0, 0)
	s.Require().NoError(err)
	rest, err := s.client.Get(s.ctx, restKey+"enc-1").Result()
	s.Require().NoError(err)

	since := s.next()
	s.beats(2)
	_, err = s.save("enc-1", since, 0, 7)

	s.Require().ErrorIs(err, sdk.ErrStaleCheckpoint)
	after, err := s.client.Get(s.ctx, restKey+"enc-1").Result()
	s.Require().NoError(err)
	s.Equal(rest, after, "the rest is what it was")
	s.Equal(int64(1), s.llen("enc-1"), "no segment was appended")
	s.Equal("1", mustGet(s.mr, verKey+"enc-1"), "the version did not move")
}

func (s *AppendedRecordRepoSuite) TestFirstSave_MustStartTheRecordAtOne() {
	s.beats(4)
	_, err := s.save("enc-1", 3, 0, 0)

	s.Require().ErrorIs(err, sdk.ErrBadRepository)
	s.False(s.mr.Exists(restKey+"enc-1"), "nothing was written")
	s.False(s.mr.Exists(logKey + "enc-1"))
	s.False(s.mr.Exists(verKey + "enc-1"))
}

func (s *AppendedRecordRepoSuite) TestSinceOne_StartsTheRecordOver() {
	s.beats(3)
	_, err := s.save("enc-1", 1, 0, 0)
	s.Require().NoError(err)
	since := s.next()
	s.beats(3)
	_, err = s.save("enc-1", since, 0, 1)
	s.Require().NoError(err)
	s.Require().Equal(int64(2), s.llen("enc-1"))

	out, err := s.appending.SaveFenced(s.ctx, &sdk.SaveFencedInput{Encounter: "enc-1", Data: s.whole(), Expected: 2})
	s.Require().NoError(err)
	s.Equal(uint64(3), out.Version)
	s.Equal(int64(1), s.llen("enc-1"), "a whole-world save holds the record as one segment")
	s.Equal(s.log.ToData(), s.loaded("enc-1").Data.Log)
}

func (s *AppendedRecordRepoSuite) TestFloor_DropsSegmentsWhollyBelowIt() {
	s.beats(3) // 1-3
	_, err := s.save("enc-1", 1, 0, 0)
	s.Require().NoError(err)
	s.beats(3) // 4-6
	_, err = s.save("enc-1", 4, 0, 1)
	s.Require().NoError(err)
	s.beats(3) // 7-9
	_, err = s.save("enc-1", 7, 5, 2)
	s.Require().NoError(err)

	elems, err := s.client.LRange(s.ctx, logKey+"enc-1", 0, -1).Result()
	s.Require().NoError(err)
	s.Require().Len(elems, 2, "1-3 is wholly below 5 and dropped; 4-6 straddles it and stays")
	s.Regexp(`^6\n`, elems[0])
	s.Regexp(`^9\n`, elems[1])
}

func (s *AppendedRecordRepoSuite) TestTheRecordSharesTheCheckpointsLifetime() {
	s.beats(2)
	_, err := s.save("enc-1", 1, 0, 0)
	s.Require().NoError(err)
	s.Equal(fencedTTL, s.mr.TTL(restKey+"enc-1"))
	s.Equal(fencedTTL, s.mr.TTL(logKey+"enc-1"))
	s.Equal(fencedTTL, s.mr.TTL(verKey+"enc-1"))

	s.mr.FastForward(time.Hour)
	_, err = s.save("enc-1", s.next(), 0, 1)
	s.Require().NoError(err)
	s.Equal(fencedTTL, s.mr.TTL(logKey+"enc-1"), "a beatless save refreshes the record's TTL with the rest's")

	s.mr.FastForward(fencedTTL + time.Second)
	s.False(s.mr.Exists(restKey + "enc-1"))
	s.False(s.mr.Exists(logKey + "enc-1"))
	s.False(s.mr.Exists(verKey + "enc-1"))
}

func (s *AppendedRecordRepoSuite) TestZeroTTL_PersistsTheRecord() {
	repo := sessionorch.NewEncounterRepository(s.client, 0).(sdk.AppendingEncounterRepository)
	s.beats(2)
	_, err := repo.SaveAppended(s.ctx, &sdk.SaveAppendedInput{
		SaveFencedInput: sdk.SaveFencedInput{Encounter: "enc-1", Data: s.rest()},
		Appended:        s.log.DataFrom(1), Since: 1,
	})
	s.Require().NoError(err)
	s.Equal(time.Duration(0), s.mr.TTL(logKey+"enc-1"))
}

func (s *AppendedRecordRepoSuite) TestPlainSave_WritesTheSameShape() {
	s.beats(5)
	s.Require().NoError(s.plain.SaveEncounter(s.ctx, "enc-1", s.whole()))

	s.Equal(int64(1), s.llen("enc-1"))
	var stored tkencounter.EncounterData
	raw, err := s.client.Get(s.ctx, restKey+"enc-1").Bytes()
	s.Require().NoError(err)
	s.Require().NoError(json.Unmarshal(raw, &stored))
	s.Empty(stored.Log.Entries, "the plain path stores the rest, not the whole blob")
	got := s.loaded("enc-1")
	s.Equal(uint64(1), got.Version)
	s.Equal(s.log.ToData(), got.Data.Log)
}

func (s *AppendedRecordRepoSuite) TestCheckpointUnderTheOldKey_IsRefusedByName() {
	b, err := json.Marshal(s.whole())
	s.Require().NoError(err)
	s.Require().NoError(s.mr.Set(legacyKey+"enc-old", string(b)))
	s.Require().NoError(s.mr.Set(verKey+"enc-old", "4"))

	_, err = s.appending.GetFencedEncounter(s.ctx, &sdk.GetFencedEncounterInput{Encounter: "enc-old"})
	s.Require().ErrorIs(err, sessionorch.ErrCheckpointBeforeAppendedRecord)
	s.NotErrorIs(err, sdk.ErrNotFound, "a run that exists is never reported missing")

	_, err = s.plain.GetEncounter(s.ctx, "enc-old")
	s.Require().ErrorIs(err, sessionorch.ErrCheckpointBeforeAppendedRecord)
}

func (s *AppendedRecordRepoSuite) TestAGapInTheStoredRecord_FailsTheRead() {
	s.beats(3)
	_, err := s.save("enc-1", 1, 0, 0)
	s.Require().NoError(err)
	s.beats(3)
	// A save that skipped seqs 4-5: only 6 is sent, so the store holds 1-3, 6.
	_, err = s.save("enc-1", 6, 0, 1)
	s.Require().NoError(err)

	_, err = s.appending.GetFencedEncounter(s.ctx, &sdk.GetFencedEncounterInput{Encounter: "enc-1"})
	s.Require().ErrorIs(err, sdk.ErrBadRepository, "an unbounded record with a hole is refused, never loaded short")
}

// mustGet reads a string key from miniredis, "" when absent.
func mustGet(mr *miniredis.Miniredis, key string) string {
	v, err := mr.Get(key)
	if err != nil {
		return ""
	}
	return v
}
