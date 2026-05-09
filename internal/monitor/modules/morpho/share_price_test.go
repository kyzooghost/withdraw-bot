package morpho

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"withdraw-bot/internal/core"
)

func TestSharePriceModuleRejectsZeroWarnThreshold(t *testing.T) {
	// Arrange
	module := SharePriceModule{
		BaselineSharePrice: big.NewInt(1_000_000),
		WarnBPS:            0,
		UrgentBPS:          100,
		Reader:             fakeSharePriceReader{price: big.NewInt(1_000_000)},
	}

	// Act
	err := module.ValidateConfig(context.Background())

	// Assert
	if err == nil {
		t.Fatal("expected warn threshold error")
	}
}

func TestSharePriceModuleRejectsZeroUrgentThreshold(t *testing.T) {
	// Arrange
	module := SharePriceModule{
		BaselineSharePrice: big.NewInt(1_000_000),
		WarnBPS:            50,
		UrgentBPS:          0,
		Reader:             fakeSharePriceReader{price: big.NewInt(1_000_000)},
	}

	// Act
	err := module.ValidateConfig(context.Background())

	// Assert
	if err == nil {
		t.Fatal("expected urgent threshold error")
	}
}

func TestSharePriceModuleRejectsWarnThresholdAboveUrgentThreshold(t *testing.T) {
	// Arrange
	module := SharePriceModule{
		BaselineSharePrice: big.NewInt(1_000_000),
		WarnBPS:            101,
		UrgentBPS:          100,
		Reader:             fakeSharePriceReader{price: big.NewInt(1_000_000)},
	}

	// Act
	err := module.ValidateConfig(context.Background())

	// Assert
	if err == nil {
		t.Fatal("expected threshold ordering error")
	}
}

func TestSharePriceModuleAcceptsValidConfig(t *testing.T) {
	// Arrange
	module := SharePriceModule{
		BaselineSharePrice: big.NewInt(1_000_000),
		WarnBPS:            50,
		UrgentBPS:          100,
		Reader:             fakeSharePriceReader{price: big.NewInt(1_000_000)},
	}

	// Act
	err := module.ValidateConfig(context.Background())

	// Assert
	if err != nil {
		t.Fatalf("validate config: %v", err)
	}
}

func TestSharePriceModuleReturnsOKWhenThereIsNoLoss(t *testing.T) {
	// Arrange
	module := SharePriceModule{
		BaselineSharePrice: big.NewInt(1_000_000),
		PreviousSharePrice: big.NewInt(1_000_000),
		WarnBPS:            50,
		UrgentBPS:          100,
		Reader:             fakeSharePriceReader{price: big.NewInt(1_000_000)},
		Clock:              core.FixedClock{Value: time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)},
	}

	// Act
	result, err := module.Monitor(context.Background())

	// Assert
	if err != nil {
		t.Fatalf("monitor share price: %v", err)
	}
	if result.Status != core.MonitorStatusOK {
		t.Fatalf("expected ok status, got %s", result.Status)
	}
}

func TestSharePriceModuleReturnsWarnWhenBaselineLossCrossesThreshold(t *testing.T) {
	// Arrange
	module := SharePriceModule{
		BaselineSharePrice: big.NewInt(1_000_000),
		WarnBPS:            50,
		UrgentBPS:          100,
		Reader:             fakeSharePriceReader{price: big.NewInt(994_000)},
		Clock:              core.FixedClock{Value: time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)},
	}

	// Act
	result, err := module.Monitor(context.Background())

	// Assert
	if err != nil {
		t.Fatalf("monitor share price: %v", err)
	}
	if result.Status != core.MonitorStatusWarn {
		t.Fatalf("expected warn status, got %s", result.Status)
	}
}

func TestSharePriceModuleReturnsWarnWhenBaselineLossEqualsWarnThreshold(t *testing.T) {
	// Arrange
	module := SharePriceModule{
		BaselineSharePrice: big.NewInt(1_000_000),
		WarnBPS:            50,
		UrgentBPS:          100,
		Reader:             fakeSharePriceReader{price: big.NewInt(995_000)},
		Clock:              core.FixedClock{Value: time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)},
	}

	// Act
	result, err := module.Monitor(context.Background())

	// Assert
	if err != nil {
		t.Fatalf("monitor share price: %v", err)
	}
	if result.Status != core.MonitorStatusWarn {
		t.Fatalf("expected warn status, got %s", result.Status)
	}
}

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

