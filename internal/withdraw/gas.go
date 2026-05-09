package withdraw

import "math/big"

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
	return FeeCaps{
		MaxFeePerGas:         capBig(bumpBPS(fees.MaxFeePerGas, policy.BumpBPS), policy.MaxFeeCap),
		MaxPriorityFeePerGas: capBig(bumpBPS(fees.MaxPriorityFeePerGas, policy.BumpBPS), policy.MaxTipCap),
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
