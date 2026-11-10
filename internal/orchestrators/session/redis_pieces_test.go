package session_test

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	redisclient "github.com/KirkDiggler/rpg-api/internal/redis"
)

// The keys the rest's pieces live under (redis_pieces.go), spelled out so a
// renamed constant is a failing test rather than a silent move.
const (
	rosterKey      = "session-enc-roster:v1alpha3:"
	knowKey        = "session-enc-know:v1alpha3:"
	journalKey     = "session-enc-journal:v1alpha3:"
	appendedRest   = "session-enc:v1alpha2:"
	piecesEncID    = "enc-1"
	memberA        = tkencounter.MemberID("member-a")
	memberB        = tkencounter.MemberID("member-b")
	observerFieldA = "o:member|member-a"
)

// PiecesRepoSuite pins the Redis encounter repository's pieces capability
// (rpg-project#897 leg b): the head, the roster and knowledge hashes and the
// journal, each value prefixed with its mark; a save writes only the pieces
// it carries and drops the removed ones; a stale or refused save writes
// nothing; every piece shares the checkpoint's TTL; and a load joins the
// pieces, returns their marks, and refuses a missing or bad piece by name.
type PiecesRepoSuite struct {
	suite.Suite
	ctx    context.Context
	mr     *miniredis.Miniredis
	client redisclient.Client
	plain  sdk.EncounterRepository
	repo   sdk.PiecesEncounterRepository
}

func TestPiecesRepoSuite(t *testing.T) { suite.Run(t, new(PiecesRepoSuite)) }

func (s *PiecesRepoSuite) SetupTest() {
	s.ctx = context.Background()
	s.mr = miniredis.RunT(s.T())
	s.client = goredis.NewClient(&goredis.Options{Addr: s.mr.Addr()})
	s.plain = sessionorch.NewEncounterRepository(s.client, fencedTTL)
	repo, ok := s.plain.(sdk.PiecesEncounterRepository)
	s.Require().True(ok, "the Redis encounter repository must carry the pieces capability")
	s.repo = repo
}

func (s *PiecesRepoSuite) TearDownTest() { _ = s.client.Close() }

// rest is a rest with two members who each sweep their sight (so two
// observer pieces), a journal and a world round of round.
func rest(round int, members ...tkencounter.MemberID) *tkencounter.EncounterData {
	w := &tkencounter.EncounterData{
		SiteHash: "site", Retention: tkencounter.RetentionUnbounded,
		Members: []tkencounter.MemberData{}, SightSweeps: map[tkencounter.MemberID]string{},
	}
	w.Clock.HighWater = round
	for _, id := range members {
		w.Members = append(w.Members, tkencounter.MemberData{ID: id, Cell: &tkencounter.PositionData{X: float64(round)}})
		w.EverMembers = append(w.EverMembers, id)
		w.SightSweeps[id] = "sweep-" + string(id)
	}
	return w
}

// split is the rest's pieces against held, as the session splits a save.
func (s *PiecesRepoSuite) split(data *tkencounter.EncounterData, held map[tkencounter.PieceKey]tkencounter.Mark) *tkencounter.SplitPiecesOutput {
	out, err := tkencounter.SplitPieces(&tkencounter.SplitPiecesInput{Data: data, Held: held})
	s.Require().NoError(err)
	return out
}

// save sends a split on expected, with an empty record (since 1).
func (s *PiecesRepoSuite) save(split *tkencounter.SplitPiecesOutput, expected uint64) (*sdk.SaveFencedOutput, error) {
	in := &sdk.SavePiecesInput{Pieces: split.Pieces, Removed: split.Removed, Marks: split.Marks}
	in.Encounter, in.Expected, in.Since = piecesEncID, expected, 1
	return s.repo.SavePieces(s.ctx, in)
}

// launch saves every piece of data at version 0 and returns the marks.
func (s *PiecesRepoSuite) launch(data *tkencounter.EncounterData) map[tkencounter.PieceKey]tkencounter.Mark {
	split := s.split(data, nil)
	out, err := s.save(split, 0)
	s.Require().NoError(err)
	s.Equal(uint64(1), out.Version)
	return split.Marks
}

