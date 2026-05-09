package signer

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
)

type Service interface {
	Address(ctx context.Context) (common.Address, error)
	SignTransaction(ctx context.Context, tx *types.Transaction, chainID *big.Int) (*types.Transaction, error)
}

type PrivateKeyService struct {
	key     *ecdsa.PrivateKey
	address common.Address
}

func NewPrivateKeyService(privateKeyHex string) (*PrivateKeyService, error) {
	clean := strings.TrimPrefix(strings.TrimSpace(privateKeyHex), "0x")
	key, err := crypto.HexToECDSA(clean)
	if err != nil {
		return nil, fmt.Errorf("parse private key: %w", err)
	}
	return &PrivateKeyService{key: key, address: crypto.PubkeyToAddress(key.PublicKey)}, nil
}

func (service *PrivateKeyService) Address(ctx context.Context) (common.Address, error) {
	return service.address, nil
}

func (service *PrivateKeyService) SignTransaction(ctx context.Context, tx *types.Transaction, chainID *big.Int) (*types.Transaction, error) {
	signer := types.LatestSignerForChainID(chainID)
	signed, err := types.SignTx(tx, signer, service.key)
	if err != nil {
		return nil, fmt.Errorf("sign transaction: %w", err)
	}
	return signed, nil
}
