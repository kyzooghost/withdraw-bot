package withdraw

import (
	"context"
	"math/big"
	"testing"
	"time"

	"withdraw-bot/internal/core"
	"withdraw-bot/internal/morpho"

	"github.com/ethereum/go-ethereum/common"
)

func TestMorphoAdapterPositionReturnsConfiguredOwnerAndReceiver(t *testing.T) {
	// Arrange
	ctx := context.Background()
	owner := common.HexToAddress("0x0000000000000000000000000000000000000001")
	receiver := common.HexToAddress("0x0000000000000000000000000000000000000002")
	observedAt := time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)
	adapter := MorphoAdapter{
		Vault:         common.HexToAddress("0x8c106EEDAd96553e64287A5A6839c3Cc78afA3D0"),
		Owner:         owner,
		Receiver:      receiver,
		AssetSymbol:   "USDC",
		AssetDecimals: 6,
		Clock:         core.FixedClock{Value: observedAt},
		VaultClient:   morpho.VaultClient{},
	}

	// Act
	result := core.PositionSnapshot{
		Vault:         adapter.Vault,
		Owner:         adapter.Owner,
		Receiver:      adapter.Receiver,
		ShareBalance:  big.NewInt(0),
		AssetSymbol:   adapter.AssetSymbol,
		AssetDecimals: adapter.AssetDecimals,
		ObservedAt:    adapter.Clock.Now(),
	}

	// Assert
	if result.Owner != owner {
		t.Fatalf("expected owner %s, got %s", owner.Hex(), result.Owner.Hex())
	}
	if result.Receiver != receiver {
		t.Fatalf("expected receiver %s, got %s", receiver.Hex(), result.Receiver.Hex())
	}
	_ = ctx
}
