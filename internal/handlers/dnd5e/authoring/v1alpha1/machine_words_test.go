package authoringv1alpha1_test

import (
	"context"
	"net"
	"reflect"
	"strings"
	"testing"
	"unicode"

	"github.com/stretchr/testify/suite"
	"go.uber.org/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/test/bufconn"
	"google.golang.org/protobuf/reflect/protoreflect"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter"
	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter/dungeonspec/readback"

	authoringpb "github.com/KirkDiggler/rpg-api-protos/gen/go/dnd5e/api/authoring/v1alpha1"
	dungeonsmock "github.com/KirkDiggler/rpg-api/internal/dungeons/mock"
	authoringhandler "github.com/KirkDiggler/rpg-api/internal/handlers/dnd5e/authoring/v1alpha1"
	authoringorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/authoring"
)

// wireNames are the four Go fields that cross under another name, because a
// wire name must survive every target language (rpg-api-protos#445). Keyed
// by the Go type's name and field; every other field crosses as its
// snake_case.
var wireNames = map[string]string{
	"BeatWord.Names":        "names_someone",
	"Written.In":            "part",
	"Arg.Default":           "default_value",
	"Connection.Not":        "negated",
	"MachineWordsTable.Who": "who_words",
	"WhoWord.Role":          "plays",
	"WhoWord.Allowed":       "allowed_in",
}

// MachineWordsSuite proves the machine words cross the wire WHOLE: every
// word, every field, driven by reflection over the engine's published Go
// types, so a field the engine adds and this api does not copy fails here
// rather than vanishing on the way to the studio.
type MachineWordsSuite struct {
	suite.Suite

	handler *authoringhandler.Handler
}

func TestMachineWordsSuite(t *testing.T) { suite.Run(t, new(MachineWordsSuite)) }

// SetupSuite builds one handler for the suite. The words are the engine's
// table and read no dungeon, so the registry is a mock that expects no call:
// compiling a shelf here would cost seconds and prove nothing about words.
func (s *MachineWordsSuite) SetupSuite() {
	orch, err := authoringorch.New(&authoringorch.Config{Dungeons: dungeonsmock.NewMockRegistry(gomock.NewController(s.T()))})
	s.Require().NoError(err)
	s.handler, err = authoringhandler.New(&authoringhandler.HandlerConfig{Orchestrator: orch})
	s.Require().NoError(err)
}

// TestEveryWordAndEveryFieldCrosses: the response equals the published
// table, field by field, for every beat, condition, act and op.
func (s *MachineWordsSuite) TestEveryWordAndEveryFieldCrosses() {
	resp, err := s.handler.GetMachineWords(worldCtx("alice"), &authoringpb.GetMachineWordsRequest{})
	s.Require().NoError(err)

	table := encounter.MachineWords()
	s.Require().NotEmpty(table.Beats)
	s.Require().NotEmpty(table.Conditions)
	s.Require().NotEmpty(table.Acts)
	s.Require().NotEmpty(table.Ops)
	s.requireSame("MachineWordsTable", reflect.ValueOf(table), resp.ProtoReflect())
	s.Regexp(`^v\d+\.\d+\.\d+`, resp.GetEncounterVersion(),
		"the engine release from the binary's build info, which a test binary carries too")
}

// TestEveryConnectionFieldCrosses: a read-back connection with every field
// set (by reflection, so a new field is set too) arrives with every field.
func (s *MachineWordsSuite) TestEveryConnectionFieldCrosses() {
	var c readback.Connection
	v := reflect.ValueOf(&c).Elem()
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		if !v.Type().Field(i).IsExported() {
			continue
		}
		switch f.Kind() {
		case reflect.String:
			f.SetString("x-" + v.Type().Field(i).Name)
		case reflect.Bool:
			f.SetBool(true)
		case reflect.Int:
			f.SetInt(7)
		default:
			s.FailNowf("a connection field of a kind this test cannot set", "%s is %s", v.Type().Field(i).Name, f.Kind())
		}
	}

	got := authoringhandler.ReadBackConnectionToProto(c)
	s.requireSame("Connection", reflect.ValueOf(c), got.ProtoReflect())
}

// TestGatedOnSignIn: like every read on this service, a caller with no
// player is refused.
func (s *MachineWordsSuite) TestGatedOnSignIn() {
	_, err := s.handler.GetMachineWords(context.Background(), &authoringpb.GetMachineWordsRequest{})
	s.Require().Error(err)
}

