package session_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"

	sessionpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha1"
	recordpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/session/v1alpha2"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	"github.com/KirkDiggler/rpg-api/internal/entities"
	recordhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/session/v1alpha2"
	"github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/sessionaccess"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	characterrepo "github.com/KirkDiggler/rpg-api/internal/repositories/character"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
)

func (s *BehaviorPatrolSuite) TestMachineTraceCaptureSurvivesReload() {
	var buf bytes.Buffer
	capture, err := sessionorch.NewRecordCapture(&sessionorch.RecordCaptureConfig{Writer: &buf, CaptureID: "api-proof"})
	s.Require().NoError(err)
	ctx := auth.WithPlayerID(context.Background(), "player-alice")
	die := &patrolDice{}
	h := newAcceptanceHarnessRecording(s.T(), die, nil, false, capture)
	s.T().Cleanup(func() {
		drain, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		s.NoError(h.manager.Drain(drain))
		s.NoError(capture.Drain(drain))
	})
	_, err = h.charRepo.Create(ctx, characterrepo.CreateInput{Character: &entities.Character{Data: armedFighter("alice", "player-alice")}})
	s.Require().NoError(err)
	raw, err := os.ReadFile(filepath.Join(dungeonstest.ContentDir(s.T()), "..", "internal", "orchestrators", "authoring", "testdata", "behavior-patrol-consumer.yaml"))
	s.Require().NoError(err)
	// A valid authored scalar beyond the Struct/JS safe-integer boundary.
	// It is configuration, not fabricated runtime progress.
	wideSource := strings.Replace(string(raw), "initial: idle", "initial: idle\n      bag: {inspection-count: 9007199254740993}", 1)
	s.Require().NotEqual(string(raw), wideSource)
	dungeon, err := sessionworld.Compile([]byte(wideSource))
	s.Require().NoError(err)
	const run = "inspection-capture-run"
	h.launchAtStart(s.T(), run, dungeon.Spec, "alice")
	original := s.storedBehavior(h, run, "west").Behaviors.Definition
	dungeon.Spec.Monsters[0].Behaviors.Instances[1].Instance.Recipe.Ref.ID = "edited-after-launch"
	for i, path := range [][]*sessionpb.Position{
		{{X: 1, Y: 2}, {X: 2, Y: 2}, {X: 3, Y: 2}, {X: 4, Y: 2}, {X: 5, Y: 2}, {X: 6, Y: 2}},
		{{X: 5, Y: 2}, {X: 4, Y: 2}, {X: 3, Y: 2}, {X: 2, Y: 2}, {X: 1, Y: 2}, {X: 0, Y: 2}},
	} {
		if i > 0 {
			s.reloadRecordingBehaviorHost(h, die, "inspection-reload", capture)
		}
		moved, moveErr := h.handler.Move(ctx, &sessionpb.MoveRequest{Session: run, Member: "alice", Path: path})
		s.Require().NoError(moveErr)
		s.awaitBehaviorMove(h, run, moved, path[len(path)-1])
	}
	s.Equal(4, die.choices)
	s.Equal(original, s.storedBehavior(h, run, "west").Behaviors.Definition)
	access, err := sessionaccess.New(h.charRepo, h.manager.Manager)
	s.Require().NoError(err)
	disabled, err := recordhandler.New(&recordhandler.HandlerConfig{Manager: h.manager.Manager, Broker: h.manager.RecordBroker, Gate: access})
	s.Require().NoError(err)
	_, err = disabled.GetRecord(ctx, &recordpb.GetRecordRequest{Session: run, Member: "alice"})
	s.Equal(codes.PermissionDenied, status.Code(err))
	view, err := h.handler.GetView(ctx, &sessionpb.GetViewRequest{Session: run, Member: "alice"})
	s.Require().NoError(err, "ordinary play is independent of the debug gate")
	s.NotContains(protojson.Format(view), "behavior_decision")
	s.NotContains(protojson.Format(view), "item/0/walk")
	reader, err := recordhandler.New(&recordhandler.HandlerConfig{Manager: h.manager.Manager, Broker: h.manager.RecordBroker, Gate: access, Enabled: true})
	s.Require().NoError(err)
	page, err := reader.GetRecord(ctx, &recordpb.GetRecordRequest{Session: run, Member: "alice", Limit: 500})
	s.Require().NoError(err)
	s.False(page.More)
	byID := map[string]*recordpb.RecordEvent{}
	for _, event := range page.Events {
		byID[event.Id] = event
	}
	drain, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	s.Require().NoError(h.manager.Drain(drain))
	s.Require().NoError(capture.Drain(drain))
	counts := map[string]int{}
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		var captured struct {
			Kind    string          `json:"kind"`
			ID      string          `json:"id"`
			Session string          `json:"session"`
			Act     string          `json:"act"`
			Beat    string          `json:"beat"`
			Fields  json.RawMessage `json:"fields"`
		}
		s.Require().NoError(json.Unmarshal([]byte(line), &captured))
		if captured.Kind != "record" {
			continue
		}
		s.Equal(run, captured.Session)
		actual, ok := byID[captured.ID]
		s.Require().True(ok, "captured ID remains available in the normal retained record")
		s.Equal(actual.Act, captured.Act)
		s.Equal(actual.Beat, captured.Beat)
		s.Equal(string(captured.Fields), string(actual.GetFieldsJson()), "live/catch-up wire and capture preserve the same owner bytes")
		counts[captured.Beat]++
	}
	s.Contains(buf.String(), "9007199254740993", "the real compiler/host capture retains the original integer")
	s.Equal(2, counts[encounter.BeatBehaviorConfig], "source edit/reload does not rebind or duplicate symbols")
	s.Positive(counts[encounter.BeatBehaviorDecision])
	s.Positive(counts[encounter.BeatBehaviorWork])
}
