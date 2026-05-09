package ethereum

import (
	"context"
	"errors"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethclient"
)

var ErrPreBroadcast = errors.New("transaction failed before broadcast")

type RPCClient interface {
	CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error)
	PendingNonceAt(ctx context.Context, account common.Address) (uint64, error)
	SuggestGasTipCap(ctx context.Context) (*big.Int, error)
	EstimateGas(ctx context.Context, call ethereum.CallMsg) (uint64, error)
	SendTransaction(ctx context.Context, tx *types.Transaction) error
	TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error)
	ChainID(ctx context.Context) (*big.Int, error)
	Close()
}

type MultiClient struct {
	primary   RPCClient
	fallbacks []RPCClient
}

func NewMultiClient(primary RPCClient, fallbacks []RPCClient) MultiClient {
	return MultiClient{primary: primary, fallbacks: fallbacks}
}

func DialMulti(ctx context.Context, primaryURL string, fallbackURLs []string) (MultiClient, error) {
	primary, err := ethclient.DialContext(ctx, primaryURL)
	if err != nil {
		return MultiClient{}, fmt.Errorf("dial primary RPC: %w", err)
	}
	fallbacks := make([]RPCClient, 0, len(fallbackURLs))
	for _, url := range fallbackURLs {
		client, err := ethclient.DialContext(ctx, url)
		if err != nil {
			primary.Close()
			return MultiClient{}, fmt.Errorf("dial fallback RPC: %w", err)
		}
		fallbacks = append(fallbacks, client)
	}
	return NewMultiClient(primary, fallbacks), nil
}

func (client MultiClient) CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	result, err := client.primary.CallContract(ctx, call, blockNumber)
	if err == nil {
		return result, nil
	}
	var lastErr error = err
	for _, fallback := range client.fallbacks {
		result, err := fallback.CallContract(ctx, call, blockNumber)
		if err == nil {
			return result, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("call contract failed on all RPC clients: %w", lastErr)
}

func (client MultiClient) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	if err := client.primary.SendTransaction(ctx, tx); err != nil {
		return fmt.Errorf("%w: %v", ErrPreBroadcast, err)
	}
	return nil
}

func (client MultiClient) Close() {
	client.primary.Close()
	for _, fallback := range client.fallbacks {
		fallback.Close()
	}
}
