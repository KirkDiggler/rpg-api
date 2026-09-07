package sessionv1alpha1

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
	"github.com/KirkDiggler/rpg-toolkit/tools/spatial"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	charactermock "github.com/KirkDiggler/rpg-api/internal/repositories/character/mock"
	tkcharacter "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/character"
)

// capturingStream satisfies grpc.ServerStreamingServer[sessionpb.Event]
// (sessionpb.SessionService_StreamEventsServer) for unit tests, mirroring the
// v2 encounter handler's own capturingStream (handler_test.go).
type capturingStream struct {
	grpc.ServerStream
	ctx  context.Context
	sent chan *sessionpb.Event
}

func newCapturingStream(ctx context.Context) *capturingStream {
	return &capturingStream{ctx: ctx, sent: make(chan *sessionpb.Event, 16)}
}

func (s *capturingStream) Context() context.Context { return s.ctx }

func (s *capturingStream) Send(evt *sessionpb.Event) error {
	select {
	case s.sent <- evt:
		return nil
	default:
		return fmt.Errorf("capturingStream buffer full")
	}
}

func (s *capturingStream) WaitForSend(t *testing.T, timeout time.Duration) *sessionpb.Event {
	t.Helper()
	select {
	case evt := <-s.sent:
		return evt
	case <-time.After(timeout):
		t.Fatalf("no event received within %s", timeout)
		return nil
	}
}

func ownedCharacterRepo(ctrl *gomock.Controller, member, playerID string) characterrepo.Repository {
	repo := charactermock.NewMockRepository(ctrl)
	repo.EXPECT().Get(gomock.Any(), characterrepo.GetInput{ID: member}).Return(
		&characterrepo.GetOutput{Character: &entities.Character{Data: &tkcharacter.Data{ID: member, PlayerID: playerID}}}, nil,
	).AnyTimes()
	return repo
}

// anyMemberOwnedBy answers every member lookup as a character controlled by
// playerID.
//
// Deliberately permissive, because the verb tests it serves are about
// TRANSLATION -- does this handler hand the SDK the right input and map its
// error to the right code -- not about entitlement. Giving each of them a
// narrowly-scoped repo would make every one of them fail for the wrong reason
// the day a member ID changed. The entitlement gate is pinned separately, by
// the ownership tests below and in handler_test.go, where a WRONG owner and a
// MISSING member are the point rather than a setup detail.
func anyMemberOwnedBy(ctrl *gomock.Controller, playerID string) characterrepo.Repository {
	repo := charactermock.NewMockRepository(ctrl)
	repo.EXPECT().Get(gomock.Any(), gomock.Any()).DoAndReturn(
		func(_ context.Context, in characterrepo.GetInput) (*characterrepo.GetOutput, error) {
			return &characterrepo.GetOutput{
				Character: &entities.Character{Data: &tkcharacter.Data{ID: in.ID, PlayerID: playerID}},
			}, nil
		},
	).AnyTimes()
	return repo
}

func TestStreamEvents_Unauthenticated_Errors(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := &Handler{characters: anyMemberOwnedBy(ctrl, "alice")}
	stream := newCapturingStream(context.Background())
	err := h.StreamEvents(&sessionpb.StreamEventsRequest{}, stream)
	requireCode(t, err, codes.Unauthenticated)
}

func TestStreamEvents_EmptyMember_InvalidArgument(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := &Handler{characters: anyMemberOwnedBy(ctrl, "alice")}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	stream := newCapturingStream(ctx)
	err := h.StreamEvents(&sessionpb.StreamEventsRequest{Session: "sess-1"}, stream)
	requireCode(t, err, codes.InvalidArgument)
}

func TestStreamEvents_CallerDoesNotOwnMember_PermissionDenied(t *testing.T) {
	ctrl := gomock.NewController(t)
	h := &Handler{characters: ownedCharacterRepo(ctrl, "char-1", "bob"), broker: sessionorch.NewBroker()}
	ctx := auth.WithPlayerID(context.Background(), "alice")
	stream := newCapturingStream(ctx)
	err := h.StreamEvents(&sessionpb.StreamEventsRequest{Session: "sess-1", Member: "char-1"}, stream)
	requireCode(t, err, codes.PermissionDenied)
}

