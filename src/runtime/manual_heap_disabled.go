//go:build !gc.manual

package runtime

const manualHeapMode = false

func manualHeapSizeValue() uintptr {
	return 0
}

func configureManualHeap() {
}

// ManualHeapFree returns zero values when the manual collector is not in use.
func ManualHeapFree() (total, largest uintptr) {
	return 0, 0
}
