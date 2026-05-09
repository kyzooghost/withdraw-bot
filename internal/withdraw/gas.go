package withdraw

import (
	"errors"
	"math/big"
)

const (
	errMaxFeePerGasRequired         = "max fee per gas is required"
	errMaxPriorityFeePerGasRequired = "max priority fee per gas is required"
	errMaxFeePerGasPositive         = "max fee per gas must be positive"
	errMaxPriorityFeePerGasPositive = "max priority fee per gas must be positive"
	errMaxFeeCapPositive            = "max fee cap must be positive"
	errMaxTipCapPositive            = "max tip cap must be positive"
	errMaxFeeBelowPriorityFee       = "max fee per gas must be greater than or equal to max priority fee per gas"
)

type FeeCaps struct {
	MaxFeePerGas         *big.Int
	MaxPriorityFeePerGas *big.Int
}

type GasPolicy struct {
	BumpBPS   int64
	MaxFeeCap *big.Int
	MaxTipCap *big.Int
}

func (policy GasPolicy) Bump(fees FeeCaps) FeeCaps {
	result, err := policy.BumpChecked(fees)
	if err != nil {
		return FeeCaps{}
	}
	return result
}

func (policy GasPolicy) BumpChecked(fees FeeCaps) (FeeCaps, error) {
	if err := policy.validateCaps(); err != nil {
		return FeeCaps{}, err
	}
	if err := fees.Validate(); err != nil {
		return FeeCaps{}, err
	}
	result := FeeCaps{
		MaxFeePerGas:         capBig(bumpBPS(fees.MaxFeePerGas, policy.BumpBPS), policy.MaxFeeCap),
		MaxPriorityFeePerGas: capBig(bumpBPS(fees.MaxPriorityFeePerGas, policy.BumpBPS), policy.MaxTipCap),
	}
	if err := result.Validate(); err != nil {
		return FeeCaps{}, err
	}
	return result, nil
}

func (policy GasPolicy) validateCaps() error {
	if policy.MaxFeeCap != nil && policy.MaxFeeCap.Sign() <= 0 {
		return errors.New(errMaxFeeCapPositive)
	}
	if policy.MaxTipCap != nil && policy.MaxTipCap.Sign() <= 0 {
		return errors.New(errMaxTipCapPositive)
	}
	return nil
}

func (fees FeeCaps) Validate() error {
	if fees.MaxFeePerGas == nil {
		return errors.New(errMaxFeePerGasRequired)
	}
	if fees.MaxPriorityFeePerGas == nil {
		return errors.New(errMaxPriorityFeePerGasRequired)
	}
	if fees.MaxFeePerGas.Sign() <= 0 {
		return errors.New(errMaxFeePerGasPositive)
	}
	if fees.MaxPriorityFeePerGas.Sign() <= 0 {
		return errors.New(errMaxPriorityFeePerGasPositive)
	}
	if fees.MaxFeePerGas.Cmp(fees.MaxPriorityFeePerGas) < 0 {
		return errors.New(errMaxFeeBelowPriorityFee)
	}
	return nil
}

func (fees FeeCaps) Clone() FeeCaps {
	return FeeCaps{
		MaxFeePerGas:         cloneBigInt(fees.MaxFeePerGas),
		MaxPriorityFeePerGas: cloneBigInt(fees.MaxPriorityFeePerGas),
	}
}

func bumpBPS(value *big.Int, bps int64) *big.Int {
	result := new(big.Int).Mul(value, big.NewInt(10_000+bps))
	result.Div(result, big.NewInt(10_000))
	return result
}

func capBig(value *big.Int, cap *big.Int) *big.Int {
	if cap != nil && value.Cmp(cap) > 0 {
		return new(big.Int).Set(cap)
	}
	return new(big.Int).Set(value)
}