func (s *PiecesRepoSuite) loaded() *sdk.GetFencedEncounterOutput {
	got, err := s.repo.GetFencedEncounter(s.ctx, &sdk.GetFencedEncounterInput{Encounter: piecesEncID})
	s.Require().NoError(err)
	return got
}

// requireLoads holds the store's read to want, byte for byte.
func (s *PiecesRepoSuite) requireLoads(want *tkencounter.EncounterData) {
	wantJSON, err := json.Marshal(want)
	s.Require().NoError(err)
	gotJSON, err := json.Marshal(s.loaded().Data)
	s.Require().NoError(err)
	s.Equal(string(wantJSON), string(gotJSON))
}

// keys is every checkpoint key of the suite's encounter.
func keys() []string {
	return []string{
		restKey + piecesEncID, verKey + piecesEncID, logKey + piecesEncID, floorKey + piecesEncID,
		rosterKey + piecesEncID, knowKey + piecesEncID, journalKey + piecesEncID,
	}
}

// snapshot is everything the store holds for the encounter, by key.
func (s *PiecesRepoSuite) snapshot() map[string]string {
	out := map[string]string{}
	for _, key := range keys() {
		if !s.mr.Exists(key) {
			continue
		}
		switch key {
		case rosterKey + piecesEncID, knowKey + piecesEncID:
			fields, err := s.client.HGetAll(s.ctx, key).Result()
			s.Require().NoError(err)
			raw, err := json.Marshal(fields)
			s.Require().NoError(err)
			out[key] = string(raw)
		case logKey + piecesEncID:
			elems, err := s.client.LRange(s.ctx, key, 0, -1).Result()
			s.Require().NoError(err)
			out[key] = strings.Join(elems, "|")
		default:
			out[key] = mustGet(s.mr, key)
		}
	}
	return out
}

// TestLaunch_WritesEveryPieceWithItsMark: the first save writes the head,
// one roster field per member beside "_", one knowledge field per observer
// beside "_", and the journal, each value its mark and a newline before its
// bytes; the load is the rest and returns the marks the save named.
func (s *PiecesRepoSuite) TestLaunch_WritesEveryPieceWithItsMark() {
	data := rest(1, memberA, memberB)
	split := s.split(data, nil)
	_, err := s.save(split, 0)
	s.Require().NoError(err)

	roster, err := s.client.HGetAll(s.ctx, rosterKey+piecesEncID).Result()
	s.Require().NoError(err)
	s.ElementsMatch([]string{"_", "m:member-a", "m:member-b"}, mapKeys(roster))
	know, err := s.client.HGetAll(s.ctx, knowKey+piecesEncID).Result()
	s.Require().NoError(err)
	s.ElementsMatch([]string{"_", observerFieldA, "o:member|member-b"}, mapKeys(know))
	s.True(s.mr.Exists(journalKey + piecesEncID))

	member := split.Pieces[tkencounter.MemberPiece(memberA)]
	s.Equal(strconv.FormatUint(uint64(member.Mark), 10)+"\n"+string(member.Bytes), roster["m:member-a"],
		"a value is its mark, a newline, its bytes")
	head := split.Pieces[tkencounter.PieceHead]
	s.Equal(strconv.FormatUint(uint64(head.Mark), 10)+"\n"+string(head.Bytes), mustGet(s.mr, restKey+piecesEncID))

	got := s.loaded()
	s.Equal(uint64(1), got.Version)
	s.Equal(split.Marks, got.Marks, "the load returns the marks stored beside the pieces")
	s.requireLoads(data)
}

