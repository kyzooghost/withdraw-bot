package core

import (
	"testing"
	"time"
)

func TestFixedClockReturnsConfiguredTime(t *testing.T) {
	// Arrange
	expected := time.Date(2026, 5, 9, 1, 2, 3, 0, time.UTC)
	clock := FixedClock{Value: expected}

	// Act
	actual := clock.Now()

	// Assert
	if !actual.Equal(expected) {
		t.Fatalf("expected %s, got %s", expected, actual)
	}
}
