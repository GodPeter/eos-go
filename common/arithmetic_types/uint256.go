package arithmeticTypes

import (
	"fmt"
	"math"
)

type Uint256 struct {
	High Uint128
	Low  Uint128
}

func (u Uint256) IsZero() bool {
	if u.High.IsZero() && u.Low.IsZero() {
		return true
	}
	return false
}

func CreateUint128(i int) Uint128{
	if i >= 0 {
		return Uint128{0, uint64(i)}
	} else {
		fmt.Println("error")
		return Uint128{}
	}
}

func (u Uint256) GetAt(i uint) bool {
	if i < 128 {
		return u.Low.GetAt(i)
	} else {
		return u.High.GetAt(i - 128)
	}
}

func (u *Uint256) Set(i uint, b uint) {
	if i < 128 {
		if b == 1 {
			u.Low.Set(i, 1)
		}
		if b == 0 {
			u.Low.Set(i, 0)
		}
	}
	if i >= 128 {
		if b == 1 {
			u.High.Set(i, 1)
		}
		if b == 0 {
			u.High.Set(i, 0)
		}
	}
}

func (u Uint256) LeftShift() Uint256 {
	if u.GetAt(127) {
		u.Low.LeftShift()
		u.High.LeftShift()
		u.Set(128, 1)
	} else {
		u.Low.LeftShift()
		u.High.LeftShift()
	}
	return u
}

func (u *Uint256) LeftShifts(shift int) {
	for i := 0; i < shift; i++ {
		u.LeftShift()
	}
}

func (u Uint256) RightShift() Uint256 {
	if u.GetAt(128) {
		u.High.RightShift()
		u.Low.RightShift()
		u.Set(127, 1)
	}
	return u
}

func (u *Uint256) RightShifts(shift int) {
	for i := 0; i < shift; i++ {
		u.LeftShift()
	}
}

func (u Uint256) Compare(v Uint256) int {
	if u.High.Compare(v.High) > 0 {
		return 1
	} else if u.High.Compare(v.High) < 0 {
		return -1
	}
	if u.Low.Compare(v.Low) > 0 {
		return 1
	} else if u.Low.Compare(v.Low) < 0 {
		return -1
	}
	return 0
}

func (u Uint256) Add(v Uint256) Uint256 {
	if u.Low.Add(v.Low).Compare(u.Low) < 0 {
		u.High = u.High.Add(v.High).Add(Uint128{0, 1})
	} else {
		u.High = u.High.Add(v.High)
	}
	u.Low = u.Low.Add(v.Low)
	return u
}

func (u Uint256) Sub(v Uint256) Uint256 {
	if u.Low.Compare(v.Low) >= 0 {
		u.Low = u.Low.Sub(v.Low)
		u.High = u.High.Sub(v.High)
	} else {
		u.Low = u.Low.Add(Uint128{math.MaxUint64, math.MaxUint64}.Sub(v.Low).Add(Uint128{0, 1}))
		u.High = u.High.Sub(v.High.Add(Uint128{0, 1}))
	}
	return u
}

func (u Uint256) Mul(v Uint256) Uint256 {
	Product := Uint256{}
	for i := 0; i < 256; i++ {
		if v.GetAt(uint(i)) {
			Product = Product.Add(u)
		}
		u.LeftShift()
	}
	return Product
}

func (u Uint256) Div(divisor Uint256) (Uint256, Uint256) {
	if divisor.IsZero() {
		fmt.Println("divisor cannot be zero")
	}
	Quotient := Uint256{}
	Remainder := Uint256{}
	One := Uint128{0, 1}
	for i := 0; i < 256; i++ {
		Remainder.LeftShift()
		Quotient.LeftShift()
		if u.GetAt(255 - uint(i)) {
			Remainder.Low = Remainder.Low.Sub(One)
		}
		if Remainder.Compare(divisor) >= 0 {
			Quotient.Low = Quotient.Low.Add(One)
			Remainder = Remainder.Sub(divisor)
		}
	}
	return Quotient, Remainder
}
