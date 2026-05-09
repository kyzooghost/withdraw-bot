package morpho

import (
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
)

const vaultABIJSON = `[
	{"type":"function","name":"asset","stateMutability":"view","inputs":[],"outputs":[{"type":"address"}]},
	{"type":"function","name":"balanceOf","stateMutability":"view","inputs":[{"name":"account","type":"address"}],"outputs":[{"type":"uint256"}]},
	{"type":"function","name":"previewRedeem","stateMutability":"view","inputs":[{"name":"shares","type":"uint256"}],"outputs":[{"type":"uint256"}]},
	{"type":"function","name":"redeem","stateMutability":"nonpayable","inputs":[{"name":"shares","type":"uint256"},{"name":"receiver","type":"address"},{"name":"onBehalf","type":"address"}],"outputs":[{"type":"uint256"}]},
	{"type":"function","name":"totalAssets","stateMutability":"view","inputs":[],"outputs":[{"type":"uint256"}]},
	{"type":"function","name":"totalSupply","stateMutability":"view","inputs":[],"outputs":[{"type":"uint256"}]},
	{"type":"function","name":"owner","stateMutability":"view","inputs":[],"outputs":[{"type":"address"}]},
	{"type":"function","name":"curator","stateMutability":"view","inputs":[],"outputs":[{"type":"address"}]},
	{"type":"function","name":"receiveSharesGate","stateMutability":"view","inputs":[],"outputs":[{"type":"address"}]},
	{"type":"function","name":"sendSharesGate","stateMutability":"view","inputs":[],"outputs":[{"type":"address"}]},
	{"type":"function","name":"receiveAssetsGate","stateMutability":"view","inputs":[],"outputs":[{"type":"address"}]},
	{"type":"function","name":"sendAssetsGate","stateMutability":"view","inputs":[],"outputs":[{"type":"address"}]},
	{"type":"function","name":"adapterRegistry","stateMutability":"view","inputs":[],"outputs":[{"type":"address"}]},
	{"type":"function","name":"liquidityAdapter","stateMutability":"view","inputs":[],"outputs":[{"type":"address"}]},
	{"type":"function","name":"liquidityData","stateMutability":"view","inputs":[],"outputs":[{"type":"bytes"}]},
	{"type":"function","name":"performanceFee","stateMutability":"view","inputs":[],"outputs":[{"type":"uint96"}]},
	{"type":"function","name":"performanceFeeRecipient","stateMutability":"view","inputs":[],"outputs":[{"type":"address"}]},
	{"type":"function","name":"managementFee","stateMutability":"view","inputs":[],"outputs":[{"type":"uint96"}]},
	{"type":"function","name":"managementFeeRecipient","stateMutability":"view","inputs":[],"outputs":[{"type":"address"}]},
	{"type":"function","name":"maxRate","stateMutability":"view","inputs":[],"outputs":[{"type":"uint64"}]},
	{"type":"function","name":"adaptersLength","stateMutability":"view","inputs":[],"outputs":[{"type":"uint256"}]},
	{"type":"function","name":"adapters","stateMutability":"view","inputs":[{"type":"uint256"}],"outputs":[{"type":"address"}]},
	{"type":"function","name":"isAllocator","stateMutability":"view","inputs":[{"type":"address"}],"outputs":[{"type":"bool"}]},
	{"type":"function","name":"isSentinel","stateMutability":"view","inputs":[{"type":"address"}],"outputs":[{"type":"bool"}]},
	{"type":"function","name":"timelock","stateMutability":"view","inputs":[{"type":"bytes4"}],"outputs":[{"type":"uint256"}]},
	{"type":"function","name":"abdicated","stateMutability":"view","inputs":[{"type":"bytes4"}],"outputs":[{"type":"bool"}]},
	{"type":"function","name":"forceDeallocatePenalty","stateMutability":"view","inputs":[{"type":"address"}],"outputs":[{"type":"uint256"}]}
]`

const erc20ABIJSON = `[
	{"type":"function","name":"balanceOf","stateMutability":"view","inputs":[{"name":"account","type":"address"}],"outputs":[{"type":"uint256"}]},
	{"type":"function","name":"decimals","stateMutability":"view","inputs":[],"outputs":[{"type":"uint8"}]}
]`

const (
	vaultMethodBalanceOf     = "balanceOf"
	vaultMethodPreviewRedeem = "previewRedeem"
	vaultMethodRedeem        = "redeem"
)

var VaultABI = mustParseABI(vaultABIJSON)
var ERC20ABI = mustParseABI(erc20ABIJSON)

func mustParseABI(raw string) abi.ABI {
	parsed, err := abi.JSON(strings.NewReader(raw))
	if err != nil {
		panic(err)
	}
	return parsed
}

func PackRedeem(shares *big.Int, receiver common.Address, owner common.Address) ([]byte, error) {
	return VaultABI.Pack(vaultMethodRedeem, shares, receiver, owner)
}
