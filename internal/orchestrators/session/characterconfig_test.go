package session

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/suite"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	"github.com/KirkDiggler/rpg-api/internal/repositories/characterconfig"
)

type CharacterConfigSuite struct {
	suite.Suite
	ctx         context.Context
	server      *miniredis.Miniredis
	client      *goredis.Client
	locker      *InProcessSessionLocker
	preferences *Preferences
	reader      *characterConfigReader
}

func TestCharacterConfigSuite(t *testing.T) { suite.Run(t, new(CharacterConfigSuite)) }

func (s *CharacterConfigSuite) SetupTest() {
	s.ctx = context.Background()
	s.server = miniredis.RunT(s.T())
	s.client = goredis.NewClient(&goredis.Options{Addr: s.server.Addr()})
	repo, err := characterconfig.NewRedis(&characterconfig.RedisConfig{Client: s.client})
	s.Require().NoError(err)
	s.locker = NewInProcessSessionLocker()
	s.preferences = &Preferences{locker: s.locker, repo: repo}
	s.reader = &characterConfigReader{repo: repo}
}

func (s *CharacterConfigSuite) TearDownTest() { _ = s.client.Close() }

func boolValue(v bool) *bool { return &v }

func (s *CharacterConfigSuite) TestSetPreference_LocksTheCharacter() {
	held, err := s.locker.LockCharacter(s.ctx, &sdk.LockCharacterInput{Character: "alice"})
	s.Require().NoError(err)

	blocked, cancel := context.WithTimeout(s.ctx, 30*time.Millisecond)
	defer cancel()
	_, err = s.preferences.SetPreference(blocked, &SetPreferenceInput{CharacterID: "alice", DiscoverySharing: boolValue(true)})
	s.ErrorIs(err, context.DeadlineExceeded, "SetPreference waits on the character's own guard")

	// Another character's guard, and every session guard, stay free.
	_, err = s.preferences.SetPreference(s.ctx, &SetPreferenceInput{CharacterID: "bob", DiscoverySharing: boolValue(true)})
	s.Require().NoError(err)
	session, err := s.locker.LockSession(s.ctx, &sdk.LockSessionInput{Session: "run"})
	s.Require().NoError(err)
	session.Release()

	held.Release()
	out, err := s.preferences.SetPreference(s.ctx, &SetPreferenceInput{CharacterID: "alice", DiscoverySharing: boolValue(true)})
	s.Require().NoError(err)
	s.Equal(boolValue(true), out.DiscoverySharing)
	s.Empty(s.locker.characters, "the guard is released after the write")
}

func (s *CharacterConfigSuite) TestSetPreference_AnAbsentPreferenceIsLeftAsItIs() {
	_, err := s.preferences.SetPreference(s.ctx, &SetPreferenceInput{CharacterID: "alice", DiscoverySharing: boolValue(false)})
	s.Require().NoError(err)
	out, err := s.preferences.SetPreference(s.ctx, &SetPreferenceInput{CharacterID: "alice"})
	s.Require().NoError(err)
	s.Equal(boolValue(false), out.DiscoverySharing)
	got, err := s.reader.DiscoverySharing(s.ctx, "alice")
	s.Require().NoError(err)
	s.Equal(boolValue(false), got)
}

func (s *CharacterConfigSuite) TestSetPreference_RefusesNoCharacter() {
	_, err := s.preferences.SetPreference(s.ctx, &SetPreferenceInput{})
	s.ErrorIs(err, ErrCharacterRequired)
	_, err = s.preferences.SetPreference(s.ctx, nil)
	s.ErrorIs(err, sdk.ErrNilInput)
}

// The reader is what the session SDK sees: not chosen is sdk.ErrNotFound, a
// choice is its value, and a write through Preferences is seen at once.
func (s *CharacterConfigSuite) TestReader_AnswersTheSDK() {
	_, err := s.reader.DiscoverySharing(s.ctx, "alice")
	s.ErrorIs(err, sdk.ErrNotFound, "not chosen")
	_, err = s.preferences.SetPreference(s.ctx, &SetPreferenceInput{CharacterID: "alice", DiscoverySharing: boolValue(true)})
	s.Require().NoError(err)
	got, err := s.reader.DiscoverySharing(s.ctx, "alice")
	s.Require().NoError(err)
	s.Equal(boolValue(true), got)
}

