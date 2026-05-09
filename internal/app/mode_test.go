package app

import "testing"

func TestParseModeReturnsFalseWhenModeIsUnknown(t *testing.T) {
	// Arrange
	input := "serve"

	// Act
	mode, ok := ParseMode(input)

	// Assert
	if ok {
		t.Fatalf("expected unknown mode to be rejected")
	}
	if mode != "" {
		t.Fatalf("expected empty mode, got %q", mode)
	}
}
