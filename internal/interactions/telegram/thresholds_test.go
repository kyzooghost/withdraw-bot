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

func TestBuildThresholdConfirmationRejectsUnknownKeyForKnownModule(t *testing.T) {
	// Arrange
	request := ThresholdSetRequest{ModuleID: "share_price_loss", Key: "unknown", Value: "50", UserID: 1}

	// Act
	_, err := BuildThresholdConfirmation(request)

	// Assert
	if err == nil {
		t.Fatalf("expected unknown threshold key to be rejected")
	}
}