// TestAMove_WritesOnlyTheChangedFields is the leg's headline: against the
// launch's marks, a save where one member moved writes the head and that
// member's field and nothing else. Every field it does not carry is poisoned
// first and still poisoned after.
func (s *PiecesRepoSuite) TestAMove_WritesOnlyTheChangedFields() {
	data := rest(1, memberA, memberB)
	marks := s.launch(data)
	data.Members[0].Cell.X = 9
	split := s.split(data, marks)
	s.Require().Len(split.Pieces, 2, "the head and member-a")

	const poison = "untouched"
	untouched := map[string]string{
		rosterKey + piecesEncID: "m:member-b", knowKey + piecesEncID: observerFieldA,
	}
	before := map[string]string{}
	for key, field := range untouched {
		v, err := s.client.HGet(s.ctx, key, field).Result()
		s.Require().NoError(err)
		before[key] = v
		s.Require().NoError(s.client.HSet(s.ctx, key, field, poison).Err())
	}
	journal := mustGet(s.mr, journalKey+piecesEncID)
	s.Require().NoError(s.mr.Set(journalKey+piecesEncID, poison))

	out, err := s.save(split, 1)
	s.Require().NoError(err)
	s.Equal(uint64(2), out.Version)
	for key, field := range untouched {
		v, getErr := s.client.HGet(s.ctx, key, field).Result()
		s.Require().NoError(getErr)
		s.Equal(poison, v, "%s %s was not written", key, field)
		s.Require().NoError(s.client.HSet(s.ctx, key, field, before[key]).Err())
	}
	s.Equal(poison, mustGet(s.mr, journalKey+piecesEncID), "the journal was not written")
	s.Require().NoError(s.mr.Set(journalKey+piecesEncID, journal))

	moved, err := s.client.HGet(s.ctx, rosterKey+piecesEncID, "m:member-a").Result()
	s.Require().NoError(err)
	s.Contains(moved, `"x":9`, "the moved member was written")
	got := s.loaded()
	s.Equal(split.Marks, got.Marks)
	s.requireLoads(data)
}

// TestARemovedMember_IsDropped: a member who left is named in Removed, its
// roster field and its observer field are dropped, and the load no longer
// holds it.
func (s *PiecesRepoSuite) TestARemovedMember_IsDropped() {
	marks := s.launch(rest(1, memberA, memberB))
	after := rest(1, memberA)
	after.EverMembers = append(after.EverMembers, memberB)
	split := s.split(after, marks)
	s.ElementsMatch([]tkencounter.PieceKey{tkencounter.MemberPiece(memberB), "o:member|member-b"}, split.Removed)

	_, err := s.save(split, 1)
	s.Require().NoError(err)

	roster, err := s.client.HGetAll(s.ctx, rosterKey+piecesEncID).Result()
	s.Require().NoError(err)
	s.ElementsMatch([]string{"_", "m:member-a"}, mapKeys(roster))
	know, err := s.client.HGetAll(s.ctx, knowKey+piecesEncID).Result()
	s.Require().NoError(err)
	s.ElementsMatch([]string{"_", observerFieldA}, mapKeys(know))
	s.requireLoads(after)
}

// TestAWholeSave_ReplacesTheHashes: a save that carries every piece (the
// whole-world paths, SaveFenced and SaveAppended) replaces both hashes, so a
// member the new world lacks cannot linger though nothing names it removed.
func (s *PiecesRepoSuite) TestAWholeSave_ReplacesTheHashes() {
	s.launch(rest(1, memberA, memberB))
	after := rest(2, memberA)
	out, err := s.repo.SaveFenced(s.ctx, &sdk.SaveFencedInput{Encounter: piecesEncID, Data: after, Expected: 1})
	s.Require().NoError(err)
	s.Equal(uint64(2), out.Version)

	roster, err := s.client.HGetAll(s.ctx, rosterKey+piecesEncID).Result()
	s.Require().NoError(err)
	s.ElementsMatch([]string{"_", "m:member-a"}, mapKeys(roster))
	s.requireLoads(after)
}

// TestStale_WritesNothing: a save on a version the store has moved past
// leaves every key as it was: head, both hashes, journal, list, floor and
// version.
func (s *PiecesRepoSuite) TestStale_WritesNothing() {
	data := rest(1, memberA, memberB)
	marks := s.launch(data)
	before := s.snapshot()
	data.Members[0].Cell.X = 9
	data.Members = data.Members[:1]

	_, err := s.save(s.split(data, marks), 7)

	s.Require().ErrorIs(err, sdk.ErrStaleCheckpoint)
	s.Equal(before, s.snapshot(), "nothing was written")
}