func TestSharePriceModuleReturnsUrgentWhenBaselineLossEqualsUrgentThreshold(t *testing.T) {
	// Arrange
	module := SharePriceModule{
		BaselineSharePrice: big.NewInt(1_000_000),
		WarnBPS:            50,
		UrgentBPS:          100,
		Reader:             fakeSharePriceReader{price: big.NewInt(990_000)},
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

func TestSharePriceModuleReturnsWarnWhenOnlyPreviousPriceHasLoss(t *testing.T) {
	// Arrange
	module := SharePriceModule{
		BaselineSharePrice: big.NewInt(1_000_000),
		PreviousSharePrice: big.NewInt(1_010_000),
		WarnBPS:            50,
		UrgentBPS:          100,
		Reader:             fakeSharePriceReader{price: big.NewInt(1_000_000)},
		Clock:              core.FixedClock{Value: time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)},
	}

	// Act
	result, err := module.Monitor(context.Background())

	// Assert
	if err != nil {
		t.Fatalf("monitor share price: %v", err)
	}
	if result.Status != core.MonitorStatusWarn {
		t.Fatalf("expected warn status, got %s", result.Status)
	}
}

func TestSharePriceModuleReturnsReaderError(t *testing.T) {
	// Arrange
	expectedErr := errors.New("read share price")
	module := SharePriceModule{
		BaselineSharePrice: big.NewInt(1_000_000),
		WarnBPS:            50,
		UrgentBPS:          100,
		Reader:             fakeSharePriceReader{err: expectedErr},
		Clock:              core.FixedClock{Value: time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)},
	}

	// Act
	_, err := module.Monitor(context.Background())

	// Assert
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected reader error, got %v", err)
	}
}

func TestSharePriceModuleReturnsErrorForNilCurrentSharePrice(t *testing.T) {
	// Arrange
	module := SharePriceModule{
		BaselineSharePrice: big.NewInt(1_000_000),
		WarnBPS:            50,
		UrgentBPS:          100,
		Reader:             fakeSharePriceReader{},
		Clock:              core.FixedClock{Value: time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)},
	}

	// Act
	_, err := module.Monitor(context.Background())

	// Assert
	if err == nil {
		t.Fatal("expected current share price error")
	}
}

func TestSharePriceModuleReturnsErrorForZeroCurrentSharePrice(t *testing.T) {
	// Arrange
	module := SharePriceModule{
		BaselineSharePrice: big.NewInt(1_000_000),
		WarnBPS:            50,
		UrgentBPS:          100,
		Reader:             fakeSharePriceReader{price: big.NewInt(0)},
		Clock:              core.FixedClock{Value: time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)},
	}

	// Act
	_, err := module.Monitor(context.Background())

	// Assert
	if err == nil {
		t.Fatal("expected current share price error")
	}
}

func TestSharePriceModuleReturnsErrorForNegativeCurrentSharePrice(t *testing.T) {
	// Arrange
	module := SharePriceModule{
		BaselineSharePrice: big.NewInt(1_000_000),
		WarnBPS:            50,
		UrgentBPS:          100,
		Reader:             fakeSharePriceReader{price: big.NewInt(-1)},
		Clock:              core.FixedClock{Value: time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)},
	}

	// Act
	_, err := module.Monitor(context.Background())

	// Assert
	if err == nil {
		t.Fatal("expected current share price error")
	}
}

func TestSharePriceModuleUsesConfiguredClockForObservedTime(t *testing.T) {
	// Arrange
	observedAt := time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)
	module := SharePriceModule{
		BaselineSharePrice: big.NewInt(1_000_000),
		WarnBPS:            50,
		UrgentBPS:          100,
		Reader:             fakeSharePriceReader{price: big.NewInt(1_000_000)},
		Clock:              core.FixedClock{Value: observedAt},
	}

	// Act
	result, err := module.Monitor(context.Background())

	// Assert
	if err != nil {
		t.Fatalf("monitor share price: %v", err)
	}
	if result.ObservedAt != observedAt {
		t.Fatalf("expected observed time %s, got %s", observedAt, result.ObservedAt)
	}
}

func TestSharePriceModuleDefaultsNilClock(t *testing.T) {
	// Arrange
	module := SharePriceModule{
		BaselineSharePrice: big.NewInt(1_000_000),
		WarnBPS:            50,
		UrgentBPS:          100,
		Reader:             fakeSharePriceReader{price: big.NewInt(1_000_000)},
	}

	// Act
	result, err := module.Monitor(context.Background())

	// Assert
	if err != nil {
		t.Fatalf("monitor share price: %v", err)
	}
	if result.ObservedAt.IsZero() {
		t.Fatal("expected observed time from default clock")
	}
}

type fakeSharePriceReader struct {
	price *big.Int
	err   error
}

func (reader fakeSharePriceReader) CurrentSharePrice(ctx context.Context) (*big.Int, error) {
	if reader.err != nil {
		return nil, reader.err
	}
	if reader.price == nil {
		return nil, nil
	}
	return new(big.Int).Set(reader.price), nil
}
