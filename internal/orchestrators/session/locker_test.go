package session

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

type SessionLockerSuite struct {
	suite.Suite
	locker *InProcessSessionLocker
}

func TestSessionLockerSuite(t *testing.T) { suite.Run(t, new(SessionLockerSuite)) }

func (s *SessionLockerSuite) SetupTest() { s.locker = NewInProcessSessionLocker() }

func (s *SessionLockerSuite) TestWaitCancellationKeepsTheOwnerAndDropsTheWaiter() {
	ctx, cancelOwner := context.WithCancel(context.Background())
	owner, err := s.locker.LockSession(ctx, &sdk.LockSessionInput{Session: "a"})
	s.Require().NoError(err)
	defer owner.Release()
	cancelOwner() // cancellation does not revoke an acquired guard mid-write

	waiting, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	out, err := s.locker.LockSession(waiting, &sdk.LockSessionInput{Session: "a"})
	s.ErrorIs(err, context.DeadlineExceeded)
	s.Nil(out)
	s.Equal(1, s.locker.entries["a"].refs)
	owner.Release()
	s.Empty(s.locker.entries)
}

func (s *SessionLockerSuite) TestDifferentSessionsAreIndependentAndReleaseIsIdempotent() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	a, err := s.locker.LockSession(ctx, &sdk.LockSessionInput{Session: "a"})
	s.Require().NoError(err)
	defer a.Release()
	b, err := s.locker.LockSession(ctx, &sdk.LockSessionInput{Session: "b"})
	s.Require().NoError(err)
	defer b.Release()
	s.Len(s.locker.entries, 2)
	a.Release()
	a.Release()
	s.Len(s.locker.entries, 1)
	b.Release()
	s.Empty(s.locker.entries)
}

func (s *SessionLockerSuite) TestInvalidAndAlreadyCanceledCallsCreateNoEntries() {
	_, err := s.locker.LockSession(context.Background(), nil)
	s.ErrorIs(err, sdk.ErrNilInput)
	_, err = s.locker.LockSession(context.Background(), &sdk.LockSessionInput{})
	s.ErrorIs(err, sdk.ErrNoSessionID)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = s.locker.LockSession(ctx, &sdk.LockSessionInput{Session: "a"})
	s.ErrorIs(err, context.Canceled)
	s.Empty(s.locker.entries)
}

func (s *SessionLockerSuite) TestReleaseKeepsTheAcquiredIdentity() {
	in := &sdk.LockSessionInput{Session: "a"}
	guard, err := s.locker.LockSession(context.Background(), in)
	s.Require().NoError(err)
	in.Session = "b"
	guard.Release()
	s.Empty(s.locker.entries)
}

func (s *SessionLockerSuite) TestContendedKeysNeverLoseAnUpdateOrLeakAnEntry() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	const workers, operations = 12, 100
	var wg sync.WaitGroup
	errors := make(chan error, workers)
	count := 0 // protected solely by the returned SDK guard, not by a test mutex
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range operations {
				guard, err := s.locker.LockSession(ctx, &sdk.LockSessionInput{Session: "contended"})
				if err != nil {
					errors <- err
					return
				}
				count++
				guard.Release()
			}
		}()
	}
	wg.Wait()
	close(errors)
	for err := range errors {
		s.Require().NoError(err)
	}
	s.Equal(workers*operations, count)
	s.Empty(s.locker.entries)
}

// TestCharacterGuardsAreAKeySpaceOfTheirOwn: a character id equal to a held
// session id is a different guard. The SDK holds a session guard and then
// takes character guards (Launch, Join, Exit); a shared key would deadlock it.
func (s *SessionLockerSuite) TestCharacterGuardsAreAKeySpaceOfTheirOwn() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	held, err := s.locker.LockSession(ctx, &sdk.LockSessionInput{Session: "same-id"})
	s.Require().NoError(err)
	character, err := s.locker.LockCharacter(ctx, &sdk.LockCharacterInput{Character: "same-id"})
	s.Require().NoError(err, "a character guard never waits on a session guard")
	character.Release()
	held.Release()
	s.Empty(s.locker.entries)
	s.Empty(s.locker.characters)
}

func (s *SessionLockerSuite) TestACharacterGuardExcludesTheSameCharacterOnly() {
	first, err := s.locker.LockCharacter(context.Background(), &sdk.LockCharacterInput{Character: "alice"})
	s.Require().NoError(err)
	other, err := s.locker.LockCharacter(context.Background(), &sdk.LockCharacterInput{Character: "bob"})
	s.Require().NoError(err)
	other.Release()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err = s.locker.LockCharacter(ctx, &sdk.LockCharacterInput{Character: "alice"})
	s.ErrorIs(err, context.DeadlineExceeded)
	first.Release()
	first.Release() // idempotent
	again, err := s.locker.LockCharacter(context.Background(), &sdk.LockCharacterInput{Character: "alice"})
	s.Require().NoError(err)
	again.Release()
	s.Empty(s.locker.characters)
}

func (s *SessionLockerSuite) TestACharacterGuardRefusesAnEmptyID() {
	_, err := s.locker.LockCharacter(context.Background(), nil)
	s.ErrorIs(err, sdk.ErrNilInput)
	_, err = s.locker.LockCharacter(context.Background(), &sdk.LockCharacterInput{})
	s.ErrorIs(err, sdk.ErrNoMemberID)
	s.Empty(s.locker.characters)
}