// TestOverGRPC: through a real gRPC server and client, the heard beat, the
// heard.claim condition and the set act arrive with their args.
func (s *MachineWordsSuite) TestOverGRPC() {
	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer(grpc.UnaryInterceptor(signedIn))
	authoringpb.RegisterAuthoringServiceServer(srv, s.handler)
	go func() { _ = srv.Serve(lis) }()
	s.T().Cleanup(srv.Stop)

	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return lis.DialContext(ctx) }),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = conn.Close() })

	resp, err := authoringpb.NewAuthoringServiceClient(conn).GetMachineWords(
		metadata.AppendToOutgoingContext(context.Background(), "player", "alice"), &authoringpb.GetMachineWordsRequest{})
	s.Require().NoError(err)

	heard := findWord(resp.GetBeats(), "heard")
	s.Require().NotNil(heard)
	s.True(heard.GetNamesSomeone())
	s.Equal("on", heard.GetWritten().GetPart())
	s.Equal("hears something", heard.GetLabel())

	claim := findWord(resp.GetConditions(), "heard.claim")
	s.Require().NotNil(claim)
	s.Equal([]string{"held", "failed"}, claim.GetValues())
	s.Equal([]string{"heard"}, claim.GetOn())
	s.Equal("when", claim.GetWritten().GetPart())
	s.Equal("fact", claim.GetWritten().GetKey())
	s.Equal("name", claim.GetWritten().GetField())
	s.Equal("it was a claim that failed", claim.GetValueLabels()["failed"])
	s.Require().Len(claim.GetArgs(), 2)
	s.Equal("value", claim.GetArgs()[0].GetName())
	s.Equal("eq", claim.GetArgs()[1].GetFixed())

	rounds := findWord(resp.GetConditions(), "rounds")
	s.Require().NotNil(rounds)
	var n *authoringpb.MachineArg
	for _, a := range rounds.GetArgs() {
		if a.GetName() == "n" {
			n = a
		}
	}
	s.Require().NotNil(n, "rounds takes a count")
	s.True(n.GetOptional(), "the count may be left out (encounter v0.225.0)")
	s.Equal("0", n.GetDefaultValue(), "and leaving it out means none")

	var spokenTo, receiver *authoringpb.MachineWhoWord
	for _, w := range resp.GetWhoWords() {
		if w.GetWord() != "spoken_to" {
			continue
		}
		switch w.GetAllowedIn() {
		case "give.to":
			spokenTo = w
		case "apply_status.to":
			receiver = w
		}
	}
	s.Require().NotNil(spokenTo, "the who-words cross the wire")
	s.Equal("taker", spokenTo.GetPlays())
	s.Equal("give.to", spokenTo.GetAllowedIn())
	s.Equal("judgement", spokenTo.GetWay())
	s.Require().NotNil(receiver, "spoken_to is also a who-word of apply_status.to")
	s.Equal("receiver", receiver.GetPlays())

	set := findWord(resp.GetActs(), "set")
	s.Require().NotNil(set)
	s.Equal("do", set.GetWritten().GetPart())
	s.Require().Len(set.GetArgs(), 2)
	id, value := set.GetArgs()[0], set.GetArgs()[1]
	s.Equal("id", id.GetName())
	s.Equal("field", id.GetGoes())
	s.Equal("key", id.GetField())
	s.Equal("flag", id.GetNames())
	s.Equal("value", value.GetName())
	s.Equal("of", value.GetKind())
	s.Equal("id", value.GetKindOf())
	s.Equal([]string{"bool", "int", "string"}, value.GetKinds())
}

// signedIn stands in for the auth boundary: the player named in metadata.
func signedIn(ctx context.Context, req any, _ *grpc.UnaryServerInfo, next grpc.UnaryHandler) (any, error) {
	md, _ := metadata.FromIncomingContext(ctx)
	if p := md.Get("player"); len(p) == 1 {
		ctx = worldCtx(p[0])
	}
	return next(ctx, req)
}

// wordOf is any machine word message.
type wordOf interface{ GetWord() string }

func findWord[T wordOf](words []T, word string) T {
	var zero T
	for _, w := range words {
		if w.GetWord() == word {
			return w
		}
	}
	return zero
}

// requireSame asserts that every exported field of the Go value has the same
// value in the message, recursively. A Go field with no wire field fails.
func (s *MachineWordsSuite) requireSame(path string, v reflect.Value, m protoreflect.Message) {
	s.T().Helper()
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		if !f.IsExported() {
			continue
		}
		name, ok := wireNames[t.Name()+"."+f.Name]
		if !ok {
			name = snake(f.Name)
		}
		fd := m.Descriptor().Fields().ByName(protoreflect.Name(name))
		s.Require().NotNilf(fd, "%s.%s has no wire field %q", path, f.Name, name)
		s.requireValue(path+"."+f.Name, v.Field(i), m.Get(fd), fd)
	}
}

func (s *MachineWordsSuite) requireValue(path string, v reflect.Value, got protoreflect.Value, fd protoreflect.FieldDescriptor) {
	s.T().Helper()
	switch {
	case fd.IsMap():
		want := map[string]string{}
		for _, k := range v.MapKeys() {
			want[k.String()] = v.MapIndex(k).String()
		}
		have := map[string]string{}
		got.Map().Range(func(k protoreflect.MapKey, val protoreflect.Value) bool {
			have[k.String()] = val.String()
			return true
		})
		s.Equalf(want, have, "%s", path)
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

// snake is a Go field name as its proto field: LabelWith is label_with.
func snake(name string) string {
	var b strings.Builder
	rs := []rune(name)
	for i, r := range rs {
		if unicode.IsUpper(r) && i > 0 && unicode.IsLower(rs[i-1]) {
			b.WriteByte('_')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}
