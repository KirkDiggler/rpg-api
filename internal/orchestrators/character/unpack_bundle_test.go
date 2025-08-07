package character

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnpackBundleItem(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "bundle reference with greatclub",
			input:    "bundle_1:0:greatclub",
			expected: "greatclub",
		},
		{
			name:     "bundle reference with different item",
			input:    "bundle_2:3:shortsword",
			expected: "shortsword",
		},
		{
			name:     "bundle reference with complex item ID",
			input:    "bundle_0:1:thieves-tools",
			expected: "thieves-tools",
		},
		{
			name:     "regular item ID without bundle prefix",
			input:    "longsword",
			expected: "longsword",
		},
		{
			name:     "regular item ID with hyphen",
			input:    "chain-mail",
			expected: "chain-mail",
		},
		{
			name:     "malformed bundle reference with too few parts",
			input:    "bundle_1:greatclub",
			expected: "bundle_1:greatclub", // Returns as-is when malformed
		},
		{
			name:     "malformed bundle reference with too many parts",
			input:    "bundle_1:0:2:greatclub",
			expected: "bundle_1:0:2:greatclub", // Returns as-is when malformed
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := unpackBundleItem(tc.input)
			assert.Equal(t, tc.expected, result)
		})
	}
}
