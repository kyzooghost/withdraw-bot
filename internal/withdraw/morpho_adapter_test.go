package withdraw

import (
	"bytes"
	"context"
	"errors"
	"math/big"
	"strings"
	"testing"
	"time"

	"withdraw-bot/internal/core"
	"withdraw-bot/internal/ethereum"
	"withdraw-bot/internal/morpho"

	geth "github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
)

const (
	testVaultMethodBalanceOf     = "balanceOf"
	testVaultMethodPreviewRedeem = "previewRedeem"
	testVaultMethodRedeem        = "redeem"
)

var errTestEstimateFailure = errors.New("underlying RPC failure includes https://secret.example")

type morphoTestRPCClient struct {
	t              *testing.T
	vault          common.Address
	owner          common.Address
	balance        *big.Int
	previewShares  *big.Int
	previewAssets  *big.Int
	gas            uint64
	estimateErr    error
	previewCalled  bool
	estimateCalled bool
}

func (client *morphoTestRPCClient) CallContract(ctx context.Context, call geth.CallMsg, blockNumber *big.Int) ([]byte, error) {
	if call.To == nil {
		client.t.Fatal("expected call target")
	}
	if *call.To != client.vault {
		client.t.Fatalf("expected call target %s, got %s", client.vault.Hex(), call.To.Hex())
	}
	if len(call.Data) < 4 {
		client.t.Fatalf("expected calldata selector, got %d bytes", len(call.Data))
	}

	balanceMethod := morpho.VaultABI.Methods[testVaultMethodBalanceOf]
	if bytes.Equal(call.Data[:4], balanceMethod.ID) {
		decoded, err := balanceMethod.Inputs.Unpack(call.Data[4:])
		if err != nil {
			client.t.Fatalf("unpack balanceOf input: %v", err)
		}
		if decoded[0].(common.Address) != client.owner {
			client.t.Fatalf("expected balanceOf owner %s, got %s", client.owner.Hex(), decoded[0].(common.Address).Hex())
		}
		return morpho.VaultABI.Methods[testVaultMethodBalanceOf].Outputs.Pack(new(big.Int).Set(client.balance))
	}

	previewMethod := morpho.VaultABI.Methods[testVaultMethodPreviewRedeem]
	if bytes.Equal(call.Data[:4], previewMethod.ID) {
		decoded, err := previewMethod.Inputs.Unpack(call.Data[4:])
		if err != nil {
			client.t.Fatalf("unpack previewRedeem input: %v", err)
		}
		shares := decoded[0].(*big.Int)
		if shares.Cmp(client.previewShares) != 0 {
			client.t.Fatalf("expected previewRedeem shares %s, got %s", client.previewShares.String(), shares.String())
		}
		client.previewCalled = true
		return morpho.VaultABI.Methods[testVaultMethodPreviewRedeem].Outputs.Pack(new(big.Int).Set(client.previewAssets))
	}

	client.t.Fatalf("unexpected call selector %x", call.Data[:4])
	return nil, nil
}

func (client *morphoTestRPCClient) PendingNonceAt(ctx context.Context, account common.Address) (uint64, error) {
	return 0, nil
}

func (client *morphoTestRPCClient) SuggestGasTipCap(ctx context.Context) (*big.Int, error) {
	return big.NewInt(0), nil
}

func (client *morphoTestRPCClient) SuggestGasPrice(ctx context.Context) (*big.Int, error) {
	return big.NewInt(0), nil
}

func (client *morphoTestRPCClient) EstimateGas(ctx context.Context, call geth.CallMsg) (uint64, error) {
	client.estimateCalled = true
	if call.To == nil {
		client.t.Fatal("expected estimate target")
	}
	if *call.To != client.vault {
		client.t.Fatalf("expected estimate target %s, got %s", client.vault.Hex(), call.To.Hex())
	}
	if client.estimateErr != nil {
		return 0, client.estimateErr
	}
	return client.gas, nil
}

func (client *morphoTestRPCClient) SendTransaction(ctx context.Context, tx *types.Transaction) error {
	return nil
}

func (client *morphoTestRPCClient) TransactionReceipt(ctx context.Context, txHash common.Hash) (*types.Receipt, error) {
	return nil, nil
}

func (client *morphoTestRPCClient) ChainID(ctx context.Context) (*big.Int, error) {
	return big.NewInt(1), nil
}

func (client *morphoTestRPCClient) Close() {}