// TestARefusedSave_LeavesTheStoreLoadable: a save the script refuses after
// its fence check (a sheet that does not exist) writes nothing, and the store
// still loads the world it held.
func (s *PiecesRepoSuite) TestARefusedSave_LeavesTheStoreLoadable() {
	data := rest(1, memberA, memberB)
	marks := s.launch(data)
	before := s.snapshot()
	moved := rest(2, memberA, memberB)
	split := s.split(moved, marks)
	in := &sdk.SavePiecesInput{Pieces: split.Pieces, Removed: split.Removed, Marks: split.Marks}
	in.Encounter, in.Expected, in.Since = piecesEncID, 1, 1
	in.Characters = []*character.Data{{ID: "ghost"}}

	_, err := s.repo.SavePieces(s.ctx, in)

	s.Require().ErrorIs(err, sdk.ErrNotFound)
	s.Equal(before, s.snapshot(), "nothing was written")
	s.requireLoads(data)
}

// TestAFirstSave_MustCarryEveryPiece: a first save split against marks the
// store does not hold carries the head alone; the script refuses it before
// any write rather than leave a head with no roster.
func (s *PiecesRepoSuite) TestAFirstSave_MustCarryEveryPiece() {
	data := rest(1, memberA)
	marks := s.split(data, nil).Marks
	data.Clock.HighWater = 2

	_, err := s.save(s.split(data, marks), 0)

	s.Require().ErrorIs(err, sdk.ErrBadRepository)
	s.Empty(s.snapshot(), "nothing was written")
}

// TestAMissingPiece_IsRefusedByName: a head whose roster, knowledge or
// journal is gone fails the read naming the piece, never loads a world short
// a shape.
func (s *PiecesRepoSuite) TestAMissingPiece_IsRefusedByName() {
	for _, tc := range []struct {
		name string
		drop func()
	}{
		{"journal", func() { s.mr.Del(journalKey + piecesEncID) }},
		{"roster", func() { s.mr.HDel(rosterKey+piecesEncID, "_") }},
		{"knowledge", func() { s.mr.HDel(knowKey+piecesEncID, "_") }},
	} {
		s.mr.FlushAll()
		s.launch(rest(1, memberA))
		tc.drop()

		_, err := s.repo.GetFencedEncounter(s.ctx, &sdk.GetFencedEncounterInput{Encounter: piecesEncID})
		s.Require().ErrorIs(err, sessionorch.ErrCheckpointMismatch, tc.name)
		s.ErrorIs(err, tkencounter.ErrMissingPiece, tc.name)
		s.ErrorIs(err, sdk.ErrBadRepository, tc.name)
		s.ErrorContains(err, tc.name, "the error names the piece")
		_, err = s.plain.GetEncounter(s.ctx, piecesEncID)
		s.ErrorIs(err, sessionorch.ErrCheckpointMismatch, tc.name)
	}
}

// TestABadPiece_IsRefusedByName: a value without a mark, a value that does
// not decode and a field that names no piece each fail the read by name.
func (s *PiecesRepoSuite) TestABadPiece_IsRefusedByName() {
	for _, tc := range []struct {
		name, field, value string
	}{
		{"no mark", "m:member-a", "{}"},
		{"undecodable", "m:member-a", "7\n{"},
		{"names no piece", "x:stray", "7\n{}"},
	} {
		s.mr.FlushAll()
		s.launch(rest(1, memberA))
		s.mr.HSet(rosterKey+piecesEncID, tc.field, tc.value)

		_, err := s.repo.GetFencedEncounter(s.ctx, &sdk.GetFencedEncounterInput{Encounter: piecesEncID})
		s.Require().ErrorIs(err, sessionorch.ErrCheckpointMismatch, tc.name)
		s.ErrorContains(err, tc.field, "%s: the error names the field", tc.name)
	}
}

// TestPiecesWithNoHead_AreNotAMissingRun: pieces left behind by a head that
// is gone are a disagreement, never reported as a run that does not exist.
func (s *PiecesRepoSuite) TestPiecesWithNoHead_AreNotAMissingRun() {
	s.launch(rest(1, memberA))
	s.mr.Del(restKey + piecesEncID)

	_, err := s.repo.GetFencedEncounter(s.ctx, &sdk.GetFencedEncounterInput{Encounter: piecesEncID})
	s.Require().ErrorIs(err, sessionorch.ErrCheckpointMismatch)
	s.NotErrorIs(err, sdk.ErrNotFound)
	s.ErrorContains(err, string(tkencounter.PieceHead))
}

