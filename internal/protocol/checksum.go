package protocol

import (
	"encoding/base64"
	"time"
)

// Checksum reproduces the official client's x-cursor-checksum transform.
//
// It MUST preserve JavaScript's 32-bit shift semantics. In the reference JS,
// `epoch >> 40` is really `epoch >> (40 % 32)` == `epoch >> 8`, because JS
// bitwise operators coerce their operand to a signed int32 and mask the shift
// count to 5 bits. A naive 64-bit shift in Go produces different bytes and the
// server rejects the request. We replicate by using a uint32 and masking the
// shift count mod 32; the trailing `& 0xff` makes logical-vs-arithmetic shift
// irrelevant for the resulting byte.
func Checksum(machineID string, nowMs int64) string {
	e := uint32(nowMs / 1_000_000) // #nosec G115 -- intentional 32-bit truncation to match JS int32 coercion (see doc comment)
	shift := func(n uint) byte { return byte((e >> (n % 32)) & 0xff) }
	b := []byte{shift(40), shift(32), shift(24), shift(16), shift(8), shift(0)}
	prev := byte(165)
	for i := range b {
		b[i] = byte((int(b[i]^prev) + i) & 0xff)
		prev = b[i]
	}
	return base64.RawURLEncoding.EncodeToString(b) + machineID
}

// ChecksumNow is the production entry point using the current wall clock.
func ChecksumNow(machineID string) string {
	return Checksum(machineID, time.Now().UnixMilli())
}
