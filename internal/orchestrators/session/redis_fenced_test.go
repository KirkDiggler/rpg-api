package session_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"

	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	"github.com/KirkDiggler/rpg-api/internal/entities"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	redisclient "github.com/KirkDiggler/rpg-api/internal/redis"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
)

// FencedRepoSuite pins the Redis encounter repository's fenced capability
// (rpg-project#879 S12a): the version is the store's, a save lands only on
// the version it loaded, and everything a save carries lands together.
type FencedRepoSuite struct {
	suite.Suite
	ctx      context.Context
	mr       *miniredis.Miniredis
	client   redisclient.Client
	fenced   sdk.FencedEncounterRepository
	plain    sdk.EncounterRepository
	sessions sdk.SessionRepository
	chars    sdk.CharacterRepository
	seats    sdk.SeatRepository
}

const fencedTTL = 24 * time.Hour

func TestFencedRepoSuite(t *testing.T) { suite.Run(t, new(FencedRepoSuite)) }

func (s *FencedRepoSuite) SetupTest() {
	s.ctx = context.Background()
	s.mr = miniredis.RunT(s.T())
	s.client = goredis.NewClient(&goredis.Options{Addr: s.mr.Addr()})
	s.plain = sessionorch.NewEncounterRepository(s.client, fencedTTL)
	fenced, ok := s.plain.(sdk.FencedEncounterRepository)
	s.Require().True(ok, "the Redis encounter repository must carry the fenced capability")
	s.fenced = fenced
	s.sessions = sessionorch.NewSessionRepository(s.client, fencedTTL)
	s.seats = sessionorch.NewSeatRepository(s.client)
	repo, err := characterrepo.NewRedis(&characterrepo.RedisConfig{Client: s.client})
	s.Require().NoError(err)
	s.chars = sessionorch.NewCharacterRepository(repo)
	for _, id := range []string{"hero-a", "hero-b"} {
		_, err := repo.Create(s.ctx, characterrepo.CreateInput{Character: &entities.Character{
			Data: &tkcharacter.Data{ID: id, PlayerID: "player-1", Name: id, HitPoints: 10, MaxHitPoints: 10},
		}})
		s.Require().NoError(err)
	}
}

func (s *FencedRepoSuite) TearDownTest() { _ = s.client.Close() }

func world(round int) *tkencounter.EncounterData {
	w := populatedStoredEncounter()
	w.Retention = round
	return w
}

func sheet(id string, hp int) *tkcharacter.Data {
	return &tkcharacter.Data{ID: id, PlayerID: "player-1", Name: id, HitPoints: hp, MaxHitPoints: 10}
}

func (s *FencedRepoSuite) launch(id string) uint64 {
	out, err := s.fenced.SaveFenced(s.ctx, &sdk.SaveFencedInput{Encounter: id, Data: world(1), Expected: 0})
	s.Require().NoError(err)
	s.Require().Equal(uint64(1), out.Version)
	return out.Version
}

func (s *FencedRepoSuite) TestTwoSavesFromOneLoad_SecondIsStale_FirstLandsWhole() {
	loaded := s.launch("enc-1")

	first := &sdk.SaveFencedInput{
		Encounter:  "enc-1",
		Data:       world(2),
		Session:    &sdk.SessionData{ID: "run-1", Encounter: "enc-1", Dungeon: "first"},
		Characters: []*tkcharacter.Data{sheet("hero-a", 7)},
		Seats:      []*sdk.SeatData{{Character: "hero-a", Session: "run-1"}},
		Expected:   loaded,
	}
	out, err := s.fenced.SaveFenced(s.ctx, first)
	s.Require().NoError(err)
	s.Equal(loaded+1, out.Version)

	late := &sdk.SaveFencedInput{
		Encounter:  "enc-1",
		Data:       world(99),
		Session:    &sdk.SessionData{ID: "run-1", Encounter: "enc-1", Dungeon: "late"},
		Characters: []*tkcharacter.Data{sheet("hero-a", 1), sheet("hero-b", 1)},
		Seats:      []*sdk.SeatData{{Character: "hero-b", Session: "run-1"}},
		Expected:   loaded,
	}
	_, err = s.fenced.SaveFenced(s.ctx, late)
	s.Require().ErrorIs(err, sdk.ErrStaleCheckpoint)

	got, err := s.fenced.GetFencedEncounter(s.ctx, &sdk.GetFencedEncounterInput{Encounter: "enc-1"})
	s.Require().NoError(err)
	s.Equal(loaded+1, got.Version, "the version advanced exactly once")
	s.Equal(2, got.Data.Retention, "the store holds the first save's world")

	sess, err := s.sessions.GetSession(s.ctx, "run-1")
	s.Require().NoError(err)
	s.Equal("first", sess.Dungeon)
	a, err := s.chars.GetCharacter(s.ctx, "hero-a")
	s.Require().NoError(err)
	s.Equal(7, a.HitPoints)
	b, err := s.chars.GetCharacter(s.ctx, "hero-b")
	s.Require().NoError(err)
	s.Equal(10, b.HitPoints, "the stale save's sheet did not land")
	_, err = s.seats.GetSeat(s.ctx, "hero-b")
	s.Require().ErrorIs(err, sdk.ErrNotFound, "the stale save's seat did not land")
}

