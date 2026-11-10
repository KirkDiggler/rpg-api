package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestNewResidency_ReadsTheKnobs pins the two residency knobs
// (rpg-project#879 S6): unset is the defaults, -1 entries keeps none, and a
// bound that cannot be honored stops the boot instead of being guessed.
func TestNewResidency_ReadsTheKnobs(t *testing.T) {
	for _, tc := range []struct {
		name, entries, maxMB string
		bounds               string
		wantErr              bool
	}{
		{name: "unset is the defaults", bounds: "keeps up to 64 sessions, 512 MiB estimated"},
		{name: "both set", entries: "8", maxMB: "128", bounds: "keeps up to 8 sessions, 128 MiB estimated"},
		{name: "minus one keeps none", entries: "-1", bounds: "keeps none (every verb loads)"},
		{name: "zero entries refused", entries: "0", wantErr: true},
		{name: "other negative refused", entries: "-2", wantErr: true},
		{name: "not a number refused", entries: "many", wantErr: true},
		{name: "zero MiB refused", maxMB: "0", wantErr: true},
		{name: "negative MiB refused", maxMB: "-5", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(envResidentEntries, tc.entries)
			t.Setenv(envResidentMaxMB, tc.maxMB)
			residency, err := newResidency()
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.bounds, residency.Bounds())
		})
	}
}