func TestMorphoAdapterPositionReadsBalanceAndReturnsConfiguredFields(t *testing.T) {
	// Arrange
	ctx := context.Background()
	owner := common.HexToAddress("0x0000000000000000000000000000000000000001")
	receiver := common.HexToAddress("0x0000000000000000000000000000000000000002")
	vault := common.HexToAddress("0x8c106EEDAd96553e64287A5A6839c3Cc78afA3D0")
	observedAt := time.Date(2026, 5, 9, 1, 0, 0, 0, time.UTC)
	balance := big.NewInt(12345)
	rpc := &morphoTestRPCClient{t: t, vault: vault, owner: owner, balance: balance}
	adapter := testMorphoAdapter(vault, owner, receiver, rpc, core.FixedClock{Value: observedAt})

	// Act
	result, err := adapter.Position(ctx)

	// Assert
	if err != nil {
		t.Fatalf("position: %v", err)
	}
	if result.Vault != vault {
		t.Fatalf("expected vault %s, got %s", vault.Hex(), result.Vault.Hex())
	}
	if result.Owner != owner {
		t.Fatalf("expected owner %s, got %s", owner.Hex(), result.Owner.Hex())
	}
	if result.Receiver != receiver {
		t.Fatalf("expected receiver %s, got %s", receiver.Hex(), result.Receiver.Hex())
	}
	if result.ShareBalance.Cmp(balance) != 0 {
		t.Fatalf("expected share balance %s, got %s", balance.String(), result.ShareBalance.String())
	}
	if result.ShareBalance == balance {
		t.Fatal("expected share balance clone")
	}
	if result.AssetSymbol != "USDC" {
		t.Fatalf("expected asset symbol USDC, got %s", result.AssetSymbol)
	}
	if result.AssetDecimals != 6 {
		t.Fatalf("expected asset decimals 6, got %d", result.AssetDecimals)
	}
	if !result.ObservedAt.Equal(observedAt) {
		t.Fatalf("expected observed time %s, got %s", observedAt, result.ObservedAt)
	}
}

func TestMorphoAdapterBuildFullExitEncodesRedeemAndZeroValue(t *testing.T) {
	// Arrange
	ctx := context.Background()
	owner := common.HexToAddress("0x0000000000000000000000000000000000000001")
	receiver := common.HexToAddress("0x0000000000000000000000000000000000000002")
	vault := common.HexToAddress("0x8c106EEDAd96553e64287A5A6839c3Cc78afA3D0")
	shares := big.NewInt(789)
	adapter := testMorphoAdapter(vault, owner, receiver, nil, core.FixedClock{})
	req := core.FullExitRequest{Vault: vault, Owner: owner, Receiver: receiver, Shares: shares}

	// Act
	result, err := adapter.BuildFullExit(ctx, req)

	// Assert
	if err != nil {
		t.Fatalf("build full exit: %v", err)
	}
	if result.To != vault {
		t.Fatalf("expected tx target %s, got %s", vault.Hex(), result.To.Hex())
	}
	if result.Value.Sign() != 0 {
		t.Fatalf("expected zero tx value, got %s", result.Value.String())
	}
	method := morpho.VaultABI.Methods[testVaultMethodRedeem]
	decoded, err := method.Inputs.Unpack(result.Data[4:])
	if err != nil {
		t.Fatalf("unpack redeem input: %v", err)
	}
	if decoded[0].(*big.Int).Cmp(shares) != 0 {
		t.Fatalf("expected shares %s, got %s", shares.String(), decoded[0].(*big.Int).String())
	}
	if decoded[1].(common.Address) != receiver {
		t.Fatalf("expected receiver %s, got %s", receiver.Hex(), decoded[1].(common.Address).Hex())
	}
	if decoded[2].(common.Address) != owner {
		t.Fatalf("expected owner %s, got %s", owner.Hex(), decoded[2].(common.Address).Hex())
	}
}

func TestMorphoAdapterSimulateFullExitReturnsExpectedAssetsAndGas(t *testing.T) {
	// Arrange
	ctx := context.Background()
	owner := common.HexToAddress("0x0000000000000000000000000000000000000001")
	receiver := common.HexToAddress("0x0000000000000000000000000000000000000002")
	vault := common.HexToAddress("0x8c106EEDAd96553e64287A5A6839c3Cc78afA3D0")
	shares := big.NewInt(789)
	assets := big.NewInt(456)
	rpc := &morphoTestRPCClient{
		t:             t,
		vault:         vault,
		owner:         owner,
		previewShares: shares,
		previewAssets: assets,
		gas:           88000,
	}
	adapter := testMorphoAdapter(vault, owner, receiver, rpc, core.FixedClock{})
	req := core.FullExitRequest{Vault: vault, Owner: owner, Receiver: receiver, Shares: shares}

	// Act
	result, err := adapter.SimulateFullExit(ctx, req)

	// Assert
	if err != nil {
		t.Fatalf("simulate full exit: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected successful simulation, got revert reason %q", result.RevertReason)
	}
	if result.ExpectedAssetUnits.Cmp(assets) != 0 {
		t.Fatalf("expected assets %s, got %s", assets.String(), result.ExpectedAssetUnits.String())
	}
	if result.ExpectedAssetUnits == assets {
		t.Fatal("expected asset units clone")
	}
	if result.GasUnits != 88000 {
		t.Fatalf("expected gas 88000, got %d", result.GasUnits)
	}
	if !rpc.previewCalled {
		t.Fatal("expected previewRedeem call")
	}
	if !rpc.estimateCalled {
		t.Fatal("expected EstimateGas call")
	}
}

