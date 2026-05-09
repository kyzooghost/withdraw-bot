package core

import (
	"math/big"
	"time"

	"github.com/ethereum/go-ethereum/common"
)

type Metric struct {
	Key   string
	Value string
	Unit  string
}

type Finding struct {
	Key      FindingKey
	Severity Severity
	Message  string
	Evidence map[string]string
}

type MonitorResult struct {
	ModuleID   MonitorModuleID
	Status     MonitorStatus
	ObservedAt time.Time
	Metrics    []Metric
	Findings   []Finding
}

type PositionSnapshot struct {
	Vault         common.Address
	Owner         common.Address
	Receiver      common.Address
	ShareBalance  *big.Int
	AssetSymbol   string
	AssetDecimals uint8
	ObservedAt    time.Time
}

type FullExitRequest struct {
	Vault    common.Address
	Owner    common.Address
	Receiver common.Address
	Shares   *big.Int
}

type TxCandidate struct {
	To    common.Address
	Data  []byte
	Value *big.Int
}

type FullExitSimulation struct {
	Success            bool
	ExpectedAssetUnits *big.Int
	GasUnits           uint64
	RevertReason       string
}
