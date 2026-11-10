package session_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/KirkDiggler/rpg-api/internal/dungeons/dungeonstest"
	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
	"github.com/KirkDiggler/rpg-api/internal/sessionworld"
)

func TestSites_WarmCompilesOncePerContent_AndLogsEachCompile(t *testing.T) {
	var lines []string
	sites, err := sessionorch.NewSites(&sessionorch.SitesConfig{
		Logf: func(format string, args ...any) { lines = append(lines, fmt.Sprintf(format, args...)) },
	})
	require.NoError(t, err)
	d, err := sessionworld.Compile([]byte(dungeonstest.HeirloomVaultYAML))
	require.NoError(t, err)

	first, err := sites.Warm(context.Background(), &d.Spec.Field)
	require.NoError(t, err)
	require.NotEmpty(t, first)
	// The same content compiled again from a fresh compile of the file: the
	// provider recognizes it and compiles nothing.
	again, err := sessionworld.Compile([]byte(dungeonstest.HeirloomVaultYAML))
	require.NoError(t, err)
	second, err := sites.Warm(context.Background(), &again.Spec.Field)
	require.NoError(t, err)

	require.Equal(t, first, second)
	require.Equal(t, 1, sites.Compiles())
	require.Len(t, lines, 1, "one log line per compile")
	require.Contains(t, lines[0], "site compiled: "+string(first))

	// A second provider is a second process: its own compile.
	other, err := sessionorch.NewSites(nil)
	require.NoError(t, err)
	_, err = other.Warm(context.Background(), &d.Spec.Field)
	require.NoError(t, err)
	require.Equal(t, 1, other.Compiles())
	require.Equal(t, 1, sites.Compiles())
}

func TestSites_Refusals(t *testing.T) {
	_, err := sessionorch.NewSites(&sessionorch.SitesConfig{Entries: -1})
	require.ErrorIs(t, err, sessionorch.ErrBadSitesConfig)

	sites, err := sessionorch.NewSites(nil)
	require.NoError(t, err)
	_, err = sites.Warm(context.Background(), nil)
	require.ErrorIs(t, err, sessionorch.ErrBadSitesConfig)
}
