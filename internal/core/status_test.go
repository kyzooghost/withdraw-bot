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

func TestWorstKnownStatusReturnsUnknownWhenInputIsEmpty(t *testing.T) {
	// Arrange
	var statuses []MonitorStatus

	// Act
	result := WorstKnownStatus(statuses)

	// Assert
	if result != MonitorStatusUnknown {
		t.Fatalf("expected %q, got %q", MonitorStatusUnknown, result)
	}
}

func TestWorstKnownStatusReturnsUnknownWhenAllStatusesAreUnknown(t *testing.T) {
	// Arrange
	statuses := []MonitorStatus{MonitorStatusUnknown, MonitorStatusUnknown}

	// Act
	result := WorstKnownStatus(statuses)

	// Assert
	if result != MonitorStatusUnknown {
		t.Fatalf("expected %q, got %q", MonitorStatusUnknown, result)
	}
}

func TestWorstKnownStatusReturnsUrgentWhenUrgentIsPresent(t *testing.T) {
	// Arrange
	statuses := []MonitorStatus{MonitorStatusWarn, MonitorStatusUrgent, MonitorStatusOK}

	// Act
	result := WorstKnownStatus(statuses)

	// Assert
	if result != MonitorStatusUrgent {
		t.Fatalf("expected %q, got %q", MonitorStatusUrgent, result)
	}
}

func TestWorstKnownStatusReturnsWarnOverOK(t *testing.T) {
	// Arrange
	statuses := []MonitorStatus{MonitorStatusOK, MonitorStatusWarn, MonitorStatusOK}

	// Act
	result := WorstKnownStatus(statuses)

	// Assert
	if result != MonitorStatusWarn {
		t.Fatalf("expected %q, got %q", MonitorStatusWarn, result)
	}
}
