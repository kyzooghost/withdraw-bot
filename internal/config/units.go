package config

import (
	"fmt"
	"math/big"
	"strings"
)

const basisPointsDenominator int64 = 10_000

func ValidateBPS(name string, value int64) error {
	if value < 0 || value > basisPointsDenominator {
		return fmt.Errorf("%s must be between 0 and 10000 bps", name)
	}
	return nil
}

func ParseDecimalUnits(name string, value string, decimals uint8) (*big.Int, error) {
	clean := strings.TrimSpace(value)
	if clean == "" {
		return nil, fmt.Errorf("%s is required", name)
	}
	if !isUnsignedBase10Decimal(clean) {
		return nil, fmt.Errorf("%s must be an unsigned base-10 decimal string", name)
	}
	amount, ok := new(big.Rat).SetString(clean)
	if !ok {
		return nil, fmt.Errorf("%s must be a decimal string", name)
	}
	scale := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(decimals)), nil)
	scaled := new(big.Rat).Mul(amount, new(big.Rat).SetInt(scale))
	if !scaled.IsInt() {
		return nil, fmt.Errorf("%s has more than %d decimal places", name, decimals)
	}
	if scaled.Sign() < 0 {
		return nil, fmt.Errorf("%s must not be negative", name)
	}
	return scaled.Num(), nil
}

func isUnsignedBase10Decimal(value string) bool {
	sawDigit := false
	sawDecimalPoint := false
	digitsAfterDecimalPoint := 0

	for i := 0; i < len(value); i++ {
		character := value[i]
		if character >= '0' && character <= '9' {
			sawDigit = true
			if sawDecimalPoint {
				digitsAfterDecimalPoint++
			}
			continue
		}
		if character == '.' && !sawDecimalPoint && sawDigit {
			sawDecimalPoint = true
			continue
		}
		return false
	}

	return sawDigit && (!sawDecimalPoint || digitsAfterDecimalPoint > 0)
}

func ParseGwei(name string, value string) (*big.Int, error) {
	units, err := ParseDecimalUnits(name, value, 9)
	if err != nil {
		return nil, err
	}
	return units, nil
}
