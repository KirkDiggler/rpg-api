package dungeons_test

import (
	"testing"
	"time"
)

// TEMPORARY: the test budget's red proof (rpg-project#911). Reverted next.
func TestBudgetProbe_SleepsSixSeconds(t *testing.T) {
	time.Sleep(6 * time.Second)
}
