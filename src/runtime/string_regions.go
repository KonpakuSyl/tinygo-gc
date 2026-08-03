//go:build gc.regions

package runtime

import (
	"internal/gclayout"
	"unsafe"
)

// stringConcatRegions is the regions ABI form of string concatenation.
func stringConcatRegions(owner *region, x, y _string) _string {
	if x.length == 0 {
		return y
	}
	if y.length == 0 {
		return x
	}
	length := x.length + y.length
	buf := regionAlloc(owner, length, gclayout.NoPtrs.AsPtr())
	memcpy(buf, unsafe.Pointer(x.ptr), x.length)
	memcpy(unsafe.Add(buf, x.length), unsafe.Pointer(y.ptr), y.length)
	return _string{ptr: (*byte)(buf), length: length}
}

// stringFromBytesRegions is the owner-aware form of string([]byte).
func stringFromBytesRegions(owner *region, x struct {
	ptr *byte
	len uintptr
	cap uintptr
}) _string {
	buf := regionAlloc(owner, x.len, gclayout.NoPtrs.AsPtr())
	memcpy(buf, unsafe.Pointer(x.ptr), x.len)
	return _string{ptr: (*byte)(buf), length: x.len}
}

// stringToBytesRegions is the owner-aware form of []byte(string).
func stringToBytesRegions(owner *region, x _string) (slice struct {
	ptr *byte
	len uintptr
	cap uintptr
}) {
	buf := regionAlloc(owner, x.length, gclayout.NoPtrs.AsPtr())
	memcpy(buf, unsafe.Pointer(x.ptr), x.length)
	slice.ptr = (*byte)(buf)
	slice.len = x.length
	slice.cap = x.length
	return
}

// stringFromRunesRegions is the owner-aware form of string([]rune).
func stringFromRunesRegions(owner *region, runeSlice []rune) (s _string) {
	for _, r := range runeSlice {
		_, numBytes := encodeUTF8(r)
		s.length += numBytes
	}
	s.ptr = (*byte)(regionAlloc(owner, s.length, gclayout.NoPtrs.AsPtr()))
	index := uintptr(0)
	for _, r := range runeSlice {
		array, numBytes := encodeUTF8(r)
		for _, c := range array[:numBytes] {
			*(*byte)(unsafe.Add(unsafe.Pointer(s.ptr), index)) = c
			index++
		}
	}
	return
}

// stringToRunesRegions is the owner-aware form of []rune(string).
func stringToRunesRegions(owner *region, s string) []rune {
	n := 0
	for range s {
		n++
	}
	runes := unsafe.Slice((*rune)(regionAlloc(owner, uintptr(n)*unsafe.Sizeof(rune(0)), gclayout.NoPtrs.AsPtr())), n)
	n = 0
	for _, r := range s {
		runes[n] = r
		n++
	}
	return runes
}

// stringFromUnicodeRegions is the owner-aware form of string(rune).
func stringFromUnicodeRegions(owner *region, x rune) _string {
	array, length := encodeUTF8(x)
	buf := regionAlloc(owner, length, gclayout.NoPtrs.AsPtr())
	memcpy(buf, unsafe.Pointer(&array), length)
	return _string{ptr: (*byte)(buf), length: length}
}
