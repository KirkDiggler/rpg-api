package session_test

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/KirkDiggler/rpg-toolkit/core"
	playclock "github.com/KirkDiggler/rpg-toolkit/play/clock"
	"github.com/KirkDiggler/rpg-toolkit/play/interrupt"
	tkencounter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/monster"
	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
)

// Canonical data without a playable world: these adapters preserve opaque
// records; SDK loading/validation and gameplay belong to their provider.
func populatedStoredSession() *sdk.SessionData {
	return &sdk.SessionData{ID: "shared-id", Encounter: "encounter-id", Dungeon: "authored-key",
		Streams: map[string]sdk.StreamCursor{"member-a": {UpTo: 17, Count: 3}, "member-b": {UpTo: 9, Count: 1}},
		NPCs:    []monster.Data{{ID: "monster-a", Name: "Stored sheet", HitPoints: 2, MaxHitPoints: 19, ArmorClass: 14}},
		Windows: interrupt.LedgerData{NextID: 8, Windows: []interrupt.WindowData{{ID: 7, Audience: "member-a", Payload: []byte("opaque payload"), At: 12}}},
	}
}
func populatedStoredEncounter() *tkencounter.EncounterData {
	return &tkencounter.EncounterData{
		Clock: playclock.TickData{Budgets: map[core.EntityID]int{"member-a": 2}, DriverProgress: map[core.EntityID]int{"member-a": 5}, HighWater: 5},
		Members: []tkencounter.MemberData{{ID: "member-a", Name: "Stored member", Cell: &tkencounter.PositionData{X: -3, Y: 4}, SpeedFeet: 25},
			{ID: "member-b", Cell: &tkencounter.PositionData{}}},
		Field:       tkencounter.FieldData{Scenery: []tkencounter.PositionData{{X: 7, Y: -2}}},
		EverMembers: []tkencounter.MemberID{"member-a", "member-b", "departed"}, Retention: 17,
		Outcome: &tkencounter.OutcomeData{Ending: "saved-ending", At: 13, Members: []tkencounter.MemberOutcomeData{{ID: "member-a", Cell: &tkencounter.PositionData{}}}},
	}
}