func TestStreamEvents_ForwardsPublishedEventsVerbatim(t *testing.T) {
	ctrl := gomock.NewController(t)
	broker := sessionorch.NewBroker()
	h := &Handler{characters: ownedCharacterRepo(ctrl, "char-1", "alice"), broker: broker}

	ctx, cancel := context.WithCancel(auth.WithPlayerID(context.Background(), "alice"))
	defer cancel()
	stream := newCapturingStream(ctx)

	done := make(chan error, 1)
	go func() {
		done <- h.StreamEvents(&sessionpb.StreamEventsRequest{Session: "sess-1", Member: "char-1"}, stream)
	}()

	// The handler's Subscribe call happens inside that goroutine, so there is
	// no signal here for exactly when it has run. Publish is best-effort and
	// silently drops to a not-yet-subscribed recipient (by contract -- see
	// broker.go), so republish on a short tick until the event shows up in
	// stream.sent rather than assuming one publish lands.
	got := waitForPublishedEvent(t, broker, stream, sdk.Event{
		Session: "sess-1", Recipient: "char-1", Seq: 1, Kind: sdk.EventMoved,
	})
	require.Equal(t, uint64(1), got.GetSeq())
	require.Equal(t, sessionpb.EventKind_EVENT_KIND_MOVED, got.GetKind())
	require.Equal(t, "char-1", got.GetRecipient())

	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("StreamEvents did not return after context cancellation")
	}
}

func TestStreamEvents_DoesNotReceiveEventsAddressedToOthers(t *testing.T) {
	ctrl := gomock.NewController(t)
	broker := sessionorch.NewBroker()
	h := &Handler{characters: ownedCharacterRepo(ctrl, "char-1", "alice"), broker: broker}

	ctx, cancel := context.WithCancel(auth.WithPlayerID(context.Background(), "alice"))
	defer cancel()
	stream := newCapturingStream(ctx)

	done := make(chan error, 1)
	go func() {
		done <- h.StreamEvents(&sessionpb.StreamEventsRequest{Session: "sess-1", Member: "char-1"}, stream)
	}()

	// Confirm the subscription is live (via a control event this recipient
	// DOES own) before asserting the negative -- otherwise "nothing arrived"
	// would be indistinguishable from "hadn't subscribed yet".
	waitForPublishedEvent(t, broker, stream, sdk.Event{
		Session: "sess-1", Recipient: "char-1", Seq: 1, Kind: sdk.EventMoved,
	})

	broker.Publish(context.Background(), []sdk.Event{ //nolint:errcheck // "someone-else" has no live subscriber yet, so nothing can drop here (rpg-api#819: Publish can error now)
		{Session: "sess-1", Recipient: "someone-else", Seq: 2, Kind: sdk.EventMoved},
	})

	select {
	case evt := <-stream.sent:
		t.Fatalf("must not receive an event addressed to a different recipient, got seq %d", evt.GetSeq())
	case <-time.After(100 * time.Millisecond):
	}

	cancel()
	<-done
}

// waitForPublishedEvent republishes evt on a short tick until it appears in
// stream.sent or the test times out. Necessary because the subscriber this
// helper is waiting on is registered inside a goroutine this test does not
// otherwise synchronize with, and Publish silently drops to a recipient with
// no live subscription yet (best-effort by contract, broker.go) rather than
// erroring -- so a single publish racing the Subscribe call is not reliable.
func waitForPublishedEvent(t *testing.T, broker *sessionorch.Broker, stream *capturingStream, evt sdk.Event) *sessionpb.Event {
	t.Helper()
	deadline := time.After(2 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case got := <-stream.sent:
			return got
		case <-ticker.C:
			broker.Publish(context.Background(), []sdk.Event{evt}) //nolint:errcheck // republished on a fast tick until it lands; a stray drop here just means another tick, not a broken test
		case <-deadline:
			t.Fatal("event never arrived on the stream")
			return nil
		}
	}
}

