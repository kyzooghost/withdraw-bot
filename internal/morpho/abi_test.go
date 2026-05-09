package morpho

import (
	"bytes"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

func TestPackRedeemEncodesSelectorAndArguments(t *testing.T) {
	// Arrange
	shares := big.NewInt(123)
	receiver := common.HexToAddress("0x0000000000000000000000000000000000000001")
	owner := common.HexToAddress("0x0000000000000000000000000000000000000002")
	expectedSelector := []byte{0xba, 0x08, 0x76, 0x52}

	// Act
	data, err := PackRedeem(shares, receiver, owner)

	// Assert
	if err != nil {
		t.Fatalf("pack redeem: %v", err)
	}
	if len(data) != 4+32*3 {
		t.Fatalf("expected redeem calldata length %d, got %d", 4+32*3, len(data))
	}

	method := VaultABI.Methods[vaultMethodRedeem]
	if !bytes.Equal(data[:4], expectedSelector) {
		t.Fatalf("expected redeem selector %x, got %x", expectedSelector, data[:4])
	}
	decoded, err := method.Inputs.Unpack(data[4:])
	if err != nil {
		t.Fatalf("unpack redeem arguments: %v", err)
	}
	if len(decoded) != 3 {
		t.Fatalf("expected 3 decoded redeem arguments, got %d", len(decoded))
	}
	decodedShares, ok := decoded[0].(*big.Int)
	if !ok {
		t.Fatalf("expected decoded shares to be *big.Int, got %T", decoded[0])
	}
	if decodedShares.Cmp(shares) != 0 {
		t.Fatalf("expected decoded shares %s, got %s", shares.String(), decodedShares.String())
	}
	decodedReceiver, ok := decoded[1].(common.Address)
	if !ok {
		t.Fatalf("expected decoded receiver to be common.Address, got %T", decoded[1])
	}
	if decodedReceiver != receiver {
		t.Fatalf("expected decoded receiver %s, got %s", receiver.Hex(), decodedReceiver.Hex())
	}
	decodedOwner, ok := decoded[2].(common.Address)
	if !ok {
		t.Fatalf("expected decoded owner to be common.Address, got %T", decoded[2])
	}
	if decodedOwner != owner {
		t.Fatalf("expected decoded owner %s, got %s", owner.Hex(), decodedOwner.Hex())
	}
}