func (s *RedisReposTestSuite) TestPopulatedReadsAreDetached_AndSameIDNamespacesAreIndependent() {
	session := populatedStoredSession()
	encounter := populatedStoredEncounter()
	s.Require().NoError(s.sessions.SaveSession(s.ctx, session))
	s.Require().NoError(s.encs.SaveEncounter(s.ctx, "shared-id", encounter))
	gotSession, err := s.sessions.GetSession(s.ctx, "shared-id")
	s.Require().NoError(err)
	gotEncounter, err := s.encs.GetEncounter(s.ctx, "shared-id")
	s.Require().NoError(err)
	s.Equal(session, gotSession)
	s.Equal(encounter, gotEncounter)
	session.Streams["member-a"] = sdk.StreamCursor{Count: 999}
	gotSession.Windows.Windows[0].Payload[0] = 'x'
	gotSession.NPCs[0].HitPoints = 999
	encounter.Clock.Budgets["member-a"] = 999
	gotEncounter.Members[0].Cell.X = 999
	againSession, err := s.sessions.GetSession(s.ctx, "shared-id")
	s.Require().NoError(err)
	s.Equal(populatedStoredSession(), againSession)
	againEncounter, err := s.encs.GetEncounter(s.ctx, "shared-id")
	s.Require().NoError(err)
	s.Equal(populatedStoredEncounter(), againEncounter)
}
func (s *RedisReposTestSuite) TestSaveOverwritesOnlyAddressedKey() {
	first := populatedStoredSession()
	s.Require().NoError(s.sessions.SaveSession(s.ctx, first))
	other := populatedStoredSession()
	other.ID = "other-id"
	s.Require().NoError(s.sessions.SaveSession(s.ctx, other))
	s.Require().NoError(s.encs.SaveEncounter(s.ctx, "shared-id", populatedStoredEncounter()))
	s.Require().NoError(s.encs.SaveEncounter(s.ctx, "other-id", populatedStoredEncounter()))
	first.Dungeon = "new-key"
	first.Streams = nil
	s.Require().NoError(s.sessions.SaveSession(s.ctx, first))
	replacement := &tkencounter.EncounterData{Retention: 9}
	s.Require().NoError(s.encs.SaveEncounter(s.ctx, "shared-id", replacement))
	got, err := s.sessions.GetSession(s.ctx, "shared-id")
	s.Require().NoError(err)
	s.Equal(first, got)
	neighbor, err := s.sessions.GetSession(s.ctx, "other-id")
	s.Require().NoError(err)
	s.Equal(other, neighbor)
	enc, err := s.encs.GetEncounter(s.ctx, "shared-id")
	s.Require().NoError(err)
	s.Equal(replacement, enc)
	otherEnc, err := s.encs.GetEncounter(s.ctx, "other-id")
	s.Require().NoError(err)
	s.Equal(populatedStoredEncounter(), otherEnc)
}
func (s *RedisReposTestSuite) TestEncounterOptionalPositionDistinguishesZeroFromAbsence() {
	for _, cell := range []*tkencounter.PositionData{nil, {}} {
		in := &tkencounter.EncounterData{Members: []tkencounter.MemberData{{ID: "member-a", Cell: cell}}}
		s.Require().NoError(s.encs.SaveEncounter(s.ctx, "enc", in))
		got, err := s.encs.GetEncounter(s.ctx, "enc")
		s.Require().NoError(err)
		s.Equal(in, got)
	}
}
func (s *RedisReposTestSuite) TestConfiguredTTLRefreshesOnSave_NotOnGet() {
	in := populatedStoredSession()
	enc := populatedStoredEncounter()
	s.Require().NoError(s.sessions.SaveSession(s.ctx, in))
	s.Require().NoError(s.encs.SaveEncounter(s.ctx, "shared-id", enc))
	s.miniredis.FastForward(23 * time.Hour)
	_, err := s.sessions.GetSession(s.ctx, "shared-id")
	s.Require().NoError(err)
	_, err = s.encs.GetEncounter(s.ctx, "shared-id")
	s.Require().NoError(err)
	s.Equal(time.Hour, s.miniredis.TTL("session:v1alpha1:shared-id"))
	s.Equal(time.Hour, s.miniredis.TTL("session-enc:v1alpha1:shared-id"))
	s.Require().NoError(s.sessions.SaveSession(s.ctx, in))
	s.Require().NoError(s.encs.SaveEncounter(s.ctx, "shared-id", enc))
	s.Equal(24*time.Hour, s.miniredis.TTL("session:v1alpha1:shared-id"))
	s.Equal(24*time.Hour, s.miniredis.TTL("session-enc:v1alpha1:shared-id"))
	s.miniredis.FastForward(24 * time.Hour)
	got, err := s.sessions.GetSession(s.ctx, "shared-id")
	s.ErrorIs(err, sdk.ErrNotFound)
	s.Nil(got)
	gotEnc, err := s.encs.GetEncounter(s.ctx, "shared-id")
	s.ErrorIs(err, sdk.ErrNotFound)
	s.Nil(gotEnc)
}
func (s *RedisReposTestSuite) TestZeroTTLDoesNotExpire() {
	sessions := sessionorch.NewSessionRepository(s.client, 0)
	encs := sessionorch.NewEncounterRepository(s.client, 0)
	s.Require().NoError(sessions.SaveSession(s.ctx, populatedStoredSession()))
	s.Require().NoError(encs.SaveEncounter(s.ctx, "enc", populatedStoredEncounter()))
	s.miniredis.FastForward(365 * 24 * time.Hour)
	got, err := sessions.GetSession(s.ctx, "shared-id")
	s.Require().NoError(err)
	s.Equal(populatedStoredSession(), got)
	gotEnc, err := encs.GetEncounter(s.ctx, "enc")
	s.Require().NoError(err)
	s.Equal(populatedStoredEncounter(), gotEnc)
}
func (s *RedisReposTestSuite) TestMalformedStoredPayloadsPreserveDecodeCause() {
	s.Require().NoError(s.miniredis.Set("session:v1alpha1:broken", "{"))
	s.Require().NoError(s.miniredis.Set("session-enc:v1alpha1:broken", "{"))
	session, err := s.sessions.GetSession(s.ctx, "broken")
	var syntax *json.SyntaxError
	s.ErrorAs(err, &syntax)
	s.NotErrorIs(err, sdk.ErrNotFound)
	s.Nil(session)
	encounter, err := s.encs.GetEncounter(s.ctx, "broken")
	s.ErrorAs(err, &syntax)
	s.NotErrorIs(err, sdk.ErrNotFound)
	s.Nil(encounter)
}
func (s *RedisReposTestSuite) TestEncounterMarshalFailureDoesNotOverwriteStoredRecord() {
	s.Require().NoError(s.encs.SaveEncounter(s.ctx, "enc", populatedStoredEncounter()))
	invalid := populatedStoredEncounter()
	invalid.Members[0].Cell.X = math.NaN()
	err := s.encs.SaveEncounter(s.ctx, "enc", invalid)
	var unsupported *json.UnsupportedValueError
	s.ErrorAs(err, &unsupported)
	got, err := s.encs.GetEncounter(s.ctx, "enc")
	s.Require().NoError(err)
	s.Equal(populatedStoredEncounter(), got)
}
func (s *RedisReposTestSuite) TestStorageReadFailuresAreNotSDKNotFound() {
	s.Require().NoError(s.client.Close())
	got, err := s.sessions.GetSession(s.ctx, "session")
	s.ErrorIs(err, goredis.ErrClosed)
	s.NotErrorIs(err, sdk.ErrNotFound)
	s.Nil(got)
	enc, err := s.encs.GetEncounter(s.ctx, "encounter")
	s.ErrorIs(err, goredis.ErrClosed)
	s.NotErrorIs(err, sdk.ErrNotFound)
	s.Nil(enc)
}

