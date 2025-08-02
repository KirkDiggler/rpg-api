package external

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/KirkDiggler/rpg-toolkit/rulebooks/dnd5e/constants"
)

func TestConvertKeyToClassID(t *testing.T) {
	testCases := []struct {
		name      string
		key       string
		wantClass constants.Class
		wantErr   bool
	}{
		{
			name:      "valid wizard key",
			key:       "wizard",
			wantClass: constants.ClassWizard,
			wantErr:   false,
		},
		{
			name:      "valid fighter key",
			key:       "fighter",
			wantClass: constants.ClassFighter,
			wantErr:   false,
		},
		{
			name:      "valid barbarian key",
			key:       "barbarian",
			wantClass: constants.ClassBarbarian,
			wantErr:   false,
		},
		{
			name:      "unknown class key",
			key:       "artificer",
			wantClass: "",
			wantErr:   true,
		},
		{
			name:      "invalid key",
			key:       "not-a-class",
			wantClass: "",
			wantErr:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			classID, err := convertKeyToClassID(tc.key)

			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "unknown class key")
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.wantClass, classID)
			}
		})
	}
}

func TestConvertKeyToRaceID(t *testing.T) {
	testCases := []struct {
		name     string
		key      string
		wantRace constants.Race
		wantErr  bool
	}{
		{
			name:     "valid human key",
			key:      "human",
			wantRace: constants.RaceHuman,
			wantErr:  false,
		},
		{
			name:     "valid dragonborn key",
			key:      "dragonborn",
			wantRace: constants.RaceDragonborn,
			wantErr:  false,
		},
		{
			name:     "valid half-elf key",
			key:      "half-elf",
			wantRace: constants.RaceHalfElf,
			wantErr:  false,
		},
		{
			name:     "unknown race key",
			key:      "aarakocra",
			wantRace: "",
			wantErr:  true,
		},
		{
			name:     "invalid key",
			key:      "not-a-race",
			wantRace: "",
			wantErr:  true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			raceID, err := convertKeyToRaceID(tc.key)

			if tc.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), "unknown race key")
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.wantRace, raceID)
			}
		})
	}
}