func (s *FencedRepoSuite) TestSaveWithSessionAndTwoSheets_WritesEveryPieceAndTheVersion() {
	loaded := s.launch("enc-1")
	out, err := s.fenced.SaveFenced(s.ctx, &sdk.SaveFencedInput{
		Encounter:  "enc-1",
		Data:       world(5),
		Session:    &sdk.SessionData{ID: "run-1", Encounter: "enc-1"},
		Characters: []*tkcharacter.Data{sheet("hero-a", 3), sheet("hero-b", 4)},
		Seats:      []*sdk.SeatData{{Character: "hero-a", Session: "run-1"}, {Character: "hero-b", Session: "run-1"}},
		Expected:   loaded,
	})
	s.Require().NoError(err)
	s.Equal(uint64(2), out.Version)

	plainWorld, err := s.plain.GetEncounter(s.ctx, "enc-1")
	s.Require().NoError(err)
	s.Equal(5, plainWorld.Retention)
	_, err = s.sessions.GetSession(s.ctx, "run-1")
	s.Require().NoError(err)
	a, err := s.chars.GetCharacter(s.ctx, "hero-a")
	s.Require().NoError(err)
	s.Equal(3, a.HitPoints)
	b, err := s.chars.GetCharacter(s.ctx, "hero-b")
	s.Require().NoError(err)
	s.Equal(4, b.HitPoints)
	seat, err := s.seats.GetSeat(s.ctx, "hero-b")
	s.Require().NoError(err)
	s.Equal("run-1", seat.Session)

	// The sheet lands in the character repository's own shape, so its
	// other readers (player listing, the character service) still decode it.
	raw, err := s.client.Get(s.ctx, characterrepo.Key("hero-a")).Bytes()
	s.Require().NoError(err)
	var stored entities.Character
	s.Require().NoError(json.Unmarshal(raw, &stored))
	s.Equal(3, stored.Data.HitPoints)
	s.Equal(time.Duration(0), s.mr.TTL(characterrepo.Key("hero-a")), "sheets carry no TTL, as Update writes them")
}

func (s *FencedRepoSuite) TestGet_ReturnsTheVersionOfTheDataItReturns() {
	v1 := s.launch("enc-1")
	_, err := s.fenced.SaveFenced(s.ctx, &sdk.SaveFencedInput{Encounter: "enc-1", Data: world(2), Expected: v1})
	s.Require().NoError(err)

	got, err := s.fenced.GetFencedEncounter(s.ctx, &sdk.GetFencedEncounterInput{Encounter: "enc-1"})
	s.Require().NoError(err)
	s.Equal(uint64(2), got.Version)
	s.Equal(2, got.Data.Retention)
}

func (s *FencedRepoSuite) TestGet_Missing_IsNotFound() {
	_, err := s.fenced.GetFencedEncounter(s.ctx, &sdk.GetFencedEncounterInput{Encounter: "nope"})
	s.Require().ErrorIs(err, sdk.ErrNotFound)
}

func (s *FencedRepoSuite) TestBlobWithoutVersion_FailsClosed() {
	b, err := json.Marshal(world(1))
	s.Require().NoError(err)
	s.Require().NoError(s.mr.Set("session-enc:v1alpha1:enc-1", string(b)))

	_, err = s.fenced.GetFencedEncounter(s.ctx, &sdk.GetFencedEncounterInput{Encounter: "enc-1"})
	s.Require().ErrorIs(err, sdk.ErrBadRepository)

	_, err = s.fenced.SaveFenced(s.ctx, &sdk.SaveFencedInput{Encounter: "enc-1", Data: world(2), Expected: 0})
	s.Require().ErrorIs(err, sdk.ErrBadRepository)
	s.False(s.mr.Exists("session-enc-ver:v1alpha1:enc-1"))
}

func (s *FencedRepoSuite) TestLaunchOnAnOccupiedID_IsStale() {
	s.launch("enc-1")
	_, err := s.fenced.SaveFenced(s.ctx, &sdk.SaveFencedInput{Encounter: "enc-1", Data: world(9), Expected: 0})
	s.Require().ErrorIs(err, sdk.ErrStaleCheckpoint)
}