type sessionWriteFailure struct{ cause error }

func (h sessionWriteFailure) DialHook(next goredis.DialHook) goredis.DialHook { return next }
func (h sessionWriteFailure) ProcessPipelineHook(next goredis.ProcessPipelineHook) goredis.ProcessPipelineHook {
	return next
}
func (h sessionWriteFailure) ProcessHook(next goredis.ProcessHook) goredis.ProcessHook {
	return func(ctx context.Context, cmd goredis.Cmder) error {
		if cmd.Name() == "set" {
			return h.cause
		}
		return next(ctx, cmd)
	}
}
func (s *RedisReposTestSuite) TestStorageWriteFailuresKeepCauseAndPreviousRecords() {
	s.Require().NoError(s.sessions.SaveSession(s.ctx, populatedStoredSession()))
	s.Require().NoError(s.encs.SaveEncounter(s.ctx, "enc", populatedStoredEncounter()))
	cause := errors.New("injected write failure")
	s.client.AddHook(sessionWriteFailure{cause: cause})
	s.ErrorIs(s.sessions.SaveSession(s.ctx, &sdk.SessionData{ID: "shared-id"}), cause)
	s.ErrorIs(s.encs.SaveEncounter(s.ctx, "enc", &tkencounter.EncounterData{}), cause)
	got, err := s.sessions.GetSession(s.ctx, "shared-id")
	s.Require().NoError(err)
	s.Equal(populatedStoredSession(), got)
	enc, err := s.encs.GetEncounter(s.ctx, "enc")
	s.Require().NoError(err)
	s.Equal(populatedStoredEncounter(), enc)
}
