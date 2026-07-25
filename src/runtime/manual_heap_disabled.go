//go:build !gc.manual

package runtime

const manualHeapMode = false

func manualHeapSizeValue() uintptr {
	return 0
}

func configureManualHeap() {
}
