package authoringv1alpha1_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	authoringpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/authoring/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/auth"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	authoringhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/authoring/v1alpha1"
	authoringorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/authoring"
)

// frontRoomKey is the shipped fixture these probes mutate: the file the
// design measured its three refusals against, and the one with a faction
// graph and an answer table to misspell.
const frontRoomKey = "reference-front-room"

// ValidateOnlyWireSuite drives PutDungeon{validate_only} end to end over a
// REAL registry constructed READ-ONLY — the shape a server boots with when
// RPG_AUTHORING_ENABLED is unset.
//
// It is the api half of rpg-project#481. Two claims, and the second is why
// the first matters: the builder's per-edit preview is answered by a registry
// that will not store a byte, and what it answers with is the ENGINE'S own
// path and sentence, not a grammar rpg-api or the web wrote down a second
// time. Where handler_test.go drives a mocked registry to pin transport, this
// one compiles real files through the real dungeonspec so the words an author
// sees are the words this stack actually produces.
type ValidateOnlyWireSuite struct {
	suite.Suite

	ctx     context.Context
	handler *authoringhandler.Handler

	// yaml is the shipped front room, verbatim, and the base every probe
	// mutates by exactly one token.
	yaml string
}

func TestValidateOnlyWireSuite(t *testing.T) {
	suite.Run(t, new(ValidateOnlyWireSuite))
}

func (s *ValidateOnlyWireSuite) SetupTest() {
	s.ctx = auth.WithPlayerID(context.Background(), "alice")

	registry, dir := dungeonstest.ScratchReadOnly(s.T())
	orch, err := authoringorch.New(&authoringorch.Config{Dungeons: registry})
	s.Require().NoError(err)
	h, err := authoringhandler.New(&authoringhandler.HandlerConfig{Orchestrator: orch})
	s.Require().NoError(err)
	s.handler = h

	raw, err := os.ReadFile(filepath.Join(dir, frontRoomKey+".yaml")) //nolint:gosec // dir is t.TempDir()
	s.Require().NoError(err)
	s.yaml = string(raw)
}

// grade sends one file through PutDungeon{validate_only}. Nothing here is
// ever a status: a file that does not compile is an OK body carrying the
// list, which is the whole reason the builder can show defects inline.
func (s *ValidateOnlyWireSuite) grade(yaml string) *authoringpb.PutDungeonResponse {
	s.T().Helper()

	resp, err := s.handler.PutDungeon(s.ctx, &authoringpb.PutDungeonRequest{
		Key: frontRoomKey, Yaml: yaml, ValidateOnly: true,
	})
	s.Require().NoError(err, "a grade is never a status, least of all on a read-only registry")

	return resp
}

// mutate replaces the FIRST occurrence of old and fails when the fixture no
// longer holds it: a probe that silently stopped mutating would grade a good
// file and pass, proving nothing.
func (s *ValidateOnlyWireSuite) mutate(old, replacement string) string {
	s.T().Helper()

	out := strings.Replace(s.yaml, old, replacement, 1)
	s.Require().NotEqual(s.yaml, out, "the front room no longer holds %q", old)

	return out
}

// TestAGoodFileGradesOnAReadOnlyRegistry: the shipped front room, unedited,
// comes back with no defects and the atlas the game would play — from a
// registry that refuses to write.
func (s *ValidateOnlyWireSuite) TestAGoodFileGradesOnAReadOnlyRegistry() {
	resp := s.grade(s.yaml)

	s.Require().Empty(resp.GetErrors(), "the shipped file compiles")
	s.Require().NotNil(resp.GetAtlas(), "and the grade carries the map")
	s.Equal(frontRoomKey, resp.GetAtlas().GetDungeonKey(),
		"the preview names its dungeon the way a session's atlas does")
}

// TestMissingFactionNamesThePlacement is the design's first probe: a
// placement standing in a faction nobody declared. The engine names the
// placement by index and the word the author typed.
func (s *ValidateOnlyWireSuite) TestMissingFactionNamesThePlacement() {
	resp := s.grade(s.mutate("faction: goblins", "faction: gobins"))

	s.Require().Len(resp.GetErrors(), 1)
	s.Equal("place[0].faction", resp.GetErrors()[0].GetPath())
	s.Contains(resp.GetErrors()[0].GetMessage(), "gobins",
		"the refusal quotes the word the author typed, so the builder needs no dictionary")
	s.Nil(resp.GetAtlas(), "no map for a file that does not compile")
}

// TestTypoedTriggerKeyNamesTheKey is the second probe: a misspelled trigger
// on the faction's answer table. The engine lists the triggers this build
// rolls, at the path the key sits on; that list is the engine's to keep, and
// nobody downstream has to hold a copy of it.
func (s *ValidateOnlyWireSuite) TestTypoedTriggerKeyNamesTheKey() {
	resp := s.grade(s.mutate("intimidate_failed:", "intimdate_failed:"))

	s.Require().Len(resp.GetErrors(), 1)
	s.Equal("factions[0].on.intimdate_failed", resp.GetErrors()[0].GetPath())
	s.Contains(resp.GetErrors()[0].GetMessage(), "intimdate_failed")
	s.Nil(resp.GetAtlas())
}

// TestUnknownKeyIsPathed is the third probe, and the one the design called
// the defect in the contract: in the v2 dialect an unknown key used to
// surface as a DECODE failure carrying a line number and the name of a Go
// type the author has never heard of. Since encounter v0.94.1
// (rpg-project#481 slice 1) the dialect walks the shape and names the key by
// path, as the single-room dialect already did, so both halves of the
// grammar refuse in one voice.
//
// THE PASSTHROUGH CONTRACT, not the engine's sentence. What api owns here is
// that the defect SURFACES at the toolkit's path, naming the key the author
// typed, free of a line number or a Go type name. The engine's full
// vocabulary is the toolkit's to change — a word added there must not break
// this passthrough — so the message is probed for those three properties
// rather than pinned to the exact string.
func (s *ValidateOnlyWireSuite) TestUnknownKeyIsPathed() {
	resp := s.grade(s.mutate("temper: { coward: 2", "tempre: { coward: 2"))

	s.Require().Len(resp.GetErrors(), 1)
	s.Equal("factions[0].tempre", resp.GetErrors()[0].GetPath())
	msg := resp.GetErrors()[0].GetMessage()
	s.Contains(msg, "tempre",
		"the refusal names the key the author typed, so the builder needs no dictionary")
	s.Contains(msg, "not a key this build reads",
		"the sentence is the engine's named-key refusal, carried through unchanged")
	s.NotContains(msg, "dungeonspec.",
		"the author never sees a Go type name")
	s.NotContains(msg, "line ",
		"nor a line number — the path is the only location an author needs")
	s.Nil(resp.GetAtlas())
}

// TestARealSaveIsStillRefused: the gate did not move, it narrowed. The same
// registry that graded every file above refuses to store one, and the
// refusal is still the proto's FailedPrecondition.
func (s *ValidateOnlyWireSuite) TestARealSaveIsStillRefused() {
	_, err := s.handler.PutDungeon(s.ctx, &authoringpb.PutDungeonRequest{
		Key: frontRoomKey, Yaml: s.yaml,
	})

	s.Require().Error(err)
	st, ok := status.FromError(err)
	s.Require().True(ok, "not a status error: %v", err)
	s.Equal(codes.FailedPrecondition, st.Code(), st.Message())
}
