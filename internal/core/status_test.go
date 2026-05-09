package core

import "testing"

func TestWorstKnownStatusIgnoresUnknownWhenKnownStatusesExist(t *testing.T) {
	// Arrange
	statuses := []MonitorStatus{MonitorStatusOK, MonitorStatusUnknown, MonitorStatusWarn}

	// Act
	result := WorstKnownStatus(statuses)

	// Assert
	if result != MonitorStatusWarn {
		t.Fatalf("expected %q, got %q", MonitorStatusWarn, result)
	}
}
