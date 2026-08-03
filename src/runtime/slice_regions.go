//go:build gc.regions

package runtime

import (
	"math/bits"
	"unsafe"
)

// sliceAppendRegions is the regions ABI form of append. Passing the owner
// explicitly keeps a returned grown slice in the caller-selected region.
func sliceAppendRegions(owner *region, srcBuf, elemsBuf unsafe.Pointer, srcLen, srcCap, elemsLen, elemSize uintptr, layout unsafe.Pointer) (unsafe.Pointer, uintptr, uintptr) {
	newLen := srcLen + elemsLen
	if elemsLen > 0 {
		srcBuf, _, srcCap = sliceGrowRegions(owner, srcBuf, srcLen, srcCap, newLen, elemSize, layout)
		memmove(unsafe.Add(srcBuf, srcLen*elemSize), elemsBuf, elemsLen*elemSize)
	}
	return srcBuf, newLen, srcCap
}

func sliceGrowRegions(owner *region, oldBuf unsafe.Pointer, oldLen, oldCap, newCap, elemSize uintptr, layout unsafe.Pointer) (unsafe.Pointer, uintptr, uintptr) {
	if oldCap >= newCap {
		return oldBuf, oldLen, oldCap
	}
	newCap = 1 << bits.Len(uint(newCap))
	buf := regionAlloc(owner, newCap*elemSize, layout)
	if oldLen > 0 {
		memmove(buf, oldBuf, oldLen*elemSize)
	}
	return buf, oldLen, newCap
}
