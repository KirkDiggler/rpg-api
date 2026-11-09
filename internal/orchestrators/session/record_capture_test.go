package session

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/suite"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"
)

type RecordCaptureSuite struct{ suite.Suite }

func TestRecordCaptureSuite(t *testing.T) { suite.Run(t, new(RecordCaptureSuite)) }

type recordWriterFunc func([]byte) (int, error)

func (f recordWriterFunc) Write(p []byte) (int, error) { return f(p) }

type capturedLine struct {
	Schema          int             `json:"schema"`
	Kind            string          `json:"kind"`
	Capture         string          `json:"capture"`
	Status          string          `json:"status"`
	HistoryComplete bool            `json:"history_complete"`
	Session         string          `json:"session"`
	ID              string          `json:"id"`
	Act             string          `json:"act"`
	At              uint64          `json:"at"`
	Beat            string          `json:"beat"`
	Fields          json.RawMessage `json:"fields"`
	Reason          string          `json:"reason"`
	LoggedAt        time.Time       `json:"logged_at"`
}

func (s *RecordCaptureSuite) lines(raw string) []capturedLine {
	lines := strings.Split(strings.TrimSpace(raw), "\n")
	out := make([]capturedLine, 0, len(lines))
	for _, line := range lines {
		var v capturedLine
		s.Require().NoError(json.Unmarshal([]byte(line), &v))
		out = append(out, v)
	}
	return out
}

func captureEvent(id string) sdk.RecordEvent {
	return sdk.RecordEvent{Session: "run", ID: id, Act: "time:act", At: 3, Beat: "behavior_decision",
		Fields: json.RawMessage(`{"bool":false,"count":0,"label":"","huge":9007199254740993,"reason":"a < b"}`)}
}

func (s *RecordCaptureSuite) TestWritesSameFieldsWithoutSubscribers() {
	var buf bytes.Buffer
	capture, err := NewRecordCapture(&RecordCaptureConfig{Writer: &buf, CaptureID: "capture-test"})
	s.Require().NoError(err)
	broker := NewRecordBroker(1)
	publisher := &recordPublisher{broker: broker, capture: capture}
	event := captureEvent("one")
	s.Require().NoError(publisher.PublishRecord(context.Background(), []sdk.RecordEvent{event}))
	s.Require().NoError(capture.Drain(context.Background()))
	rows := s.lines(buf.String())
	s.Require().Len(rows, 3)
	s.Equal("started", rows[0].Status)
	s.False(rows[0].HistoryComplete)
	s.Equal("record", rows[1].Kind)
	s.Equal(1, rows[1].Schema)
	s.Equal("capture-test", rows[1].Capture)
	s.Equal(event.Session, rows[1].Session)
	s.Equal(event.ID, rows[1].ID)
	s.Equal(event.Act, rows[1].Act)
	s.Equal(event.At, rows[1].At)
	s.Equal(event.Beat, rows[1].Beat)
	s.Equal(string(event.Fields), string(rows[1].Fields), "opaque bytes preserve integers and strings without reinterpreting them")
	s.False(rows[1].LoggedAt.IsZero())
	s.Equal("drained", rows[2].Status)
	s.False(rows[2].HistoryComplete, "a clean shutdown does not invent earlier history")
}

func (s *RecordCaptureSuite) TestSlowWriterDoesNotBlockPublishAndOwnsAcceptedBytes() {
	var buf bytes.Buffer
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	s.T().Cleanup(func() { once.Do(func() { close(release) }) })
	writer := recordWriterFunc(func(p []byte) (int, error) {
		select {
		case <-started:
		default:
			close(started)
			<-release
		}
		return buf.Write(p)
	})
	capture, err := NewRecordCapture(&RecordCaptureConfig{Writer: writer, Capacity: 1})
	s.Require().NoError(err)
	<-started
	events := []sdk.RecordEvent{captureEvent("one")}
	original := string(events[0].Fields)
	published := make(chan error, 1)
	go func() { published <- capture.PublishRecord(context.Background(), events) }()
	select {
	case err := <-published:
		s.Require().NoError(err)
	case <-time.After(3 * time.Second):
		s.FailNow("publishing waited for the capture writer")
	}
	events[0].ID = "changed"
	events[0].Fields[0] = '!'
	once.Do(func() { close(release) })
	s.Require().NoError(capture.Drain(context.Background()))
	rows := s.lines(buf.String())
	s.Equal("one", rows[1].ID)
	s.Equal(original, string(rows[1].Fields))
}

