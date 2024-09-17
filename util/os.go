package util

const SYS64 = 64
const SYS32 = 32

func SysteamBits() uint8 {
	intSize := 32 << (^uint(0) >> 63)
	return uint8(intSize)
}
