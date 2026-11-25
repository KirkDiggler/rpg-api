package authoringv1alpha1_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/status"

	authoringpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/authoring/v1alpha1"
	dungeonsmock "github.com/KirkDiggler/rpg-api/internal/dungeons/mock"
	authoringhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/authoring/v1alpha1"
	authoringorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/authoring"
)

// StatusWordsSuite proves the status words and a status's sentence cross the
// wire as the engine gives them: every word, every field, and a refusal
// exactly as the rules say it (rpg-project#1097).
type StatusWordsSuite struct {
	suite.Suite

	handler *authoringhandler.Handler
}

func TestStatusWordsSuite(t *testing.T) { suite.Run(t, new(StatusWordsSuite)) }

// SetupSuite builds one handler; the registry is a mock that expects no call
// because the words and the sentence read no dungeon.
func (s *StatusWordsSuite) SetupSuite() {
	orch, err := authoringorch.New(&authoringorch.Config{Dungeons: dungeonsmock.NewMockRegistry(gomock.NewController(s.T()))})
	s.Require().NoError(err)
	s.handler, err = authoringhandler.New(&authoringhandler.HandlerConfig{Orchestrator: orch})
	s.Require().NoError(err)
}

// TestEveryWordAndEveryFieldCrosses: the response equals status.Words, field
// by field, by reflection over status.Word, status.Value and status.Bounds.
func (s *StatusWordsSuite) TestEveryWordAndEveryFieldCrosses() {
	resp, err := s.handler.GetStatusWords(worldCtx("alice"), &authoringpb.GetStatusWordsRequest{})
	s.Require().NoError(err)

	table := status.Words(&status.WordsInput{}).Words
	s.Require().NotEmpty(table)
	field := resp.ProtoReflect().Descriptor().Fields().ByName("words")
	s.Require().NotNil(field)
	s.requireValue("Words", reflect.ValueOf(table), resp.ProtoReflect().Get(field), field)
	s.Regexp(`^v\d+\.\d+\.\d+`, resp.GetRulebookVersion(),
		"the rulebook release from the binary's build info, which a test binary carries too")
}

const feasted = `{"name":"Feasted","changes":[{"max_hit_points":2}],"when_given":[{"heal":2}],"maintenance":[{"ends_on":"run"}],"again":"ignored"}`

// TestTheFeastIsSaid: the sentence and parts equal what status.Say returns.
func (s *StatusWordsSuite) TestTheFeastIsSaid() {
	want, err := status.Say(&status.SayInput{ID: "feasted", Status: json.RawMessage(feasted)})
	s.Require().NoError(err)
	s.Require().Len(want.Parts, 4)

	resp, err := s.handler.SayStatus(worldCtx("alice"), &authoringpb.SayStatusRequest{Id: "feasted", StatusJson: feasted})
	s.Require().NoError(err)
	s.Empty(resp.GetErrors())
	s.Equal(want.Name, resp.GetName())
	s.Equal(want.Sentence, resp.GetSentence())
	s.Require().Len(resp.GetParts(), len(want.Parts))
	for i, p := range want.Parts {
		s.Equal(p.Path, resp.GetParts()[i].GetPath())
		s.Equal(p.Text, resp.GetParts()[i].GetText())
	}
}

// TestARefusalMarksItsRow: a refusal is one error with the rules' own path
// and message; a whole-status refusal has an empty path.
func (s *StatusWordsSuite) TestARefusalMarksItsRow() {
	for name, tc := range map[string]struct{ json, path string }{
		"a repeated word":       {`{"name":"Held","changes":[{"cannot":["move","fly"]}],"maintenance":[{"ends_on":"run"}],"again":"ignored"}`, "changes[0].cannot[1]"},
		"a number out of range": {`{"name":"Quick","changes":[{"initiative":5000}],"maintenance":[{"ends_on":"run"}],"again":"ignored"}`, "changes[0].initiative"},
		"not an object":         {`[1,2]`, ""},
	} {
		s.Run(name, func() {
			_, err := status.Say(&status.SayInput{ID: "held", Status: json.RawMessage(tc.json)})
			var refusal *status.Refusal
			s.Require().True(errors.As(err, &refusal), "the rules refuse this input")
			s.Equal(tc.path, refusal.Path)

			resp, err := s.handler.SayStatus(worldCtx("alice"), &authoringpb.SayStatusRequest{Id: "held", StatusJson: tc.json})
			s.Require().NoError(err)
			s.Empty(resp.GetSentence())
			s.Empty(resp.GetParts())
			s.Require().Len(resp.GetErrors(), 1)
			s.Equal(refusal.Path, resp.GetErrors()[0].GetPath())
			s.Equal(refusal.Says, resp.GetErrors()[0].GetMessage())
		})
	}
}

// TestGatedOnSignIn: like every RPC on this service, a caller with no player
// is refused.
func (s *StatusWordsSuite) TestGatedOnSignIn() {
	_, err := s.handler.GetStatusWords(s.T().Context(), &authoringpb.GetStatusWordsRequest{})
	s.Require().Error(err)
	_, err = s.handler.SayStatus(s.T().Context(), &authoringpb.SayStatusRequest{})
	s.Require().Error(err)
}

// requireSame asserts that every exported field of the Go value has the same
// value in the message, recursively. A Go field with no wire field fails.
func (s *StatusWordsSuite) requireSame(path string, v reflect.Value, m protoreflect.Message) {
	s.T().Helper()
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		fd := m.Descriptor().Fields().ByName(protoreflect.Name(snake(f.Name)))
		s.Require().NotNilf(fd, "%s.%s has no wire field %q", path, f.Name, snake(f.Name))
		s.requireValue(path+"."+f.Name, v.Field(i), m.Get(fd), fd)
	}
}

func (s *StatusWordsSuite) requireValue(path string, v reflect.Value, got protoreflect.Value, fd protoreflect.FieldDescriptor) {
	s.T().Helper()
	switch {
	case fd.IsList():
		list := got.List()
		s.Require().Equalf(v.Len(), list.Len(), "%s: length", path)
		for i := 0; i < v.Len(); i++ {
			elem := v.Index(i)
			if elem.Kind() == reflect.Struct {
				s.requireSame(path, elem, list.Get(i).Message())
				continue
			}
			s.Equalf(elem.String(), list.Get(i).String(), "%s[%d]", path, i)
		}
	case v.Kind() == reflect.Pointer:
		if v.IsNil() {
			s.Falsef(hasAny(got.Message()), "%s: nil on the engine, set on the wire", path)
			return
		}
		s.Truef(got.Message().IsValid(), "%s: set on the engine, absent on the wire", path)
		s.requireSame(path, v.Elem(), got.Message())
	case v.Kind() == reflect.Struct:
		s.requireSame(path, v, got.Message())
	case v.Kind() == reflect.String:
		s.Equalf(v.String(), got.String(), "%s", path)
	case v.Kind() == reflect.Bool:
		s.Equalf(v.Bool(), got.Bool(), "%s", path)
	case v.Kind() == reflect.Int:
		s.Equalf(v.Int(), got.Int(), "%s", path)
	default:
		s.FailNowf("a field of a kind this test cannot compare", "%s is %s", path, v.Kind())
	}
}

// hasAny is whether any field of the message is set.
func hasAny(m protoreflect.Message) bool {
	found := false
	m.Range(func(protoreflect.FieldDescriptor, protoreflect.Value) bool { found = true; return false })
	return found
}