func TestMorphoAdapterSimulateFullExitSanitizesEstimateFailure(t *testing.T) {
	// Arrange
	ctx := context.Background()
	owner := common.HexToAddress("0x0000000000000000000000000000000000000001")
	receiver := common.HexToAddress("0x0000000000000000000000000000000000000002")
	vault := common.HexToAddress("0x8c106EEDAd96553e64287A5A6839c3Cc78afA3D0")
	shares := big.NewInt(789)
	assets := big.NewInt(456)
	rpc := &morphoTestRPCClient{
		t:             t,
		vault:         vault,
		owner:         owner,
		previewShares: shares,
		previewAssets: assets,
		estimateErr:   errTestEstimateFailure,
	}
	adapter := testMorphoAdapter(vault, owner, receiver, rpc, core.FixedClock{})
	req := core.FullExitRequest{Vault: vault, Owner: owner, Receiver: receiver, Shares: shares}

	// Act
	result, err := adapter.SimulateFullExit(ctx, req)

	// Assert
	if err != nil {
		t.Fatalf("simulate full exit: %v", err)
	}
	if result.Success {
		t.Fatal("expected failed simulation")
	}
	if result.ExpectedAssetUnits.Cmp(assets) != 0 {
		t.Fatalf("expected assets %s, got %s", assets.String(), result.ExpectedAssetUnits.String())
	}
	if result.RevertReason != simulationFailedReason {
		t.Fatalf("expected sanitized revert reason %q, got %q", simulationFailedReason, result.RevertReason)
	}
	if strings.Contains(result.RevertReason, errTestEstimateFailure.Error()) {
		t.Fatal("expected revert reason not to leak RPC error")
	}
	if strings.Contains(result.RevertReason, "secret.example") {
		t.Fatal("expected revert reason not to leak RPC host")
	}
}

func TestMorphoAdapterPositionRejectsMismatchedVaultClient(t *testing.T) {
	// Arrange
	ctx := context.Background()
	owner := common.HexToAddress("0x0000000000000000000000000000000000000001")
	receiver := common.HexToAddress("0x0000000000000000000000000000000000000002")
	vault := common.HexToAddress("0x8c106EEDAd96553e64287A5A6839c3Cc78afA3D0")
	otherVault := common.HexToAddress("0x0000000000000000000000000000000000000003")
	adapter := testMorphoAdapter(vault, owner, receiver, nil, core.FixedClock{})
	adapter.VaultClient.Vault = otherVault

	// Act
	_, err := adapter.Position(ctx)

	// Assert
	if err == nil {
		t.Fatal("expected vault mismatch error")
	}
	if !strings.Contains(err.Error(), "vault client mismatch") {
		t.Fatalf("expected vault mismatch error, got %v", err)
	}
}

func TestMorphoAdapterBuildFullExitRejectsInvalidRequest(t *testing.T) {
	// Arrange
	ctx := context.Background()
	owner := common.HexToAddress("0x0000000000000000000000000000000000000001")
	receiver := common.HexToAddress("0x0000000000000000000000000000000000000002")
	vault := common.HexToAddress("0x8c106EEDAd96553e64287A5A6839c3Cc78afA3D0")
	adapter := testMorphoAdapter(vault, owner, receiver, nil, core.FixedClock{})
	req := core.FullExitRequest{Vault: vault, Owner: owner, Receiver: receiver, Shares: big.NewInt(0)}

	// Act
	_, err := adapter.BuildFullExit(ctx, req)

	// Assert
	if err == nil {
		t.Fatal("expected invalid shares error")
	}
	if !strings.Contains(err.Error(), "shares must be positive") {
		t.Fatalf("expected invalid shares error, got %v", err)
	}
}

func testMorphoAdapter(vault common.Address, owner common.Address, receiver common.Address, rpc ethereum.RPCClient, clock core.Clock) MorphoAdapter {
	var multiClient ethereum.MultiClient
	if rpc != nil {
		multiClient = ethereum.NewMultiClient(rpc, nil)
	}
	return MorphoAdapter{
		Ethereum:      multiClient,
		VaultClient:   morpho.VaultClient{Ethereum: multiClient, Vault: vault},
		Vault:         vault,
		Owner:         owner,
		Receiver:      receiver,
		AssetSymbol:   "USDC",
		AssetDecimals: 6,
		Clock:         clock,
	}
}