func (s *FencedRepoSuite) TestLingeringVersionWithoutBlob_DoesNotRefuseALaunch() {
	s.Require().NoError(s.mr.Set("session-enc-ver:v1alpha1:enc-1", "7"))
	out, err := s.fenced.SaveFenced(s.ctx, &sdk.SaveFencedInput{Encounter: "enc-1", Data: world(1), Expected: 0})
	s.Require().NoError(err)
	s.Equal(uint64(1), out.Version)
}

func (s *FencedRepoSuite) TestVersionSharesTheBlobsLifetime() {
	s.launch("enc-1")
	s.Equal(fencedTTL, s.mr.TTL("session-enc:v1alpha1:enc-1"))
	s.Equal(fencedTTL, s.mr.TTL("session-enc-ver:v1alpha1:enc-1"))

	s.mr.FastForward(fencedTTL + time.Second)
	s.False(s.mr.Exists("session-enc:v1alpha1:enc-1"))
	s.False(s.mr.Exists("session-enc-ver:v1alpha1:enc-1"))
	s.launch("enc-1")
}

func (s *FencedRepoSuite) TestSessionBlobKeepsTheSessionTTL() {
	loaded := s.launch("enc-1")
	_, err := s.fenced.SaveFenced(s.ctx, &sdk.SaveFencedInput{
		Encounter: "enc-1", Data: world(2), Session: &sdk.SessionData{ID: "run-1", Encounter: "enc-1"}, Expected: loaded,
	})
	s.Require().NoError(err)
	s.Equal(fencedTTL, s.mr.TTL("session:v1alpha1:run-1"))
}

func (s *FencedRepoSuite) TestMissingSheet_FailsClosed_WritingNothing() {
	loaded := s.launch("enc-1")
	_, err := s.fenced.SaveFenced(s.ctx, &sdk.SaveFencedInput{
		Encounter:  "enc-1",
		Data:       world(2),
		Session:    &sdk.SessionData{ID: "run-1", Encounter: "enc-1"},
		Characters: []*tkcharacter.Data{sheet("hero-a", 2), sheet("ghost", 2)},
		Expected:   loaded,
	})
	s.Require().ErrorIs(err, sdk.ErrNotFound)
	a, err := s.chars.GetCharacter(s.ctx, "hero-a")
	s.Require().NoError(err)
	s.Equal(10, a.HitPoints)
	s.assertUntouched("enc-1", loaded)
}

func (s *FencedRepoSuite) TestSheetMovedToAnotherPlayer_FailsClosed() {
	loaded := s.launch("enc-1")
	moved := sheet("hero-a", 2)
	moved.PlayerID = "player-2"
	_, err := s.fenced.SaveFenced(s.ctx, &sdk.SaveFencedInput{
		Encounter: "enc-1", Data: world(2), Session: &sdk.SessionData{ID: "run-1", Encounter: "enc-1"},
		Characters: []*tkcharacter.Data{moved}, Expected: loaded,
	})
	s.Require().ErrorIs(err, sdk.ErrBadRepository)
	s.assertUntouched("enc-1", loaded)
}

func (s *FencedRepoSuite) TestPlainSave_AdvancesTheVersion() {
	loaded := s.launch("enc-1")
	s.Require().NoError(s.plain.SaveEncounter(s.ctx, "enc-1", world(3)))
	_, err := s.fenced.SaveFenced(s.ctx, &sdk.SaveFencedInput{Encounter: "enc-1", Data: world(4), Expected: loaded})
	s.Require().ErrorIs(err, sdk.ErrStaleCheckpoint, "a plain write lands a save, so it moves the checkpoint")
}

func (s *FencedRepoSuite) TestScriptSurvivesAFlush() {
	loaded := s.launch("enc-1")
	s.Require().NoError(s.client.ScriptFlush(s.ctx).Err())
	_, err := s.fenced.SaveFenced(s.ctx, &sdk.SaveFencedInput{Encounter: "enc-1", Data: world(2), Expected: loaded})
	s.Require().NoError(err, "NOSCRIPT falls back to EVAL")
}

// assertUntouched proves a refused save wrote nothing: the world is still
// the launch's, the version has not moved and no session record landed.
func (s *FencedRepoSuite) assertUntouched(enc string, loaded uint64) {
	got, err := s.fenced.GetFencedEncounter(s.ctx, &sdk.GetFencedEncounterInput{Encounter: enc})
	s.Require().NoError(err)
	s.Equal(loaded, got.Version)
	s.Equal(1, got.Data.Retention, "the world is still the launch's")
	_, err = s.sessions.GetSession(s.ctx, "run-1")
	s.Require().ErrorIs(err, sdk.ErrNotFound, "the refused save's session record did not land")
}
