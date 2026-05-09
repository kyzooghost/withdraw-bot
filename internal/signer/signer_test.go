package signer

import (
	"context"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

const testPrivateKey = "0x0123456789012345678901234567890123456789012345678901234567890123"

func TestPrivateKeyServiceSignsTransactionForExpectedAddress(t *testing.T) {
	// Arrange
	ctx := context.Background()
	service, err := NewPrivateKeyService(testPrivateKey)
	if err != nil {
		t.Fatalf("create signer: %v", err)
	}
	chainID := big.NewInt(1)
	tx := types.NewTx(&types.DynamicFeeTx{
		ChainID:   chainID,
		Nonce:     1,
		GasTipCap: big.NewInt(1),
		GasFeeCap: big.NewInt(2),
		Gas:       21000,
		To:        &common.Address{},
		Value:     big.NewInt(0),
		Data:      nil,
	})

	// Act
	signed, err := service.SignTransaction(ctx, tx, chainID)

	// Assert
	if err != nil {
		t.Fatalf("sign transaction: %v", err)
	}
	sender, err := types.Sender(types.LatestSignerForChainID(chainID), signed)
	if err != nil {
		t.Fatalf("recover sender: %v", err)
	}
	expected, err := service.Address(ctx)
	if err != nil {
		t.Fatalf("read signer address: %v", err)
	}
	if sender != expected {
		t.Fatalf("expected sender %s, got %s", expected.Hex(), sender.Hex())
	}
}

func TestPrivateKeyServiceRejectsNilTransaction(t *testing.T) {
	// Arrange
	ctx := context.Background()
	service, err := NewPrivateKeyService(testPrivateKey)
	if err != nil {
		t.Fatalf("create signer: %v", err)
	}

	// Act
	_, err = service.SignTransaction(ctx, nil, big.NewInt(1))

	// Assert
	if err == nil {
		t.Fatalf("expected nil transaction to be rejected")
	}
}

func TestPrivateKeyServiceRejectsNilChainID(t *testing.T) {
	// Arrange
	ctx := context.Background()
	service, err := NewPrivateKeyService(testPrivateKey)
	if err != nil {
		t.Fatalf("create signer: %v", err)
	}
	tx := types.NewTx(&types.DynamicFeeTx{
		ChainID:   big.NewInt(1),
		Nonce:     1,
		GasTipCap: big.NewInt(1),
		GasFeeCap: big.NewInt(2),
		Gas:       21000,
		To:        &common.Address{},
		Value:     big.NewInt(0),
		Data:      nil,
	})

	// Act
	_, err = service.SignTransaction(ctx, tx, nil)

	// Assert
	if err == nil {
		t.Fatalf("expected nil chain ID to be rejected")
	}
}
