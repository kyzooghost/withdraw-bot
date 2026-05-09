package core

import "time"

type Clock interface {
	Now() time.Time
}

type SystemClock struct{}

func (SystemClock) Now() time.Time {
	return time.Now().UTC()
}

type FixedClock struct {
	Value time.Time
}

func (clock FixedClock) Now() time.Time {
	return clock.Value
}
