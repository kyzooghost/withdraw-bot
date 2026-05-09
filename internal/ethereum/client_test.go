package ethereum

import (
	"context"
	"errors"
	"math/big"
	"reflect"
	"testing"

	geth "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

const (
	testPrimaryName       = "primary"
	testFallbackName      = "fallback"
	testOtherFallbackName = "other-fallback"
	testPrimaryURL        = "primary-url"
	testFallbackURL       = "fallback-url"
	testBadFallbackURL    = "bad-fallback-url"
	testCallContractOp    = "CallContract"
	testPendingNonceAtOp  = "PendingNonceAt"
	testSuggestGasTipOp   = "SuggestGasTipCap"
	testEstimateGasOp     = "EstimateGas"
	testReceiptOp         = "TransactionReceipt"
	testChainIDOp         = "ChainID"
	testSendOp            = "SendTransaction"
	testCloseOp           = "Close"
)

var (
	errTestPrimaryFailure  = errors.New("primary failed")
	errTestFallbackFailure = errors.New("fallback failed")
	errTestDialFailure     = errors.New("dial failed")
)

type testRPCClient struct {
	name       string
	calls      *[]string
	errs       map[string]error
	callResult []byte
	nonce      uint64
	tipCap     *big.Int
	gas        uint64
	receipt    *types.Receipt
	chainID    *big.Int
	closed     bool
}

func (client *testRPCClient) CallContract(ctx context.Context, call geth.CallMsg, blockNumber *big.Int) ([]byte, error) {
	client.record(testCallContractOp)
	if err := client.errs[testCallContractOp]; err != nil {
		return nil, err
	}
	return client.callResult, nil
}

func (client *testRPCClient) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	client.record(testPendingNonceAtOp)
	if err := client.errs[testPendingNonceAtOp]; err != nil {
		return 0, err
	}
	return client.nonce, nil
}

func (client *testRPCClient) SuggestGasTipCap(ctx context.Context) (*big.Int, error) {
	client.record(testSuggestGasTipOp)
	if err := client.errs[testSuggestGasTipOp]; err != nil {
		return nil, err
	}
	return client.tipCap, nil
}

func (client *testRPCClient) EstimateGas(ctx context.Context, call geth.CallMsg) (uint64, error) {
	client.record(testEstimateGasOp)
	if err := client.errs[testEstimateGasOp]; err != nil {
		return 0, err
	}
	return client.gas, nil
}

func (client *testRPCClient) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	client.record(testSendOp)
	return client.errs[testSendOp]
}

func (client *testRPCClient) TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	client.record(testReceiptOp)
	if err := client.errs[testReceiptOp]; err != nil {
		return nil, err
	}
	return client.receipt, nil
}

func (client *testRPCClient) ChainID(ctx context.Context) (*big.Int, error) {
	client.record(testChainIDOp)
	if err := client.errs[testChainIDOp]; err != nil {
		return nil, err
	}
	return client.chainID, nil
}

func (client *testRPCClient) Close() {
	client.record(testCloseOp)
	client.closed = true
}

func (client *testRPCClient) record(operation string) {
	if client.calls == nil {
		return
	}
	*client.calls = append(*client.calls, client.name+"."+operation)
}

