//go:build gc.manual

package runtime

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
