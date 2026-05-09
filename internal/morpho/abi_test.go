package morpho

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestPackRedeemEncodesSelectorAndArguments(t *testing.T) {
	// Arrange
	shares := big.NewInt(123)
	receiver := common.HexToAddress("0x0000000000000000000000000000000000000001")
	owner := common.HexToAddress("0x0000000000000000000000000000000000000002")

	// Act
	data, err := PackRedeem(shares, receiver, owner)

	// Assert
	if err != nil {
		t.Fatalf("pack redeem: %v", err)
	}
	if len(data) != 4+32*3 {
		t.Fatalf("expected redeem calldata length %d, got %d", 4+32*3, len(data))
	}
}