// TestStreamEvents_ForwardsTypedBodyPerKind is the end-to-end half of the
// oneof-body projection: convert_test.go proves eventToProto's mapping
// directly, this proves the SAME mapping survives the full
// Publish -> Subscribe -> StreamEvents -> stream.Send path, one case per
// body kind (rpg-toolkit#941, rpg-project#249's design brief).
func TestStreamEvents_ForwardsTypedBodyPerKind(t *testing.T) {
	tests := []struct {
		name    string
		kind    sdk.EventKind
		body    sdk.EventBody
		checkPB func(t *testing.T, got *sessionpb.Event)
	}{
		{
			name: "TurnEnded", kind: sdk.EventTurnEnded,
			body: sdk.TurnEndedBody{Member: "char-1", Next: "goblin-1"},
			checkPB: func(t *testing.T, got *sessionpb.Event) {
				require.Equal(t, "char-1", got.GetTurnEnded().GetMember())
				require.Equal(t, "goblin-1", got.GetTurnEnded().GetNext())
			},
		},
		{
			name: "Downed", kind: sdk.EventDowned,
			body: sdk.DownedBody{Member: "goblin-1"},
			checkPB: func(t *testing.T, got *sessionpb.Event) {
				require.Equal(t, "goblin-1", got.GetDowned().GetMember())
			},
		},
		{
			name: "Struck", kind: sdk.EventStruck,
			body: sdk.StruckBody{
				Attacker: "char-1", Target: "goblin-1", Roll: 18, Total: 21, Against: 13, Damage: 6,
				Attack: sdk.AttackRef{Ref: "dnd5e:weapons:longsword", Name: "Longsword", DamageType: sdk.DamageSlashing},
			},
			checkPB: func(t *testing.T, got *sessionpb.Event) {
				require.Equal(t, int32(6), got.GetStruck().GetDamage())
				require.Equal(t, "dnd5e:weapons:longsword", got.GetStruck().GetAttack().GetRef())
				require.Equal(t, "Longsword", got.GetStruck().GetAttack().GetName())
				require.Equal(t, sessionpb.DamageType_DAMAGE_TYPE_SLASHING, got.GetStruck().GetAttack().GetDamageType())
			},
		},
		{
			name: "Missed", kind: sdk.EventMissed,
			body: sdk.MissedBody{
				Attacker: "char-1", Target: "goblin-1", Roll: 4, Total: 7, Against: 13,
				Attack: sdk.AttackRef{Ref: "dnd5e:weapons:longsword", Name: "Longsword", DamageType: sdk.DamageSlashing},
			},
			checkPB: func(t *testing.T, got *sessionpb.Event) {
				require.Equal(t, int32(4), got.GetMissed().GetRoll())
				require.Equal(t, "dnd5e:weapons:longsword", got.GetMissed().GetAttack().GetRef())
			},
		},
		{
			name: "FightStarted", kind: sdk.EventFightStarted,
			body: sdk.FightStartedBody{Members: []string{"char-1", "goblin-1"}},
			checkPB: func(t *testing.T, got *sessionpb.Event) {
				require.Equal(t, []string{"char-1", "goblin-1"}, got.GetFightStarted().GetMembers())
			},
		},
		{
			name: "FightEnded", kind: sdk.EventFightEnded,
			body: sdk.FightEndedBody{Cause: sdk.DissolveByDecision},
			checkPB: func(t *testing.T, got *sessionpb.Event) {
				require.Equal(t, sessionpb.DissolveKind_DISSOLVE_KIND_BY_DECISION, got.GetFightEnded().GetCause())
			},
		},
		{
			name: "Moved", kind: sdk.EventMoved,
			body: sdk.MovedBody{Member: "char-1", To: spatial.Position{X: 3, Y: 4}},
			checkPB: func(t *testing.T, got *sessionpb.Event) {
				require.Equal(t, "char-1", got.GetMoved().GetMember())
				require.Equal(t, 3.0, got.GetMoved().GetTo().GetX())
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			broker := sessionorch.NewBroker()
			h := &Handler{characters: ownedCharacterRepo(ctrl, "char-1", "alice"), broker: broker}

			ctx, cancel := context.WithCancel(auth.WithPlayerID(context.Background(), "alice"))
			defer cancel()
			stream := newCapturingStream(ctx)

			done := make(chan error, 1)
			go func() {
				done <- h.StreamEvents(&sessionpb.StreamEventsRequest{Session: "sess-1", Member: "char-1"}, stream)
			}()

			got := waitForPublishedEvent(t, broker, stream, sdk.Event{
				Session: "sess-1", Recipient: "char-1", Seq: 1, Kind: tc.kind, Body: tc.body,
			})
			require.Equal(t, eventKindToProto(tc.kind), got.GetKind())
			tc.checkPB(t, got)

			cancel()
			<-done
		})
	}
}