func TestMultiClientReadMethodsUsePrimaryThenFallbacks(t *testing.T) {
	tests := []struct {
		name      string
		operation string
		call      func(context.Context, MultiClient) (string, error)
		expected  string
	}{
		{
			name:      "call contract",
			operation: testCallContractOp,
			call: func(ctx context.Context, client MultiClient) (string, error) {
				result, err := client.CallContract(ctx, geth.CallMsg{}, nil)
				return string(result), err
			},
			expected: "ok",
		},
		{
			name:      "pending nonce",
			operation: testPendingNonceAtOp,
			call: func(ctx context.Context, client MultiClient) (string, error) {
				result, err := client.PendingNonceAt(ctx, common.Address{})
				return new(big.Int).SetUint64(result).String(), err
			},
			expected: "7",
		},
		{
			name:      "gas tip cap",
			operation: testSuggestGasTipOp,
			call: func(ctx context.Context, client MultiClient) (string, error) {
				result, err := client.SuggestGasTipCap(ctx)
				return result.String(), err
			},
			expected: "3",
		},
		{
			name:      "estimate gas",
			operation: testEstimateGasOp,
			call: func(ctx context.Context, client MultiClient) (string, error) {
				result, err := client.EstimateGas(ctx, geth.CallMsg{})
				return new(big.Int).SetUint64(result).String(), err
			},
			expected: "21000",
		},
		{
			name:      "transaction receipt",
			operation: testReceiptOp,
			call: func(ctx context.Context, client MultiClient) (string, error) {
				result, err := client.TransactionReceipt(ctx, common.Hash{})
				return result.TxHash.Hex(), err
			},
			expected: common.HexToHash("0x1234").Hex(),
		},
		{
			name:      "chain ID",
			operation: testChainIDOp,
			call: func(ctx context.Context, client MultiClient) (string, error) {
				result, err := client.ChainID(ctx)
				return result.String(), err
			},
			expected: "1",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Arrange
			ctx := context.Background()
			var calls []string
			primary := testClient(testPrimaryName, &calls)
			primary.errs[test.operation] = errTestPrimaryFailure
			fallback := testClient(testFallbackName, &calls)
			fallback.errs[test.operation] = errTestFallbackFailure
			otherFallback := testClient(testOtherFallbackName, &calls)
			client := NewMultiClient(primary, []RPCClient{fallback, otherFallback})

			// Act
			result, err := test.call(ctx, client)

			// Assert
			if err != nil {
				t.Fatalf("expected read to succeed: %v", err)
			}
			if result != test.expected {
				t.Fatalf("expected result %q, got %q", test.expected, result)
			}
			expectedCalls := []string{
				testPrimaryName + "." + test.operation,
				testFallbackName + "." + test.operation,
				testOtherFallbackName + "." + test.operation,
			}
			if !reflect.DeepEqual(calls, expectedCalls) {
				t.Fatalf("expected calls %v, got %v", expectedCalls, calls)
			}
		})
	}
}

func TestMultiClientSendTransactionDoesNotClassifyAmbiguousErrorAsPreBroadcast(t *testing.T) {
	// Arrange
	ctx := context.Background()
	primary := testClient(testPrimaryName, nil)
	primary.errs[testSendOp] = errors.New("ambiguous send failure")
	client := NewMultiClient(primary, nil)

	// Act
	err := client.SendTransaction(ctx, types.NewTx(&types.DynamicFeeTx{ChainID: big.NewInt(1)}))

	// Assert
	if err == nil {
		t.Fatalf("expected send transaction to fail")
	}
	if errors.Is(err, ErrPreBroadcast) {
		t.Fatalf("expected ambiguous send failure not to be classified as pre-broadcast")
	}
}

func TestMultiClientCloseSkipsNilFallbacks(t *testing.T) {
	// Arrange
	var calls []string
	primary := testClient(testPrimaryName, &calls)
	fallback := testClient(testFallbackName, &calls)
	client := NewMultiClient(primary, []RPCClient{nil, fallback})

	// Act
	client.Close()

	// Assert
	if !primary.closed {
		t.Fatalf("expected primary to be closed")
	}
	if !fallback.closed {
		t.Fatalf("expected fallback to be closed")
	}
	expectedCalls := []string{
		testPrimaryName + "." + testCloseOp,
		testFallbackName + "." + testCloseOp,
	}
	if !reflect.DeepEqual(calls, expectedCalls) {
		t.Fatalf("expected calls %v, got %v", expectedCalls, calls)
	}
}

func TestDialMultiClosesOpenedFallbacksWhenLaterFallbackDialFails(t *testing.T) {
	// Arrange
	ctx := context.Background()
	primary := testClient(testPrimaryName, nil)
	fallback := testClient(testFallbackName, nil)
	dialed := map[string]RPCClient{
		testPrimaryURL:  primary,
		testFallbackURL: fallback,
	}
	dial := func(ctx context.Context, url string) (RPCClient, error) {
		if url == testBadFallbackURL {
			return nil, errTestDialFailure
		}
		return dialed[url], nil
	}

	// Act
	_, err := dialMulti(ctx, testPrimaryURL, []string{testFallbackURL, testBadFallbackURL}, dial)

	// Assert
	if err == nil {
		t.Fatalf("expected dial failure")
	}
	if !primary.closed {
		t.Fatalf("expected primary to be closed")
	}
	if !fallback.closed {
		t.Fatalf("expected opened fallback to be closed")
	}
	if errors.Is(err, ErrPreBroadcast) {
		t.Fatalf("expected dial failure not to be classified as pre-broadcast")
	}
}

func testClient(name string, calls *[]string) *testRPCClient {
	return &testRPCClient{
		name:       name,
		calls:      calls,
		errs:       make(map[string]error),
		callResult: []byte("ok"),
		nonce:      7,
		tipCap:     big.NewInt(3),
		gas:        21000,
		receipt:    &types.Receipt{TxHash: common.HexToHash("0x1234")},
		chainID:    big.NewInt(1),
	}
}