// TestAnEarlierShape_IsRefusedByName: a leg (c) rest under v1alpha2, or a
// single blob under v1alpha1, with no head under v1alpha3, is refused by
// name; neither is reported missing.
func (s *PiecesRepoSuite) TestAnEarlierShape_IsRefusedByName() {
	for _, key := range []string{appendedRest, legacyKey} {
		s.mr.FlushAll()
		b, err := json.Marshal(rest(1, memberA))
		s.Require().NoError(err)
		s.Require().NoError(s.mr.Set(key+piecesEncID, string(b)))
		s.Require().NoError(s.mr.Set(verKey+piecesEncID, "4"))

		_, err = s.repo.GetFencedEncounter(s.ctx, &sdk.GetFencedEncounterInput{Encounter: piecesEncID})
		s.Require().ErrorIs(err, sessionorch.ErrCheckpointBeforePieces, key)
		s.NotErrorIs(err, sdk.ErrNotFound, key)
		_, err = s.plain.GetEncounter(s.ctx, piecesEncID)
		s.Require().ErrorIs(err, sessionorch.ErrCheckpointBeforePieces, key)
	}
}

// TestEveryKey_SharesTheCheckpointsTTL: a save that writes the head alone
// still renews every piece's TTL with the head's, so the hashes and the
// journal never expire before the head that names them; past the TTL every
// key is gone together. TTL 0 persists every key.
func (s *PiecesRepoSuite) TestEveryKey_SharesTheCheckpointsTTL() {
	data := rest(1, memberA)
	marks := s.launch(data)
	s.mr.FastForward(time.Hour)
	data.Clock.HighWater = 2
	split := s.split(data, marks)
	s.Require().Len(split.Pieces, 1, "the head alone")
	_, err := s.save(split, 1)
	s.Require().NoError(err)
	for _, key := range keys() {
		if key == logKey+piecesEncID {
			continue // an empty record stores no list
		}
		s.Equal(fencedTTL, s.mr.TTL(key), key)
	}
	s.mr.FastForward(fencedTTL + time.Second)
	s.Empty(s.snapshot(), "every key expired together")

	persist := sessionorch.NewEncounterRepository(s.client, 0).(sdk.PiecesEncounterRepository)
	split = s.split(data, nil)
	in := &sdk.SavePiecesInput{Pieces: split.Pieces, Marks: split.Marks}
	in.Encounter, in.Since = piecesEncID, 1
	_, err = persist.SavePieces(s.ctx, in)
	s.Require().NoError(err)
	for _, key := range keys() {
		s.Equal(time.Duration(0), s.mr.TTL(key), key)
	}
}

// TestThePlainSave_WritesTheSameShape: SaveEncounter (a host without the
// fence) writes every piece under the same keys, and the fenced read loads
// it with its marks.
func (s *PiecesRepoSuite) TestThePlainSave_WritesTheSameShape() {
	data := rest(1, memberA, memberB)
	s.Require().NoError(s.plain.SaveEncounter(s.ctx, piecesEncID, data))

	roster, err := s.client.HGetAll(s.ctx, rosterKey+piecesEncID).Result()
	s.Require().NoError(err)
	s.ElementsMatch([]string{"_", "m:member-a", "m:member-b"}, mapKeys(roster))
	got := s.loaded()
	s.Equal(uint64(1), got.Version)
	s.Equal(s.split(data, nil).Marks, got.Marks)
	s.requireLoads(data)
}

// TestSavePiecesWithoutMarks_IsRefused: a pieces save must name the marks
// the session will hold; without them the store cannot tell a whole save
// from a partial one.
func (s *PiecesRepoSuite) TestSavePiecesWithoutMarks_IsRefused() {
	split := s.split(rest(1, memberA), nil)
	in := &sdk.SavePiecesInput{Pieces: split.Pieces}
	in.Encounter, in.Since = piecesEncID, 1

	_, err := s.repo.SavePieces(s.ctx, in)

	s.Require().ErrorIs(err, sdk.ErrBadRepository)
	s.Empty(s.snapshot())
}

// mapKeys is a hash's fields.
func mapKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