// capturingLogHandler records every slog.Record handed to it so a test can
// assert on which trace line StreamEvents actually emitted. Copilot's own
// finding on PR #821: the send trace used to log before stream.Send ran, so
// it could claim an event was forwarded when Send actually failed (a
// disconnected or full client stream) -- the two tests below pin the fix
// that moved the trace to after a successful Send, with a distinct line for
// a failed one.
type capturingLogHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *capturingLogHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h *capturingLogHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	h.records = append(h.records, r)
	h.mu.Unlock()
	return nil
}

func (h *capturingLogHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *capturingLogHandler) WithGroup(string) slog.Handler      { return h }

func (h *capturingLogHandler) messages() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]string, len(h.records))
	for i, r := range h.records {
		out[i] = r.Message
	}
	return out
}

// withCapturedLogs swaps slog's default logger for the duration of the test,
// restoring the previous one on cleanup, so a test can assert on what
// StreamEvents actually logged rather than merely that it compiled a line.
func withCapturedLogs(t *testing.T) *capturingLogHandler {
	t.Helper()
	h := &capturingLogHandler{}
	prev := slog.Default()
	slog.SetDefault(slog.New(h))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return h
}

func TestStreamEvents_SendTrace_OnlyLogsAfterASuccessfulSend(t *testing.T) {
	logs := withCapturedLogs(t)

	ctrl := gomock.NewController(t)
	broker := sessionorch.NewBroker()
	h := &Handler{characters: ownedCharacterRepo(ctrl, "char-1", "alice"), broker: broker}

	ctx, cancel := context.WithCancel(auth.WithPlayerID(context.Background(), "alice"))
	defer cancel()
	stream := newCapturingStream(ctx)

	done := make(chan error, 1)
	go func() {
		done <- h.StreamEvents(&sessionpb.StreamEventsRequest{Session: "sess-1", Member: "char-1"}, stream)
	}()

	waitForPublishedEvent(t, broker, stream, sdk.Event{
		Session: "sess-1", Recipient: "char-1", Seq: 1, Kind: sdk.EventMoved,
	})

	cancel()
	<-done

	require.Contains(t, logs.messages(), "session stream: forwarded event",
		"a successfully sent event must be traced as forwarded")
	require.NotContains(t, logs.messages(), "session stream: send failed, event not forwarded")
}

func TestStreamEvents_SendTrace_LogsFailureNotForwardedWhenSendErrors(t *testing.T) {
	logs := withCapturedLogs(t)

	ctrl := gomock.NewController(t)
	broker := sessionorch.NewBroker()
	h := &Handler{characters: ownedCharacterRepo(ctrl, "char-1", "alice"), broker: broker}

	ctx := auth.WithPlayerID(context.Background(), "alice")
	// A stream whose Send always fails: a zero-capacity channel nobody
	// reads from means the non-blocking select inside Send can never take
	// its success branch.
	stream := &capturingStream{ctx: ctx, sent: make(chan *sessionpb.Event)}

	done := make(chan error, 1)
	go func() {
		done <- h.StreamEvents(&sessionpb.StreamEventsRequest{Session: "sess-1", Member: "char-1"}, stream)
	}()

	// Republish on a tick (the same reason waitForPublishedEvent does --
	// Subscribe happens inside the goroutine above with no other signal for
	// when it has registered) until StreamEvents' own Send failure ends the
	// call.
	deadline := time.After(2 * time.Second)
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	evt := sdk.Event{Session: "sess-1", Recipient: "char-1", Seq: 1, Kind: sdk.EventMoved}
waitForFailure:
	for {
		select {
		case err := <-done:
			require.Error(t, err, "a Send that always fails must end the stream with that error")
			break waitForFailure
		case <-ticker.C:
			_ = broker.Publish(context.Background(), []sdk.Event{evt}) //nolint:errcheck // best-effort republish until StreamEvents observes it
		case <-deadline:
			t.Fatal("StreamEvents never returned")
		}
	}

	require.Contains(t, logs.messages(), "session stream: send failed, event not forwarded",
		"a Send that fails must be traced as such, never claimed as forwarded")
	require.NotContains(t, logs.messages(), "session stream: forwarded event")
}
