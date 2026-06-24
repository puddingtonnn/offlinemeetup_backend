package cache

import (
	"math/rand/v2"
	"time"
)

// jitter добавляет к ttl случайный разброс ±10%, чтобы ключи не истекали
// синхронно (защита от cache avalanche). ttl <= 0 возвращается без изменений.
// math/rand/v2 потокобезопасен и не требует seed.
func jitter(ttl time.Duration) time.Duration {
	delta := ttl / 10
	if delta <= 0 {
		return ttl
	}
	// rand.Int64N(2*delta+1) ∈ [0, 2*delta] → результат ∈ [ttl-delta, ttl+delta].
	return ttl - delta + time.Duration(rand.Int64N(int64(2*delta)+1))
}
