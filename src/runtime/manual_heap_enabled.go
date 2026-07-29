//go:build gc.manual

package runtime

import "unsafe"

const manualHeapMode = true

// manualHeapSize is populated by the build driver after package compilation.
var manualHeapSize string

func manualHeapSizeValue() uintptr {
	if manualHeapSize == "" {
		runtimePanic("manual heap size is not configured")
	}

	var size uintptr
	for i := 0; i < len(manualHeapSize); i++ {
		digit := manualHeapSize[i] - '0'
		if digit > 9 || size > (^uintptr(0)-uintptr(digit))/10 {
			runtimePanic("invalid manual heap size")
		}
		size = size*10 + uintptr(digit)
	}
	if size == 0 {
		runtimePanic("manual heap size is zero")
	}
	return size
}

// ManualHeapFree returns the total number of free bytes and the largest single
// allocation payload supported by the manual heap. The total includes all free
// blocks, while the largest value accounts for fragmentation and the object
// header added to each allocation.
func ManualHeapFree() (total, largest uintptr) {
	gcLock.Lock()
	headerSize := align(unsafe.Sizeof(objHeader{}))
	for freeRange := freeRanges; freeRange != nil; freeRange = freeRange.nextLen {
		rangeSize := freeRange.len * bytesPerBlock
		total += rangeSize
		if rangeSize > headerSize && rangeSize-headerSize > largest {
			largest = rangeSize - headerSize
		}
		for more := freeRange.nextWithLen; more != nil; more = more.next {
			total += rangeSize
		}
	}
	gcLock.Unlock()
	return total, largest
}
