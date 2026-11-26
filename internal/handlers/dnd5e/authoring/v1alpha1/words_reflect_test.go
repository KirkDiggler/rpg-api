package authoringv1alpha1_test

import (
	"reflect"

	"github.com/stretchr/testify/suite"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// wordsReflect is the comparison the words suites share: it holds a table of
// words from the rules against the same table on the wire, field by field, by
// reflection. Embed it where suite.Suite would be.
type wordsReflect struct {
	suite.Suite
}

// requireSame asserts that every exported field of the Go value has the same
// value in the message, recursively. A Go field with no wire field fails.
func (s *wordsReflect) requireSame(path string, v reflect.Value, m protoreflect.Message) {
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

func (s *wordsReflect) requireValue(path string, v reflect.Value, got protoreflect.Value, fd protoreflect.FieldDescriptor) {
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
