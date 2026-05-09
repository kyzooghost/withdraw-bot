package withdraw

import (
	"math/big"
	"testing"
)

func TestBumpFeesIncreasesFeesAndRespectsCaps(t *testing.T) {
	// Arrange
	policy := GasPolicy{BumpBPS: 1250, MaxFeeCap: big.NewInt(112), MaxTipCap: big.NewInt(10)}
	fees := FeeCaps{MaxFeePerGas: big.NewInt(100), MaxPriorityFeePerGas: big.NewInt(8)}

	// Act
	result := policy.Bump(fees)

	// Assert
	if result.MaxFeePerGas.String() != "112" {
		t.Fatalf("expected max fee cap 112, got %s", result.MaxFeePerGas.String())
	}
	if result.MaxPriorityFeePerGas.String() != "9" {
		t.Fatalf("expected priority fee 9, got %s", result.MaxPriorityFeePerGas.String())
	}
}
