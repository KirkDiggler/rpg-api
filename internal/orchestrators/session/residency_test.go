package session_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	sdk "github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/session"

	sessionorch "github.com/KirkDiggler/rpg-api/internal/orchestrators/session"
)

func TestNewResidency_Bounds(t *testing.T) {
	r, err := sessionorch.NewResidency(nil)
	require.NoError(t, err)
	require.Equal(t, "keeps up to 64 sessions, 512 MiB estimated", r.Bounds())

	r, err = sessionorch.NewResidency(&sessionorch.ResidencyConfig{Entries: -3})
	require.NoError(t, err)
	require.Equal(t, "keeps none (every verb loads)", r.Bounds())

	_, err = sessionorch.NewResidency(&sessionorch.ResidencyConfig{MaxMB: -1})
	require.ErrorIs(t, err, sessionorch.ErrBadResidencyConfig)
}

func TestResidency_CountsHitsAndMissesByReason(t *testing.T) {
	r, err := sessionorch.NewResidency(nil)
	require.NoError(t, err)
	r.Hit("s1")
	r.Hit("s2")
	r.Miss("s1", sdk.MissVersionMoved)
	r.Miss("s3", sdk.MissNoCopy)
	r.Miss("s3", sdk.MissNoCopy)

	got := r.Stats()
	require.Equal(t, sessionorch.ResidencyStats{
		Hits:   2,
		Misses: map[sdk.MissReason]int64{sdk.MissVersionMoved: 1, sdk.MissNoCopy: 2},
	}, got)
	require.Equal(t, int64(3), got.MissCount())

	got.Misses[sdk.MissNoCopy] = 99
	require.Equal(t, int64(2), r.Stats().Misses[sdk.MissNoCopy], "Stats hands out a copy")
}
