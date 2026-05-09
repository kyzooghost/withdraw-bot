package morpho

import (
	"context"
	"fmt"
	"math/big"

	"withdraw-bot/internal/ethereum"

	geth "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
)

type VaultClient struct {
	Ethereum ethereum.MultiClient
	Vault    common.Address
}

func (client VaultClient) call(ctx context.Context, method string, args ...any) ([]any, error) {
	data, err := VaultABI.Pack(method, args...)
	if err != nil {
		return nil, fmt.Errorf("pack %s call: %w", method, err)
	}
	raw, err := client.Ethereum.CallContract(ctx, geth.CallMsg{To: &client.Vault, Data: data}, nil)
	if err != nil {
		return nil, fmt.Errorf("call %s: %w", method, err)
	}
	out, err := VaultABI.Unpack(method, raw)
	if err != nil {
		return nil, fmt.Errorf("unpack %s: %w", method, err)
	}
	return out, nil
}

func (client VaultClient) BalanceOf(ctx context.Context, owner common.Address) (*big.Int, error) {
	out, err := client.call(ctx, vaultMethodBalanceOf, owner)
	if err != nil {
		return nil, err
	}
	value, err := uint256Output(vaultMethodBalanceOf, out)
	if err != nil {
		return nil, err
	}
	return value, nil
}

func (client VaultClient) PreviewRedeem(ctx context.Context, shares *big.Int) (*big.Int, error) {
	out, err := client.call(ctx, vaultMethodPreviewRedeem, shares)
	if err != nil {
		return nil, err
	}
	value, err := uint256Output(vaultMethodPreviewRedeem, out)
	if err != nil {
		return nil, err
	}
	return value, nil
}

func uint256Output(method string, out []any) (*big.Int, error) {
	if len(out) != 1 {
		return nil, fmt.Errorf("unpack %s output: expected 1 value, got %d", method, len(out))
	}
	value, ok := out[0].(*big.Int)
	if !ok {
		return nil, fmt.Errorf("unpack %s output: expected uint256", method)
	}
	return new(big.Int).Set(value), nil
}
