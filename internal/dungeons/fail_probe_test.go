package dungeons_test

import "testing"

// TEMPORARY: proves a failing test turns the CI test step red (rpg-project#911). Reverted next.
func TestPipefailProbe_Fails(t *testing.T) {
	t.Fatal("probe")
}