func (s *RecordCaptureSuite) TestOverflowIsExplicitAndDoesNotStarveBrowserDelivery() {
	var buf bytes.Buffer
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	s.T().Cleanup(func() { once.Do(func() { close(release) }) })
	writes := 0
	writer := recordWriterFunc(func(p []byte) (int, error) {
		writes++
		if writes == 1 {
			close(started)
			<-release
		}
		return buf.Write(p)
	})
	reported := make(chan error, 1)
	capture, err := NewRecordCapture(&RecordCaptureConfig{Writer: writer, Capacity: 1, ReportError: func(err error) { reported <- err }})
	s.Require().NoError(err)
	<-started
	broker := NewRecordBroker(3)
	sub, err := broker.Subscribe("run")
	s.Require().NoError(err)
	defer func() { _ = sub.Close() }()
	publisher := &recordPublisher{broker: broker, capture: capture}
	s.Require().NoError(publisher.PublishRecord(context.Background(), []sdk.RecordEvent{captureEvent("one")}))
	s.ErrorIs(publisher.PublishRecord(context.Background(), []sdk.RecordEvent{captureEvent("two")}), ErrRecordCaptureOverflow)
	first, second := <-sub.Units(), <-sub.Units()
	s.Equal("one", first[0].ID)
	s.Equal("two", second[0].ID, "a degraded capture does not drop browser delivery")
	once.Do(func() { close(release) })
	s.ErrorIs(capture.Drain(context.Background()), ErrRecordCaptureOverflow)
	rows := s.lines(buf.String())
	s.Require().Len(rows, 3)
	s.Equal("one", rows[1].ID, "already accepted work still drains")
	s.Equal("incomplete", rows[2].Status)
	s.Equal("queue_full", rows[2].Reason)
	s.ErrorIs(<-reported, ErrRecordCaptureOverflow)
	s.ErrorIs(capture.PublishRecord(context.Background(), []sdk.RecordEvent{captureEvent("three")}), ErrRecordCaptureOverflow)
}

func (s *RecordCaptureSuite) TestWriteFailureAndShortWriteAreExplicit() {
	for _, short := range []bool{false, true} {
		s.Run(map[bool]string{false: "failure", true: "short write"}[short], func() {
			boom := errors.New("capture disk unavailable")
			writer := recordWriterFunc(func(p []byte) (int, error) {
				if short {
					return len(p) - 1, nil
				}
				return 0, boom
			})
			reported := make(chan error, 1)
			capture, err := NewRecordCapture(&RecordCaptureConfig{Writer: writer, ReportError: func(err error) { reported <- err }})
			s.Require().NoError(err)
			err = capture.Drain(context.Background())
			s.ErrorIs(err, ErrRecordCaptureFailed)
			if short {
				s.ErrorIs(err, io.ErrShortWrite)
			} else {
				s.ErrorIs(err, boom)
			}
			s.ErrorIs(<-reported, ErrRecordCaptureFailed)
		})
	}
}

func (s *RecordCaptureSuite) TestDrainWaitsForAcceptedBatchesAndHonorsDeadline() {
	var buf bytes.Buffer
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	s.T().Cleanup(func() { once.Do(func() { close(release) }) })
	writes := 0
	writer := recordWriterFunc(func(p []byte) (int, error) {
		writes++
		if writes == 1 {
			close(started)
			<-release
		}
		return buf.Write(p)
	})
	capture, err := NewRecordCapture(&RecordCaptureConfig{Writer: writer})
	s.Require().NoError(err)
	<-started
	s.Require().NoError(capture.PublishRecord(context.Background(), []sdk.RecordEvent{captureEvent("one")}))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.ErrorIs(capture.Drain(ctx), context.Canceled)
	s.ErrorIs(capture.PublishRecord(context.Background(), []sdk.RecordEvent{captureEvent("late")}), ErrRecordCaptureClosed)
	once.Do(func() { close(release) })
	s.Require().NoError(capture.Drain(context.Background()))
	s.Equal("one", s.lines(buf.String())[1].ID)
}

func (s *RecordCaptureSuite) TestDoesNotClaimCompleteMidRunCapture() {
	var buf bytes.Buffer
	capture, err := NewRecordCapture(&RecordCaptureConfig{Writer: &buf})
	s.Require().NoError(err)
	s.Require().NoError(capture.PublishRecord(context.Background(), []sdk.RecordEvent{captureEvent("already-running")}))
	s.Require().NoError(capture.Drain(context.Background()))
	rows := s.lines(buf.String())
	s.False(rows[0].HistoryComplete)
	s.Equal("already-running", rows[1].ID)
	s.Equal("behavior_decision", rows[1].Beat, "capture does not manufacture absent earlier configuration")
	s.False(rows[2].HistoryComplete)
}

func (s *RecordCaptureSuite) TestPanickingWriterDoesNotPanicTheServer() {
	reported := make(chan error, 1)
	capture, err := NewRecordCapture(&RecordCaptureConfig{Writer: recordWriterFunc(func([]byte) (int, error) { panic("broken writer") }), ReportError: func(err error) { reported <- err }})
	s.Require().NoError(err)
	s.ErrorIs(capture.Drain(context.Background()), ErrRecordCaptureFailed)
	s.ErrorContains(<-reported, "broken writer")
}

func (s *RecordCaptureSuite) TestInvalidConfigurationRefuses() {
	for _, cfg := range []*RecordCaptureConfig{nil, {}, {Writer: io.Discard, Capacity: -1}} {
		capture, err := NewRecordCapture(cfg)
		s.Error(err)
		s.Nil(capture)
	}
}
