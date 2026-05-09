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

const (
	errNilPrimaryClient = "ethereum RPC primary client is nil"
	errNilTransaction   = "transaction is nil"
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

var _ RPCClient = MultiClient{}

type MultiClient struct {
	primary   RPCClient
	fallbacks []RPCClient
}

func NewMultiClient(primary RPCClient, fallbacks []RPCClient) MultiClient {
	return MultiClient{primary: primary, fallbacks: fallbacks}
}

func DialMulti(ctx context.Context, primaryURL string, fallbackURLs []string) (MultiClient, error) {
	return dialMulti(ctx, primaryURL, fallbackURLs, func(ctx context.Context, url string) (RPCClient, error) {
		return ethclient.DialContext(ctx, url)
	})
}

func dialMulti(ctx context.Context, primaryURL string, fallbackURLs []string, dial func(context.Context, string) (RPCClient, error)) (MultiClient, error) {
	primary, err := dial(ctx, primaryURL)
	if err != nil {
		return MultiClient{}, fmt.Errorf("dial primary RPC: %w", err)
	}
	fallbacks := make([]RPCClient, 0, len(fallbackURLs))
	for _, url := range fallbackURLs {
		client, err := dial(ctx, url)
		if err != nil {
			primary.Close()
			closeClients(fallbacks)
			return MultiClient{}, fmt.Errorf("dial fallback RPC: %w", err)
		}
		fallbacks = append(fallbacks, client)
	}
	return NewMultiClient(primary, fallbacks), nil
}

func (client MultiClient) CallContract(ctx context.Context, call ethereum.CallMsg, blockNumber *big.Int) ([]byte, error) {
	clients, err := client.rpcClients()
	if err != nil {
		return nil, fmt.Errorf("call contract: %w", err)
	}
	var lastErr error
	for _, rpc := range clients {
		result, err := rpc.CallContract(ctx, call, blockNumber)
		if err == nil {
			return result, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("call contract failed on all RPC clients: %w", lastErr)
}

func (client MultiClient) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	clients, err := client.rpcClients()
	if err != nil {
		return 0, fmt.Errorf("pending nonce: %w", err)
	}
	var lastErr error
	for _, rpc := range clients {
		result, err := rpc.PendingNonceAt(ctx, account)
		if err == nil {
			return result, nil
		}
		lastErr = err
	}
	return 0, fmt.Errorf("pending nonce failed on all RPC clients: %w", lastErr)
}

func (client MultiClient) SuggestGasTipCap(ctx context.Context) (*big.Int, error) {
	clients, err := client.rpcClients()
	if err != nil {
		return nil, fmt.Errorf("suggest gas tip cap: %w", err)
	}
	var lastErr error
	for _, rpc := range clients {
		result, err := rpc.SuggestGasTipCap(ctx)
		if err == nil {
			return result, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("suggest gas tip cap failed on all RPC clients: %w", lastErr)
}

func (client MultiClient) EstimateGas(ctx context.Context, call ethereum.CallMsg) (uint64, error) {
	clients, err := client.rpcClients()
	if err != nil {
		return 0, fmt.Errorf("estimate gas: %w", err)
	}
	var lastErr error
	for _, rpc := range clients {
		result, err := rpc.EstimateGas(ctx, call)
		if err == nil {
			return result, nil
		}
		lastErr = err
	}
	return 0, fmt.Errorf("estimate gas failed on all RPC clients: %w", lastErr)
}

func (client MultiClient) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	if client.primary == nil {
		return fmt.Errorf("send transaction: %s", errNilPrimaryClient)
	}
	if tx == nil {
		return fmt.Errorf("send transaction: %s", errNilTransaction)
	}
	if err := client.primary.SendTransaction(ctx, tx); err != nil {
		return fmt.Errorf("send transaction: %w", err)
	}
	return nil
}

func (client MultiClient) TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	clients, err := client.rpcClients()
	if err != nil {
		return nil, fmt.Errorf("transaction receipt: %w", err)
	}
	var lastErr error
	for _, rpc := range clients {
		result, err := rpc.TransactionReceipt(ctx, txHash)
		if err == nil {
			return result, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("transaction receipt failed on all RPC clients: %w", lastErr)
}

func (client MultiClient) ChainID(ctx context.Context) (*big.Int, error) {
	clients, err := client.rpcClients()
	if err != nil {
		return nil, fmt.Errorf("chain ID: %w", err)
	}
	var lastErr error
	for _, rpc := range clients {
		result, err := rpc.ChainID(ctx)
		if err == nil {
			return result, nil
		}
		lastErr = err
	}
	return nil, fmt.Errorf("chain ID failed on all RPC clients: %w", lastErr)
}

func (client MultiClient) Close() {
	if client.primary != nil {
		client.primary.Close()
	}
	closeClients(client.fallbacks)
}

func (client MultiClient) rpcClients() ([]RPCClient, error) {
	if client.primary == nil {
		return nil, errors.New(errNilPrimaryClient)
	}
	clients := make([]RPCClient, 0, 1+len(client.fallbacks))
	clients = append(clients, client.primary)
	for _, fallback := range client.fallbacks {
		if fallback != nil {
			clients = append(clients, fallback)
		}
	}
	return clients, nil
}

func closeClients(clients []RPCClient) {
	for _, client := range clients {
		if client != nil {
			client.Close()
		}
	}
}
