package authoringv1alpha1_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter/dungeonspec"

	authoringpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/authoring/v1alpha1"
	"github.com/KirkDiggler/rpg-api/internal/dungeons"
	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	authoringhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/authoring/v1alpha1"
	authoringorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/authoring"
)

// gateBarredStep is gate-1's step when someone walks into him at his post:
// to challenged, pick from the barred list, mark the moment.
const gateBarredStep = "behaviorDefinitions.gate-1-policy.recipe.states.posted.transitions[3]"

// TestPutDungeon_ValidateOnly_CarriesAStepsActs: every act of a step is a
// connection (encounter v0.228.0, rpg-project#1001), with where it sits in
// the step: the castle gate guard's barred step does three things.
func (s *ReadBackWireSuite) TestPutDungeon_ValidateOnly_CarriesAStepsActs() {
	resp := s.gradedCastle()
	s.Require().Empty(resp.GetErrors())

	acts := map[string]*authoringpb.ReadBackConnection{}
	for _, st := range statements(resp.GetReadBack()) {
		if st.GetPath() != gateBarredStep || st.GetKind() != "routine.wake" {
			continue
		}
		for _, c := range st.GetConnects() {
			if c.GetKind() == "act" {
				acts[c.GetId()] = c
			}
		}
	}
	s.Require().Contains(acts, "to:challenged")
	s.Equal("to", acts["to:challenged"].GetAt())
	s.Require().Contains(acts, "pick:barred")
	s.Equal("do[0]", acts["pick:barred"].GetAt())
	s.Require().Contains(acts, "anchor")
	s.Equal("does", acts["anchor"].GetRole())
	s.Equal("do[1]", acts["anchor"].GetAt())
}

// TestPutDungeon_ValidateOnly_AnActCarriesItsValue: an act's value crosses
// as a JSON literal (a count "3", a flag's value "true"); the castle's acts
// carry none, so a small hall's step is given two that do.
func (s *ReadBackWireSuite) TestPutDungeon_ValidateOnly_AnActCarriesItsValue() {
	const key = "my-memory-hall"
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "orchestrators", "authoring", "testdata", "behavior-patrol-consumer.yaml"))
	s.Require().NoError(err)
	src := string(dungeonstest.AsKey(s.T(), raw, "behavior-patrol-consumer", key))
	edited := strings.Replace(src, "- {on: time, do: [{pick: look}]}",
		"- {on: time, do: [{pick: look}, {wake_in: 3}, {set: {key: seen, value: true}}]}", 1)
	edited = strings.Replace(edited, "      initial: idle\n", "      initial: idle\n      bag: {seen: false}\n", 1)
	s.Require().NotEqual(src, edited)

	resp := s.grade(key, edited)
	s.Require().Empty(resp.GetErrors())
	acts := map[string]*authoringpb.ReadBackConnection{}
	for _, st := range statements(resp.GetReadBack()) {
		for _, c := range st.GetConnects() {
			if c.GetKind() == "act" {
				acts[c.GetId()] = c
			}
		}
	}
	s.Require().Contains(acts, "wake_in")
	s.Equal("3", acts["wake_in"].GetIs())
	s.Equal("do[1]", acts["wake_in"].GetAt())
	s.Require().Contains(acts, "set:seen")
	s.Equal("true", acts["set:seen"].GetIs(), "a JSON literal")
	s.Equal("do[2]", acts["set:seen"].GetAt())
}

// ShippedCastleHashSuite: GetDungeon on the shipped castle returns the hash
// the engine takes of it. The hash's encoding is the engine release's, so
// it is compared with the engine's own, never pinned.
type ShippedCastleHashSuite struct{ suite.Suite }

func TestShippedCastleHashSuite(t *testing.T) { suite.Run(t, new(ShippedCastleHashSuite)) }

func (s *ShippedCastleHashSuite) TestGetDungeonReturnsTheEnginesHash() {
	raw, err := os.ReadFile(filepath.Join(dungeonstest.ContentDir(s.T()), dungeons.DefaultKey+".yaml")) //nolint:gosec // the shipped castle
	s.Require().NoError(err)
	dir := s.T().TempDir()
	s.Require().NoError(os.WriteFile(filepath.Join(dir, dungeons.DefaultKey+".yaml"), raw, 0o600))
	shipped, err := dungeons.NewFileRegistry(dir, dungeonstest.Projector(s.T()))
	s.Require().NoError(err)
	orch, err := authoringorch.New(&authoringorch.Config{Dungeons: dungeonstest.Join(s.T(), shipped, false)})
	s.Require().NoError(err)
	h, err := authoringhandler.New(&authoringhandler.HandlerConfig{Orchestrator: orch})
	s.Require().NoError(err)

	got, err := h.GetDungeon(worldCtx("alice"), &authoringpb.GetDungeonRequest{Key: dungeons.DefaultKey})
	s.Require().NoError(err)

	compiled, err := dungeonspec.Load(raw)
	s.Require().NoError(err)
	want, err := compiled.ContentHash()
	s.Require().NoError(err)
	s.Equal(string(want), got.GetContentHash())
}