// The legacy exploration profile is read once, as the initial value, through
// the old three branches, and is never written.
func (s *CharacterConfigSuite) TestReader_LegacyRecordReadOnce() {
	for _, tc := range []struct {
		name   string
		legacy string
		want   *bool
	}{
		{name: "explicit choice wins", legacy: `{"character":"alice","discovery_sharing":false}`, want: boolValue(false)},
		{name: "no choice reads not private", legacy: `{"character":"alice"}`, want: boolValue(true)},
		{name: "no choice, private", legacy: `{"character":"alice","private":true}`, want: boolValue(false)},
		{name: "no record is not chosen"},
	} {
		s.Run(tc.name, func() {
			s.server.FlushAll()
			if tc.legacy != "" {
				s.Require().NoError(s.server.Set("character-exploration:v1:alice", tc.legacy))
			}
			got, err := s.reader.DiscoverySharing(s.ctx, "alice")
			if tc.want == nil {
				s.ErrorIs(err, sdk.ErrNotFound)
			} else {
				s.Require().NoError(err)
				s.Equal(tc.want, got)
			}
			s.Require().NoError(s.server.Set("character-exploration:v1:alice", `{"character":"alice","discovery_sharing":true,"private":false}`))
			again, err := s.reader.DiscoverySharing(s.ctx, "alice")
			if tc.want == nil {
				s.ErrorIs(err, sdk.ErrNotFound, "a later legacy record has no authority")
			} else {
				s.Require().NoError(err)
				s.Equal(tc.want, again, "the legacy record is not read a second time")
			}
		})
	}
}

// TestSetPreference_NeverDeadlocksWithASessionVerb is the lock order R2 relies
// on: Launch, Join and Exit hold a session guard and then take character
// guards; SetPreference takes a character guard alone and never a session
// guard. A preference write on a character the verb holds waits for the verb
// and then runs; one on any other character, or while only the session guard
// is held, runs at once. No cycle exists, so neither side waits forever.
func (s *CharacterConfigSuite) TestSetPreference_NeverDeadlocksWithASessionVerb() {
	ctx, cancel := context.WithTimeout(s.ctx, 2*time.Second)
	defer cancel()

	// The verb: session guard first, then its party's character guards.
	session, err := s.locker.LockSession(ctx, &sdk.LockSessionInput{Session: "run"})
	s.Require().NoError(err)
	_, err = s.preferences.SetPreference(ctx, &SetPreferenceInput{CharacterID: "alice", DiscoverySharing: boolValue(true)})
	s.Require().NoError(err, "a held session guard alone never blocks a preference write")
	alice, err := s.locker.LockCharacter(ctx, &sdk.LockCharacterInput{Character: "alice"})
	s.Require().NoError(err)

	written := make(chan error, 1)
	go func() {
		_, setErr := s.preferences.SetPreference(ctx, &SetPreferenceInput{CharacterID: "alice", DiscoverySharing: boolValue(false)})
		written <- setErr
	}()
	_, err = s.preferences.SetPreference(ctx, &SetPreferenceInput{CharacterID: "bob", DiscoverySharing: boolValue(true)})
	s.Require().NoError(err, "a character the verb does not hold is free")
	select {
	case early := <-written:
		s.FailNow("the write on a held character ran inside the verb", "%v", early)
	case <-time.After(30 * time.Millisecond):
	}

	alice.Release()
	session.Release()
	select {
	case setErr := <-written:
		s.Require().NoError(setErr, "the waiting write runs once the verb releases")
	case <-ctx.Done():
		s.FailNow("the preference write deadlocked behind the session verb")
	}
	got, err := s.reader.DiscoverySharing(ctx, "alice")
	s.Require().NoError(err)
	s.Equal(boolValue(false), got)
	s.Empty(s.locker.entries)
	s.Empty(s.locker.characters)
}

// TestSetPreference_InterleavesWithSessionVerbsUnderLoad races many session
// verbs, each holding its session guard and then two character guards, with
// preference writes on the same characters. Everything finishes.
func (s *CharacterConfigSuite) TestSetPreference_InterleavesWithSessionVerbsUnderLoad() {
	ctx, cancel := context.WithTimeout(s.ctx, 5*time.Second)
	defer cancel()
	const rounds = 50
	var wg sync.WaitGroup
	failures := make(chan error, 4*rounds)
	for i := 0; i < rounds; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			session, err := s.locker.LockSession(ctx, &sdk.LockSessionInput{Session: []string{"keep", "castle"}[i%2]})
			if err != nil {
				failures <- err
				return
			}
			defer session.Release()
			held := make([]func(), 0, 2)
			defer func() {
				for _, release := range held {
					release()
				}
			}()
			for _, id := range []string{"alice", "bob"} {
				guard, err := s.locker.LockCharacter(ctx, &sdk.LockCharacterInput{Character: id})
				if err != nil {
					failures <- err
					return
				}
				held = append(held, guard.Release)
			}
		}(i)
		go func(i int) {
			defer wg.Done()
			id := []string{"alice", "bob"}[i%2]
			if _, err := s.preferences.SetPreference(ctx, &SetPreferenceInput{CharacterID: id, DiscoverySharing: boolValue(i%3 == 0)}); err != nil {
				failures <- err
			}
		}(i)
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		s.Require().NoError(err)
	}
	s.Empty(s.locker.entries)
	s.Empty(s.locker.characters)
}
