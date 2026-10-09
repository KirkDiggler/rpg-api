package sessionworld

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	tkdungeonspec "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/encounter/dungeonspec"
)

// TestAnUnknownScenarioStillAnswersAsAFieldError pins that moving the endings
// to the SDK left an author's bad binding where it was: a ValidationError
// whose one defect sits on the binding block's path.
func TestAnUnknownScenarioStillAnswersAsAFieldError(t *testing.T) {
	raw, err := os.ReadFile(raiderCampPath)
	require.NoError(t, err)
	const sugar = "scenarios:\n  hold-out: { convince: raiders }\n"
	require.Contains(t, string(raw), sugar)

	bad := strings.Replace(string(raw), sugar, "scenarios:\n  no-such-scenario: { convince: raiders }\n", 1)
	_, err = Compile([]byte(bad))
	require.Error(t, err)
	var verr *tkdungeonspec.ValidationError
	require.ErrorAs(t, err, &verr)
	require.Len(t, verr.Errors, 1)
	require.Equal(t, "scenarios.no-such-scenario", verr.Errors[0].Path)
}
