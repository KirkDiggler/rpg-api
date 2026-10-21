package session_test

// TEMPORARY (rpg-project#631, E5c c1): the v1 event stream's test helpers,
// kept only so this package compiles while each scene moves to the act
// stream. Every call fails the test that makes it. This file is deleted
// before the PR is ready.

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
)

type recordingStream struct {
	grpc.ServerStream
	ctx context.Context
}

func newRecordingStream(ctx context.Context) *recordingStream { return &recordingStream{ctx: ctx} }

func (s *recordingStream) Context() context.Context     { return s.ctx }
func (s *recordingStream) Send(*sessionpb.Event) error  { return nil }
func (s *recordingStream) snapshot() []*sessionpb.Event { return nil }
func waitForLive(t *testing.T, _ any, _, _ string, _ *recordingStream) {
	t.Helper()
	t.Fatal("v1 StreamEvents is deleted (rpg-project#631): move this scene to the act stream")
}
func waitForQuiescence(t *testing.T, _ *recordingStream, _ time.Duration) []*sessionpb.Event {
	t.Helper()
	t.Fatal("v1 StreamEvents is deleted (rpg-project#631): move this scene to the act stream")
	return nil
}
func containsKind(events []*sessionpb.Event, kind sessionpb.EventKind) bool {
	for _, e := range events {
		if e.GetKind() == kind {
			return true
		}
	}
	return false
}
func assertContiguousSeqs(t *testing.T, _ []*sessionpb.Event, _ string) {
	t.Helper()
	t.Fatal("v1 seq contiguity has no act-stream meaning (rpg-project#631)")
}
