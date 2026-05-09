package app

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestRunChecksReturnsFirstFailure(t *testing.T) {
	// Arrange
	expectedErr := errors.New("chain ID mismatch")
	calls := 0
	checks := []Check{
		func(ctx context.Context) error {
			calls++
			return nil
		},
		func(ctx context.Context) error {
			calls++
			return expectedErr
		},
		func(ctx context.Context) error {
			calls++
			return nil
		},
	}

	// Act
	err := RunChecks(context.Background(), checks)

	// Assert
	if err == nil {
		t.Fatalf("expected first failure")
	}
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected wrapped failure, got %v", err)
	}
	if !strings.Contains(err.Error(), "config check 2 failed") {
		t.Fatalf("expected check index in error, got %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected checks to stop after first failure, got %d calls", calls)
	}
}
