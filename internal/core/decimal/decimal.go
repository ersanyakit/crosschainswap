package decimal

import (
	"math/big"
	"strings"
)

const Scale = 18

func Parse(value string) *big.Rat {
	out, ok := new(big.Rat).SetString(strings.TrimSpace(value))
	if !ok {
		return new(big.Rat)
	}
	return out
}

func FractionalDigits(value string) int {
	value = strings.TrimSpace(value)
	if idx := strings.IndexAny(value, "."); idx >= 0 {
		return len(strings.TrimRight(value[idx+1:], "0"))
	}
	return 0
}

func String(value *big.Rat) string {
	return Trim(value.FloatString(Scale))
}

func Cmp(left string, right string) int {
	return Parse(left).Cmp(Parse(right))
}

func Min(left string, right string) string {
	if Cmp(left, right) <= 0 {
		return left
	}
	return right
}

func Add(left string, right string) string {
	return String(new(big.Rat).Add(Parse(left), Parse(right)))
}

func SubFloorZero(left string, right string) string {
	out := new(big.Rat).Sub(Parse(left), Parse(right))
	if out.Sign() < 0 {
		return "0"
	}
	return String(out)
}

func Mul(left string, right string) string {
	return String(new(big.Rat).Mul(Parse(left), Parse(right)))
}

func Trim(value string) string {
	value = strings.TrimRight(value, "0")
	value = strings.TrimRight(value, ".")
	if value == "" || value == "-0" {
		return "0"
	}
	return value
}
