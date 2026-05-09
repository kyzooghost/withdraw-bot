package morpho

import (
	"context"
	"math/big"
	"testing"
	"time"

	"withdraw-bot/internal/core"
)

func TestSharePriceModuleReturnsUrgentWhenBaselineLossCrossesThreshold(t *testing.T) {
	// Arrange
	module := SharePriceModule{
		BaselineSharePrice: big.NewInt(1_000_000),
		PreviousSharePrice: big.NewInt(995_000),
		WarnBPS:            50,
		UrgentBPS:          100,
		Reader:             fakeSharePriceReader{price: big.NewInt(989_000)},
		Clock:              core.FixedClock{Value: time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)},
	}

	// Act
	result, err := module.Monitor(context.Background())

	// Assert
	if err != nil {
		t.Fatalf("monitor share price: %v", err)
	}
	if result.Status != core.MonitorStatusUrgent {
		t.Fatalf("expected urgent status, got %s", result.Status)
	}
}

type fakeSharePriceReader struct {
	price *big.Int
}

func (reader fakeSharePriceReader) CurrentSharePrice(ctx context.Context) (*big.Int, error) {
	return new(big.Int).Set(reader.price), nil
}
