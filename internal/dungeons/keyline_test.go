package dungeons

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type KeyLineSuite struct{ suite.Suite }

func TestKeyLineSuite(t *testing.T) { suite.Run(t, new(KeyLineSuite)) }

func (s *KeyLineSuite) TestPlainLine() {
	got, err := rewriteKeyLine([]byte("version: 4\nkey: my-castle\nname: x\n"), "my-castle", "my-keep")
	s.Require().NoError(err)
	s.Equal("version: 4\nkey: my-keep\nname: x\n", string(got))
}

func (s *KeyLineSuite) TestKeptComment() {
	got, err := rewriteKeyLine([]byte("key: my-castle   # the author's note\nversion: 4\n"), "my-castle", "my-keep")
	s.Require().NoError(err)
	s.Equal("key: my-keep   # the author's note\nversion: 4\n", string(got))
}

func (s *KeyLineSuite) TestQuotedKey() {
	for _, line := range []string{`key: "my-castle"`, `key: 'my-castle'`} {
		got, err := rewriteKeyLine([]byte("version: 4\n"+line+"\n"), "my-castle", "my-keep")
		s.Require().NoError(err, line)
		s.Equal("version: 4\nkey: my-keep\n", string(got), line)
	}
}

func (s *KeyLineSuite) TestCarriageReturnKept() {
	got, err := rewriteKeyLine([]byte("version: 4\r\nkey: my-castle\r\nname: x\r\n"), "my-castle", "my-keep")
	s.Require().NoError(err)
	s.Equal("version: 4\r\nkey: my-keep\r\nname: x\r\n", string(got))
}

func (s *KeyLineSuite) TestZeroMatches() {
	_, err := rewriteKeyLine([]byte("version: 4\nkey: other\n"), "my-castle", "my-keep")
	s.Error(err)
}

func (s *KeyLineSuite) TestTwoMatches() {
	_, err := rewriteKeyLine([]byte("key: my-castle\nversion: 4\nkey: my-castle\n"), "my-castle", "my-keep")
	s.Error(err)
}

func (s *KeyLineSuite) TestANestedKeyIsNotTheRootLine() {
	raw := []byte("version: 4\nkey: my-castle\nroom:\n  key: my-castle\n")
	got, err := rewriteKeyLine(raw, "my-castle", "my-keep")
	s.Require().NoError(err)
	s.Equal("version: 4\nkey: my-keep\nroom:\n  key: my-castle\n", string(got))

	_, err = rewriteKeyLine([]byte("version: 4\nroom:\n  key: my-castle\n"), "my-castle", "my-keep")
	s.Error(err, "only a nested key line is zero root lines")
}

func (s *KeyLineSuite) TestAPrefixIsNotTheKey() {
	_, err := rewriteKeyLine([]byte("key: my-castle-two\n"), "my-castle", "my-keep")
	s.Error(err)
}
