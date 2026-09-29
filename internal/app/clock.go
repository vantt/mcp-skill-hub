package app

import "time"

// Clock supplies time to application services and can be replaced in tests.
type Clock interface {
	Now() time.Time
}

// SystemClock reads the current local system time.
type SystemClock struct{}

// Now implements Clock.
func (SystemClock) Now() time.Time {
	return time.Now()
}
