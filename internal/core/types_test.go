package core

import (
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

const (
	testMetricKey                   = "share_price"
	testMetricValue                 = "100"
	testUpdatedMetricValue          = "90"
	testMetricUnit                  = "assets_per_share"
	testAddressHex                  = "0x0000000000000000000000000000000000000001"
	testEvidenceKeyBlock            = "block"
	testEvidenceValue               = "123"
	testUpdatedEvidenceValue        = "456"
	testFindingMessage              = "share price moved"
	testUpdatedFindingMessage       = "updated message"
	testOriginalAmount        int64 = 100
	testOriginalAmountText          = "100"
	testUpdatedAmount               = 50
	testOriginalDataByte      byte  = 0x01
	testSecondDataByte        byte  = 0x02
	testUpdatedDataByte       byte  = 0xff
)

func testCoreAddress() common.Address {
	return common.HexToAddress(testAddressHex)
}

func TestMonitorResultCloneCopiesMetricsSlice(t *testing.T) {
	// Arrange
	original := MonitorResult{
		Metrics: []Metric{
			{Key: testMetricKey, Value: testMetricValue, Unit: testMetricUnit},
		},
	}

	// Act
	clone := original.Clone()
	clone.Metrics[0].Value = testUpdatedMetricValue

	// Assert
	if original.Metrics[0].Value != testMetricValue {
		t.Fatalf("expected original metric value to remain %q, got %q", testMetricValue, original.Metrics[0].Value)
	}
}

func TestMonitorResultCloneCopiesFindingsEvidence(t *testing.T) {
	// Arrange
	original := MonitorResult{
		Findings: []Finding{
			{
				Key:      FindingSharePriceLoss,
				Severity: SeverityWarn,
				Message:  testFindingMessage,
				Evidence: map[string]string{
					testEvidenceKeyBlock: testEvidenceValue,
				},
			},
		},
	}

	// Act
	clone := original.Clone()
	clone.Findings[0].Evidence[testEvidenceKeyBlock] = testUpdatedEvidenceValue

	// Assert
	if original.Findings[0].Evidence[testEvidenceKeyBlock] != testEvidenceValue {
		t.Fatalf("expected original evidence to remain %q, got %q", testEvidenceValue, original.Findings[0].Evidence[testEvidenceKeyBlock])
	}
}

func TestMonitorResultCloneCopiesFindingsSlice(t *testing.T) {
	// Arrange
	original := MonitorResult{
		Findings: []Finding{
			{
				Key:      FindingSharePriceLoss,
				Severity: SeverityWarn,
				Message:  testFindingMessage,
			},
		},
	}

	// Act
	clone := original.Clone()
	clone.Findings[0].Message = testUpdatedFindingMessage

	// Assert
	if original.Findings[0].Message != testFindingMessage {
		t.Fatalf("expected original finding message to remain %q, got %q", testFindingMessage, original.Findings[0].Message)
	}
}

func TestPositionSnapshotCloneCopiesShareBalance(t *testing.T) {
	// Arrange
	original := PositionSnapshot{
		Vault:        testCoreAddress(),
		ShareBalance: big.NewInt(testOriginalAmount),
	}

	// Act
	clone := original.Clone()
	clone.ShareBalance.SetInt64(testUpdatedAmount)

	// Assert
	if original.ShareBalance.Cmp(big.NewInt(testOriginalAmount)) != 0 {
		t.Fatalf("expected original share balance to remain %s, got %s", testOriginalAmountText, original.ShareBalance)
	}
}

func TestFullExitRequestCloneCopiesShares(t *testing.T) {
	// Arrange
	original := FullExitRequest{
		Vault:  testCoreAddress(),
		Shares: big.NewInt(testOriginalAmount),
	}

	// Act
	clone := original.Clone()
	clone.Shares.SetInt64(testUpdatedAmount)

	// Assert
	if original.Shares.Cmp(big.NewInt(testOriginalAmount)) != 0 {
		t.Fatalf("expected original shares to remain %s, got %s", testOriginalAmountText, original.Shares)
	}
}

func TestTxCandidateCloneCopiesData(t *testing.T) {
	// Arrange
	original := TxCandidate{
		To:   testCoreAddress(),
		Data: []byte{testOriginalDataByte, testSecondDataByte},
	}

	// Act
	clone := original.Clone()
	clone.Data[0] = testUpdatedDataByte

	// Assert
	if original.Data[0] != testOriginalDataByte {
		t.Fatalf("expected original data byte to remain %#x, got %#x", testOriginalDataByte, original.Data[0])
	}
}

func TestTxCandidateCloneCopiesValue(t *testing.T) {
	// Arrange
	original := TxCandidate{
		To:    testCoreAddress(),
		Value: big.NewInt(testOriginalAmount),
	}

	// Act
	clone := original.Clone()
	clone.Value.SetInt64(testUpdatedAmount)

	// Assert
	if original.Value.Cmp(big.NewInt(testOriginalAmount)) != 0 {
		t.Fatalf("expected original value to remain %s, got %s", testOriginalAmountText, original.Value)
	}
}

func TestFullExitSimulationCloneCopiesExpectedAssetUnits(t *testing.T) {
	// Arrange
	original := FullExitSimulation{
		Success:            true,
		ExpectedAssetUnits: big.NewInt(testOriginalAmount),
	}

	// Act
	clone := original.Clone()
	clone.ExpectedAssetUnits.SetInt64(testUpdatedAmount)

	// Assert
	if original.ExpectedAssetUnits.Cmp(big.NewInt(testOriginalAmount)) != 0 {
		t.Fatalf("expected original expected asset units to remain %s, got %s", testOriginalAmountText, original.ExpectedAssetUnits)
	}
}
