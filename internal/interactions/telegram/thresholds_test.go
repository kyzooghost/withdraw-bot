package telegram

import "testing"

func TestBuildThresholdConfirmationRejectsUnknownModule(t *testing.T) {
	// Arrange
	request := ThresholdSetRequest{ModuleID: "unknown", Key: "loss_warn_bps", Value: "50", UserID: 1}

	// Act
	_, err := BuildThresholdConfirmation(request)

	// Assert
	if err == nil {
		t.Fatalf("expected unknown module to be rejected")
	}
}
