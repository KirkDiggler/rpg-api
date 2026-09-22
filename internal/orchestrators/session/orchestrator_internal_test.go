package session

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultPresentationIDsUseOneSeparator(t *testing.T) {
	id := newDefaultPresentationIDs().Generate()

	require.Regexp(t,
		regexp.MustCompile(`^presentation_[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`),
		id,
	)
	require.NotContains(t, id, "presentation-_",
		"idgen.UUIDGenerator already inserts the separator after its prefix")
}
