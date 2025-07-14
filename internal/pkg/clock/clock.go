// Package clock provides time utilities for the application
package clock

import "time"

//go:generate mockgen -destination=mock/mock.go -package=mockclock github.com/KirkDiggler/rpg-api/internal/pkg/clock Clock

// Clock provides time functionality
type Clock interface {
	Now() time.Time
}

// Real implements Clock using actual system time
type Real struct{}

// Now returns the current time
func (c *Real) Now() time.Time {
	return time.Now()
}

// New returns a new real clock
func New() Clock {
	return &Real{}
}
